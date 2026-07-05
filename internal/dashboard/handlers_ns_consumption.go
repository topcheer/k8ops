package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NSConsumptionResult is the namespace resource consumption & cost attribution analysis.
type NSConsumptionResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         NSCSourceSummary `json:"summary"`
	ByNamespace     []NSCEntry       `json:"byNamespace"`
	TopConsumers    []NSCEntry       `json:"topConsumers"`
	IdleNamespaces  []NSCEntry       `json:"idleNamespaces"`
	WasteAnalysis   NSCWasteAnalysis `json:"wasteAnalysis"`
	CostConfig      NSCCostConfig    `json:"costConfig"`
	Recommendations []string         `json:"recommendations"`
}

// NSCSourceSummary aggregates cluster-wide consumption.
type NSCSourceSummary struct {
	TotalNamespaces   int     `json:"totalNamespaces"`
	ActiveNamespaces  int     `json:"activeNamespaces"`
	IdleNamespaces    int     `json:"idleNamespaces"`
	TotalCPUReqmCPU   float64 `json:"totalCPUReqmCPU"` // millicores
	TotalMemReqMB     float64 `json:"totalMemReqMB"`   // MB
	TotalStorageGB    float64 `json:"totalStorageGB"`
	TotalCPULimitmCPU float64 `json:"totalCPULimitmCPU"`
	TotalMemLimitMB   float64 `json:"totalMemLimitMB"`
	EstMonthlyCost    float64 `json:"estMonthlyCost"`
	AvgEfficiency     float64 `json:"avgEfficiency"` // req/limit ratio, %
}

// NSCEntry describes one namespace's consumption.
type NSCEntry struct {
	Namespace       string  `json:"namespace"`
	PodCount        int     `json:"podCount"`
	CPUReqmCPU      float64 `json:"cpuReqmCPU"`
	CPULimitmCPU    float64 `json:"cpuLimitmCPU"`
	MemReqMB        float64 `json:"memReqMB"`
	MemLimitMB      float64 `json:"memLimitMB"`
	StorageGB       float64 `json:"storageGB"`
	ContainerCount  int     `json:"containerCount"`
	EstMonthlyCost  float64 `json:"estMonthlyCost"`
	CPUEfficiency   float64 `json:"cpuEfficiency"`   // req/limit %
	MemEfficiency   float64 `json:"memEfficiency"`   // req/limit %
	OverCommitRatio float64 `json:"overCommitRatio"` // limit/req
	CostSharePct    float64 `json:"costSharePct"`
	IsIdle          bool    `json:"isIdle"`
	RiskLevel       string  `json:"riskLevel"`
}

// NSCWasteAnalysis identifies resource waste.
type NSCWasteAnalysis struct {
	OverProvisionedNS  int     `json:"overProvisionedNS"`  // req << limit (>5x)
	UnderProvisionedNS int     `json:"underProvisionedNS"` // req close to limit
	IdleCost           float64 `json:"idleCost"`           // wasted on idle NS
	WastedCPUmCPU      float64 `json:"wastedCPUmCPU"`      // limit - req gap across cluster
	WastedMemMB        float64 `json:"wastedMemMB"`
	WasteScore         int     `json:"wasteScore"` // 0-100 (higher = more waste)
}

// NSCCostConfig holds pricing assumptions.
type NSCCostConfig struct {
	CPUPricePerCorePerMonth   float64 `json:"cpuPricePerCorePerMonth"`
	MemPricePerGBPerMonth     float64 `json:"memPricePerGBPerMonth"`
	StoragePricePerGBPerMonth float64 `json:"storagePricePerGBPerMonth"`
}

