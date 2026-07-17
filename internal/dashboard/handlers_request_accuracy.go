package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RequestAccuracyResult analyzes how accurately workload resource requests
// match actual needs. It compares requests vs limits to identify
// over-provisioned (wasted cost) and under-provisioned (throttle/OOM risk)
// workloads. Provides actionable right-sizing recommendations.
type RequestAccuracyResult struct {
	ScannedAt          time.Time         `json:"scannedAt"`
	Summary            ReqAccSummary     `json:"summary"`
	Containers         []ReqAccContainer `json:"containers"`
	ByNamespace        []ReqAccNS        `json:"byNamespace"`
	RightsizingSavings ReqAccSavings     `json:"rightsizingSavings"`
	HealthScore        int               `json:"healthScore"`
	Grade              string            `json:"grade"`
	Recommendations    []string          `json:"recommendations"`
}

type ReqAccSummary struct {
	TotalContainers    int     `json:"totalContainers"`
	WithRequests       int     `json:"withRequests"`
	WithLimits         int     `json:"withLimits"`
	NoRequests         int     `json:"noRequests"`       // containers without any requests
	NoLimits           int     `json:"noLimits"`         // containers without any limits
	OverProvisioned    int     `json:"overProvisioned"`  // request >> actual need (heuristic)
	UnderProvisioned   int     `json:"underProvisioned"` // limit < request (risk)
	Balanced           int     `json:"balanced"`
	OvercommitRatio    float64 `json:"overcommitRatio"` // total limits / total requests for CPU
	MemOvercommitRatio float64 `json:"memOvercommitRatio"`
}

type ReqAccContainer struct {
	Name             string  `json:"name"`
	Namespace        string  `json:"namespace"`
	Workload         string  `json:"workload"`
	Kind             string  `json:"kind"`
	ReqCPU           float64 `json:"reqCPU"` // cores
	ReqMem           float64 `json:"reqMemGB"`
	LimitCPU         float64 `json:"limitCPU"`
	LimitMem         float64 `json:"limitMemGB"`
	CPUOvercommit    float64 `json:"cpuOvercommit"` // limit/req ratio, >1 = overcommitted
	MemOvercommit    float64 `json:"memOvercommit"`
	Status           string  `json:"status"` // balanced, over-provisioned, under-provisioned, no-limits
	RiskLevel        string  `json:"riskLevel"`
	SuggestedReqCPU  float64 `json:"suggestedReqCPU"`
	SuggestedReqMem  float64 `json:"suggestedReqMemGB"`
	PotentialSaveCPU float64 `json:"potentialSaveCPU"`
	PotentialSaveMem float64 `json:"potentialSaveMemGB"`
}

type ReqAccNS struct {
	Namespace      string  `json:"namespace"`
	ContainerCount int     `json:"containerCount"`
	TotalReqCPU    float64 `json:"totalReqCPU"`
	TotalReqMem    float64 `json:"totalReqMemGB"`
	OverprovCount  int     `json:"overprovCount"`
	UnderprovCount int     `json:"underprovCount"`
	NoReqCount     int     `json:"noReqCount"`
}

type ReqAccSavings struct {
	WastedCPU            float64 `json:"wastedCPU"` // cores that could be freed
	WastedMem            float64 `json:"wastedMemGB"`
	EstimatedMonthlyCost float64 `json:"estimatedMonthlyCostUSD"`
	AffectedContainers   int     `json:"affectedContainers"`
}

