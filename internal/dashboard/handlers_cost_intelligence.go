package dashboard

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostIntelligenceResult is the cost intelligence & spend forecast engine.
// It goes beyond static cost snapshots to provide trend analysis, anomaly detection,
// spend forecasting, and FinOps maturity scoring.
type CostIntelligenceResult struct {
	ScannedAt    time.Time             `json:"scannedAt"`
	Summary      CostIntelSummary      `json:"summary"`
	ByNamespace  []CostIntelNS         `json:"byNamespace"`
	Forecast     SpendForecast         `json:"forecast"`
	Anomalies    []CostAnomaly         `json:"anomalies"`
	TopOpportunities []SavingsRanking  `json:"topOpportunities"`
	FinOpsScore  FinOpsScore           `json:"finOpsScore"`
	Recommendations []string           `json:"recommendations"`
}

// CostIntelSummary aggregates cost intelligence statistics.
type CostIntelSummary struct {
	TotalNamespaces  int     `json:"totalNamespaces"`
	TotalPods        int     `json:"totalPods"`
	TotalWorkloads   int     `json:"totalWorkloads"`
	MonthlySpend     float64 `json:"monthlySpend"`     // estimated monthly spend in USD
	DailySpend       float64 `json:"dailySpend"`       // estimated daily spend
	AnnualProjection float64 `json:"annualProjection"` // monthly * 12
	AvgCostPerPod    float64 `json:"avgCostPerPod"`
	AvgCostPerNS     float64 `json:"avgCostPerNS"`
	MedianNSSpend    float64 `json:"medianNSSpend"`    // median namespace spend
	TopNSPctOfSpend  float64 `json:"topNSPctOfSpend"`  // top 3 namespaces' share of spend
	CPUHourlyRate    float64 `json:"cpuHourlyRate"`
	MemHourlyRate    float64 `json:"memHourlyRate"`
}

// CostIntelNS is per-namespace cost intelligence.
type CostIntelNS struct {
	Namespace     string  `json:"namespace"`
	PodCount      int     `json:"podCount"`
	WorkloadCount int     `json:"workloadCount"`
	CPUCores      float64 `json:"cpuCores"`      // total CPU requested in cores
	MemGB         float64 `json:"memGB"`         // total memory requested in GB
	MonthlyCost   float64 `json:"monthlyCost"`
	DailyCost     float64 `json:"dailyCost"`
	PctOfSpend    float64 `json:"pctOfSpend"`
	CostPerPod    float64 `json:"costPerPod"`
	// Efficiency metrics
	OverRequestRatio float64 `json:"overRequestRatio"` // limit/request ratio, >3 = wasteful
	UnderutilizedPods int    `json:"underutilizedPods"`  // pods with very low requests
	SpendVelocity    string  `json:"spendVelocity"`      // increasing, stable, decreasing
	RiskLevel        string  `json:"riskLevel"`          // low, moderate, high, critical
}

// SpendForecast predicts future spend based on current allocation patterns.
type SpendForecast struct {
	CurrentMonthly   float64 `json:"currentMonthly"`   // current monthly spend
	ProjectedMonthly float64 `json:"projectedMonthly"` // projected next month spend (with trend)
	GrowthRate       float64 `json:"growthRate"`       // estimated monthly growth rate %
	ProjectedAnnual  float64 `json:"projectedAnnual"`  // annual projection
	BudgetRecommendation float64 `json:"budgetRecommendation"` // suggested monthly budget
	DaysOfDataAvailable int  `json:"daysOfDataAvailable"` // days of trend data
	Confidence       string  `json:"confidence"`       // high, medium, low
}

// CostAnomaly identifies unusual spending patterns.
type CostAnomaly struct {
	Type       string  `json:"type"`       // concentration-spike, over-request, idle-waste, runaway-growth
	Namespace  string  `json:"namespace"`
	Detail     string  `json:"detail"`
	Impact     float64 `json:"impactMonthlyCost"`
	Severity   string  `json:"severity"`   // critical, warning, info
}