// handleNSConsumption analyzes namespace resource consumption and cost attribution.
// GET /api/scalability/ns-consumption
func (s *Server) handleNSConsumption(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Default cost config (AWS-like pricing)
	costConfig := NSCCostConfig{
		CPUPricePerCorePerMonth:   28.0, // $28/core/month
		MemPricePerGBPerMonth:     3.8,  // $3.8/GB/month
		StoragePricePerGBPerMonth: 0.10, // $0.10/GB/month
	}

	// Build per-namespace consumption
	nsData := make(map[string]*NSCEntry)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry := nscGetOrCreate(nsData, pod.Namespace)
		entry.PodCount++

		for _, c := range pod.Spec.Containers {
			entry.ContainerCount++

			if req := c.Resources.Requests.Cpu(); req != nil {
				entry.CPUReqmCPU += float64(req.MilliValue())
			}
			if lim := c.Resources.Limits.Cpu(); lim != nil {
				entry.CPULimitmCPU += float64(lim.MilliValue())
			}
			if req := c.Resources.Requests.Memory(); req != nil {
				entry.MemReqMB += float64(req.Value()) / (1024 * 1024)
			}
			if lim := c.Resources.Limits.Memory(); lim != nil {
				entry.MemLimitMB += float64(lim.Value()) / (1024 * 1024)
			}
		}
	}

	// PVC storage per namespace
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}
		entry := nscGetOrCreate(nsData, pvc.Namespace)
		if cap := pvc.Status.Capacity[corev1.ResourceStorage]; !cap.IsZero() {
			entry.StorageGB += float64(cap.Value()) / (1024 * 1024 * 1024)
		}
	}

	result := NSConsumptionResult{ScannedAt: time.Now(), CostConfig: costConfig}

	// Calculate derived metrics
	var totalCost float64
	for _, entry := range nsData {
		// Cost calculation
		entry.EstMonthlyCost = nscCalcCost(entry, costConfig)
		totalCost += entry.EstMonthlyCost

		// Efficiency
		if entry.CPULimitmCPU > 0 {
			entry.CPUEfficiency = entry.CPUReqmCPU / entry.CPULimitmCPU * 100
		} else {
			entry.CPUEfficiency = 100 // no limits = fully used
		}
		if entry.MemLimitMB > 0 {
			entry.MemEfficiency = entry.MemReqMB / entry.MemLimitMB * 100
		} else {
			entry.MemEfficiency = 100
		}

		// Over-commit ratio
		if entry.CPUReqmCPU > 0 {
			entry.OverCommitRatio = entry.CPULimitmCPU / entry.CPUReqmCPU
		}

		// Idle: no running pods
		if entry.PodCount == 0 {
			entry.IsIdle = true
		}

		entry.RiskLevel = nscAssessRisk(*entry)
		result.ByNamespace = append(result.ByNamespace, *entry)
	}

	// Summary
	result.Summary.TotalNamespaces = len(result.ByNamespace)
	for _, e := range result.ByNamespace {
		if e.IsIdle {
			result.Summary.IdleNamespaces++
		} else {
			result.Summary.ActiveNamespaces++
		}
		result.Summary.TotalCPUReqmCPU += e.CPUReqmCPU
		result.Summary.TotalMemReqMB += e.MemReqMB
		result.Summary.TotalStorageGB += e.StorageGB
		result.Summary.TotalCPULimitmCPU += e.CPULimitmCPU
		result.Summary.TotalMemLimitMB += e.MemLimitMB
	}
	result.Summary.EstMonthlyCost = totalCost

	// Avg efficiency
	if result.Summary.TotalCPULimitmCPU > 0 {
		result.Summary.AvgEfficiency = result.Summary.TotalCPUReqmCPU / result.Summary.TotalCPULimitmCPU * 100
	}

	// Cost share %
	for i := range result.ByNamespace {
		if totalCost > 0 {
			result.ByNamespace[i].CostSharePct = result.ByNamespace[i].EstMonthlyCost / totalCost * 100
		}
	}

	// Sort by cost descending
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].EstMonthlyCost > result.ByNamespace[j].EstMonthlyCost
	})

	// Top consumers (max 10)
	if len(result.ByNamespace) > 10 {
		result.TopConsumers = result.ByNamespace[:10]
	} else {
		result.TopConsumers = result.ByNamespace
	}

	// Idle namespaces
	for _, e := range result.ByNamespace {
		if e.IsIdle {
			result.IdleNamespaces = append(result.IdleNamespaces, e)
		}
	}

	// Waste analysis
	result.WasteAnalysis = nscAnalyzeWaste(result.ByNamespace, result.Summary, costConfig)

	// Recommendations
	result.Recommendations = nscGenRecs(result.Summary, result.WasteAnalysis, result.TopConsumers, result.IdleNamespaces)

	writeJSON(w, result)
}

