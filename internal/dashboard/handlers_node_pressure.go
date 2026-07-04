package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePressureResult is the full node condition and resource pressure analysis.
type NodePressureResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         NodePressureSummary `json:"summary"`
	Nodes           []NodePressureEntry `json:"nodes"`
	TopRisks        []NodePressureRisk  `json:"topRisks"`
	Recommendations []string            `json:"recommendations"`
}

// NodePressureSummary aggregates cluster-wide node pressure metrics.
type NodePressureSummary struct {
	TotalNodes         int `json:"totalNodes"`
	HealthyNodes       int `json:"healthyNodes"`
	NodesWithPressure  int `json:"nodesWithPressure"`
	DiskPressure       int `json:"diskPressure"`
	MemoryPressure     int `json:"memoryPressure"`
	PIDPressure        int `json:"pidPressure"`
	NetworkUnavailable int `json:"networkUnavailable"`
	CPUHigh            int `json:"cpuHigh"`    // >80% CPU requested
	MemoryHigh         int `json:"memoryHigh"` // >85% memory requested
	DiskHigh           int `json:"diskHigh"`   // >80% disk usage
	CordonedNodes      int `json:"cordonedNodes"`
	NotReadyNodes      int `json:"notReadyNodes"`
	PressureScore      int `json:"pressureScore"` // 0-100
}

// NodePressureEntry describes pressure conditions for one node.
type NodePressureEntry struct {
	Name        string          `json:"name"`
	Status      string          `json:"status"`      // ready / not-ready / cordoned
	CPUUsagePct float64         `json:"cpuUsagePct"` // requested / allocatable * 100
	MemUsagePct float64         `json:"memUsagePct"`
	PodUsagePct float64         `json:"podUsagePct"` // running pods / capacity
	Conditions  []NodeCondition `json:"conditions"`
	Allocatable NodeAllocatable `json:"allocatable"`
	Requested   NodeRequested   `json:"requested"`
	RiskLevel   string          `json:"riskLevel"` // critical / high / medium / low
	Issues      []string        `json:"issues"`
}

// NodeCondition is a simplified node condition.
type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Since   string `json:"since,omitempty"`
}

// NodeAllocatable is the allocatable resources on a node.
type NodeAllocatable struct {
	CPUm  int64 `json:"cpuM"` // millicores
	MemMB int64 `json:"memMB"`
	Pods  int64 `json:"pods"`
}

// NodeRequested is the sum of pod requests on a node.
type NodeRequested struct {
	CPUm  int64 `json:"cpuM"`
	MemMB int64 `json:"memMB"`
	Pods  int64 `json:"pods"`
}

// NodePressureRisk is a top risk node summary.
type NodePressureRisk struct {
	Node      string `json:"node"`
	RiskLevel string `json:"riskLevel"`
	Reasons   string `json:"reasons"`
}