// SavingsRanking ranks optimization opportunities by estimated savings.
type SavingsRanking struct {
	Rank            int     `json:"rank"`
	Type            string  `json:"type"` // right-size-cpu, right-size-mem, remove-idle, consolidate, spot-migrate
	Namespace       string  `json:"namespace"`
	Target          string  `json:"target"` // workload or resource name
	CurrentCost     float64 `json:"currentCostMonthly"`
	EstimatedSavings float64 `json:"estimatedSavingsMonthly"`
	AnnualSavings   float64 `json:"estimatedSavingsAnnual"`
	Effort          string  `json:"effort"` // low, medium, high
	Confidence      string  `json:"confidence"`
}

// FinOpsScore evaluates FinOps maturity across key dimensions.
type FinOpsScore struct {
	Grade             string  `json:"grade"` // A-F
	Score             int     `json:"score"` // 0-100
	VisibilityScore   int     `json:"visibilityScore"`   // can you see costs?
	OptimizationScore int     `json:"optimizationScore"` // are you acting on recommendations?
	BudgetScore       int     `json:"budgetScore"`       // are budgets enforced?
	EfficiencyScore   int     `json:"efficiencyScore"`   // is resource utilization good?
	AllocationScore   int     `json:"allocationScore"`   // is cost allocated to teams?
	Findings          []string `json:"findings"`
}

