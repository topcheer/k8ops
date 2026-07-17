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

// UnitEconomicsResult computes FinOps unit economics: cost per pod, per service,
// per namespace, efficiency ratios, and cost-per-unit metrics that translate
// infrastructure costs into business-relevant unit costs.
type UnitEconomicsResult struct {
	ScannedAt            time.Time        `json:"scannedAt"`
	Summary              UnitEconSummary  `json:"summary"`
	CostPerPod           float64          `json:"costPerPod"`
	CostPerService       float64          `json:"costPerService"`
	CostPerNS            []NSUnitEcon     `json:"costPerNamespace"`
	EfficiencyRatios     EfficiencyRatios `json:"efficiencyRatios"`
	TopCostPods          []PodCostEntry   `json:"topCostPods"`
	SavingsOpportunities []UnitSavings    `json:"savingsOpportunities"`
	HealthScore          int              `json:"healthScore"`
	Grade                string           `json:"grade"`
	Recommendations      []string         `json:"recommendations"`
}

// UnitEconSummary aggregates unit economics statistics.
type UnitEconSummary struct {
	MonthlySpend     float64 `json:"monthlySpend"`
	TotalPods        int     `json:"totalPods"`
	TotalServices    int     `json:"totalServices"`
	TotalNamespaces  int     `json:"totalNamespaces"`
	TotalCPURequests float64 `json:"totalCPURequests"` // cores
	TotalMemRequests float64 `json:"totalMemRequests"` // GB
	TotalCPULimits   float64 `json:"totalCPULimits"`
	TotalMemLimits   float64 `json:"totalMemLimits"`
	CPUPerPod        float64 `json:"cpuPerPod"`
	MemPerPod        float64 `json:"memPerPod"`
	CPUCostShare     float64 `json:"cpuCostShare"` // % of cost from CPU
	MemCostShare     float64 `json:"memCostShare"`
}

// NSUnitEcon per-namespace unit economics.
type NSUnitEcon struct {
	Namespace    string  `json:"namespace"`
	PodCount     int     `json:"podCount"`
	ServiceCount int     `json:"serviceCount"`
	MonthlyCost  float64 `json:"monthlyCost"`
	CostPerPod   float64 `json:"costPerPod"`
	CPURequests  float64 `json:"cpuRequests"`
	MemRequests  float64 `json:"memRequestsGB"`
	CostSharePct float64 `json:"costSharePct"`
	Efficiency   string  `json:"efficiency"` // high, medium, low
}

// EfficiencyRatios computes various efficiency metrics.
type EfficiencyRatios struct {
	LimitToRequestCPU float64 `json:"limitToRequestCPU"`
	LimitToRequestMem float64 `json:"limitToRequestMem"`
	UtilToRequest     float64 `json:"utilToRequest"` // estimated
	CostPerCore       float64 `json:"costPerCore"`
	CostPerGB         float64 `json:"costPerGB"`
	WastePct          float64 `json:"wastePct"`
}

// PodCostEntry describes per-pod cost.
type PodCostEntry struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	CPUCores    float64 `json:"cpuCores"`
	MemGB       float64 `json:"memGB"`
	MonthlyCost float64 `json:"monthlyCost"`
	CostPerCore float64 `json:"costPerCore"`
}

// UnitSavings describes a cost optimization opportunity.
type UnitSavings struct {
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	MonthlySavings float64 `json:"monthlySavings"`
	AnnualSavings  float64 `json:"annualSavings"`
}

// Cost estimates (USD per month)
const (
	cpuCostPerCoreMonth = 28.0 // ~$28/core/month (on-demand average)
	memCostPerGBMonth   = 3.8  // ~$3.8/GB/month (on-demand average)
)