// nscCalcCost estimates monthly cost for a namespace.
func nscCalcCost(entry *NSCEntry, config NSCCostConfig) float64 {
	cpuCost := (entry.CPUReqmCPU / 1000) * config.CPUPricePerCorePerMonth
	memCost := (entry.MemReqMB / 1024) * config.MemPricePerGBPerMonth
	storageCost := entry.StorageGB * config.StoragePricePerGBPerMonth
	return cpuCost + memCost + storageCost
}

// nscAssessRisk determines risk level.
func nscAssessRisk(entry NSCEntry) string {
	if entry.IsIdle && entry.PodCount == 0 {
		return "low"
	}
	if entry.OverCommitRatio > 5 {
		return "high" // extreme over-commit
	}
	if entry.OverCommitRatio > 3 {
		return "medium"
	}
	return "low"
}

// nscAnalyzeWaste identifies resource waste.
func nscAnalyzeWaste(entries []NSCEntry, summary NSCSourceSummary, config NSCCostConfig) NSCWasteAnalysis {
	var waste NSCWasteAnalysis

	for _, e := range entries {
		if e.OverCommitRatio > 5 {
			waste.OverProvisionedNS++
		}
		if e.CPULimitmCPU > 0 && e.CPUEfficiency < 20 {
			waste.OverProvisionedNS++
		}
		if e.IsIdle {
			waste.IdleCost += e.EstMonthlyCost
		}
		waste.WastedCPUmCPU += e.CPULimitmCPU - e.CPUReqmCPU
		waste.WastedMemMB += e.MemLimitMB - e.MemReqMB
	}

	// Waste score: higher = more waste
	wasteScore := 0
	if summary.TotalCPULimitmCPU > 0 {
		wastePct := (summary.TotalCPULimitmCPU - summary.TotalCPUReqmCPU) / summary.TotalCPULimitmCPU * 100
		wasteScore = int(wastePct)
	}
	if waste.IdleCost > 0 && summary.EstMonthlyCost > 0 {
		wasteScore += int(waste.IdleCost / summary.EstMonthlyCost * 50)
	}
	if wasteScore > 100 {
		wasteScore = 100
	}
	waste.WasteScore = wasteScore

	return waste
}

// nscGenRecs generates actionable recommendations.
func nscGenRecs(summary NSCSourceSummary, waste NSCWasteAnalysis, topConsumers []NSCEntry, idleNS []NSCEntry) []string {
	var recs []string

	if summary.EstMonthlyCost > 0 {
		recs = append(recs, fmt.Sprintf("Estimated cluster monthly cost: $%.2f (CPU: $%.2f, Memory: $%.2f, Storage: $%.2f)",
			summary.EstMonthlyCost,
			(summary.TotalCPUReqmCPU/1000)*28.0,
			(summary.TotalMemReqMB/1024)*3.8,
			summary.TotalStorageGB*0.10))
	}
	if len(topConsumers) > 0 {
		top := topConsumers[0]
		recs = append(recs, fmt.Sprintf("Top consumer: %s (%.1f%% of cluster cost, $%.2f/month, %d pods)",
			top.Namespace, top.CostSharePct, top.EstMonthlyCost, top.PodCount))
	}
	if waste.IdleCost > 0 {
		recs = append(recs, fmt.Sprintf("%d idle namespace(s) cost $%.2f/month — clean up unused namespaces", len(idleNS), waste.IdleCost))
	}
	if waste.OverProvisionedNS > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have >5x over-provisioning (limit >> request) — right-size resource limits", waste.OverProvisionedNS))
	}
	if waste.WastedCPUmCPU > 0 {
		recs = append(recs, fmt.Sprintf("%.0f millicores of CPU capacity wasted in limit-request gap — reduce limits to save costs", waste.WastedCPUmCPU))
	}
	if summary.AvgEfficiency < 50 {
		recs = append(recs, fmt.Sprintf("Average CPU efficiency is %.0f%% (request/limit) — consider VPA for right-sizing", summary.AvgEfficiency))
	}

	return recs
}

func nscGetOrCreate(m map[string]*NSCEntry, ns string) *NSCEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &NSCEntry{Namespace: ns}
	m[ns] = e
	return e
}