// handleCostIntelligence provides cost trend analysis, spend forecasting,
// anomaly detection, and FinOps maturity scoring.
// GET /api/scalability/cost-intelligence
func (s *Server) handleCostIntelligence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CostIntelligenceResult{ScannedAt: time.Now()}

	// Pricing model: approximate cloud rates
	const cpuCostPerHour = 0.034   // $ per vCPU-hour
	const memCostPerGBHour = 0.004 // $ per GB-hour
	result.Summary.CPUHourlyRate = cpuCostPerHour
	result.Summary.MemHourlyRate = memCostPerGBHour

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// List all namespaces
	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list namespaces")
		return
	}

	// List all pods for resource allocation analysis
	podList, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pods")
		return
	}

	// Build per-namespace cost data
	type nsData struct {
		cpuMilli    int64
		memBytes    int64
		limitCPUMilli int64
		limitMemBytes int64
		podCount    int
		workloads   map[string]bool
		oldestPodAge time.Duration
		newestPodAge time.Duration
		underutilPods int
	}

	nsMap := make(map[string]*nsData)
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		nsMap[ns.Name] = &nsData{
			workloads: make(map[string]bool),
		}
	}

	now := time.Now()
	for _, pod := range podList.Items {
		nsName := pod.Namespace
		if systemNS[nsName] {
			continue
		}
	ndata, ok := nsMap[nsName]
		if !ok {
			ndata = &nsData{workloads: make(map[string]bool)}
			nsMap[nsName] = ndata
		}

		ndata.podCount++
		result.Summary.TotalPods++

		// Track workload owners
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" {
				// Try to get the Deployment name from ReplicaSet
				ndata.workloads["Deployment/"+strings.TrimSuffix(ref.Name, getRSSuffix(ref.Name))] = true
			} else {
				ndata.workloads[ref.Kind+"/"+ref.Name] = true
			}
		}

		// Track pod age
		podAge := now.Sub(pod.CreationTimestamp.Time)
		if ndata.oldestPodAge == 0 || podAge > ndata.oldestPodAge {
			ndata.oldestPodAge = podAge
		}
		if ndata.newestPodAge == 0 || podAge < ndata.newestPodAge {
			ndata.newestPodAge = podAge
		}

		// Sum resource requests and limits
		podCPURequest := int64(0)
		podMemRequest := int64(0)
		podCPULimit := int64(0)
		podMemLimit := int64(0)

		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					podCPURequest += cpu.MilliValue()
				}
				if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					podMemRequest += mem.Value()
				}
			}
			if c.Resources.Limits != nil {
				if cpu, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
					podCPULimit += cpu.MilliValue()
				}
				if mem, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
					podMemLimit += mem.Value()
				}
			}
		}

		ndata.cpuMilli += podCPURequest
		ndata.memBytes += podMemRequest
		ndata.limitCPUMilli += podCPULimit
		ndata.limitMemBytes += podMemLimit

		// Check for underutilized pods (very low requests)
		if podCPURequest > 0 && podCPURequest < 50 { // < 50m CPU
			ndata.underutilPods++
		}
		if podCPURequest == 0 && podMemRequest == 0 {
			ndata.underutilPods++
		}
	}

	// Calculate costs per namespace
	var allCosts []float64
	for nsName, nd := range nsMap {
		cpuCores := float64(nd.cpuMilli) / 1000.0
		memGB := float64(nd.memBytes) / (1024 * 1024 * 1024)
		cpuMonthly := cpuCores * cpuCostPerHour * 24 * 30
		memMonthly := memGB * memCostPerGBHour * 24 * 30
		monthlyCost := cpuMonthly + memMonthly

		result.Summary.MonthlySpend += monthlyCost
		result.Summary.TotalWorkloads += len(nd.workloads)

		allCosts = append(allCosts, monthlyCost)

		// Calculate over-request ratio
		overReqRatio := 0.0
		if nd.cpuMilli > 0 && nd.limitCPUMilli > 0 {
			overReqRatio = float64(nd.limitCPUMilli) / float64(nd.cpuMilli)
		}

		// Determine spend velocity (based on pod age spread)
		velocity := "stable"
		riskLevel := "low"
		if nd.newestPodAge > 0 && nd.oldestPodAge > 0 {
			ageSpread := nd.oldestPodAge - nd.newestPodAge
			if nd.newestPodAge < 24*time.Hour && nd.podCount > 5 {
				velocity = "increasing"
				riskLevel = "moderate"
			}
			if ageSpread > 0 && nd.newestPodAge < 6*time.Hour && nd.podCount > 10 {
				velocity = "increasing"
				riskLevel = "high"
			}
		}
		if monthlyCost > 500 {
			if riskLevel == "low" {
				riskLevel = "moderate"
			}
			if monthlyCost > 2000 {
				riskLevel = "high"
			}
		}

		result.ByNamespace = append(result.ByNamespace, CostIntelNS{
			Namespace:        nsName,
			PodCount:         nd.podCount,
			WorkloadCount:    len(nd.workloads),
			CPUCores:         cpuCores,
			MemGB:            memGB,
			MonthlyCost:      roundCost(monthlyCost),
			DailyCost:        roundCost(monthlyCost / 30),
			PctOfSpend:       0, // filled after totals
			CostPerPod:       roundCost(safeDiv(monthlyCost, float64(nd.podCount))),
			OverRequestRatio: round2(overReqRatio),
			UnderutilizedPods: nd.underutilPods,
			SpendVelocity:    velocity,
			RiskLevel:        riskLevel,
		})
	}

	// Sort namespaces by cost descending
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MonthlyCost > result.ByNamespace[j].MonthlyCost
	})

	// Fill in percentage of spend
	result.Summary.TotalNamespaces = len(result.ByNamespace)
	for i := range result.ByNamespace {
		if result.Summary.MonthlySpend > 0 {
			result.ByNamespace[i].PctOfSpend = round2(
				result.ByNamespace[i].MonthlyCost / result.Summary.MonthlySpend * 100)
		}
	}

	// Calculate summary stats
	result.Summary.DailySpend = roundCost(result.Summary.MonthlySpend / 30)
	result.Summary.AnnualProjection = roundCost(result.Summary.MonthlySpend * 12)
	result.Summary.AvgCostPerPod = roundCost(safeDiv(result.Summary.MonthlySpend, float64(result.Summary.TotalPods)))
	result.Summary.AvgCostPerNS = roundCost(safeDiv(result.Summary.MonthlySpend, float64(result.Summary.TotalNamespaces)))

	// Median namespace spend
	if len(allCosts) > 0 {
		sort.Float64s(allCosts)
		mid := len(allCosts) / 2
		if len(allCosts)%2 == 0 && mid > 0 {
			result.Summary.MedianNSSpend = roundCost((allCosts[mid-1] + allCosts[mid]) / 2)
		} else {
			result.Summary.MedianNSSpend = roundCost(allCosts[mid])
		}
	}

	// Top 3 namespaces' share of spend
	top3Spend := 0.0
	for i := 0; i < len(result.ByNamespace) && i < 3; i++ {
		top3Spend += result.ByNamespace[i].MonthlyCost
	}
	if result.Summary.MonthlySpend > 0 {
		result.Summary.TopNSPctOfSpend = round2(top3Spend / result.Summary.MonthlySpend * 100)
	}

	// Detect cost anomalies
	result.Anomalies = detectCostAnomalies(result.ByNamespace, result.Summary)

	// Build spend forecast
	result.Forecast = buildSpendForecast(result.Summary, result.ByNamespace)

	// Rank optimization opportunities
	result.TopOpportunities = rankSavingsOpportunities(result.ByNamespace, cpuCostPerHour, memCostPerGBHour)

	// Calculate FinOps maturity score
	result.FinOpsScore = calculateFinOpsScore(result.Summary, result.ByNamespace, nsList.Items, systemNS)

	// Generate recommendations
	result.Recommendations = generateCostIntelRecs(result)

	writeJSON(w, result)
}

