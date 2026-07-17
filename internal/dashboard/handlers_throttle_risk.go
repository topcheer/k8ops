package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ThrottleRiskResult analyzes pod resource throttling risk and CPU pressure.
type ThrottleRiskResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ThrottleSummary   `json:"summary"`
	AtRiskPods      []ThrottleRiskPod `json:"atRiskPods"`
	PressureNodes   []PressureNode    `json:"pressureNodes"`
	RiskScore       int               `json:"riskScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type ThrottleSummary struct {
	TotalPods        int    `json:"totalPods"`
	PodsWithLimits   int    `json:"podsWithLimits"`
	PodsWithRequests int    `json:"podsWithRequests"`
	OverLimitPods    int    `json:"overLimitPods"`
	CPUThrottled     int    `json:"cpuThrottled"`
	MemPressure      int    `json:"memPressure"`
	AvgCPURequest    string `json:"avgCPURequest"`
	AvgMemRequest    string `json:"avgMemRequest"`
}

type ThrottleRiskPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Risk      string `json:"risk"`
	Severity  string `json:"severity"`
}

type PressureNode struct {
	Name   string  `json:"name"`
	CPUPct float64 `json:"cpuPct"`
	MemPct float64 `json:"memPct"`
	Status string  `json:"status"`
}

// handleThrottleRisk analyzes pod resource throttling risk and CPU pressure.
// GET /api/operations/throttle-risk
func (s *Server) handleThrottleRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ThrottleRiskResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Node pressure analysis
	for _, node := range nodes.Items {
		cpuCap := node.Status.Allocatable[corev1.ResourceCPU]
		memCap := node.Status.Allocatable[corev1.ResourceMemory]
		nodeCPU := float64(cpuCap.MilliValue()) / 1000.0
		nodeMem := float64(memCap.Value()) / (1024 * 1024 * 1024)

		nodeAllocatedCPU := 0.0
		nodeAllocatedMem := 0.0
		for _, pod := range pods.Items {
			if pod.Spec.NodeName != node.Name {
				continue
			}
			for _, c := range pod.Spec.Containers {
				if c.Resources.Requests != nil {
					if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						nodeAllocatedCPU += float64(q.MilliValue()) / 1000.0
					}
					if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						nodeAllocatedMem += float64(q.Value()) / (1024 * 1024 * 1024)
					}
				}
			}
		}

		cpuPct := 0.0
		if nodeCPU > 0 {
			cpuPct = nodeAllocatedCPU / nodeCPU * 100
		}
		memPct := 0.0
		if nodeMem > 0 {
			memPct = nodeAllocatedMem / nodeMem * 100
		}

		status := "healthy"
		if cpuPct > 80 || memPct > 85 {
			status = "critical"
		} else if cpuPct > 70 || memPct > 75 {
			status = "warning"
		}
		result.PressureNodes = append(result.PressureNodes, PressureNode{
			Name: node.Name, CPUPct: cpuPct, MemPct: memPct, Status: status,
		})
	}

	// Pod-level analysis
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			limits := c.Resources.Limits
			requests := c.Resources.Requests

			if limits != nil && len(limits) > 0 {
				result.Summary.PodsWithLimits++
			}
			if requests != nil && len(requests) > 0 {
				result.Summary.PodsWithRequests++
			}

			// Check for CPU limit = very low (throttle risk)
			if limits != nil {
				if q, ok := limits[corev1.ResourceCPU]; ok {
					millicores := q.MilliValue()
					if millicores > 0 && millicores < 100 {
						result.Summary.CPUThrottled++
						result.AtRiskPods = append(result.AtRiskPods, ThrottleRiskPod{
							Name: pod.Name, Namespace: pod.Namespace,
							Risk:     fmt.Sprintf("CPU limit %dm (<100m) — likely throttling", millicores),
							Severity: "medium",
						})
					}
				}
			}

			// Check for no limits = unbounded
			if limits == nil || len(limits) == 0 {
				result.Summary.OverLimitPods++
				result.AtRiskPods = append(result.AtRiskPods, ThrottleRiskPod{
					Name: pod.Name, Namespace: pod.Namespace,
					Risk:     "No resource limits — can consume unlimited resources",
					Severity: "high",
				})
			}
		}
	}

	// Score
	score := 100
	if result.Summary.TotalPods > 0 {
		limitRatio := float64(result.Summary.PodsWithLimits) / float64(result.Summary.TotalPods)
		score = int(limitRatio * 60)
	}
	for _, pn := range result.PressureNodes {
		if pn.Status == "critical" {
			score -= 15
		} else if pn.Status == "warning" {
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.RiskScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.RiskScore)

	sort.Slice(result.AtRiskPods, func(i, j int) bool {
		return result.AtRiskPods[i].Severity > result.AtRiskPods[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Throttle risk: %d/100 (grade %s) — %d/%d pods with limits, %d unbounded", result.RiskScore, result.Grade, result.Summary.PodsWithLimits, result.Summary.TotalPods, result.Summary.OverLimitPods))
	if result.Summary.OverLimitPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pods without resource limits — add CPU/memory limits", result.Summary.OverLimitPods))
	}
	for _, pn := range result.PressureNodes {
		if pn.Status == "critical" {
			recs = append(recs, fmt.Sprintf("Node '%s' at %.0f%% CPU / %.0f%% Mem — near capacity", pn.Name, pn.CPUPct, pn.MemPct))
		}
	}
	if len(recs) == 1 {
		recs = append(recs, "Resource throttling risk is low — all pods have appropriate limits")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
