package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.36 — Operations Dimension (Round 9)
// 1. HPA Scaling Event Tracker — recent scale-up/down events
// 2. Node Condition History — condition transitions & flapping
// 3. ConfigMap Change Tracker — recent config changes & staleness
// ============================================================

// ---------------------------------------------------------------
// 1. HPA Scaling Event Tracker
// ---------------------------------------------------------------

type HPAScalingResult1936 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         HPAScalingSummary1936 `json:"summary"`
	Events          []HPAScalingEntry1936 `json:"events"`
	ThrashingHPAs   []HPAThrashEntry1936  `json:"thrashingHPAs"`
	Recommendations []string              `json:"recommendations"`
}

type HPAScalingSummary1936 struct {
	TotalScaleEvents int `json:"totalScaleEvents"`
	ScaleUpEvents    int `json:"scaleUpEvents"`
	ScaleDownEvents  int `json:"scaleDownEvents"`
	ThrashingCount   int `json:"thrashingCount"`
	RecentEvents24h  int `json:"recentEvents24h"`
	MaxEventsPerHPA  int `json:"maxEventsPerHPA"`
}

type HPAScalingEntry1936 struct {
	Timestamp    string `json:"timestamp"`
	HPAName      string `json:"hpaName"`
	Namespace    string `json:"namespace"`
	Direction    string `json:"direction"`
	FromReplicas int32  `json:"fromReplicas"`
	ToReplicas   int32  `json:"toReplicas"`
	Reason       string `json:"reason"`
}

type HPAThrashEntry1936 struct {
	HPAName    string `json:"hpaName"`
	Namespace  string `json:"namespace"`
	EventCount int    `json:"eventCount"`
	Severity   string `json:"severity"`
	Detail     string `json:"detail"`
}

func (s *Server) handleHPAScalingEvents(w http.ResponseWriter, r *http.Request) {
	result := HPAScalingResult1936{ScannedAt: time.Now()}
	score := 100

	// Query HPA-related events
	evList, err := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{
		FieldSelector: "type=Normal",
	})
	if err != nil {
		writeJSON(w, result)
		return
	}

	hpaEventCount := make(map[string]int) // "ns/hpa" -> count
	now := time.Now()

	for _, ev := range evList.Items {
		// Look for HPA scaling events
		isHPAScale := false
		direction := ""
		if ev.InvolvedObject.Kind == "HorizontalPodAutoscaler" {
			isHPAScale = true
		}
		// Also check message content
		msgLower := strings.ToLower(ev.Message)
		if strings.Contains(msgLower, "new size") || strings.Contains(msgLower, "scaled") {
			isHPAScale = true
		}
		if !isHPAScale {
			continue
		}

		if strings.Contains(msgLower, "up") || strings.Contains(msgLower, "new size:") {
			if strings.Contains(msgLower, "up") {
				direction = "scale-up"
			} else {
				direction = "scale"
			}
		}
		if strings.Contains(msgLower, "down") {
			direction = "scale-down"
		}
		if direction == "" {
			direction = "scale"
		}

		age := ""
		if !ev.LastTimestamp.IsZero() {
			age = fmt.Sprintf("%.0fh ago", now.Sub(ev.LastTimestamp.Time).Hours())
		} else if !ev.EventTime.IsZero() {
			age = fmt.Sprintf("%.0fh ago", now.Sub(ev.EventTime.Time).Hours())
		}

		entry := HPAScalingEntry1936{
			Timestamp: age,
			HPAName:   ev.InvolvedObject.Name,
			Namespace: ev.InvolvedObject.Namespace,
			Direction: direction,
			Reason:    ev.Reason,
		}
		result.Events = append(result.Events, entry)
		result.Summary.TotalScaleEvents++

		if direction == "scale-up" {
			result.Summary.ScaleUpEvents++
		} else if direction == "scale-down" {
			result.Summary.ScaleDownEvents++
		}

		// Count per HPA for thrashing detection
		key := fmt.Sprintf("%s/%s", ev.InvolvedObject.Namespace, ev.InvolvedObject.Name)
		hpaEventCount[key]++
		if hpaEventCount[key] > result.Summary.MaxEventsPerHPA {
			result.Summary.MaxEventsPerHPA = hpaEventCount[key]
		}

		// Recent events (24h)
		var ts time.Time
		if !ev.LastTimestamp.IsZero() {
			ts = ev.LastTimestamp.Time
		} else if !ev.EventTime.IsZero() {
			ts = ev.EventTime.Time
		}
		if !ts.IsZero() && now.Sub(ts).Hours() <= 24 {
			result.Summary.RecentEvents24h++
		}
	}

	// Detect thrashing (HPAs with >10 scaling events)
	for key, count := range hpaEventCount {
		if count > 10 {
			parts := strings.SplitN(key, "/", 2)
			ns, name := "", ""
			if len(parts) == 2 {
				ns = parts[0]
				name = parts[1]
			}
			severity := "medium"
			if count > 20 {
				severity = "high"
			}
			result.ThrashingHPAs = append(result.ThrashingHPAs, HPAThrashEntry1936{
				HPAName: name, Namespace: ns, EventCount: count,
				Severity: severity,
				Detail:   fmt.Sprintf("%d scaling events — HPA is thrashing, tune stabilizationWindowSeconds", count),
			})
			result.Summary.ThrashingCount++
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.ThrashingCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d HPAs thrashing (>10 events) — tune stabilization window", result.Summary.ThrashingCount))
	}
	if result.Summary.RecentEvents24h > 50 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d scaling events in 24h — review HPA thresholds", result.Summary.RecentEvents24h))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Condition History
// ---------------------------------------------------------------

type NodeCondResult1936 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeCondSummary1936 `json:"summary"`
	Nodes           []NodeCondEntry1936 `json:"nodes"`
	FlappingNodes   []NodeFlapEntry1936 `json:"flappingNodes"`
	Recommendations []string            `json:"recommendations"`
}