// detectCostAnomalies identifies unusual spending patterns.
func detectCostAnomalies(namespaces []CostIntelNS, summary CostIntelSummary) []CostAnomaly {
	var anomalies []CostAnomaly

	// 1. Cost concentration: single namespace > 40% of spend
	for _, ns := range namespaces {
		if ns.PctOfSpend > 40 && summary.MonthlySpend > 100 {
			sev := "warning"
			if ns.PctOfSpend > 60 {
				sev = "critical"
			}
			anomalies = append(anomalies, CostAnomaly{
				Type:      "concentration-spike",
				Namespace: ns.Namespace,
				Detail:    fmt.Sprintf("Namespace %s consumes %.1f%% of total cluster spend ($%.0f/month)", ns.Namespace, ns.PctOfSpend, ns.MonthlyCost),
				Impact:    ns.MonthlyCost,
				Severity:  sev,
			})
		}
	}

	// 2. Over-requested: high limit/request ratio
	for _, ns := range namespaces {
		if ns.OverRequestRatio > 5 && ns.MonthlyCost > 50 {
			anomalies = append(anomalies, CostAnomaly{
				Type:      "over-request",
				Namespace: ns.Namespace,
				Detail:    fmt.Sprintf("Namespace %s has limit/request ratio of %.1fx — limits are excessively higher than requests, indicating over-provisioning", ns.Namespace, ns.OverRequestRatio),
				Impact:    ns.MonthlyCost * 0.3, // estimate 30% waste
				Severity:  "warning",
			})
		}
	}

	// 3. Underutilized pods
	for _, ns := range namespaces {
		if ns.UnderutilizedPods > 3 && ns.UnderutilizedPods > ns.PodCount/2 {
			anomalies = append(anomalies, CostAnomaly{
				Type:      "idle-waste",
				Namespace: ns.Namespace,
				Detail:    fmt.Sprintf("Namespace %s has %d/%d pods with minimal or no resource requests — likely idle or underutilized", ns.Namespace, ns.UnderutilizedPods, ns.PodCount),
				Impact:    safeDiv(ns.MonthlyCost, 4),
				Severity:  "info",
			})
		}
	}

	// 4. Runaway growth
	for _, ns := range namespaces {
		if ns.SpendVelocity == "increasing" && ns.RiskLevel == "high" {
			anomalies = append(anomalies, CostAnomaly{
				Type:      "runaway-growth",
				Namespace: ns.Namespace,
				Detail:    fmt.Sprintf("Namespace %s shows rapid resource growth ($%.0f/month, %d pods, velocity: increasing) — investigate for runaway scaling", ns.Namespace, ns.MonthlyCost, ns.PodCount),
				Impact:    ns.MonthlyCost,
				Severity:  "critical",
			})
		}
	}

	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].Impact > anomalies[j].Impact
	})

	return anomalies
}

// buildSpendForecast creates a spend projection based on current patterns.
func buildSpendForecast(summary CostIntelSummary, namespaces []CostIntelNS) SpendForecast {
	forecast := SpendForecast{
		CurrentMonthly: roundCost(summary.MonthlySpend),
		ProjectedAnnual: roundCost(summary.MonthlySpend * 12),
	}

	// Estimate growth rate from velocity patterns
	increasingNS := 0
	for _, ns := range namespaces {
		if ns.SpendVelocity == "increasing" {
			increasingNS++
		}
	}

	// Growth rate estimation
	totalNS := len(namespaces)
	if totalNS > 0 {
		growthPct := float64(increasingNS) / float64(totalNS) * 15.0 // up to 15% growth
		if growthPct > 20 {
			growthPct = 20
		}
		forecast.GrowthRate = round2(growthPct)
	}

	// Projected monthly = current * (1 + growthRate/100)
	forecast.ProjectedMonthly = roundCost(summary.MonthlySpend * (1 + forecast.GrowthRate/100))
	forecast.ProjectedAnnual = roundCost(forecast.ProjectedMonthly * 12)

	// Budget recommendation: 10% buffer over projected
	forecast.BudgetRecommendation = roundCost(forecast.ProjectedMonthly * 1.1)

	// Confidence based on data availability
	if summary.TotalNamespaces > 5 && summary.TotalPods > 30 {
		forecast.Confidence = "high"
		forecast.DaysOfDataAvailable = 30
	} else if summary.TotalNamespaces > 2 {
		forecast.Confidence = "medium"
		forecast.DaysOfDataAvailable = 14
	} else {
		forecast.Confidence = "low"
		forecast.DaysOfDataAvailable = 7
	}

	return forecast
}