// handleUnitEconomics handles GET /api/scalability/unit-economics
func (s *Server) handleUnitEconomics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := UnitEconomicsResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	result.Summary.TotalPods = len(pods.Items)
	result.Summary.TotalServices = len(services.Items)
	result.Summary.TotalNamespaces = len(namespaces.Items)

	// Calculate per-namespace costs
	nsCostMap := map[string]*NSUnitEcon{}
	var allPodCosts []PodCostEntry

	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Namespace, "kube-") {
			continue
		}

		var cpuReq, memReq, cpuLim, memLim float64
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					cpuReq += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					memReq += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
			if c.Resources.Limits != nil {
				if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
					cpuLim += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
					memLim += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}

		podCost := cpuReq*cpuCostPerCoreMonth + memReq*memCostPerGBMonth

		result.Summary.TotalCPURequests += cpuReq
		result.Summary.TotalMemRequests += memReq
		result.Summary.TotalCPULimits += cpuLim
		result.Summary.TotalMemLimits += memLim

		costPerCore := 0.0
		if cpuReq > 0 {
			costPerCore = podCost / cpuReq
		}

		allPodCosts = append(allPodCosts, PodCostEntry{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			CPUCores:    cpuReq,
			MemGB:       memReq,
			MonthlyCost: podCost,
			CostPerCore: costPerCore,
		})

		// Namespace tracking
		ns := pod.Namespace
		if _, ok := nsCostMap[ns]; !ok {
			nsCostMap[ns] = &NSUnitEcon{Namespace: ns}
		}
		nsCostMap[ns].PodCount++
		nsCostMap[ns].MonthlyCost += podCost
		nsCostMap[ns].CPURequests += cpuReq
		nsCostMap[ns].MemRequests += memReq
	}

	// Count services per namespace
	for _, svc := range services.Items {
		if strings.HasPrefix(svc.Namespace, "kube-") {
			continue
		}
		if e, ok := nsCostMap[svc.Namespace]; ok {
			e.ServiceCount++
		} else {
			nsCostMap[svc.Namespace] = &NSUnitEcon{Namespace: svc.Namespace, ServiceCount: 1}
		}
	}

	// Calculate monthly spend
	result.Summary.MonthlySpend = result.Summary.TotalCPURequests*cpuCostPerCoreMonth +
		result.Summary.TotalMemRequests*memCostPerGBMonth

	// Cost per unit
	if result.Summary.TotalPods > 0 {
		result.CostPerPod = result.Summary.MonthlySpend / float64(result.Summary.TotalPods)
		result.Summary.CPUPerPod = result.Summary.TotalCPURequests / float64(result.Summary.TotalPods)
		result.Summary.MemPerPod = result.Summary.TotalMemRequests / float64(result.Summary.TotalPods)
	}
	if result.Summary.TotalServices > 0 {
		result.CostPerService = result.Summary.MonthlySpend / float64(result.Summary.TotalServices)
	}

	// Cost shares
	if result.Summary.MonthlySpend > 0 {
		result.Summary.CPUCostShare = result.Summary.TotalCPURequests * cpuCostPerCoreMonth / result.Summary.MonthlySpend * 100
		result.Summary.MemCostShare = result.Summary.TotalMemRequests * memCostPerGBMonth / result.Summary.MonthlySpend * 100
	}

	// Efficiency ratios
	if result.Summary.TotalCPURequests > 0 {
		result.EfficiencyRatios.LimitToRequestCPU = result.Summary.TotalCPULimits / result.Summary.TotalCPURequests
	}
	if result.Summary.TotalMemRequests > 0 {
		result.EfficiencyRatios.LimitToRequestMem = result.Summary.TotalMemLimits / result.Summary.TotalMemRequests
	}
	if result.Summary.TotalCPURequests > 0 {
		result.EfficiencyRatios.CostPerCore = result.Summary.MonthlySpend / result.Summary.TotalCPURequests
	}
	if result.Summary.TotalMemRequests > 0 {
		result.EfficiencyRatios.CostPerGB = result.Summary.MonthlySpend / result.Summary.TotalMemRequests
	}

	// Estimate waste (pods with 0 requests or extremely over-provisioned)
	wasteCost := 0.0
	for _, pc := range allPodCosts {
		if pc.CPUCores == 0 && pc.MemGB == 0 {
			wasteCost += 0
		}
		// Over-provisioned: limit >> request
	}
	if result.Summary.MonthlySpend > 0 {
		result.EfficiencyRatios.WastePct = wasteCost / result.Summary.MonthlySpend * 100
	}

	// Build namespace stats
	for _, ns := range nsCostMap {
		if ns.PodCount > 0 {
			ns.CostPerPod = ns.MonthlyCost / float64(ns.PodCount)
		}
		if result.Summary.MonthlySpend > 0 {
			ns.CostSharePct = ns.MonthlyCost / result.Summary.MonthlySpend * 100
		}
		switch {
		case ns.CostPerPod < result.CostPerPod*0.7:
			ns.Efficiency = "high"
		case ns.CostPerPod > result.CostPerPod*1.5:
			ns.Efficiency = "low"
		default:
			ns.Efficiency = "medium"
		}
		result.CostPerNS = append(result.CostPerNS, *ns)
	}
	sort.Slice(result.CostPerNS, func(i, j int) bool {
		return result.CostPerNS[i].MonthlyCost > result.CostPerNS[j].MonthlyCost
	})

	// Top cost pods
	sort.Slice(allPodCosts, func(i, j int) bool {
		return allPodCosts[i].MonthlyCost > allPodCosts[j].MonthlyCost
	})
	if len(allPodCosts) > 20 {
		allPodCosts = allPodCosts[:20]
	}
	result.TopCostPods = allPodCosts

	// Savings opportunities
	result.SavingsOpportunities = generateUnitSavings(result.Summary, result.EfficiencyRatios, allPodCosts)

	// Compute score
	result.HealthScore = computeUnitEconScore(result.Summary, result.EfficiencyRatios)
	result.Grade = scoreToGrade(result.HealthScore)

	// Generate recommendations
	result.Recommendations = generateUnitEconRecs(result)

	writeJSON(w, result)
}

