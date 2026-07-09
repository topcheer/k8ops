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

// KubeletHealthResult is the full kubelet & container runtime health analysis.
type KubeletHealthResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         KubeletHealthSummary `json:"summary"`
	ByNode          []KubeletNodeEntry   `json:"byNode"`
	UnhealthyNodes  []KubeletNodeEntry   `json:"unhealthyNodes"`
	RuntimeVersions map[string]int       `json:"runtimeVersions"` // runtime version → node count
	OSVersions      map[string]int       `json:"osVersions"`      // OS image → node count
	Issues          []KubeletIssue       `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

// KubeletHealthSummary aggregates cluster-wide kubelet health.
type KubeletHealthSummary struct {
	TotalNodes          int `json:"totalNodes"`
	HealthyNodes        int `json:"healthyNodes"`
	UnhealthyNodes      int `json:"unhealthyNodes"`
	VersionSkew         int `json:"versionSkew"` // nodes with different kubelet version
	RuntimeSkew         int `json:"runtimeSkew"` // nodes with different container runtime
	KubeletVersions     int `json:"distinctKubeletVersions"`
	RuntimeVersions     int `json:"distinctRuntimeVersions"`
	NodesWithConditions int `json:"nodesWithConditions"` // nodes with active condition problems
	OldHeartbeatNodes   int `json:"oldHeartbeatNodes"`   // node heartbeat > 60s old
	HealthScore         int `json:"healthScore"`         // 0-100
}

// KubeletNodeEntry describes kubelet health for one node.
type KubeletNodeEntry struct {
	NodeName        string             `json:"nodeName"`
	Healthy         bool               `json:"healthy"`
	KubeletVersion  string             `json:"kubeletVersion"`
	RuntimeVersion  string             `json:"runtimeVersion"`
	OSImage         string             `json:"osImage"`
	Architecture    string             `json:"architecture"`
	KernelVersion   string             `json:"kernelVersion"`
	LastHeartbeat   time.Time          `json:"lastHeartbeat"`
	HeartbeatAgeSec float64            `json:"heartbeatAgeSeconds"`
	Conditions      []KubeletCondition `json:"conditions,omitempty"`
	PodCount        int                `json:"podCount"`
	IsControlPlane  bool               `json:"isControlPlane"`
	RiskLevel       string             `json:"riskLevel"`
}

// KubeletCondition describes a single node condition relevant to kubelet health.
type KubeletCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

// KubeletIssue is a detected kubelet or runtime problem.
type KubeletIssue struct {
	NodeName string `json:"nodeName"`
	Severity string `json:"severity"`
	Category string `json:"category"` // version_skew, runtime_skew, heartbeat, condition
	Message  string `json:"message"`
}

// handleKubeletHealth analyzes kubelet and container runtime health across all nodes.
// GET /api/operations/kubelet-health
func (s *Server) handleKubeletHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod count per node
	podCountPerNode := map[string]int{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			podCountPerNode[pod.Spec.NodeName]++
		}
	}

	now := time.Now()
	result := KubeletHealthResult{
		ScannedAt:       now,
		RuntimeVersions: map[string]int{},
		OSVersions:      map[string]int{},
	}
	result.Summary.TotalNodes = len(nodes.Items)

	// Track version distributions
	kubeletVersions := map[string]int{}
	runtimeVersions := map[string]int{}

	// Determine majority versions
	for _, node := range nodes.Items {
		kv := node.Status.NodeInfo.KubeletVersion
		rv := node.Status.NodeInfo.ContainerRuntimeVersion
		kubeletVersions[kv]++
		runtimeVersions[rv]++

		// Parse runtime type (e.g., "containerd://1.7.0" -> "containerd")
		rtType := parseRuntimeType(rv)
		result.RuntimeVersions[rtType]++
		result.OSVersions[node.Status.NodeInfo.OSImage]++
	}

	majorKubeletVer := getMajorityKey(kubeletVersions)
	majorRuntimeVer := getMajorityKey(runtimeVersions)

	result.Summary.KubeletVersions = len(kubeletVersions)
	result.Summary.RuntimeVersions = len(runtimeVersions)

	for _, node := range nodes.Items {
		entry := KubeletNodeEntry{
			NodeName:       node.Name,
			KubeletVersion: node.Status.NodeInfo.KubeletVersion,
			RuntimeVersion: node.Status.NodeInfo.ContainerRuntimeVersion,
			OSImage:        node.Status.NodeInfo.OSImage,
			Architecture:   node.Status.NodeInfo.Architecture,
			KernelVersion:  node.Status.NodeInfo.KernelVersion,
			PodCount:       podCountPerNode[node.Name],
			IsControlPlane: isControlPlaneNode(&node),
		}

		// Last heartbeat
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				entry.LastHeartbeat = cond.LastHeartbeatTime.Time
				entry.HeartbeatAgeSec = now.Sub(cond.LastHeartbeatTime.Time).Seconds()
				break
			}
		}

		// Check conditions
		healthy := true
		var conds []KubeletCondition
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if cond.Status != corev1.ConditionTrue {
					healthy = false
					result.Summary.NodesWithConditions++
					result.Issues = append(result.Issues, KubeletIssue{
						NodeName: node.Name,
						Severity: "critical",
						Category: "condition",
						Message:  fmt.Sprintf("Node is NotReady: %s - %s", cond.Reason, cond.Message),
					})
				}
				continue
			}
			// Other conditions (DiskPressure, MemoryPressure, PIDPressure, NetworkUnavailable)
			if cond.Status == corev1.ConditionTrue {
				healthy = false
				result.Summary.NodesWithConditions++
				conds = append(conds, KubeletCondition{
					Type:               string(cond.Type),
					Status:             string(cond.Status),
					Reason:             cond.Reason,
					Message:            cond.Message,
					LastTransitionTime: cond.LastTransitionTime.Format(time.RFC3339),
				})
				result.Issues = append(result.Issues, KubeletIssue{
					NodeName: node.Name,
					Severity: "high",
					Category: "condition",
					Message:  fmt.Sprintf("%s active: %s", cond.Type, cond.Message),
				})
			}
		}
		entry.Conditions = conds

		// Check heartbeat staleness
		if entry.HeartbeatAgeSec > 60 {
			healthy = false
			result.Summary.OldHeartbeatNodes++
			severity := "medium"
			if entry.HeartbeatAgeSec > 300 {
				severity = "critical"
			} else if entry.HeartbeatAgeSec > 120 {
				severity = "high"
			}
			result.Issues = append(result.Issues, KubeletIssue{
				NodeName: node.Name,
				Severity: severity,
				Category: "heartbeat",
				Message:  fmt.Sprintf("Kubelet heartbeat is %.0fs old (last: %s)", entry.HeartbeatAgeSec, entry.LastHeartbeat.Format(time.RFC3339)),
			})
		}

		// Check version skew
		if node.Status.NodeInfo.KubeletVersion != majorKubeletVer {
			result.Summary.VersionSkew++
			severity := "low"
			if isMajorVersionDiff(node.Status.NodeInfo.KubeletVersion, majorKubeletVer) {
				severity = "medium"
			}
			result.Issues = append(result.Issues, KubeletIssue{
				NodeName: node.Name,
				Severity: severity,
				Category: "version_skew",
				Message:  fmt.Sprintf("Kubelet version %s differs from cluster majority %s", node.Status.NodeInfo.KubeletVersion, majorKubeletVer),
			})
		}

		// Check runtime skew
		if node.Status.NodeInfo.ContainerRuntimeVersion != majorRuntimeVer {
			result.Summary.RuntimeSkew++
			result.Issues = append(result.Issues, KubeletIssue{
				NodeName: node.Name,
				Severity: "low",
				Category: "runtime_skew",
				Message:  fmt.Sprintf("Container runtime %s differs from cluster majority %s", node.Status.NodeInfo.ContainerRuntimeVersion, majorRuntimeVer),
			})
		}

		// Risk level
		entry.RiskLevel = assessKubeletRisk(entry.HeartbeatAgeSec, len(conds), node.Status.NodeInfo.KubeletVersion != majorKubeletVer)
		entry.Healthy = healthy

		if healthy {
			result.Summary.HealthyNodes++
		} else {
			result.Summary.UnhealthyNodes++
			result.UnhealthyNodes = append(result.UnhealthyNodes, entry)
		}

		result.ByNode = append(result.ByNode, entry)
	}

	// Sort nodes: unhealthy first, then by heartbeat age
	sort.Slice(result.ByNode, func(i, j int) bool {
		if result.ByNode[i].Healthy != result.ByNode[j].Healthy {
			return !result.ByNode[i].Healthy
		}
		return result.ByNode[i].HeartbeatAgeSec > result.ByNode[j].HeartbeatAgeSec
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Issues[i].Severity] < sevOrder[result.Issues[j].Severity]
	})
	if len(result.Issues) > 50 {
		result.Issues = result.Issues[:50]
	}

	result.Summary.HealthScore = kubeletHealthScore(result.Summary)
	result.Recommendations = kubeletHealthRecommendations(&result)

	writeJSON(w, result)
}

// parseRuntimeType extracts the runtime type from a version string like "containerd://1.7.0".
func parseRuntimeType(rt string) string {
	if idx := strings.Index(rt, "://"); idx > 0 {
		return rt[:idx]
	}
	return rt
}

// getMajorityKey returns the key with the highest count.
func getMajorityKey(counts map[string]int) string {
	max := 0
	result := ""
	for k, v := range counts {
		if v > max {
			max = v
			result = k
		}
	}
	return result
}

// isMajorVersionDiff checks if two kubelet versions differ by a major version number.
func isMajorVersionDiff(v1, v2 string) bool {
	m1 := extractMajorVersion(v1)
	m2 := extractMajorVersion(v2)
	return m1 != "" && m2 != "" && m1 != m2
}

// extractMajorVersion extracts the major.minor version (e.g., "v1.28" from "v1.28.4+k3s1").
func extractMajorVersion(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// isControlPlaneNode checks if a node has the control-plane label.
func isControlPlaneNode(node *corev1.Node) bool {
	if node.Labels == nil {
		return false
	}
	if v, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok || node.Labels["node-role.kubernetes.io/control-plane"] == "" {
		_ = v
		return true
	}
	if node.Labels["node-role.kubernetes.io/master"] == "" {
		return true
	}
	return false
}

// assessKubeletRisk determines overall risk level for a node.
func assessKubeletRisk(heartbeatAge float64, activeConds int, versionSkew bool) string {
	risk := 0
	if heartbeatAge > 300 {
		risk += 3
	} else if heartbeatAge > 120 {
		risk += 2
	} else if heartbeatAge > 60 {
		risk += 1
	}
	risk += activeConds
	if versionSkew {
		risk++
	}

	switch {
	case risk >= 3:
		return "critical"
	case risk >= 2:
		return "high"
	case risk >= 1:
		return "medium"
	default:
		return "low"
	}
}

// kubeletHealthScore computes a 0-100 health score.
func kubeletHealthScore(s KubeletHealthSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}

	score := 100

	// Penalize unhealthy nodes
	unhealthyRatio := float64(s.UnhealthyNodes) / float64(s.TotalNodes)
	score -= int(unhealthyRatio * 50)

	// Penalize version skew
	if s.VersionSkew > 0 {
		score -= min(15, s.VersionSkew*3)
	}

	// Penalize runtime skew
	if s.RuntimeSkew > 0 {
		score -= min(10, s.RuntimeSkew*2)
	}

	// Penalize stale heartbeats
	if s.OldHeartbeatNodes > 0 {
		score -= min(15, s.OldHeartbeatNodes*5)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// kubeletHealthRecommendations generates actionable recommendations.
func kubeletHealthRecommendations(r *KubeletHealthResult) []string {
	var recs []string

	if r.Summary.UnhealthyNodes > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) are unhealthy — investigate kubelet logs, node conditions, and consider draining/cordoning if needed",
			r.Summary.UnhealthyNodes,
		))
	}

	if r.Summary.VersionSkew > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have a different kubelet version — upgrade all nodes to the same version to avoid compatibility issues",
			r.Summary.VersionSkew,
		))
	}

	if r.Summary.RuntimeSkew > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have a different container runtime version — standardize on one runtime version to avoid behavior differences",
			r.Summary.RuntimeSkew,
		))
	}

	if r.Summary.OldHeartbeatNodes > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have stale kubelet heartbeats (>60s) — check kubelet process health, network connectivity, and node load",
			r.Summary.OldHeartbeatNodes,
		))
	}

	if r.Summary.NodesWithConditions > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have active conditions (DiskPressure, MemoryPressure, etc.) — investigate resource exhaustion or misconfiguration",
			r.Summary.NodesWithConditions,
		))
	}

	if r.Summary.RuntimeVersions > 2 {
		recs = append(recs, fmt.Sprintf(
			"%d distinct kubelet versions detected — consolidate to a single version for consistency",
			r.Summary.KubeletVersions,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "All nodes are healthy with consistent kubelet and runtime versions — no issues detected")
	}

	return recs
}