// rankSavingsOpportunities identifies and ranks cost optimization actions.
func rankSavingsOpportunities(namespaces []CostIntelNS, cpuRate, memRate float64) []SavingsRanking {
	var opportunities []SavingsRanking

	for _, ns := range namespaces {
		// Right-size CPU: if over-request ratio is high, estimate 30% CPU savings
		if ns.OverRequestRatio > 3 && ns.CPUCores > 0.5 {
			savings := ns.CPUCores * 0.3 * cpuRate * 24 * 30
			if savings > 5 {
				opportunities = append(opportunities, SavingsRanking{
					Type:             "right-size-cpu",
					Namespace:        ns.Namespace,
					Target:           fmt.Sprintf("%s (all workloads)", ns.Namespace),
					CurrentCost:      ns.MonthlyCost,
					EstimatedSavings: roundCost(savings),
					AnnualSavings:    roundCost(savings * 12),
					Effort:           "medium",
					Confidence:       "medium",
				})
			}
		}

		// Remove idle pods
		if ns.UnderutilizedPods > 2 {
			idleCost := safeDiv(ns.MonthlyCost, float64(ns.PodCount)) * float64(ns.UnderutilizedPods)
			if idleCost > 5 {
				opportunities = append(opportunities, SavingsRanking{
					Type:             "remove-idle",
					Namespace:        ns.Namespace,
					Target:           fmt.Sprintf("%d underutilized pods", ns.UnderutilizedPods),
					CurrentCost:      ns.MonthlyCost,
					EstimatedSavings: roundCost(idleCost * 0.5),
					AnnualSavings:    roundCost(idleCost * 0.5 * 12),
					Effort:           "low",
					Confidence:       "high",
				})
			}
		}

		// Consolidate for large namespaces
		if ns.PodCount > 20 && ns.MonthlyCost > 200 {
			consolidationSavings := ns.MonthlyCost * 0.1
			opportunities = append(opportunities, SavingsRanking{
				Type:             "consolidate",
				Namespace:        ns.Namespace,
				Target:           fmt.Sprintf("Pod consolidation (%d pods)", ns.PodCount),
				CurrentCost:      ns.MonthlyCost,
				EstimatedSavings: roundCost(consolidationSavings),
				AnnualSavings:    roundCost(consolidationSavings * 12),
				Effort:           "high",
				Confidence:       "low",
			})
		}

		// Spot migration for stateless workloads
		if ns.MonthlyCost > 100 && ns.PodCount > 5 {
			spotSavings := ns.MonthlyCost * 0.4 // ~60% spot discount
			opportunities = append(opportunities, SavingsRanking{
				Type:             "spot-migrate",
				Namespace:        ns.Namespace,
				Target:           fmt.Sprintf("Stateless workloads in %s", ns.Namespace),
				CurrentCost:      ns.MonthlyCost,
				EstimatedSavings: roundCost(spotSavings),
				AnnualSavings:    roundCost(spotSavings * 12),
				Effort:           "medium",
				Confidence:       "medium",
			})
		}
	}

	// Sort by estimated savings descending
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].EstimatedSavings > opportunities[j].EstimatedSavings
	})

	// Assign ranks
	for i := range opportunities {
		opportunities[i].Rank = i + 1
	}

	// Limit to top 15
	if len(opportunities) > 15 {
		opportunities = opportunities[:15]
	}

	return opportunities
}