// handleNodePressure analyzes node conditions and resource pressure.
// GET /api/operations/node-pressure
func (s *Server) handleNodePressure(w http.ResponseWriter, r *http.Request) {
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

	// Build pod resource requests per node
	nodeRequests := make(map[string]*NodeRequested)
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			nodeName := pod.Spec.NodeName
			if nodeName == "" {
				continue
			}
			req := nodeRequests[nodeName]
			if req == nil {
				req = &NodeRequested{}
				nodeRequests[nodeName] = req
			}
			req.Pods++
			for _, c := range pod.Spec.Containers {
				if r := c.Resources.Requests.Cpu(); r != nil && !r.IsZero() {
					req.CPUm += r.MilliValue()
				}
				if r := c.Resources.Requests.Memory(); r != nil && !r.IsZero() {
					req.MemMB += r.Value() / (1024 * 1024)
				}
			}
		}
	}

	result := NodePressureResult{ScannedAt: time.Now()}

	for _, node := range nodes.Items {
		entry := NodePressureEntry{
			Name: node.Name,
		}

		// Node status
		entry.Status = "ready"
		if node.Spec.Unschedulable {
			entry.Status = "cordoned"
			result.Summary.CordonedNodes++
		}
		if !isNodeReady(&node) {
			if entry.Status == "ready" {
				entry.Status = "not-ready"
			}
			result.Summary.NotReadyNodes++
		}

		// Allocatable
		allocCPUm := node.Status.Allocatable.Cpu().MilliValue()
		allocMemMB := node.Status.Allocatable.Memory().Value() / (1024 * 1024)
		allocPods := int64(0)
		if p := node.Status.Allocatable.Pods(); p != nil {
			allocPods = p.Value()
		}
		entry.Allocatable = NodeAllocatable{CPUm: allocCPUm, MemMB: allocMemMB, Pods: allocPods}

		// Requested
		req := nodeRequests[node.Name]
		if req != nil {
			entry.Requested = *req
		} else {
			entry.Requested = NodeRequested{}
		}

		// Usage percentages
		if allocCPUm > 0 {
			entry.CPUUsagePct = float64(entry.Requested.CPUm) / float64(allocCPUm) * 100
		}
		if allocMemMB > 0 {
			entry.MemUsagePct = float64(entry.Requested.MemMB) / float64(allocMemMB) * 100
		}
		if allocPods > 0 {
			entry.PodUsagePct = float64(entry.Requested.Pods) / float64(allocPods) * 100
		}

		// Conditions
		var issues []string
		for _, cond := range node.Status.Conditions {
			nc := NodeCondition{
				Type:   string(cond.Type),
				Status: string(cond.Status),
				Reason: cond.Reason,
			}
			if cond.Message != "" && len(cond.Message) > 200 {
				nc.Message = cond.Message[:200] + "..."
			} else {
				nc.Message = cond.Message
			}
			if !cond.LastTransitionTime.IsZero() {
				nc.Since = time.Since(cond.LastTransitionTime.Time).Round(time.Second).String()
			}
			entry.Conditions = append(entry.Conditions, nc)

			// Flag pressure conditions
			if cond.Status == corev1.ConditionTrue {
				switch cond.Type {
				case corev1.NodeDiskPressure:
					result.Summary.DiskPressure++
					issues = append(issues, fmt.Sprintf("DiskPressure: %s", cond.Reason))
				case corev1.NodeMemoryPressure:
					result.Summary.MemoryPressure++
					issues = append(issues, fmt.Sprintf("MemoryPressure: %s", cond.Reason))
				case corev1.NodePIDPressure:
					result.Summary.PIDPressure++
					issues = append(issues, fmt.Sprintf("PIDPressure: %s", cond.Reason))
				case corev1.NodeNetworkUnavailable:
					result.Summary.NetworkUnavailable++
					issues = append(issues, fmt.Sprintf("NetworkUnavailable: %s", cond.Reason))
				}
			}
		}

		// Resource usage thresholds
		if entry.CPUUsagePct > 80 {
			result.Summary.CPUHigh++
			issues = append(issues, fmt.Sprintf("CPU usage high (%.0f%%)", entry.CPUUsagePct))
		}
		if entry.MemUsagePct > 85 {
			result.Summary.MemoryHigh++
			issues = append(issues, fmt.Sprintf("Memory usage high (%.0f%%)", entry.MemUsagePct))
		}
		if entry.PodUsagePct > 80 {
			issues = append(issues, fmt.Sprintf("Pod density high (%.0f%%)", entry.PodUsagePct))
		}

		// Risk level
		entry.Issues = issues
		entry.RiskLevel = assessNodePressureRisk(entry)
		if entry.Status == "not-ready" {
			entry.RiskLevel = "critical"
		}

		// Summary
		result.Summary.TotalNodes++
		if entry.RiskLevel == "low" && entry.Status == "ready" {
			result.Summary.HealthyNodes++
		}
		if len(issues) > 0 {
			result.Summary.NodesWithPressure++
		}

		result.Nodes = append(result.Nodes, entry)

		// Top risks
		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.TopRisks = append(result.TopRisks, NodePressureRisk{
				Node:      node.Name,
				RiskLevel: entry.RiskLevel,
				Reasons:   joinIssues(issues),
			})
		}
	}

	// Sort nodes by risk
	sort.Slice(result.Nodes, func(i, j int) bool {
		return nodePressureRank(result.Nodes[i].RiskLevel) < nodePressureRank(result.Nodes[j].RiskLevel)
	})

	// Sort top risks
	sort.Slice(result.TopRisks, func(i, j int) bool {
		return nodePressureRank(result.TopRisks[i].RiskLevel) < nodePressureRank(result.TopRisks[j].RiskLevel)
	})

	// Score
	result.Summary.PressureScore = calculateNodePressureScore(result.Summary)

	// Recommendations
	result.Recommendations = generateNodePressureRecs(result.Summary)

	writeJSON(w, result)
}

// assessNodePressureRisk determines risk level.
func assessNodePressureRisk(entry NodePressureEntry) string {
	risk := 0

	for _, c := range entry.Conditions {
		if c.Status == "True" {
			switch c.Type {
			case "DiskPressure":
				risk += 25
			case "MemoryPressure":
				risk += 25
			case "PIDPressure":
				risk += 20
			case "NetworkUnavailable":
				risk += 30
			}
		}
	}

	if entry.CPUUsagePct > 90 {
		risk += 20
	} else if entry.CPUUsagePct > 80 {
		risk += 10
	}

	if entry.MemUsagePct > 95 {
		risk += 20
	} else if entry.MemUsagePct > 85 {
		risk += 10
	}

	if entry.PodUsagePct > 90 {
		risk += 10
	}

	switch {
	case risk >= 40:
		return "critical"
	case risk >= 20:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// calculateNodePressureScore computes 0-100.
func calculateNodePressureScore(s NodePressureSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	score := 100
	score -= s.DiskPressure * 10
	score -= s.MemoryPressure * 10
	score -= s.PIDPressure * 8
	score -= s.NetworkUnavailable * 12
	score -= s.NotReadyNodes * 15
	score -= s.CPUHigh * 3
	score -= s.MemoryHigh * 3
	if score < 0 {
		score = 0
	}
	return score
}

// generateNodePressureRecs produces actionable advice.
func generateNodePressureRecs(s NodePressureSummary) []string {
	var recs []string

	if s.DiskPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have DiskPressure — clean up images, logs, and unused volumes", s.DiskPressure))
	}
	if s.MemoryPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have MemoryPressure — reduce pod density or add node memory", s.MemoryPressure))
	}
	if s.PIDPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have PIDPressure — too many processes, check for fork bombs or process leaks", s.PIDPressure))
	}
	if s.NetworkUnavailable > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have NetworkUnavailable — check CNI plugin and node network config", s.NetworkUnavailable))
	}
	if s.NotReadyNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are NotReady — investigate kubelet, container runtime, or node health", s.NotReadyNodes))
	}
	if s.CPUHigh > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have >80%% CPU request saturation — consider scaling or rebalancing pods", s.CPUHigh))
	}
	if s.MemoryHigh > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have >85%% memory request saturation — add capacity or reduce pod density", s.MemoryHigh))
	}
	if s.CordonedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are cordoned — uncordon after maintenance or replace if decommissioned", s.CordonedNodes))
	}
	if s.PressureScore < 60 {
		recs = append(recs, fmt.Sprintf("Node pressure score is %d/100 — investigate pressure conditions immediately", s.PressureScore))
	}

	return recs
}

func nodePressureRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}