type NodeCondSummary1936 struct {
	TotalNodes      int `json:"totalNodes"`
	HealthyNodes    int `json:"healthyNodes"`
	NodesWithIssues int `json:"nodesWithIssues"`
	DiskPressure    int `json:"diskPressureNodes"`
	MemPressure     int `json:"memPressureNodes"`
	PIDPressure     int `json:"pidPressureNodes"`
	NetworkIssues   int `json:"networkUnavailableNodes"`
	NotReady        int `json:"notReadyNodes"`
}

type NodeCondEntry1936 struct {
	Name           string            `json:"name"`
	Ready          bool              `json:"ready"`
	Conditions     map[string]string `json:"conditions"`
	IssueCount     int               `json:"issueCount"`
	LastTransition string            `json:"lastTransition"`
}

type NodeFlapEntry1936 struct {
	Node     string `json:"node"`
	CondType string `json:"conditionType"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleNodeCondHistory(w http.ResponseWriter, r *http.Request) {
	result := NodeCondResult1936{ScannedAt: time.Now()}
	score := 100

	nodeList, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		conds := make(map[string]string)
		issueCount := 0
		ready := false
		var lastTrans string

		for _, cond := range node.Status.Conditions {
			status := string(cond.Status)
			conds[string(cond.Type)] = status

			if !cond.LastTransitionTime.IsZero() {
				lastTrans = fmt.Sprintf("%.0fd ago", time.Since(cond.LastTransitionTime.Time).Hours()/24)
			}

			if cond.Type == corev1.NodeReady {
				ready = (status == "True")
				if !ready {
					result.Summary.NotReady++
					result.FlappingNodes = append(result.FlappingNodes, NodeFlapEntry1936{
						Node: node.Name, CondType: "Ready", Severity: "critical",
						Detail: "Node is NotReady",
					})
					score -= 20
				}
			} else if status == "True" {
				issueCount++
				switch cond.Type {
				case corev1.NodeDiskPressure:
					result.Summary.DiskPressure++
					result.FlappingNodes = append(result.FlappingNodes, NodeFlapEntry1936{
						Node: node.Name, CondType: "DiskPressure", Severity: "high",
						Detail: "Disk pressure — node storage nearly full",
					})
					score -= 10
				case corev1.NodeMemoryPressure:
					result.Summary.MemPressure++
					result.FlappingNodes = append(result.FlappingNodes, NodeFlapEntry1936{
						Node: node.Name, CondType: "MemoryPressure", Severity: "high",
						Detail: "Memory pressure — node memory nearly exhausted",
					})
					score -= 10
				case corev1.NodePIDPressure:
					result.Summary.PIDPressure++
					result.FlappingNodes = append(result.FlappingNodes, NodeFlapEntry1936{
						Node: node.Name, CondType: "PIDPressure", Severity: "medium",
						Detail: "PID pressure — process limit approaching",
					})
					score -= 5
				case corev1.NodeNetworkUnavailable:
					result.Summary.NetworkIssues++
					score -= 10
				}
			}
		}

		entry := NodeCondEntry1936{
			Name: node.Name, Ready: ready, Conditions: conds,
			IssueCount: issueCount, LastTransition: lastTrans,
		}
		result.Nodes = append(result.Nodes, entry)

		if issueCount == 0 && ready {
			result.Summary.HealthyNodes++
		} else {
			result.Summary.NodesWithIssues++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.NotReady > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes NotReady — investigate immediately", result.Summary.NotReady))
	}
	if result.Summary.DiskPressure > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with disk pressure — clean up images and logs", result.Summary.DiskPressure))
	}
	if result.Summary.MemPressure > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with memory pressure — add nodes or reduce pod density", result.Summary.MemPressure))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. ConfigMap Change Tracker
// ---------------------------------------------------------------

type ConfigChangeResult1936 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ConfigChangeSummary1936 `json:"summary"`
	RecentChanges   []ConfigChangeEntry1936 `json:"recentChanges"`
	StaleConfigs    []ConfigStaleEntry1936  `json:"staleConfigs"`
	LargeConfigs    []ConfigLargeEntry1936  `json:"largeConfigs"`
	Recommendations []string                `json:"recommendations"`
}

type ConfigChangeSummary1936 struct {
	TotalConfigMaps int `json:"totalConfigMaps"`
	Changed24h      int `json:"changedIn24h"`
	Changed7d       int `json:"changedIn7d"`
	StaleCount      int `json:"staleCount"`
	LargeCount      int `json:"largeConfigMapCount"`
	ImmutableCount  int `json:"immutableCount"`
	TotalDataKeys   int `json:"totalDataKeys"`
}

type ConfigChangeEntry1936 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"ageSinceChange"`
	KeyCount  int    `json:"keyCount"`
	DataSize  int    `json:"dataSizeBytes"`
}

