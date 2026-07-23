package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.34 — Scalability & HA Dimension (Round 8 Final)
// 1. Scheduler Queue Depth — scheduling pressure analysis
// 2. Pod Spread Violation — topology spread constraint violations
// 3. HA Topology Score — multi-zone/failure-domain HA readiness
// ============================================================

// ---------------------------------------------------------------
// 1. Scheduler Queue Depth
// ---------------------------------------------------------------

type SchedQueueResult1934 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SchedQueueSummary1934 `json:"summary"`
	PendingPods     []SchedQueueEntry1934 `json:"pendingPods"`
	Bottlenecks     []SchedBottleneck1934 `json:"bottlenecks"`
	Recommendations []string              `json:"recommendations"`
}

type SchedQueueSummary1934 struct {
	TotalPods    int     `json:"totalPods"`
	RunningPods  int     `json:"runningPods"`
	PendingPods  int     `json:"pendingPods"`
	FailedPods   int     `json:"failedPods"`
	PendingHours float64 `json:"oldestPendingHours"`
	AvgSchedAge  string  `json:"avgSchedAge"`
	TotalNodes   int     `json:"totalNodes"`
}

type SchedQueueEntry1934 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Phase     string  `json:"phase"`
	Reason    string  `json:"reason"`
	PendHours float64 `json:"pendingHours"`
	NodeName  string  `json:"nodeName"`
}

type SchedBottleneck1934 struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleSchedQueueDepth(w http.ResponseWriter, r *http.Request) {
	result := SchedQueueResult1934{ScannedAt: time.Now()}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodeList.Items)

	var oldestPending float64
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++

		switch pod.Status.Phase {
		case corev1.PodRunning:
			result.Summary.RunningPods++
		case corev1.PodPending:
			result.Summary.PendingPods++
			pendHrs := time.Since(pod.CreationTimestamp.Time).Hours()
			if pendHrs > oldestPending {
				oldestPending = pendHrs
			}
			reason := ""
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil {
					reason = cs.State.Waiting.Reason
					break
				}
			}
			if reason == "" && len(pod.Status.Conditions) > 0 {
				for _, cond := range pod.Status.Conditions {
					if cond.Reason != "" {
						reason = cond.Reason
						break
					}
				}
			}
			entry := SchedQueueEntry1934{
				Name: pod.Name, Namespace: pod.Namespace,
				Phase: "Pending", Reason: reason,
				PendHours: pendHrs, NodeName: pod.Spec.NodeName,
			}
			result.PendingPods = append(result.PendingPods, entry)

			if reason == "Unschedulable" || reason == "SchedulingDisabled" {
				result.Bottlenecks = append(result.Bottlenecks, SchedBottleneck1934{
					Type: "unschedulable", Severity: "high",
					Detail: fmt.Sprintf("%s: %s", pod.Name, reason),
				})
				score -= 5
			}
			if pendHrs > 1 {
				result.Bottlenecks = append(result.Bottlenecks, SchedBottleneck1934{
					Type: "stuck-pending", Severity: "medium",
					Detail: fmt.Sprintf("%s pending for %.1f hours", pod.Name, pendHrs),
				})
				score -= 2
			}
		case corev1.PodFailed:
			result.Summary.FailedPods++
			score -= 3
		}
	}

	result.Summary.PendingHours = oldestPending
	if result.Summary.RunningPods > 0 {
		result.Summary.AvgSchedAge = fmt.Sprintf("%.0f pods running on %d nodes", float64(result.Summary.RunningPods), result.Summary.TotalNodes)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PendingPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pending pods — check resource availability and node capacity", result.Summary.PendingPods))
	}
	if len(result.Bottlenecks) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d scheduling bottlenecks — investigate constraints", len(result.Bottlenecks)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Spread Violation
// ---------------------------------------------------------------

type PodSpreadResult1934 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         PodSpreadSummary1934     `json:"summary"`
	Violations      []PodSpreadViolation1934 `json:"violations"`
	NodeBalance     []NodeBalanceEntry1934   `json:"nodeBalance"`
	Recommendations []string                 `json:"recommendations"`
}