// calculateFinOpsScore evaluates FinOps maturity across key dimensions.
func calculateFinOpsScore(summary CostIntelSummary, namespaces []CostIntelNS, allNS []corev1.Namespace, systemNS map[string]bool) FinOpsScore {
	score := FinOpsScore{}

	// 1. Visibility score: can you see costs? (based on resource coverage)
	totalPodsWithRequests := 0
	totalPods := 0
	for _, ns := range namespaces {
		totalPods += ns.PodCount
		if ns.UnderutilizedPods < ns.PodCount {
			totalPodsWithRequests += ns.PodCount - ns.UnderutilizedPods
		}
	}
	if totalPods > 0 {
		score.VisibilityScore = int(float64(totalPodsWithRequests) / float64(totalPods) * 100)
	}

	// 2. Optimization score: are recommendations being acted on?
	overReqCount := 0
	idleCount := 0
	for _, ns := range namespaces {
		if ns.OverRequestRatio > 3 {
			overReqCount++
		}
		if ns.UnderutilizedPods > 2 {
			idleCount++
		}
	}
	if len(namespaces) > 0 {
		issueRate := float64(overReqCount+idleCount) / float64(len(namespaces)) * 100
		score.OptimizationScore = int(math.Max(0, 100-issueRate))
	}

	// 3. Budget score: are budgets enforced? (check namespace annotations)
	nsWithBudget := 0
	for _, ns := range allNS {
		if systemNS[ns.Name] {
			continue
		}
		annotations := ns.GetAnnotations()
		if annotations != nil {
			for k := range annotations {
				if strings.Contains(k, "budget") || strings.Contains(k, "cost-limit") ||
					strings.Contains(k, "spend-limit") {
					nsWithBudget++
					break
				}
			}
		}
	}
	// Count actual non-system namespaces in the list
	totalAppNS := 0
	for _, ns := range allNS {
		if !systemNS[ns.Name] {
			totalAppNS++
		}
	}
	if totalAppNS > 0 {
		score.BudgetScore = int(float64(nsWithBudget) / float64(totalAppNS) * 100)
	} else {
		score.BudgetScore = 0
	}

	// 4. Efficiency score: is resource utilization good?
	avgOverReq := 0.0
	count := 0
	for _, ns := range namespaces {
		if ns.OverRequestRatio > 0 {
			avgOverReq += ns.OverRequestRatio
			count++
		}
	}
	if count > 0 {
		avgOverReq /= float64(count)
		// Ideal ratio is 1.5-2x; penalize >3x
		if avgOverReq <= 2.0 {
			score.EfficiencyScore = 90
		} else if avgOverReq <= 3.0 {
			score.EfficiencyScore = 70
		} else if avgOverReq <= 5.0 {
			score.EfficiencyScore = 50
		} else {
			score.EfficiencyScore = 25
		}
	} else {
		score.EfficiencyScore = 50
	}

	// 5. Allocation score: is cost allocated to teams? (check for labels)
	nsWithTeamLabel := 0
	for _, ns := range allNS {
		if systemNS[ns.Name] {
			continue
		}
		labels := ns.GetLabels()
		if labels != nil {
			for k := range labels {
				if strings.Contains(k, "team") || strings.Contains(k, "owner") ||
					strings.Contains(k, "dept") || strings.Contains(k, "cost-center") ||
					strings.Contains(k, "department") {
					nsWithTeamLabel++
					break
				}
			}
		}
	}
	if totalAppNS > 0 {
		score.AllocationScore = int(float64(nsWithTeamLabel) / float64(totalAppNS) * 100)
	}

	// Overall score
	score.Score = (score.VisibilityScore + score.OptimizationScore + score.BudgetScore +
		score.EfficiencyScore + score.AllocationScore) / 5

	// Grade
	switch {
	case score.Score >= 90:
		score.Grade = "A"
	case score.Score >= 80:
		score.Grade = "B"
	case score.Score >= 70:
		score.Grade = "C"
	case score.Score >= 60:
		score.Grade = "D"
	default:
		score.Grade = "F"
	}

	// Findings
	if score.VisibilityScore < 50 {
		score.Findings = append(score.Findings, "Low visibility: many pods lack resource requests, making cost tracking unreliable")
	}
	if score.OptimizationScore < 50 {
		score.Findings = append(score.Findings, "Optimization gaps detected: multiple namespaces have over-provisioned or idle resources")
	}
	if score.BudgetScore < 30 {
		score.Findings = append(score.Findings, "Budget enforcement is weak: few namespaces have budget annotations — add 'budget' or 'cost-limit' annotations")
	}
	if score.EfficiencyScore < 50 {
		score.Findings = append(score.Findings, "Resource efficiency is poor: average limit/request ratio is excessively high")
	}
	if score.AllocationScore < 30 {
		score.Findings = append(score.Findings, "Cost allocation is immature: add team/owner/department labels to namespaces for chargeback")
	}
	if score.Score >= 80 {
		score.Findings = append(score.Findings, "Strong FinOps posture: maintain current practices and continue optimizing")
	}

	return score
}

