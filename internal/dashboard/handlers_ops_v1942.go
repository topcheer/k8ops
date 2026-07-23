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
// v19.42 — Operations Dimension (Round 10)
// 1. Pod Restart Storm Detector — rapid restart detection
// 2. Event Storm Analyzer — abnormal event volume & rate
// 3. Node Taint Impact — taints preventing scheduling
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Restart Storm Detector
// ---------------------------------------------------------------

type RestartStormResult1942 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RestartStormSummary1942 `json:"summary"`
	StormPods       []RestartStormEntry1942 `json:"stormPods"`
	ByNamespace     []RestartNSStat1942     `json:"byNamespace"`
	Recommendations []string                `json:"recommendations"`
}

type RestartStormSummary1942 struct {
	TotalPods        int `json:"totalPods"`
	PodsWithRestarts int `json:"podsWithRestarts"`
	StormCount       int `json:"stormCount"`
	CriticalCount    int `json:"criticalCount"`
	TotalRestarts    int `json:"totalRestarts"`
	MaxRestarts      int `json:"maxRestarts"`
}

type RestartStormEntry1942 struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Restarts    int     `json:"restartCount"`
	LastState   string  `json:"lastState"`
	AgeHours    float64 `json:"ageHours"`
	RestartRate float64 `json:"restartRatePerHour"`
	Severity    string  `json:"severity"`
}

type RestartNSStat1942 struct {
	Namespace     string `json:"namespace"`
	TotalRestarts int    `json:"totalRestarts"`
	PodCount      int    `json:"podCount"`
}

func (s *Server) handleRestartStormV2(w http.ResponseWriter, r *http.Request) {
	result := RestartStormResult1942{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]*RestartNSStat1942)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := 0
		lastState := ""
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
			if cs.LastTerminationState.Terminated != nil {
				lastState = cs.LastTerminationState.Terminated.Reason
			}
		}

		if totalRestarts > 0 {
			result.Summary.PodsWithRestarts++
			result.Summary.TotalRestarts += totalRestarts
			if totalRestarts > result.Summary.MaxRestarts {
				result.Summary.MaxRestarts = totalRestarts
			}

			ageHours := time.Since(pod.CreationTimestamp.Time).Hours()
			if ageHours < 1 {
				ageHours = 1
			}
			restartRate := float64(totalRestarts) / ageHours

			// Storm: >5 restarts OR >0.5 restarts/hour
			severity := "low"
			isStorm := false
			if totalRestarts > 20 || restartRate > 2 {
				severity = "critical"
				result.Summary.CriticalCount++
				isStorm = true
				score -= 5
			} else if totalRestarts > 10 || restartRate > 1 {
				severity = "high"
				isStorm = true
				score -= 3
			} else if totalRestarts > 5 || restartRate > 0.5 {
				severity = "medium"
				isStorm = true
				score -= 1
			}

			if isStorm {
				result.Summary.StormCount++
				result.StormPods = append(result.StormPods, RestartStormEntry1942{
					Name: pod.Name, Namespace: pod.Namespace,
					Restarts: totalRestarts, LastState: lastState,
					AgeHours: ageHours, RestartRate: restartRate,
					Severity: severity,
				})
			}
		}

		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &RestartNSStat1942{Namespace: ns}
		}
		nsStats[ns].TotalRestarts += totalRestarts
		nsStats[ns].PodCount++
	}

	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalRestarts > result.ByNamespace[j].TotalRestarts
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.StormCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods in restart storm — investigate CrashLoopBackOff", result.Summary.StormCount))
	}
	if result.Summary.CriticalCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d critical pods (>20 restarts or >2/hr) — immediate attention", result.Summary.CriticalCount))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Event Storm Analyzer
// ---------------------------------------------------------------

type EventStormResult1942 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         EventStormSummary1942   `json:"summary"`
	TopEvents       []EventStormEntry1942   `json:"topEvents"`
	ByReason        []EventReasonStat1942   `json:"byReason"`
	Warnings        []EventStormWarning1942 `json:"warnings"`
	Recommendations []string                `json:"recommendations"`
}

type EventStormSummary1942 struct {
	TotalEvents   int  `json:"totalEvents"`
	WarningEvents int  `json:"warningEvents"`
	NormalEvents  int  `json:"normalEvents"`
	EventsLast1h  int  `json:"eventsLast1h"`
	EventsLast24h int  `json:"eventsLast24h"`
	UniqueReasons int  `json:"uniqueReasons"`
	StormDetected bool `json:"stormDetected"`
}

type EventStormEntry1942 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	Type      string `json:"type"`
	Count     int32  `json:"count"`
	LastSeen  string `json:"lastSeen"`
}