// handleRequestAccuracy handles GET /api/scalability/request-accuracy
func (s *Server) handleRequestAccuracy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RequestAccuracyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})

	var totalReqCPU, totalReqMem, totalLimitCPU, totalLimitMem float64
	nsMap := make(map[string]*ReqAccNS)
	var allContainers []ReqAccContainer
	var wastedCPU, wastedMem float64

	processContainers := func(workloadName, ns, kind string, replicas int, containers []corev1.Container) {
		if isSystemNamespace(ns) {
			return
		}
		for _, c := range containers {
			if _, ok := nsMap[ns]; !ok {
				nsMap[ns] = &ReqAccNS{Namespace: ns}
			}
			nsEntry := nsMap[ns]
			nsEntry.ContainerCount++
			result.Summary.TotalContainers++

			rac := ReqAccContainer{
				Name:      c.Name,
				Namespace: ns,
				Workload:  workloadName,
				Kind:      kind,
			}

			hasReq := false
			hasLimit := false

			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				rac.ReqCPU = req.AsApproximateFloat64() * float64(replicas)
				hasReq = true
				totalReqCPU += rac.ReqCPU
				nsEntry.TotalReqCPU += rac.ReqCPU
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				rac.ReqMem = req.AsApproximateFloat64() / 1e9 * float64(replicas)
				hasReq = true
				totalReqMem += rac.ReqMem
				nsEntry.TotalReqMem += rac.ReqMem
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				rac.LimitCPU = lim.AsApproximateFloat64() * float64(replicas)
				hasLimit = true
				totalLimitCPU += rac.LimitCPU
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				rac.LimitMem = lim.AsApproximateFloat64() / 1e9 * float64(replicas)
				hasLimit = true
				totalLimitMem += rac.LimitMem
			}

			if hasReq {
				result.Summary.WithRequests++
			} else {
				result.Summary.NoRequests++
				nsEntry.NoReqCount++
			}
			if hasLimit {
				result.Summary.WithLimits++
			} else {
				result.Summary.NoLimits++
			}

			// Calculate overcommit ratios
			if rac.ReqCPU > 0 && rac.LimitCPU > 0 {
				rac.CPUOvercommit = rac.LimitCPU / rac.ReqCPU
			}
			if rac.ReqMem > 0 && rac.LimitMem > 0 {
				rac.MemOvercommit = rac.LimitMem / rac.ReqMem
			}

			// Classify container
			switch {
			case !hasReq && !hasLimit:
				rac.Status = "no-limits"
				rac.RiskLevel = "high"
			case !hasReq:
				rac.Status = "no-requests"
				rac.RiskLevel = "high"
			case !hasLimit:
				rac.Status = "no-limits"
				rac.RiskLevel = "medium"
			case rac.LimitCPU > 0 && rac.ReqCPU > 0 && rac.LimitCPU/rac.ReqCPU > 4:
				rac.Status = "over-provisioned"
				rac.RiskLevel = "medium"
				result.Summary.OverProvisioned++
				nsEntry.OverprovCount++
				// Suggest right-sizing: reduce request to 50% of current
				rac.SuggestedReqCPU = rac.ReqCPU * 0.5
				rac.PotentialSaveCPU = rac.ReqCPU - rac.SuggestedReqCPU
				wastedCPU += rac.PotentialSaveCPU
				if rac.ReqMem > 0 {
					rac.SuggestedReqMem = rac.ReqMem * 0.5
					rac.PotentialSaveMem = rac.ReqMem - rac.SuggestedReqMem
					wastedMem += rac.PotentialSaveMem
				}
			case rac.LimitMem > 0 && rac.ReqMem > 0 && rac.LimitMem < rac.ReqMem:
				rac.Status = "under-provisioned"
				rac.RiskLevel = "high"
				result.Summary.UnderProvisioned++
				nsEntry.UnderprovCount++
			default:
				rac.Status = "balanced"
				rac.RiskLevel = "low"
				result.Summary.Balanced++
			}

			allContainers = append(allContainers, rac)
		}
	}

	for _, d := range deployments.Items {
		replicas := int(ptrInt32(d.Spec.Replicas))
		processContainers(d.Name, d.Namespace, "Deployment", replicas, d.Spec.Template.Spec.Containers)
	}
	for _, ss := range statefulsets.Items {
		replicas := int(ptrInt32(ss.Spec.Replicas))
		processContainers(ss.Name, ss.Namespace, "StatefulSet", replicas, ss.Spec.Template.Spec.Containers)
	}
	for _, ds := range daemonsets.Items {
		// DaemonSet: 1 copy per node, assume 1 for simplicity
		processContainers(ds.Name, ds.Namespace, "DaemonSet", 1, ds.Spec.Template.Spec.Containers)
	}

	// Overcommit ratios
	if totalReqCPU > 0 {
		result.Summary.OvercommitRatio = totalLimitCPU / totalReqCPU
	}
	if totalReqMem > 0 {
		result.Summary.MemOvercommitRatio = totalLimitMem / totalReqMem
	}

	// Namespace breakdown
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalReqCPU > result.ByNamespace[j].TotalReqCPU
	})

	// Savings estimate ($0.03/CPU-hour, $0.004/GB-hour)
	result.RightsizingSavings = ReqAccSavings{
		WastedCPU:            wastedCPU,
		WastedMem:            wastedMem,
		EstimatedMonthlyCost: wastedCPU*0.03*24*30 + wastedMem*0.004*24*30,
		AffectedContainers:   result.Summary.OverProvisioned,
	}

	// Health score
	result.HealthScore = computeReqAccScore(&result)
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	case result.HealthScore >= 35:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	// Sort containers by potential waste descending
	sort.Slice(allContainers, func(i, j int) bool {
		return allContainers[i].PotentialSaveCPU > allContainers[j].PotentialSaveCPU
	})
	result.Containers = allContainers

	result.Recommendations = buildReqAccRecs(&result)

	writeJSON(w, result)
}