// generateCostIntelRecs produces actionable recommendations.
func generateCostIntelRecs(result CostIntelligenceResult) []string {
	var recs []string

	// Top cost namespace
	if len(result.ByNamespace) > 0 && result.ByNamespace[0].MonthlyCost > 0 {
		top := result.ByNamespace[0]
		recs = append(recs, fmt.Sprintf("Focus on '%s' namespace: $%.0f/month (%.1f%% of total spend) — largest optimization target", top.Namespace, top.MonthlyCost, top.PctOfSpend))
	}

	// Annual projection insight
	if result.Summary.AnnualProjection > 0 {
		recs = append(recs, fmt.Sprintf("Current annual spend projection: $%.0f — with %.1f%% growth, next year could reach $%.0f", result.Summary.AnnualProjection, result.Forecast.GrowthRate, result.Forecast.ProjectedAnnual))
	}

	// Top savings opportunity
	if len(result.TopOpportunities) > 0 {
		top := result.TopOpportunities[0]
		recs = append(recs, fmt.Sprintf("Highest-impact optimization: %s in '%s' — $%.0f/month ($%.0f/year) potential savings", top.Type, top.Namespace, top.EstimatedSavings, top.AnnualSavings))
	}

	// Budget recommendation
	if result.FinOpsScore.BudgetScore < 30 {
		recs = append(recs, "Add budget annotations (e.g., 'budget.example.com/limit: \"500\"') to namespaces to enable proactive budget alerts")
	}

	// Cost allocation
	if result.FinOpsScore.AllocationScore < 30 {
		recs = append(recs, "Add team/owner/department labels to namespaces for accurate cost chargeback and accountability")
	}

	// Concentration risk
	if result.Summary.TopNSPctOfSpend > 60 {
		recs = append(recs, fmt.Sprintf("Cost concentration risk: top 3 namespaces consume %.1f%% of spend — diversify or optimize these namespaces first", result.Summary.TopNSPctOfSpend))
	}

	// Anomaly alerts
	critCount := 0
	for _, a := range result.Anomalies {
		if a.Severity == "critical" {
			critCount++
		}
	}
	if critCount > 0 {
		recs = append(recs, fmt.Sprintf("%d critical cost anomalies detected — review and remediate immediately", critCount))
	}

	// Efficiency
	totalPotentialSavings := 0.0
	for _, opp := range result.TopOpportunities {
		totalPotentialSavings += opp.EstimatedSavings
	}
	if totalPotentialSavings > 0 {
		recs = append(recs, fmt.Sprintf("Total identifiable savings: $%.0f/month ($%.0f/year) across %d optimization opportunities", totalPotentialSavings, totalPotentialSavings*12, len(result.TopOpportunities)))
	}

	return recs
}

// getRSSuffix extracts the hash suffix from a ReplicaSet name.
// ReplicaSets created by Deployments have a 9+ char hash suffix.
func getRSSuffix(name string) string {
	// Look for the last '-' followed by alphanumeric hash
	idx := strings.LastIndex(name, "-")
	if idx > 0 && idx < len(name)-1 {
		suffix := name[idx+1:]
		// Check if it looks like a hash (alphanumeric, length >= 5)
		if len(suffix) >= 5 {
			isHash := true
			for _, c := range suffix {
				if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
					isHash = false
					break
				}
			}
			if isHash {
				return suffix
			}
		}
	}
	return ""
}

// roundCost rounds to 2 decimal places.
func roundCost(v float64) float64 {
	return math.Round(v*100) / 100
}

// round2 rounds to 1 decimal place.
func round2(v float64) float64 {
	return math.Round(v*10) / 10
}

// safeDiv prevents division by zero.
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}


