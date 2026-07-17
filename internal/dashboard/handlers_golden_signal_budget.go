package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GoldenSignalBudgetResult unifies the four SRE golden signals (latency, traffic,
// errors, saturation) into a composite health budget per workload.
type GoldenSignalBudgetResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         GoldenSignalSummary `json:"summary"`
	ByWorkload      []GoldenSignalEntry `json:"byWorkload"`
	CriticalSignals []GoldenSignalEntry `json:"criticalSignals"`
	CompositeScore  int                 `json:"compositeScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type GoldenSignalSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	LatencyHealthy    int     `json:"latencyHealthy"`
	TrafficHealthy    int     `json:"trafficHealthy"`
	ErrorsHealthy     int     `json:"errorsHealthy"`
	SaturationHealthy int     `json:"saturationHealthy"`
	CriticalWorkloads int     `json:"criticalWorkloads"`
	AvgBudget         float64 `json:"avgBudgetPct"`
}

type GoldenSignalEntry struct {
	Workload        string  `json:"workload"`
	Namespace       string  `json:"namespace"`
	LatencyScore    int     `json:"latencyScore"`
	TrafficScore    int     `json:"trafficScore"`
	ErrorScore      int     `json:"errorScore"`
	SaturationScore int     `json:"saturationScore"`
	CompositeScore  int     `json:"compositeScore"`
	BudgetPct       float64 `json:"budgetPct"`
	Status          string  `json:"status"`
	TopSignal       string  `json:"topSignal"`
	IssueDetail     string  `json:"issueDetail"`
}

// handleGoldenSignalBudget handles GET /api/operations/golden-signal-budget
func (s *Server) handleGoldenSignalBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GoldenSignalBudgetResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Build node saturation map
	nodeSat := make(map[string]float64) // node -> utilization estimate
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		nodeSat[node.Name] = 0 // will be filled by pod requests
		_ = cpuAlloc
	}

	// Aggregate per-workload metrics
	wlMap := make(map[string]*GoldenSignalEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		if _, ok := wlMap[key]; !ok {
			wlMap[key] = &GoldenSignalEntry{
				Workload:  wlName,
				Namespace: pod.Namespace,
			}
		}
		entry := wlMap[key]

		// --- LATENCY signal: estimate from restart delays ---
		// More restarts = higher latency variance
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 3 {
				entry.LatencyScore -= 10
			}
		}

		// --- TRAFFIC signal: estimate from pod readiness ---
		readyCount := 0
		totalContainers := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalContainers++
			if cs.Ready {
				readyCount++
			}
		}
		if totalContainers > 0 {
			entry.TrafficScore += (readyCount * 100 / totalContainers) / 10
		}

		// --- ERROR signal: non-running pods, OOM kills ---
		if pod.Status.Phase != corev1.PodRunning {
			entry.ErrorScore -= 20
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				if cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					entry.ErrorScore -= 15
				}
			}
		}

		// --- SATURATION signal: resource requests relative to allocatable ---
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				if req.AsApproximateFloat64() > 1.0 {
					entry.SaturationScore -= 5
				}
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				if lim.AsApproximateFloat64() > 2.0 {
					entry.SaturationScore -= 5
				}
			}
		}
	}

	var entries []GoldenSignalEntry
	totalBudget := 0.0

	for _, e := range wlMap {
		// Clamp scores to 0-100
		e.LatencyScore = goldenClampScore(e.LatencyScore)
		e.TrafficScore = goldenClampScore(e.TrafficScore)
		e.ErrorScore = goldenClampScore(e.ErrorScore)
		e.SaturationScore = goldenClampScore(e.SaturationScore)

		// Composite: weighted average (latency 30%, traffic 20%, errors 30%, saturation 20%)
		e.CompositeScore = (e.LatencyScore*30 + e.TrafficScore*20 + e.ErrorScore*30 + e.SaturationScore*20) / 100
		e.BudgetPct = float64(e.CompositeScore)

		// Classify
		switch {
		case e.CompositeScore >= 80:
			e.Status = "healthy"
		case e.CompositeScore >= 60:
			e.Status = "warning"
		case e.CompositeScore >= 40:
			e.Status = "degraded"
			result.Summary.CriticalWorkloads++
		default:
			e.Status = "critical"
			result.Summary.CriticalWorkloads++
		}

		// Top signal = weakest dimension
		minScore := e.LatencyScore
		e.TopSignal = "latency"
		if e.TrafficScore < minScore {
			minScore = e.TrafficScore
			e.TopSignal = "traffic"
		}
		if e.ErrorScore < minScore {
			minScore = e.ErrorScore
			e.TopSignal = "errors"
		}
		if e.SaturationScore < minScore {
			e.TopSignal = "saturation"
		}

		// Build issue detail for unhealthy workloads
		if e.Status != "healthy" {
			e.IssueDetail = fmt.Sprintf("%s 信号最弱 (score=%d)", e.TopSignal, minScore)
		}

		// Track per-signal health
		if e.LatencyScore >= 70 {
			result.Summary.LatencyHealthy++
		}
		if e.TrafficScore >= 70 {
			result.Summary.TrafficHealthy++
		}
		if e.ErrorScore >= 70 {
			result.Summary.ErrorsHealthy++
		}
		if e.SaturationScore >= 70 {
			result.Summary.SaturationHealthy++
		}

		totalBudget += e.BudgetPct
		entries = append(entries, *e)
	}

	result.Summary.TotalWorkloads = len(entries)
	if len(entries) > 0 {
		result.Summary.AvgBudget = totalBudget / float64(len(entries))
	}

	// Sort by composite score ascending (worst first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CompositeScore < entries[j].CompositeScore
	})
	result.ByWorkload = entries

	// Collect critical signals
	for _, e := range entries {
		if e.Status == "critical" || e.Status == "degraded" {
			result.CriticalSignals = append(result.CriticalSignals, e)
		}
	}

	result.CompositeScore = int(result.Summary.AvgBudget)
	switch {
	case result.CompositeScore >= 80:
		result.Grade = "A"
	case result.CompositeScore >= 60:
		result.Grade = "B"
	case result.CompositeScore >= 40:
		result.Grade = "C"
	case result.CompositeScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildGoldenSignalRecs(&result)
	writeJSON(w, result)
}

func goldenClampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func buildGoldenSignalRecs(r *GoldenSignalBudgetResult) []string {
	recs := []string{
		fmt.Sprintf("黄金信号预算: %d 工作负载, 平均预算 %.1f%%", r.Summary.TotalWorkloads, r.Summary.AvgBudget),
	}
	if r.Summary.CriticalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个工作负载处于 degraded/critical 状态", r.Summary.CriticalWorkloads))
	}
	// Signal-level breakdown
	recs = append(recs, fmt.Sprintf("信号健康: 延迟 %d, 流量 %d, 错误 %d, 饱和 %d",
		r.Summary.LatencyHealthy, r.Summary.TrafficHealthy, r.Summary.ErrorsHealthy, r.Summary.SaturationHealthy))
	if len(r.CriticalSignals) > 0 {
		top := r.CriticalSignals[0]
		recs = append(recs, fmt.Sprintf("最高风险: %s/%s (%s, composite=%d)", top.Namespace, top.Workload, top.IssueDetail, top.CompositeScore))
	}
	if result_CompositeScoreLow(r) {
		recs = append(recs, "建议: 优先修复 error 和 latency 信号, 添加资源限制降低饱和度")
	}
	return recs
}

func result_CompositeScoreLow(r *GoldenSignalBudgetResult) bool {
	return r.CompositeScore < 60
}