type EventReasonStat1942 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type EventStormWarning1942 struct {
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleEventStormV2(w http.ResponseWriter, r *http.Request) {
	result := EventStormResult1942{ScannedAt: time.Now()}
	score := 100
	reasonCounts := make(map[string]int)
	now := time.Now()

	evList, err := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, ev := range evList.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvents++

		if ev.Type == "Warning" {
			result.Summary.WarningEvents++
		} else {
			result.Summary.NormalEvents++
		}

		reasonCounts[ev.Reason]++

		var ts time.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}
		if !ts.IsZero() {
			if now.Sub(ts).Hours() <= 1 {
				result.Summary.EventsLast1h++
			}
			if now.Sub(ts).Hours() <= 24 {
				result.Summary.EventsLast24h++
			}
		}

		if int(ev.Count) > 5 && ev.Type == "Warning" {
			entry := EventStormEntry1942{
				Name: ev.InvolvedObject.Name, Namespace: ev.Namespace,
				Reason: ev.Reason, Type: ev.Type, Count: ev.Count,
				LastSeen: fmt.Sprintf("%.0fm ago", now.Sub(ts).Minutes()),
			}
			result.TopEvents = append(result.TopEvents, entry)
		}
	}

	result.Summary.UniqueReasons = len(reasonCounts)
	result.Summary.StormDetected = result.Summary.EventsLast1h > 50

	for reason, count := range reasonCounts {
		result.ByReason = append(result.ByReason, EventReasonStat1942{Reason: reason, Count: count})
		if count > 20 && reason != "" {
			severity := "medium"
			if count > 100 {
				severity = "high"
			}
			result.Warnings = append(result.Warnings, EventStormWarning1942{
				Reason: reason, Severity: severity,
				Detail: fmt.Sprintf("%d events with reason '%s' — investigate root cause", count, reason),
			})
			if severity == "high" {
				score -= 5
			} else {
				score -= 2
			}
		}
	}
	sort.Slice(result.ByReason, func(i, j int) bool {
		return result.ByReason[i].Count > result.ByReason[j].Count
	})

	if result.Summary.StormDetected {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.StormDetected {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Event storm detected — %d events in last hour", result.Summary.EventsLast1h))
	}
	if result.Summary.WarningEvents > 50 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d warning events — investigate common reasons", result.Summary.WarningEvents))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Taint Impact
// ---------------------------------------------------------------

type TaintImpactResult1942 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         TaintImpactSummary1942  `json:"summary"`
	Taints          []TaintImpactEntry1942  `json:"taints"`
	BlockedPods     []TaintBlockedEntry1942 `json:"blockedPods"`
	Recommendations []string                `json:"recommendations"`
}

type TaintImpactSummary1942 struct {
	TotalNodes       int `json:"totalNodes"`
	TaintedNodes     int `json:"taintedNodes"`
	TotalTaints      int `json:"totalTaints"`
	NoScheduleTaints int `json:"noScheduleTaints"`
	NoExecuteTaints  int `json:"noExecuteTaints"`
	CordonedNodes    int `json:"cordonedNodes"`
	NodesReady       int `json:"nodesReady"`
}

type TaintImpactEntry1942 struct {
	Node     string `json:"node"`
	Key      string `json:"key"`
	Effect   string `json:"effect"`
	Value    string `json:"value"`
	Severity string `json:"severity"`
}

type TaintBlockedEntry1942 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	NodeName  string `json:"nodeName"`
}

func (s *Server) handleTaintImpact(w http.ResponseWriter, r *http.Request) {
	result := TaintImpactResult1942{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		isReady := true
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status != "True" {
				isReady = false
				break
			}
		}
		if isReady {
			result.Summary.NodesReady++
		}

		// Cordon check
		if node.Spec.Unschedulable {
			result.Summary.CordonedNodes++
		}

		for _, taint := range node.Spec.Taints {
			result.Summary.TotalTaints++
			severity := "low"
			if taint.Effect == "NoExecute" {
				result.Summary.NoExecuteTaints++
				severity = "high"
				score -= 5
			} else if taint.Effect == "NoSchedule" {
				result.Summary.NoScheduleTaints++
				severity = "medium"
				score -= 2
			}

			result.Taints = append(result.Taints, TaintImpactEntry1942{
				Node: node.Name, Key: taint.Key,
				Effect: string(taint.Effect), Value: taint.Value,
				Severity: severity,
			})
		}

		if len(node.Spec.Taints) > 0 {
			result.Summary.TaintedNodes++
		}
	}

	// Find pending pods that may be blocked by taints
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodPending && pod.Spec.NodeName == "" {
			reason := "Pending pod not scheduled"
			for _, cond := range pod.Status.Conditions {
				if cond.Reason == "Unschedulable" {
					reason = cond.Message
					break
				}
			}
			if strings.Contains(strings.ToLower(reason), "taint") || strings.Contains(strings.ToLower(reason), "tolerat") {
				result.BlockedPods = append(result.BlockedPods, TaintBlockedEntry1942{
					PodName: pod.Name, Namespace: pod.Namespace,
					Reason: reason, NodeName: "",
				})
				score -= 5
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.CordonedNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d cordoned nodes — uncordon when ready", result.Summary.CordonedNodes))
	}
	if result.Summary.NoExecuteTaints > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d NoExecute taints — evicting pods from nodes", result.Summary.NoExecuteTaints))
	}
	if len(result.BlockedPods) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods blocked by taints — add tolerations", len(result.BlockedPods)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