func computeReqAccScore(r *RequestAccuracyResult) int {
	if r.Summary.TotalContainers == 0 {
		return 100
	}
	score := 100
	noReqPct := pctInt(r.Summary.NoRequests, r.Summary.TotalContainers)
	noLimPct := pctInt(r.Summary.NoLimits, r.Summary.TotalContainers)
	score -= int(noReqPct / 3)
	score -= int(noLimPct / 5)
	if r.Summary.OvercommitRatio > 3 {
		score -= 15
	} else if r.Summary.OvercommitRatio > 2 {
		score -= 8
	}
	if r.Summary.MemOvercommitRatio > 2 {
		score -= 10
	}
	if r.Summary.UnderProvisioned > 0 {
		score -= r.Summary.UnderProvisioned * 5
	}
	balancedPct := pctInt(r.Summary.Balanced, r.Summary.TotalContainers)
	if balancedPct < 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

func buildReqAccRecs(r *RequestAccuracyResult) []string {
	recs := []string{}
	if r.Summary.NoRequests > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器未设置资源请求，可能导致调度质量下降", r.Summary.NoRequests))
	}
	if r.Summary.NoLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器未设置资源限制，存在资源争抢风险", r.Summary.NoLimits))
	}
	if r.Summary.OverProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器过度分配，优化后可节省约 %.2f 核 CPU 和 %.2f GB 内存",
			r.Summary.OverProvisioned, r.RightsizingSavings.WastedCPU, r.RightsizingSavings.WastedMem))
	}
	if r.Summary.OvercommitRatio > 2 {
		recs = append(recs, fmt.Sprintf("CPU 超售比 %.1fx，超过安全阈值 2x", r.Summary.OvercommitRatio))
	}
	if r.Summary.UnderProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器内存限制低于请求，存在 OOM 风险", r.Summary.UnderProvisioned))
	}
	if r.RightsizingSavings.EstimatedMonthlyCost > 0 {
		recs = append(recs, fmt.Sprintf("资源优化预计每月可节省 $%.2f", r.RightsizingSavings.EstimatedMonthlyCost))
	}
	if len(recs) == 0 {
		recs = append(recs, "资源配置合理，建议定期审查以持续优化")
	}
	return recs
}

// Resource quantity helper for tests
var _ = resource.MustParse
var _ = appsv1.Deployment{}
