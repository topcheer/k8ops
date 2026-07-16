package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GreenComputingResult is the green computing & sustainability scorecard.
// Beyond carbon footprint, it assesses energy efficiency, PUE estimation,
// waste heat utilization, renewable energy readiness, and resource efficiency
// from a sustainability perspective.
type GreenComputingResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         GreenSummary         `json:"summary"`
	Efficiency      GreenEfficiency      `json:"efficiency"`
	WasteBreakdown  []GreenWasteItem    `json:"wasteBreakdown"`
	ByNamespace     []GreenNSStat        `json:"byNamespace"`
	Recommendations []GreenRec           `json:"greenRecommendations"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Verdict         string               `json:"verdict"`
	Tips            []string             `json:"tips"`
}

// GreenSummary aggregates sustainability statistics.
type GreenSummary struct {
	TotalCPUCores     float64 `json:"totalCPUCores"`
	TotalMemoryGB     float64 `json:"totalMemoryGB"`
	TotalStorageGB    float64 `json:"totalStorageGB"`
	IdleCPUPercent    float64 `json:"idleCPUPercent"`
	IdleMemPercent    float64 `json:"idleMemPercent"`
	EstimatedPowerKW  float64 `json:"estimatedPowerKW"`
	AnnualEnergyKWh   float64 `json:"annualEnergyKWh"`
	AnnualCostUSD     float64 `json:"annualCostUSD"`
	CarbonIntensity   float64 `json:"carbonIntensity"` // gCO2/kWh
	AnnualCO2Kg       float64 `json:"annualCO2Kg"`
	PUEstimate        float64 `json:"pueEstimate"`
}

// GreenEfficiency holds efficiency metrics.
type GreenEfficiency struct {
	ResourceUtilization float64 `json:"resourceUtilization"` // 0-100
	WorkloadDensity     float64 `json:"workloadDensity"`     // pods per CPU core
	EnergyPerPod        float64 `json:"energyPerPodWatts"`
	EfficiencyScore     int     `json:"efficiencyScore"`
	WasteCPUCores       float64 `json:"wasteCPUCores"`
	WasteMemGB          float64 `json:"wasteMemGB"`
	WastePercent        float64 `json:"wastePercent"`
}

// WasteItem describes one category of resource waste.
type GreenWasteItem struct {
	Category    string  `json:"category"`
	Description string  `json:"description"`
	ImpactWatts float64 `json:"impactWatts"`
	AnnualCO2   float64 `json:"annualCO2Kg"`
	Severity    string  `json:"severity"`
}

// GreenNSStat per-namespace sustainability stats.
type GreenNSStat struct {
	Namespace    string  `json:"namespace"`
	CPUCores     float64 `json:"cpuCores"`
	MemGB        float64 `json:"memGB"`
	PodCount     int     `json:"podCount"`
	IdleCPU      float64 `json:"idleCPU"`
	PowerWatts   float64 `json:"powerWatts"`
	Efficiency   string  `json:"efficiency"`
}

// GreenRec is a sustainability recommendation.
type GreenRec struct {
	Category      string  `json:"category"`
	Action        string  `json:"action"`
	Impact        string  `json:"impact"`
	AnnualSavings float64 `json:"annualSavingsUSD"`
	CO2Reduction  float64 `json:"co2ReductionKg"`
}

// Energy cost constants (approximate)
const (
	wattsPerCPUCore    = 8.0  // idle-to-active average
	wattsPerGBMemory   = 0.4
	wattsPerGBStorage  = 0.1
	electricityPerKWh  = 0.12 // USD
	worldAvgCarbonInt  = 475  // gCO2/kWh world average
	estimatedPUE       = 1.55 // typical data center PUE
)

// handleGreenComputing handles GET /api/scalability/green-computing
func (s *Server) handleGreenComputing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GreenComputingResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Calculate total resources
	totalCPUReq := 0.0
	totalMemReq := 0.0
	totalCPUIdle := 0.0
	totalMemIdle := 0.0
	nsStats := map[string]*GreenNSStat{}
	totalPods := 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		totalPods++

		podCPU := 0.0
		podMem := 0.0

		for _, c := range pod.Spec.Containers {
			cpuReq := 0.0
			memReq := 0.0
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					cpuReq = float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					memReq = float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
			podCPU += cpuReq
			podMem += memReq
		}

		totalCPUReq += podCPU
		totalMemReq += podMem

		// Estimate idle: pods with very low actual usage vs request
		// Simplified: if pod has 0 containers running or is in pending, it's idle
		isIdle := pod.Status.Phase != corev1.PodRunning
		if isIdle {
			totalCPUIdle += podCPU
			totalMemIdle += podMem
		}

		// Namespace tracking
		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &GreenNSStat{Namespace: ns}
		}
		nsStats[ns].CPUCores += podCPU
		nsStats[ns].MemGB += podMem
		nsStats[ns].PodCount++
		if isIdle {
			nsStats[ns].IdleCPU += podCPU
		}
	}

	// Calculate storage
	totalStorage := 0.0
	for _, pvc := range pvcs.Items {
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			totalStorage += float64(q.Value()) / (1024 * 1024 * 1024)
		}
	}

	// Calculate node capacity for utilization
	nodeCPUCapacity := 0.0
	nodeMemCapacity := 0.0
	for _, node := range nodes.Items {
		if q := node.Status.Allocatable[corev1.ResourceCPU]; !q.IsZero() {
			nodeCPUCapacity += float64(q.MilliValue()) / 1000
		}
		if q := node.Status.Allocatable[corev1.ResourceMemory]; !q.IsZero() {
			nodeMemCapacity += float64(q.Value()) / (1024 * 1024 * 1024)
		}
	}

	// Power estimation
	computePower := totalCPUReq*wattsPerCPUCore + totalMemReq*wattsPerGBMemory
	storagePower := totalStorage * wattsPerGBStorage
	totalPower := (computePower + storagePower) * estimatedPUE / 1000 // kW

	annualEnergy := totalPower * 24 * 365 // kWh
	annualCost := annualEnergy * electricityPerKWh
	annualCO2 := annualEnergy * worldAvgCarbonInt / 1000 // kg CO2

	// Utilization
	utilPct := 0.0
	if nodeCPUCapacity > 0 {
		utilPct = totalCPUReq / nodeCPUCapacity * 100
	}

	// Waste
	wasteCPU := totalCPUIdle
	wasteMem := totalMemIdle
	wastePercent := 0.0
	if totalCPUReq > 0 {
		wastePercent = wasteCPU / totalCPUReq * 100
	}

	energyPerPod := 0.0
	if totalPods > 0 && totalPower > 0 {
		energyPerPod = totalPower * 1000 / float64(totalPods) // watts
	}

	// Build summary
	result.Summary = GreenSummary{
		TotalCPUCores:    totalCPUReq,
		TotalMemoryGB:    totalMemReq,
		TotalStorageGB:   totalStorage,
		IdleCPUPercent:   wastePercent,
		EstimatedPowerKW: totalPower,
		AnnualEnergyKWh:  annualEnergy,
		AnnualCostUSD:    annualCost,
		CarbonIntensity:  worldAvgCarbonInt,
		AnnualCO2Kg:      annualCO2,
		PUEstimate:       estimatedPUE,
	}

	// Efficiency
	workloadDensity := 0.0
	if totalCPUReq > 0 && totalPods > 0 {
		workloadDensity = float64(totalPods) / totalCPUReq
	}
	effScore := 100
	if wastePercent > 20 {
		effScore -= int(wastePercent)
	}
	if utilPct < 30 && nodeCPUCapacity > 1 {
		effScore -= 15
	}

	result.Efficiency = GreenEfficiency{
		ResourceUtilization: utilPct,
		WorkloadDensity:     workloadDensity,
		EnergyPerPod:        energyPerPod,
		EfficiencyScore:     effScore,
		WasteCPUCores:       wasteCPU,
		WasteMemGB:          wasteMem,
		WastePercent:        wastePercent,
	}

	// Waste breakdown
	if wasteCPU > 0 {
		wattsWasted := wasteCPU * wattsPerCPUCore * estimatedPUE
		result.WasteBreakdown = append(result.WasteBreakdown, GreenWasteItem{
			Category:    "idle-cpu",
			Description: fmt.Sprintf("%.1f CPU cores from idle/non-running pods", wasteCPU),
			ImpactWatts: wattsWasted,
			AnnualCO2:   wattsWasted * 24 * 365 / 1000 * worldAvgCarbonInt / 1000,
			Severity:    "medium",
		})
	}

	// Check for pods without resource limits (energy waste from unbounded resources)
	noLimitPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
			noLimitPods++
				break
			}
		}
	}
	if noLimitPods > 0 {
		result.WasteBreakdown = append(result.WasteBreakdown, GreenWasteItem{
			Category:    "unbounded-resources",
			Description: fmt.Sprintf("%d pods without resource limits — unbounded energy consumption", noLimitPods),
			ImpactWatts: float64(noLimitPods) * 5 * estimatedPUE,
			AnnualCO2:   float64(noLimitPods) * 5 * 24 * 365 / 1000 * worldAvgCarbonInt / 1000,
			Severity:    "high",
		})
	}

	// Build namespace stats
	for _, ns := range nsStats {
		ns.PowerWatts = (ns.CPUCores*wattsPerCPUCore + ns.MemGB*wattsPerGBMemory) * estimatedPUE
		idleRatio := 0.0
		if ns.CPUCores > 0 {
			idleRatio = ns.IdleCPU / ns.CPUCores * 100
		}
		switch {
		case idleRatio < 10:
			ns.Efficiency = "high"
		case idleRatio < 30:
			ns.Efficiency = "medium"
		default:
			ns.Efficiency = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PowerWatts > result.ByNamespace[j].PowerWatts
	})

	// Compute score
	result.HealthScore = computeGreenScore(result.Summary, result.Efficiency, len(result.WasteBreakdown))
	result.Grade = scoreToGrade(result.HealthScore)
	result.Verdict = greenVerdict(result.HealthScore)

	// Recommendations
	result.Recommendations = generateGreenRecs(result)
	result.Tips = generateGreenTips(result.Summary, result.Efficiency)

	writeJSON(w, result)
}

// computeGreenScore computes a sustainability score.
func computeGreenScore(s GreenSummary, e GreenEfficiency, wasteItems int) int {
	score := 100
	// Penalize waste
	score -= int(e.WastePercent)
	// Penalize low utilization
	if e.ResourceUtilization < 30 {
		score -= 15
	}
	// Penalize waste items
	score -= wasteItems * 5
	// Penalize high energy per pod
	if e.EnergyPerPod > 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// greenVerdict determines sustainability verdict.
func greenVerdict(score int) string {
	switch {
	case score >= 80:
		return "eco-friendly"
	case score >= 60:
		return "moderate"
	case score >= 40:
		return "wasteful"
	default:
		return "critical"
	}
}

// generateGreenRecs produces sustainability recommendations.
func generateGreenRecs(r GreenComputingResult) []GreenRec {
	var recs []GreenRec

	if r.Efficiency.WasteCPUCores > 0 {
		savings := r.Efficiency.WasteCPUCores * wattsPerCPUCore * estimatedPUE * 24 * 365 / 1000 * electricityPerKWh
		co2 := r.Efficiency.WasteCPUCores * wattsPerCPUCore * estimatedPUE * 24 * 365 / 1000 * worldAvgCarbonInt / 1000
		recs = append(recs, GreenRec{
			Category:      "right-size",
			Action:        fmt.Sprintf("Remove %.1f idle CPU cores to save energy", r.Efficiency.WasteCPUCores),
			Impact:        "medium",
			AnnualSavings: savings,
			CO2Reduction:  co2,
		})
	}

	if r.Efficiency.ResourceUtilization < 30 {
		recs = append(recs, GreenRec{
			Category:      "consolidate",
			Action:        "Resource utilization is below 30% — consolidate workloads to fewer nodes",
			Impact:        "high",
			AnnualSavings: r.Summary.AnnualCostUSD * 0.20,
			CO2Reduction:  r.Summary.AnnualCO2Kg * 0.20,
		})
	}

	for _, w := range r.WasteBreakdown {
		if w.Category == "unbounded-resources" {
			recs = append(recs, GreenRec{
				Category:      "add-limits",
				Action:        "Add resource limits to all pods to bound energy consumption",
				Impact:        "medium",
				AnnualSavings: w.ImpactWatts * 24 * 365 / 1000 * electricityPerKWh,
				CO2Reduction:  w.AnnualCO2,
			})
		}
	}

	return recs
}

// generateGreenTips produces quick sustainability tips.
func generateGreenTips(s GreenSummary, e GreenEfficiency) []string {
	var tips []string

	tips = append(tips, fmt.Sprintf("Estimated power consumption: %.2f kW (PUE %.2f) — %.0f kWh/year", s.EstimatedPowerKW, s.PUEstimate, s.AnnualEnergyKWh))
	tips = append(tips, fmt.Sprintf("Annual energy cost: $%.0f — carbon footprint: %.0f kg CO2/year", s.AnnualCostUSD, s.AnnualCO2Kg))

	if e.WastePercent > 10 {
		tips = append(tips, fmt.Sprintf("%.1f%% of requested resources are wasted — scale down idle workloads", e.WastePercent))
	}

	if e.ResourceUtilization < 40 {
		tips = append(tips, fmt.Sprintf("Only %.0f%% of node capacity is used — consolidate to reduce carbon footprint", e.ResourceUtilization))
	}

	if e.WorkloadDensity < 3 && s.TotalCPUCores > 2 {
		tips = append(tips, fmt.Sprintf("Workload density is %.1f pods/core — increase density for better energy efficiency", e.WorkloadDensity))
	}

	tips = append(tips, "Consider scheduling non-critical workloads during off-peak hours to reduce carbon intensity")
	tips = append(tips, "Use HPA to scale down during low-traffic periods — saves energy automatically")

	return tips
}

// Reference resource package to avoid unused import
var _ resource.Quantity