type ConfigStaleEntry1936 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
}

type ConfigLargeEntry1936 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Size      int    `json:"sizeBytes"`
	KeyCount  int    `json:"keyCount"`
}

func (s *Server) handleConfigChangeTracker(w http.ResponseWriter, r *http.Request) {
	result := ConfigChangeResult1936{ScannedAt: time.Now()}
	score := 100

	cmList, err := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	now := time.Now()

	for _, cm := range cmList.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		result.Summary.TotalConfigMaps++

		keyCount := len(cm.Data) + len(cm.BinaryData)
		result.Summary.TotalDataKeys += keyCount

		dataSize := 0
		for _, v := range cm.Data {
			dataSize += len(v)
		}
		for _, v := range cm.BinaryData {
			dataSize += len(v)
		}

		ageHours := now.Sub(cm.CreationTimestamp.Time).Hours()
		ageDays := ageHours / 24

		// Immutable check
		if cm.Immutable != nil && *cm.Immutable {
			result.Summary.ImmutableCount++
		}

		// Recent changes
		if ageHours < 24 {
			result.Summary.Changed24h++
			result.RecentChanges = append(result.RecentChanges, ConfigChangeEntry1936{
				Name: cm.Name, Namespace: cm.Namespace,
				Age:      fmt.Sprintf("%.1fh", ageHours),
				KeyCount: keyCount, DataSize: dataSize,
			})
		} else if ageHours < 168 {
			result.Summary.Changed7d++
		}

		// Stale: not changed in >180 days
		if ageDays > 180 {
			result.Summary.StaleCount++
			result.StaleConfigs = append(result.StaleConfigs, ConfigStaleEntry1936{
				Name: cm.Name, Namespace: cm.Namespace,
				Age:    fmt.Sprintf("%.0fd", ageDays),
				Reason: "ConfigMap not updated in >180 days — verify still needed",
			})
			score -= 1
		}

		// Large ConfigMaps (>1MB) can cause etcd issues
		if dataSize > 1048576 {
			result.Summary.LargeCount++
			result.LargeConfigs = append(result.LargeConfigs, ConfigLargeEntry1936{
				Name: cm.Name, Namespace: cm.Namespace,
				Size: dataSize, KeyCount: keyCount,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.LargeCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ConfigMaps >1MB — move to external config store", result.Summary.LargeCount))
	}
	if result.Summary.StaleCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale ConfigMaps (>180 days) — clean up unused", result.Summary.StaleCount))
	}
	if result.Summary.Changed24h > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ConfigMaps changed in last 24h — verify intentional", result.Summary.Changed24h))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