type PodSpreadSummary1934 struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	WithSpread     int     `json:"withSpreadConstraints"`
	Violations     int     `json:"spreadViolations"`
	TotalNodes     int     `json:"totalNodes"`
	ImbalanceScore float64 `json:"imbalanceScore"`
	MaxSkew        int     `json:"maxSkew"`
}

type PodSpreadViolation1934 struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
	Skew      int    `json:"skew"`
}

type NodeBalanceEntry1934 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
	CPUPct   int    `json:"cpuPct"`
	MemPct   int    `json:"memPct"`
}

func (s *Server) handlePodSpreadViolation(w http.ResponseWriter, r *http.Request) {
	result := PodSpreadResult1934{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodeList.Items)

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Group pods by workload and node
	type wlKey struct{ ns, name string }
	wlNodes := make(map[wlKey]map[string]int)
	nodePodCount := make(map[string]int)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}
		key := wlKey{ns: pod.Namespace, name: appName}
		if wlNodes[key] == nil {
			wlNodes[key] = make(map[string]int)
		}
		wlNodes[key][pod.Spec.NodeName]++
		nodePodCount[pod.Spec.NodeName]++
	}

	maxSkew := 0
	for key, nodes := range wlNodes {
		result.Summary.TotalWorkloads++
		if len(nodes) <= 1 {
			continue // single node or single replica, can't violate spread
		}
		// Calculate skew (max - min across nodes)
		maxC := 0
		minC := 999999
		for _, c := range nodes {
			if c > maxC {
				maxC = c
			}
			if c < minC {
				minC = c
			}
		}
		skew := maxC - minC
		if skew > maxSkew {
			maxSkew = skew
		}
		if skew > 1 && len(nodes) > 1 {
			result.Summary.Violations++
			severity := "medium"
			if skew > 3 {
				severity = "high"
			}
			result.Violations = append(result.Violations, PodSpreadViolation1934{
				Workload: key.name, Namespace: key.ns,
				Violation: fmt.Sprintf("Pod skew=%d across %d nodes — uneven distribution", skew, len(nodes)),
				Severity:  severity, Skew: skew,
			})
			score -= 3
		}
	}

	result.Summary.MaxSkew = maxSkew
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.ImbalanceScore = float64(result.Summary.Violations) * 100 / float64(result.Summary.TotalWorkloads)
	}

	// Node balance
	for _, node := range nodeList.Items {
		result.NodeBalance = append(result.NodeBalance, NodeBalanceEntry1934{
			Node: node.Name, PodCount: nodePodCount[node.Name],
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.Violations > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pod spread violations — add topologySpreadConstraints", result.Summary.Violations))
	}
	if maxSkew > 2 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Max skew=%d — rebalance pods across nodes", maxSkew))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. HA Topology Score
// ---------------------------------------------------------------

type HATopoResult1934 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         HATopoSummary1934        `json:"summary"`
	FailureDomains  []FailureDomainEntry1934 `json:"failureDomains"`
	WorkloadHA      []WorkloadHAEntry1934    `json:"workloadHA"`
	Risks           []HATopoRisk1934         `json:"risks"`
	Recommendations []string                 `json:"recommendations"`
}

type HATopoSummary1934 struct {
	TotalNodes    int `json:"totalNodes"`
	TotalZones    int `json:"totalZones"`
	TotalRegions  int `json:"totalRegions"`
	WorkloadCount int `json:"workloadCount"`
	HACompliant   int `json:"haCompliant"`
	NonHA         int `json:"nonHA"`
	SingleReplica int `json:"singleReplicaWorkloads"`
	SingleNode    int `json:"singleNodeWorkloads"`
}

type FailureDomainEntry1934 struct {
	DomainType string `json:"domainType"`
	Name       string `json:"name"`
	NodeCount  int    `json:"nodeCount"`
	PodCount   int    `json:"podCount"`
}

type WorkloadHAEntry1934 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	NodeCount int    `json:"nodeCount"`
	ZoneCount int    `json:"zoneCount"`
	IsHA      bool   `json:"isHA"`
}

