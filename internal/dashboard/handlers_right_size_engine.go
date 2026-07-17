package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RightSizeEngineResult analyzes resource requests and generates
// concrete right-sizing recommendation patches.
type RightSizeEngineResult struct {
	ScannedAt        time.Time        `json:"scannedAt"`
	Summary          RightSizeSummary `json:"summary"`
	Recommendations  []RightSizeRec   `json:"recommendations"`
	PatchBatch       []RightSizePatch `json:"patchBatch"`
	PotentialSavings RightSizeSavings `json:"potentialSavings"`
	HealthScore      int              `json:"healthScore"`
	Grade            string           `json:"grade"`
	Tips             []string         `json:"tips"`
}

type RightSizeSummary struct {
	TotalContainers int `json:"totalContainers"`
	Oversized       int `json:"oversized"`
	Undersized      int `json:"undersized"`
	NoRequests      int `json:"noRequests"`
	WellSized       int `json:"wellSized"`
}

type RightSizeRec struct {
	Workload  string           `json:"workload"`
	Namespace string           `json:"namespace"`
	Container string           `json:"container"`
	Kind      string           `json:"kind"`
	Current   RightSizeCurrent `json:"current"`
	Suggested RightSizeCurrent `json:"suggested"`
	Reason    string           `json:"reason"`
	Severity  string           `json:"severity"`
	Action    string           `json:"action"`
}

type RightSizeCurrent struct {
	ReqCPU    string  `json:"reqCPU"`
	ReqMem    string  `json:"reqMem"`
	LimitCPU  string  `json:"limitCPU"`
	LimitMem  string  `json:"limitMem"`
	ReqCPUVal float64 `json:"reqCPUVal"`
	ReqMemVal float64 `json:"reqMemVal"`
}

type RightSizePatch struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	PatchJSON string `json:"patchJSON"`
	Command   string `json:"command"`
}

type RightSizeSavings struct {
	CPUCoresFreed float64 `json:"cpuCoresFreed"`
	MemGBFreed    float64 `json:"memGBFreed"`
	MonthlyCost   float64 `json:"estimatedMonthlyCostUSD"`
}

// handleRightSizeEngine handles GET /api/scalability/right-size-engine
func (s *Server) handleRightSizeEngine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RightSizeEngineResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var recs []RightSizeRec
	var patches []RightSizePatch
	var cpuFreed, memFreed float64

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := int(ptrInt32(d.Spec.Replicas))
		if replicas == 0 {
			continue
		}

		for _, c := range d.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			reqCPU := c.Resources.Requests.Cpu().AsApproximateFloat64()
			reqMem := c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			limCPU := c.Resources.Limits.Cpu().AsApproximateFloat64()
			limMem := c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9

			cur := RightSizeCurrent{
				ReqCPU: formatCPUStr(reqCPU), ReqMem: fmt.Sprintf("%.0fMi", reqMem*1024),
				LimitCPU: formatCPUStr(limCPU), LimitMem: fmt.Sprintf("%.0fMi", limMem*1024),
				ReqCPUVal: reqCPU, ReqMemVal: reqMem,
			}

			// No requests
			if reqCPU == 0 && reqMem == 0 {
				result.Summary.NoRequests++
				recs = append(recs, RightSizeRec{
					Workload: d.Name, Namespace: d.Namespace, Container: c.Name, Kind: "Deployment",
					Current: cur, Suggested: RightSizeCurrent{ReqCPU: "100m", ReqMem: "128Mi"},
					Reason: "No resource requests set", Severity: "high",
					Action: "Add resources.requests",
				})
				continue
			}

			// Oversized: CPU > 2 cores or Mem > 4GB per container
			if reqCPU > 2 || reqMem > 4 {
				result.Summary.Oversized++
				sugCPU := reqCPU * 0.5
				sugMem := reqMem * 0.5
				sug := RightSizeCurrent{
					ReqCPU: formatCPUStr(sugCPU), ReqMem: fmt.Sprintf("%.0fMi", sugMem*1024),
					ReqCPUVal: sugCPU, ReqMemVal: sugMem,
				}
				cpuFreed += (reqCPU - sugCPU) * float64(replicas)
				memFreed += (reqMem - sugMem) * float64(replicas)

				reason := "Resource requests oversized"
				if reqCPU > 4 {
					reason = fmt.Sprintf("CPU request %.1f cores far exceeds typical", reqCPU)
				}
				if reqMem > 8 {
					reason = fmt.Sprintf("Memory request %.1fGB far exceeds typical", reqMem)
				}

				recs = append(recs, RightSizeRec{
					Workload: d.Name, Namespace: d.Namespace, Container: c.Name, Kind: "Deployment",
					Current: cur, Suggested: sug,
					Reason: reason, Severity: "medium",
					Action: "Halve requests, observe and fine-tune",
				})

				patchJSON := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s","resources":{"requests":{"cpu":"%s","memory":"%.0fMi"}}}]}}}}`,
					c.Name, formatCPUStr(sugCPU), sugMem*1024)
				patches = append(patches, RightSizePatch{
					Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment",
					PatchJSON: patchJSON,
					Command:   fmt.Sprintf("kubectl patch deployment %s -n %s --type=strategic -p '%s'", d.Name, d.Namespace, patchJSON),
				})
				continue
			}

			// Limit/Request ratio too high
			if limCPU > 0 && reqCPU > 0 && limCPU/reqCPU > 4 {
				result.Summary.Undersized++
				recs = append(recs, RightSizeRec{
					Workload: d.Name, Namespace: d.Namespace, Container: c.Name, Kind: "Deployment",
					Current: cur, Suggested: RightSizeCurrent{LimitCPU: formatCPUStr(reqCPU * 2)},
					Reason: fmt.Sprintf("Limit/Request ratio %.1f too high", limCPU/reqCPU), Severity: "low",
					Action: "Set limit to 2x request",
				})
				continue
			}

			result.Summary.WellSized++
		}
	}

	sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(recs, func(i, j int) bool {
		return sevOrder[recs[i].Severity] < sevOrder[recs[j].Severity]
	})

	result.Recommendations = recs
	result.PatchBatch = patches

	result.PotentialSavings = RightSizeSavings{
		CPUCoresFreed: cpuFreed,
		MemGBFreed:    memFreed,
		MonthlyCost:   cpuFreed*0.03*24*30 + memFreed*0.004*24*30,
	}

	if result.Summary.TotalContainers > 0 {
		result.HealthScore = result.Summary.WellSized * 100 / result.Summary.TotalContainers
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

	result.Tips = []string{
		fmt.Sprintf("%d oversized containers, optimizing frees %.1f CPU cores + %.1f GB mem", result.Summary.Oversized, cpuFreed, memFreed),
		fmt.Sprintf("Estimated savings: $%.2f/month", result.PotentialSavings.MonthlyCost),
		"Apply patches during low-traffic, observe 24h before fine-tuning",
	}

	writeJSON(w, result)
}

func formatCPUStr(cores float64) string {
	if cores < 1 {
		return fmt.Sprintf("%dm", int(cores*1000))
	}
	return fmt.Sprintf("%.1f", cores)
}
