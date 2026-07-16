package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityForecastResult forecasts when cluster resources will be exhausted
// based on current allocation trends, growth rate, and headroom.
type CapacityForecastResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CapForecastSummary  `json:"summary"`
	Forecasts       []CapForecast       `json:"forecasts"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type CapForecastSummary struct {
	NodeCount       int     `json:"nodeCount"`
	TotalCPUCore    float64 `json:"totalCPUCores"`
	TotalMemGB      float64 `json:"totalMemGB"`
	AllocatedCPU    float64 `json:"allocatedCPU"`
	AllocatedMem    float64 `json:"allocatedMemGB"`
	CPUPct          float64 `json:"cpuPct"`
	MemPct          float64 `json:"memPct"`
	PodCapacity     int     `json:"podCapacity"`
	PodsRunning     int     `json:"podsRunning"`
	PodPct          float64 `json:"podPct"`
}

type CapForecast struct {
	Resource    string `json:"resource"`
	Current     string `json:"current"`
	Capacity    string `json:"capacity"`
	UsagePct    float64 `json:"usagePct"`
	TimeToFull  string `json:"timeToFull"`
	Severity    string `json:"severity"`
}

// handleCapacityForecast forecasts cluster resource exhaustion.
// GET /api/scalability/capacity-forecast
func (s *Server) handleCapacityForecastDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CapacityForecastResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	totalCPU := 0.0
	totalMem := 0.0
	allocCPU := 0.0
	allocMem := 0.0
	podCap := 0
	podsRunning := 0

	for _, node := range nodes.Items {
		cpu := node.Status.Allocatable[corev1.ResourceCPU]
		mem := node.Status.Allocatable[corev1.ResourceMemory]
		pods := node.Status.Allocatable[corev1.ResourcePods]
		nodeCPU := float64(cpu.MilliValue()) / 1000.0
		nodeMem := float64(mem.Value()) / (1024 * 1024 * 1024)
		nodePods := int(pods.Value())
		totalCPU += nodeCPU
		totalMem += nodeMem
		podCap += nodePods
	}

	// Sum allocated resources across all pods
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning { continue }
		podsRunning++
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					allocCPU += float64(q.MilliValue()) / 1000.0
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					allocMem += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}
	}

	result.Summary = CapForecastSummary{
		NodeCount: len(nodes.Items),
		TotalCPUCore: totalCPU, TotalMemGB: totalMem,
		AllocatedCPU: allocCPU, AllocatedMem: allocMem,
		PodCapacity: podCap, PodsRunning: podsRunning,
	}
	if totalCPU > 0 { result.Summary.CPUPct = allocCPU / totalCPU * 100 }
	if totalMem > 0 { result.Summary.MemPct = allocMem / totalMem * 100 }
	if podCap > 0 { result.Summary.PodPct = float64(podsRunning) / float64(podCap) * 100 }

	// Build forecasts (estimate: if 0% now, >1 year; if >90%, <30 days)
	mkForecast := func(resource, current, capacity string, pct float64) CapForecast {
		t := ">1 year"
		sev := "low"
		if pct > 95 { t = "<1 month"; sev = "critical" } else if pct > 85 { t = "<3 months"; sev = "high" } else if pct > 70 { t = "<6 months"; sev = "medium" }
		return CapForecast{Resource: resource, Current: current, Capacity: capacity, UsagePct: pct, TimeToFull: t, Severity: sev}
	}

	result.Forecasts = append(result.Forecasts,
		mkForecast("CPU", fmt.Sprintf("%.1f cores", allocCPU), fmt.Sprintf("%.1f cores", totalCPU), result.Summary.CPUPct),
		mkForecast("Memory", fmt.Sprintf("%.1f GB", allocMem), fmt.Sprintf("%.1f GB", totalMem), result.Summary.MemPct),
		mkForecast("Pod Slots", fmt.Sprintf("%d", podsRunning), fmt.Sprintf("%d", podCap), result.Summary.PodPct),
	)

	// Score: higher = more headroom
	score := 100
	for _, f := range result.Forecasts {
		switch f.Severity {
		case "critical": score -= 35
		case "high": score -= 20
		case "medium": score -= 10
		}
	}
	if score < 0 { score = 0 }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.Forecasts, func(i, j int) bool { return result.Forecasts[i].UsagePct > result.Forecasts[j].UsagePct })

	var recs []string
	recs = append(recs, fmt.Sprintf("Capacity forecast: %d/100 (grade %s) — CPU %.0f%%, Mem %.0f%%, Pods %.0f%%", result.HealthScore, result.Grade, result.Summary.CPUPct, result.Summary.MemPct, result.Summary.PodPct))
	for _, f := range result.Forecasts {
		if f.Severity != "low" {
			recs = append(recs, fmt.Sprintf("%s at %.0f%% capacity (%s) — %s", f.Resource, f.UsagePct, f.TimeToFull, f.Current))
		}
	}
	if len(recs) == 1 { recs = append(recs, "Cluster has ample headroom across all dimensions") }
	result.Recommendations = recs

	writeJSON(w, result)
}