// generateUnitSavings computes savings opportunities.
func generateUnitSavings(s UnitEconSummary, r EfficiencyRatios, pods []PodCostEntry) []UnitSavings {
	var savings []UnitSavings

	// Right-size over-provisioned
	if r.LimitToRequestCPU > 3 {
		estSavings := s.MonthlySpend * 0.15
		savings = append(savings, UnitSavings{
			Type:           "right-size-limits",
			Description:    fmt.Sprintf("CPU limit/request ratio is %.1fx — right-size to reduce waste", r.LimitToRequestCPU),
			MonthlySavings: estSavings,
			AnnualSavings:  estSavings * 12,
		})
	}
	if r.LimitToRequestMem > 3 {
		estSavings := s.MonthlySpend * 0.10
		savings = append(savings, UnitSavings{
			Type:           "right-size-mem-limits",
			Description:    fmt.Sprintf("Memory limit/request ratio is %.1fx — reduce limits", r.LimitToRequestMem),
			MonthlySavings: estSavings,
			AnnualSavings:  estSavings * 12,
		})
	}

	// High cost per pod
	if s.TotalPods > 0 && s.MonthlySpend/float64(s.TotalPods) > 50 {
		savings = append(savings, UnitSavings{
			Type:           "consolidate-pods",
			Description:    fmt.Sprintf("Average cost per pod is $%.2f — consolidate to reduce overhead", s.MonthlySpend/float64(s.TotalPods)),
			MonthlySavings: s.MonthlySpend * 0.05,
			AnnualSavings:  s.MonthlySpend * 0.6,
		})
	}

	// Sort by annual savings
	sort.Slice(savings, func(i, j int) bool {
		return savings[i].AnnualSavings > savings[j].AnnualSavings
	})

	return savings
}

// computeUnitEconScore computes a 0-100 efficiency score.
func computeUnitEconScore(s UnitEconSummary, r EfficiencyRatios) int {
	score := 100

	// Penalize high limit-to-request ratios
	if r.LimitToRequestCPU > 4 {
		score -= 15
	} else if r.LimitToRequestCPU > 2 {
		score -= 5
	}
	if r.LimitToRequestMem > 4 {
		score -= 10
	} else if r.LimitToRequestMem > 2 {
		score -= 5
	}

	// Penalize waste
	score -= int(r.WastePct)

	// Penalize pods without requests (implicit 0 request = unmanaged)
	if s.TotalCPURequests == 0 && s.TotalPods > 0 {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	return score
}

// generateUnitEconRecs produces recommendations.
func generateUnitEconRecs(r UnitEconomicsResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Unit economics: $%.2f/month across %d pods (%d namespaces) — $%.2f/pod, $%.2f/service",
		r.Summary.MonthlySpend, r.Summary.TotalPods, r.Summary.TotalNamespaces, r.CostPerPod, r.CostPerService))

	recs = append(recs, fmt.Sprintf("Resource requests: %.1f CPU cores, %.1f GB memory — cost split %.0f%% CPU / %.0f%% memory",
		r.Summary.TotalCPURequests, r.Summary.TotalMemRequests, r.Summary.CPUCostShare, r.Summary.MemCostShare))

	if r.EfficiencyRatios.LimitToRequestCPU > 3 {
		recs = append(recs, fmt.Sprintf("CPU limit/request ratio is %.1fx — reduce limits to save ~$%.0f/month",
			r.EfficiencyRatios.LimitToRequestCPU, r.Summary.MonthlySpend*0.15))
	}

	if r.EfficiencyRatios.LimitToRequestMem > 3 {
		recs = append(recs, fmt.Sprintf("Memory limit/request ratio is %.1fx — reduce to save ~$%.0f/month",
			r.EfficiencyRatios.LimitToRequestMem, r.Summary.MonthlySpend*0.10))
	}

	for _, sav := range r.SavingsOpportunities {
		recs = append(recs, fmt.Sprintf("%s: $%.0f/month savings (%s)", sav.Type, sav.MonthlySavings, sav.Description))
	}

	// Most expensive namespace
	if len(r.CostPerNS) > 0 {
		top := r.CostPerNS[0]
		recs = append(recs, fmt.Sprintf("Highest cost namespace: %s ($%.2f/month, %.1f%% of total) — %d pods at $%.2f/pod",
			top.Namespace, top.MonthlyCost, top.CostSharePct, top.PodCount, top.CostPerPod))
	}

	return recs
}