type HATopoRisk1934 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleHATopoScore(w http.ResponseWriter, r *http.Request) {
	result := HATopoResult1934{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodeList.Items)

	// Collect failure domains
	zoneMap := make(map[string]int)
	regionMap := make(map[string]int)
	nodePods := make(map[string]int)
	for _, node := range nodeList.Items {
		zone := node.Labels["topology.kubernetes.io/zone"]
		region := node.Labels["topology.kubernetes.io/region"]
		if zone != "" {
			zoneMap[zone]++
		}
		if region != "" {
			regionMap[region]++
		}
	}
	result.Summary.TotalZones = len(zoneMap)
	result.Summary.TotalRegions = len(regionMap)

	for z, c := range zoneMap {
		result.FailureDomains = append(result.FailureDomains, FailureDomainEntry1934{DomainType: "zone", Name: z, NodeCount: c})
	}
	for r2, c := range regionMap {
		result.FailureDomains = append(result.FailureDomains, FailureDomainEntry1934{DomainType: "region", Name: r2, NodeCount: c})
	}
	if result.Summary.TotalZones == 0 {
		result.FailureDomains = append(result.FailureDomains, FailureDomainEntry1934{
			DomainType: "zone", Name: "unknown", NodeCount: result.Summary.TotalNodes,
		})
	}

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Group by workload
	type wlKey struct{ ns, name string }
	wlNodes := make(map[wlKey]map[string]bool)
	wlZones := make(map[wlKey]map[string]bool)
	wlReplicas := make(map[wlKey]int)
	nodeZoneMap := make(map[string]string)

	for _, node := range nodeList.Items {
		nodeZoneMap[node.Name] = node.Labels["topology.kubernetes.io/zone"]
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}
		key := wlKey{ns: pod.Namespace, name: appName}
		wlReplicas[key]++
		if wlNodes[key] == nil {
			wlNodes[key] = make(map[string]bool)
			wlZones[key] = make(map[string]bool)
		}
		wlNodes[key][pod.Spec.NodeName] = true
		nodePods[pod.Spec.NodeName]++
		z := nodeZoneMap[pod.Spec.NodeName]
		if z != "" {
			wlZones[key][z] = true
		}
	}

	for key, nodes := range wlNodes {
		replicas := wlReplicas[key]
		nodeCount := len(nodes)
		zoneCount := len(wlZones[key])
		isHA := replicas >= 2 && nodeCount >= 2

		entry := WorkloadHAEntry1934{
			Name: key.name, Namespace: key.ns,
			Replicas: replicas, NodeCount: nodeCount, ZoneCount: zoneCount, IsHA: isHA,
		}
		result.WorkloadHA = append(result.WorkloadHA, entry)
		result.Summary.WorkloadCount++

		if isHA {
			result.Summary.HACompliant++
		} else {
			result.Summary.NonHA++
			if replicas == 1 {
				result.Summary.SingleReplica++
				result.Risks = append(result.Risks, HATopoRisk1934{
					Name: key.name, Namespace: key.ns,
					RiskType: "single-replica", Severity: "high",
					Detail: "Single replica — no HA, data loss on node failure",
				})
				score -= 2
			} else if nodeCount == 1 {
				result.Summary.SingleNode++
				result.Risks = append(result.Risks, HATopoRisk1934{
					Name: key.name, Namespace: key.ns,
					RiskType: "single-node", Severity: "medium",
					Detail: fmt.Sprintf("%d replicas on 1 node — SPOF", replicas),
				})
				score -= 3
			}
		}
	}

	// Single zone risk
	if result.Summary.TotalZones <= 1 && result.Summary.TotalNodes > 1 {
		result.Risks = append(result.Risks, HATopoRisk1934{
			Name: "cluster", Namespace: "",
			RiskType: "single-zone", Severity: "medium",
			Detail: "All nodes in single zone — zone failure = total outage",
		})
		score -= 5
	}

	// Update pod count on failure domains
	for i := range result.FailureDomains {
		for node, pods := range nodePods {
			if nodeZoneMap[node] == result.FailureDomains[i].Name || result.FailureDomains[i].Name == "unknown" {
				result.FailureDomains[i].PodCount += pods
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SingleReplica > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d single-replica workloads — increase replicas for HA", result.Summary.SingleReplica))
	}
	if result.Summary.SingleNode > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads on single node — add podAntiAffinity for node diversity", result.Summary.SingleNode))
	}
	if result.Summary.TotalZones <= 1 {
		result.Recommendations = append(result.Recommendations, "Single zone cluster — add nodes in different zones for HA")
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
