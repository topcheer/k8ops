package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvictionRiskResult predicts which pods are at imminent risk of eviction
// based on node conditions, resource pressure (memory/disk), QoS class,
// OOM history, and priority class.
type EvictionRiskResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         EvictionSummary        `json:"summary"`
	AtRiskPods      []EvictionEntry        `json:"atRiskPods"`
	NodePressureMap []EvictionNodePressure `json:"nodePressureMap"`
	RiskScore       int                    `json:"riskScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type EvictionSummary struct {
	TotalPods     int `json:"totalPods"`
	AtRisk        int `json:"atRiskCount"`
	CriticalRisk  int `json:"criticalRiskCount"`
	HighRisk      int `json:"highRiskCount"`
	BurstableQoS  int `json:"burstableQoS"`
	BestEffortQoS int `json:"bestEffortQoS"`
	LowPriority   int `json:"lowPriorityClass"`
	PressureNodes int `json:"nodesUnderPressure"`
}

type EvictionEntry struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	NodeName      string   `json:"nodeName"`
	Workload      string   `json:"workload"`
	QoSClass      string   `json:"qosClass"`
	PriorityClass string   `json:"priorityClass"`
	RiskScore     int      `json:"riskScore"`
	RiskLevel     string   `json:"riskLevel"`
	RiskFactors   []string `json:"riskFactors"`
	OOMCount      int      `json:"oomCount"`
	RestartCount  int      `json:"restartCount"`
	MemRequestMB  float64  `json:"memRequestMB"`
	MemLimitMB    float64  `json:"memLimitMB"`
}

type EvictionNodePressure struct {
	NodeName       string   `json:"nodeName"`
	Conditions     []string `json:"conditions"`
	MemoryPressure bool     `json:"memoryPressure"`
	DiskPressure   bool     `json:"diskPressure"`
	PIDPressure    bool     `json:"pidPressure"`
	Ready          bool     `json:"ready"`
	AffectedPods   int      `json:"affectedPods"`
}

// handleEvictionRisk handles GET /api/scalability/eviction-risk
func (s *Server) handleEvictionRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EvictionRiskResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node pressure map
	nodePressure := make(map[string]*EvictionNodePressure)
	for _, node := range nodes.Items {
		entry := &EvictionNodePressure{
			NodeName: node.Name,
			Ready:    true,
		}
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				entry.MemoryPressure = true
				entry.Conditions = append(entry.Conditions, "MemoryPressure")
			case corev1.NodeDiskPressure:
				entry.DiskPressure = true
				entry.Conditions = append(entry.Conditions, "DiskPressure")
			case corev1.NodePIDPressure:
				entry.PIDPressure = true
				entry.Conditions = append(entry.Conditions, "PIDPressure")
			case corev1.NodeReady:
				entry.Ready = true
			}
		}
		// NotReady check
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				entry.Ready = false
				entry.Conditions = append(entry.Conditions, "NotReady")
			}
		}
		nodePressure[node.Name] = entry
		if entry.MemoryPressure || entry.DiskPressure || entry.PIDPressure || !entry.Ready {
			result.Summary.PressureNodes++
		}
	}

	// Analyze each pod
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}

		result.Summary.TotalPods++

		entry := EvictionEntry{
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			NodeName:      pod.Spec.NodeName,
			QoSClass:      string(pod.Status.QOSClass),
			PriorityClass: pod.Spec.PriorityClassName,
		}

		// Track QoS class
		if pod.Status.QOSClass == corev1.PodQOSBurstable {
			result.Summary.BurstableQoS++
		}
		if pod.Status.QOSClass == corev1.PodQOSBestEffort {
			result.Summary.BestEffortQoS++
		}
		if pod.Spec.PriorityClassName == "" || pod.Spec.PriorityClassName == "low" {
			result.Summary.LowPriority++
		}

		// Calculate memory requests/limits
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemRequestMB += float64(req.Value()) / (1024 * 1024)
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				entry.MemLimitMB += float64(lim.Value()) / (1024 * 1024)
			}
		}

		// Get workload name
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				entry.Workload = ref.Name
				break
			}
		}

		// Get restart count and OOM indicators
		for _, cs := range pod.Status.ContainerStatuses {
			entry.RestartCount += int(cs.RestartCount)
			// Check termination reasons for OOM
			if cs.LastTerminationState.Terminated != nil {
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "OOMKilled" {
					entry.OOMCount++
				}
			}
		}

		// Calculate risk score
		riskScore := 0
		var factors []string

		// Node pressure factors
		if np, ok := nodePressure[pod.Spec.NodeName]; ok {
			if np.MemoryPressure {
				riskScore += 30
				factors = append(factors, "node-memory-pressure")
			}
			if np.DiskPressure {
				riskScore += 25
				factors = append(factors, "node-disk-pressure")
			}
			if np.PIDPressure {
				riskScore += 20
				factors = append(factors, "node-pid-pressure")
			}
			if !np.Ready {
				riskScore += 40
				factors = append(factors, "node-not-ready")
			}
			np.AffectedPods++
		}

		// QoS class risk
		if pod.Status.QOSClass == corev1.PodQOSBestEffort {
			riskScore += 25
			factors = append(factors, "best-effort-qos")
		} else if pod.Status.QOSClass == corev1.PodQOSBurstable {
			riskScore += 10
			factors = append(factors, "burstable-qos")
		}

		// No memory limit = higher eviction risk
		if entry.MemLimitMB == 0 {
			riskScore += 15
			factors = append(factors, "no-memory-limit")
		}

		// OOM history
		if entry.OOMCount > 0 {
			riskScore += entry.OOMCount * 15
			factors = append(factors, fmt.Sprintf("oom-history(%d)", entry.OOMCount))
		}

		// High restart count
		if entry.RestartCount > 5 {
			riskScore += 10
			factors = append(factors, fmt.Sprintf("high-restarts(%d)", entry.RestartCount))
		}

		// Low priority class
		if entry.PriorityClass == "" {
			riskScore += 5
		}

		// Clamp
		if riskScore > 100 {
			riskScore = 100
		}
		entry.RiskScore = riskScore
		entry.RiskFactors = factors

		switch {
		case riskScore >= 50:
			entry.RiskLevel = "critical"
			result.Summary.CriticalRisk++
		case riskScore >= 30:
			entry.RiskLevel = "high"
			result.Summary.HighRisk++
		case riskScore >= 15:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.AtRiskPods = append(result.AtRiskPods, entry)
			result.Summary.AtRisk++
		}
	}

	// Sort at-risk pods by risk score descending
	sort.Slice(result.AtRiskPods, func(i, j int) bool {
		return result.AtRiskPods[i].RiskScore > result.AtRiskPods[j].RiskScore
	})

	// Node pressure entries
	for _, np := range nodePressure {
		if len(np.Conditions) > 0 || np.AffectedPods > 0 {
			result.NodePressureMap = append(result.NodePressureMap, *np)
		}
	}
	sort.Slice(result.NodePressureMap, func(i, j int) bool {
		return result.NodePressureMap[i].AffectedPods > result.NodePressureMap[j].AffectedPods
	})

	// Risk score: lower at-risk ratio = higher score
	if result.Summary.TotalPods > 0 {
		atRiskRatio := float64(result.Summary.AtRisk) / float64(result.Summary.TotalPods)
		result.RiskScore = int((1 - atRiskRatio) * 100)
	}

	switch {
	case result.RiskScore >= 80:
		result.Grade = "A"
	case result.RiskScore >= 60:
		result.Grade = "B"
	case result.RiskScore >= 40:
		result.Grade = "C"
	case result.RiskScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildEvictionRiskRecs(&result)
	writeJSON(w, result)
}

func buildEvictionRiskRecs(r *EvictionRiskResult) []string {
	recs := []string{
		fmt.Sprintf("驱逐风险: %d/%d Pod 处于风险中", r.Summary.AtRisk, r.Summary.TotalPods),
	}
	if r.Summary.CriticalRisk > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个 Pod 面临严重驱逐风险", r.Summary.CriticalRisk))
	}
	if r.Summary.PressureNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d 个节点处于资源压力状态", r.Summary.PressureNodes))
	}
	if r.Summary.BestEffortQoS > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 BestEffort QoS Pod 首先被驱逐", r.Summary.BestEffortQoS))
	}
	if r.RiskScore < 60 {
		recs = append(recs, "建议: 为所有 Pod 设置 resource requests/limits, 提升至 Guaranteed QoS")
	}
	return recs
}
