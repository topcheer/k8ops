package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadEfficiencyResult evaluates how efficiently workloads use their
// allocated resources by checking request-to-limit ratios, replica waste,
// and configuration anti-patterns.
type WorkloadEfficiencyResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         EfficiencySummary   `json:"summary"`
	Findings        []EfficiencyFinding `json:"findings"`
	ByNamespace     []EfficiencyNS      `json:"byNamespace"`
	WasteEstimate   EfficiencyWaste     `json:"wasteEstimate"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type EfficiencySummary struct {
	TotalWorkloads     int     `json:"totalWorkloads"`
	EfficientWorkloads int     `json:"efficientWorkloads"`
	WastefulWorkloads  int     `json:"wastefulWorkloads"`
	OverProvisioned    int     `json:"overProvisioned"`
	UnderProvisioned   int     `json:"underProvisioned"`
	IdleReplicas       int     `json:"idleReplicas"`
	TotalWasteUSD      float64 `json:"totalWasteUSD"`
}

type EfficiencyFinding struct {
	Workload  string  `json:"workload"`
	Namespace string  `json:"namespace"`
	Issue     string  `json:"issue"`
	Severity  string  `json:"severity"`
	Detail    string  `json:"detail"`
	WasteUSD  float64 `json:"wasteUSD"`
}

type EfficiencyNS struct {
	Namespace string  `json:"namespace"`
	Workloads int     `json:"workloads"`
	Waste     float64 `json:"wasteUSD"`
	Score     int     `json:"score"`
}

type EfficiencyWaste struct {
	TotalCPUWaste float64 `json:"totalCPUWaste"`
	TotalMemWaste float64 `json:"totalMemWasteGB"`
	TotalWasteUSD float64 `json:"totalWasteUSD"`
	WastePct      float64 `json:"wastePct"`
}

// handleWorkloadEfficiency handles GET /api/product/workload-efficiency
func (s *Server) handleWorkloadEfficiency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := WorkloadEfficiencyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Calculate unit costs
	nodeCPUCapacity := 0.0
	for _, n := range nodes.Items {
		nodeCPUCapacity += n.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	cpuUnitCost := 0.0 // $/core/month
	if nodeCPUCapacity > 0 {
		cpuUnitCost = float64(len(nodes.Items)) * 100.0 * 0.6 / nodeCPUCapacity
	}
	memUnitCost := 3.5 // $/GB/month approx

	var totalReqCPUAll, totalReqMemAll float64
	var findings []EfficiencyFinding
	nsMap := make(map[string]*EfficiencyNS)
	var totalCPUWaste, totalMemWaste, totalWaste float64

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		if _, ok := nsMap[d.Namespace]; !ok {
			nsMap[d.Namespace] = &EfficiencyNS{Namespace: d.Namespace}
		}
		nsMap[d.Namespace].Workloads++

		replicas := int(ptrInt32(d.Spec.Replicas))
		var reqCPU, reqMem, limCPU, limMem float64

		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests["cpu"]; ok {
				reqCPU += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests["memory"]; ok {
				reqMem += v.AsApproximateFloat64() / 1e9
			}
			if v, ok := c.Resources.Limits["cpu"]; ok {
				limCPU += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Limits["memory"]; ok {
				limMem += v.AsApproximateFloat64() / 1e9
			}
		}

		// Multiply by replicas
		totalReqCPUAll += reqCPU * float64(replicas)
		totalReqMemAll += reqMem * float64(replicas)

		isEfficient := true

		// Check 1: Over-provisioned (requests very high relative to limits)
		if reqCPU > 2 || reqMem > 4 {
			result.Summary.OverProvisioned++
			isEfficient = false
			waste := reqCPU*0.5*cpuUnitCost + reqMem*0.5*memUnitCost
			totalCPUWaste += reqCPU * 0.5 * float64(replicas)
			totalMemWaste += reqMem * 0.5 * float64(replicas)
			totalWaste += waste
			findings = append(findings, EfficiencyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue: "over-provisioned", Severity: "medium",
				Detail:   fmt.Sprintf("CPU req %.1f cores, Mem req %.1f GB per pod", reqCPU, reqMem),
				WasteUSD: waste,
			})
			nsMap[d.Namespace].Waste += waste
		}

		// Check 2: Limit/Request ratio too high (>4x)
		if limCPU > 0 && reqCPU > 0 && limCPU/reqCPU > 4 {
			isEfficient = false
			findings = append(findings, EfficiencyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue: "limit/request ratio high", Severity: "low",
				Detail: fmt.Sprintf("CPU limit %.1f / request %.1f = %.1fx", limCPU, reqCPU, limCPU/reqCPU),
			})
		}

		// Check 3: Zero replicas (idle)
		if replicas == 0 {
			result.Summary.IdleReplicas++
			isEfficient = false
			findings = append(findings, EfficiencyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue: "idle (0 replicas)", Severity: "low",
				Detail: "Deployment exists but scaled to 0, may be abandoned",
			})
		}

		// Check 4: No requests
		if reqCPU == 0 && reqMem == 0 {
			result.Summary.UnderProvisioned++
			isEfficient = false
			findings = append(findings, EfficiencyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue: "no resource requests", Severity: "high",
				Detail: "No CPU/memory requests set, scheduling and HPA won't work",
			})
		}

		// Check 5: Single replica (no HA)
		if replicas == 1 {
			findings = append(findings, EfficiencyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue: "single replica (no HA)", Severity: "medium",
				Detail: "Single replica has no fault tolerance",
			})
		}

		if isEfficient {
			result.Summary.EfficientWorkloads++
		} else {
			result.Summary.WastefulWorkloads++
		}
	}

	// NS stats
	for _, ns := range nsMap {
		if ns.Workloads > 0 {
			ns.Score = (ns.Workloads - int(ns.Waste/10)) * 100 / ns.Workloads
			if ns.Score < 0 {
				ns.Score = 0
			}
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Waste > result.ByNamespace[j].Waste
	})

	// Waste estimate
	result.WasteEstimate = EfficiencyWaste{
		TotalCPUWaste: totalCPUWaste,
		TotalMemWaste: totalMemWaste,
		TotalWasteUSD: totalWaste,
	}
	totalAllocCost := totalReqCPUAll*cpuUnitCost + totalReqMemAll*memUnitCost
	if totalAllocCost > 0 {
		result.WasteEstimate.WastePct = totalWaste / totalAllocCost * 100
	}
	result.Summary.TotalWasteUSD = totalWaste

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].WasteUSD > findings[j].WasteUSD
	})
	result.Findings = findings

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.EfficientWorkloads * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildEfficiencyRecs(&result)
	writeJSON(w, result)
}

func buildEfficiencyRecs(r *WorkloadEfficiencyResult) []string {
	recs := []string{}
	if r.Summary.OverProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载过度分配资源", r.Summary.OverProvisioned))
	}
	if r.Summary.IdleReplicas > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载副本数为 0，可能已废弃", r.Summary.IdleReplicas))
	}
	if r.Summary.UnderProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载缺少资源请求", r.Summary.UnderProvisioned))
	}
	if r.WasteEstimate.TotalWasteUSD > 0 {
		recs = append(recs, fmt.Sprintf("预计浪费 $%.2f/月 (%.0f%%)", r.WasteEstimate.TotalWasteUSD, r.WasteEstimate.WastePct))
	}
	if len(recs) == 0 {
		recs = append(recs, "工作负载效率良好")
	}
	return recs
}
