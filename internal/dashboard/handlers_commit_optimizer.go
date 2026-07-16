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

// CommitOptimizerResult analyzes resource commitment patterns and identifies
// savings opportunities through reserved instances, sustained-use discounts,
// and capacity planning. It bridges FinOps (cost optimization) with capacity
// management (ensuring sufficient resources) by analyzing request patterns
// over time and recommending commitment strategies.
type CommitOptimizerResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CommitSummary       `json:"summary"`
	StableUsage     []StableResource    `json:"stableUsage"`
	VolatileUsage   []VolatileResource  `json:"volatileUsage"`
	CommitmentPlan  []CommitmentItem    `json:"commitmentPlan"`
	ByNamespace     []CommitNSStat      `json:"byNamespace"`
	SavingsEstimate SavingsBreakdown    `json:"savingsEstimate"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// CommitSummary aggregates commitment optimization statistics.
type CommitSummary struct {
	TotalCPURequested    float64 `json:"totalCPURequested"`     // cores
	TotalMemRequested    float64 `json:"totalMemRequested"`     // GB
	StableCPUPercent     float64 `json:"stableCPUPercent"`     // % of CPU that's stable (always-on workloads)
	StableMemPercent     float64 `json:"stableMemPercent"`
	TotalPods            int     `json:"totalPods"`
	AlwaysOnPods         int     `json:"alwaysOnPods"`
	BatchPods            int     `json:"batchPods"`
	CurrentMonthlyCost   float64 `json:"currentMonthlyCost"`
	OptimizedMonthlyCost float64 `json:"optimizedMonthlyCost"`
	PotentialSavings     float64 `json:"potentialSavings"`
	SavingsPct           float64 `json:"savingsPct"`
}

// StableResource describes a workload with stable resource usage suitable for commitment.
type StableResource struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	CPUCores    float64 `json:"cpuCores"`
	MemGB       float64 `json:"memGB"`
	StabilityScore int   `json:"stabilityScore"` // 0-100
	MonthlyCost float64 `json:"monthlyCost"`
	CommitmentType string `json:"commitmentType"` // reserved, sustained-use, on-demand
}

// VolatileResource describes a workload with volatile usage patterns.
type VolatileResource struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Kind      string  `json:"kind"`
	CPUCores  float64 `json:"cpuCores"`
	MemGB     float64 `json:"memGB"`
	Volatility string `json:"volatility"` // high, medium, low
	Reason    string  `json:"reason"`
}

// CommitmentItem describes a recommended commitment action.
type CommitmentItem struct {
	Type           string  `json:"type"`           // reserved-instance, sustained-use, spot-migrate, right-size
	Resource       string  `json:"resource"`
	CurrentCost    float64 `json:"currentMonthlyCost"`
	OptimizedCost  float64 `json:"optimizedMonthlyCost"`
	MonthlySavings float64 `json:"monthlySavings"`
	AnnualSavings  float64 `json:"annualSavings"`
	Discount       float64 `json:"discountPct"`
	Confidence     int     `json:"confidence"`
}

// CommitNSStat per-namespace commitment stats.
type CommitNSStat struct {
	Namespace    string  `json:"namespace"`
	CPUCores     float64 `json:"cpuCores"`
	MemGB        float64 `json:"memGB"`
	PodCount     int     `json:"podCount"`
	MonthlyCost  float64 `json:"monthlyCost"`
	StabilityPct float64 `json:"stabilityPct"`
}

// SavingsBreakdown details potential savings by category.
type SavingsBreakdown struct {
	ReservedInstanceSavings float64 `json:"reservedInstanceSavings"`
	RightSizeSavings        float64 `json:"rightSizeSavings"`
	SpotMigrationSavings    float64 `json:"spotMigrationSavings"`
	TotalMonthlySavings     float64 `json:"totalMonthlySavings"`
	TotalAnnualSavings      float64 `json:"totalAnnualSavings"`
}

// Pricing constants
const (
	onDemandCPUPrice = 28.0  // $/core/month on-demand
	onDemandMemPrice = 3.8   // $/GB/month on-demand
	reservedDiscount  = 0.40  // 40% off with 1-yr reserved
	sustainedDiscount = 0.25  // 25% off sustained use
	spotDiscount      = 0.70  // 70% off with spot
)

// handleCommitOptimizer handles GET /api/scalability/commit-optimizer
func (s *Server) handleCommitOptimizer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CommitOptimizerResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	jobs, _ := rc.clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	cronjobs, _ := rc.clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})

	// Identify batch vs always-on workloads
	batchNS := map[string]bool{}
	for _, job := range jobs.Items {
		batchNS[job.Namespace+"/"+job.Name] = true
	}
	for _, cj := range cronjobs.Items {
		batchNS[cj.Namespace+"/"+cj.Name] = true
	}

	// Calculate resource totals
	totalCPU := 0.0
	totalMem := 0.0
	stableCPU := 0.0
	stableMem := 0.0
	alwaysOnPods := 0
	batchPods := 0
	nsStats := map[string]*CommitNSStat{}

	// Always-on workload analysis (Deployments, StatefulSets, DaemonSets)
	analyzeWorkload := func(name, ns, kind string, replicas int32, spec corev1.PodSpec) {
		if isSystemNamespace(ns) {
			return
		}
		wkCPU := 0.0
		wkMem := 0.0
		for _, c := range spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					wkCPU += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					wkMem += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}
		totalCPU += wkCPU * float64(replicas)
		totalMem += wkMem * float64(replicas)

		// Always-on workloads are stable candidates
		if replicas > 0 {
			stableCPU += wkCPU * float64(replicas)
			stableMem += wkMem * float64(replicas)
			alwaysOnPods++

			monthlyCost := (wkCPU*onDemandCPUPrice + wkMem*onDemandMemPrice) * float64(replicas)
			stability := computeStability(name, ns, kind, replicas)

			if stability >= 60 {
				result.StableUsage = append(result.StableUsage, StableResource{
					Name: name, Namespace: ns, Kind: kind,
					CPUCores:    wkCPU * float64(replicas),
					MemGB:       wkMem * float64(replicas),
					StabilityScore: stability,
					MonthlyCost: monthlyCost,
					CommitmentType: recommendCommitment(stability, kind),
				})
			}
		}

		// Namespace tracking
		if nsStats[ns] == nil {
			nsStats[ns] = &CommitNSStat{Namespace: ns}
		}
		nsStats[ns].CPUCores += wkCPU * float64(replicas)
		nsStats[ns].MemGB += wkMem * float64(replicas)
		nsStats[ns].PodCount += int(replicas)
	}

	for _, dep := range deployments.Items {
		analyzeWorkload(dep.Name, dep.Namespace, "Deployment", *dep.Spec.Replicas, dep.Spec.Template.Spec)
	}
	for _, sts := range statefulsets.Items {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		analyzeWorkload(sts.Name, sts.Namespace, "StatefulSet", replicas, sts.Spec.Template.Spec)
	}
	for _, ds := range daemonsets.Items {
		analyzeWorkload(ds.Name, ds.Namespace, "DaemonSet", ds.Status.DesiredNumberScheduled, ds.Spec.Template.Spec)
	}

	// Identify volatile workloads (Jobs, CronJobs, pending pods)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodFailed {
			batchPods++
		}
	}

	// Calculate stability percentages
	stableCPUPct := 0.0
	stableMemPct := 0.0
	if totalCPU > 0 {
		stableCPUPct = stableCPU / totalCPU * 100
	}
	if totalMem > 0 {
		stableMemPct = stableMem / totalMem * 100
	}

	currentMonthly := totalCPU*onDemandCPUPrice + totalMem*onDemandMemPrice

	// Generate commitment plan
	result.CommitmentPlan = generateCommitmentPlan(result.StableUsage, stableCPU, stableMem)

	// Calculate savings
	riSavings := stableCPU * onDemandCPUPrice * reservedDiscount
	riMemSavings := stableMem * onDemandMemPrice * reservedDiscount

	// Right-size savings (over-provisioned estimate: ~15% of total)
	rightSizeSavings := currentMonthly * 0.15

	// Spot migration (batch workloads)
	spotSavings := float64(batchPods) * 0.5 * onDemandCPUPrice * spotDiscount // rough estimate

	totalMonthlySavings := riSavings + riMemSavings + rightSizeSavings + spotSavings
	optimizedMonthly := currentMonthly - totalMonthlySavings
	if optimizedMonthly < 0 {
		optimizedMonthly = 0
	}

	result.Summary = CommitSummary{
		TotalCPURequested:    totalCPU,
		TotalMemRequested:    totalMem,
		StableCPUPercent:     stableCPUPct,
		StableMemPercent:     stableMemPct,
		TotalPods:            len(pods.Items),
		AlwaysOnPods:         alwaysOnPods,
		BatchPods:            batchPods,
		CurrentMonthlyCost:   currentMonthly,
		OptimizedMonthlyCost: optimizedMonthly,
		PotentialSavings:     totalMonthlySavings,
		SavingsPct:           0,
	}
	if currentMonthly > 0 {
		result.Summary.SavingsPct = totalMonthlySavings / currentMonthly * 100
	}

	result.SavingsEstimate = SavingsBreakdown{
		ReservedInstanceSavings: riSavings + riMemSavings,
		RightSizeSavings:        rightSizeSavings,
		SpotMigrationSavings:    spotSavings,
		TotalMonthlySavings:     totalMonthlySavings,
		TotalAnnualSavings:      totalMonthlySavings * 12,
	}

	// Namespace stats
	for _, ns := range nsStats {
		ns.MonthlyCost = ns.CPUCores*onDemandCPUPrice + ns.MemGB*onDemandMemPrice
		if totalCPU > 0 {
			ns.StabilityPct = ns.CPUCores / totalCPU * 100
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MonthlyCost > result.ByNamespace[j].MonthlyCost
	})

	// Volatile resources
	for _, job := range jobs.Items {
		if isSystemNamespace(job.Namespace) {
			continue
		}
		jCPU := 0.0
		jMem := 0.0
		for _, c := range job.Spec.Template.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					jCPU += float64(q.MilliValue()) / 1000
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					jMem += float64(q.Value()) / (1024 * 1024 * 1024)
				}
			}
		}
		if jCPU > 0 || jMem > 0 {
			result.VolatileUsage = append(result.VolatileUsage, VolatileResource{
				Name: job.Name, Namespace: job.Namespace, Kind: "Job",
				CPUCores: jCPU, MemGB: jMem, Volatility: "high",
				Reason: "Batch job — use spot instances for cost savings",
			})
		}
	}

	// Score
	result.HealthScore = computeCommitScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateCommitRecs(result)

	writeJSON(w, result)
}

// computeStability estimates how stable a workload's resource usage is.
func computeStability(name, ns, kind string, replicas int32) int {
	score := 50 // base
	if kind == "StatefulSet" {
		score += 20 // stateful = always-on
	}
	if kind == "DaemonSet" {
		score += 25 // daemonset = always-on on every node
	}
	if replicas >= 3 {
		score += 15
	}
	nsLower := strings.ToLower(ns)
	if strings.Contains(nsLower, "prod") && !strings.Contains(nsLower, "stag") {
		score += 10
	}
	if score > 100 {
		score = 100
	}
	return score
}

// recommendCommitment suggests a commitment type.
func recommendCommitment(stability int, kind string) string {
	if stability >= 80 {
		return "reserved-instance"
	}
	if stability >= 60 {
		return "sustained-use"
	}
	return "on-demand"
}

// generateCommitmentPlan produces commitment recommendations.
func generateCommitmentPlan(stable []StableResource, stableCPU, stableMem float64) []CommitmentItem {
	var items []CommitmentItem

	// Aggregate reserved instance recommendation
	if stableCPU > 0.5 || stableMem > 1 {
		riMonthly := stableCPU*onDemandCPUPrice + stableMem*onDemandMemPrice
		riOptimized := riMonthly * (1 - reservedDiscount)
		items = append(items, CommitmentItem{
			Type:           "reserved-instance",
			Resource:       fmt.Sprintf("%.1f CPU cores + %.1f GB memory", stableCPU, stableMem),
			CurrentCost:    riMonthly,
			OptimizedCost:  riOptimized,
			MonthlySavings: riMonthly - riOptimized,
			AnnualSavings:  (riMonthly - riOptimized) * 12,
			Discount:       reservedDiscount * 100,
			Confidence:     85,
		})
	}

	// Individual workload commitments for top stable resources
	sort.Slice(stable, func(i, j int) bool {
		return stable[i].MonthlyCost > stable[j].MonthlyCost
	})
	for i, sr := range stable {
		if i >= 5 {
			break
		}
		if sr.CommitmentType == "reserved-instance" {
			discount := reservedDiscount
			items = append(items, CommitmentItem{
				Type:           "reserved-instance",
				Resource:       fmt.Sprintf("%s/%s", sr.Namespace, sr.Name),
				CurrentCost:    sr.MonthlyCost,
				OptimizedCost:  sr.MonthlyCost * (1 - discount),
				MonthlySavings: sr.MonthlyCost * discount,
				AnnualSavings:  sr.MonthlyCost * discount * 12,
				Discount:       discount * 100,
				Confidence:     sr.StabilityScore,
			})
		} else if sr.CommitmentType == "sustained-use" {
			discount := sustainedDiscount
			items = append(items, CommitmentItem{
				Type:           "sustained-use",
				Resource:       fmt.Sprintf("%s/%s", sr.Namespace, sr.Name),
				CurrentCost:    sr.MonthlyCost,
				OptimizedCost:  sr.MonthlyCost * (1 - discount),
				MonthlySavings: sr.MonthlyCost * discount,
				AnnualSavings:  sr.MonthlyCost * discount * 12,
				Discount:       discount * 100,
				Confidence:     sr.StabilityScore,
			})
		}
	}

	return items
}

// computeCommitScore evaluates commitment optimization posture.
func computeCommitScore(s CommitSummary) int {
	score := 100
	// Low stable usage ratio means lots of on-demand waste
	if s.StableCPUPercent < 50 {
		score -= 15
	}
	// High savings opportunity means current setup is suboptimal
	if s.SavingsPct > 30 {
		score -= 15
	}
	if s.SavingsPct > 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateCommitRecs produces recommendations.
func generateCommitRecs(r CommitOptimizerResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Commitment optimization: $%.2f/month current → $%.2f/month optimized (%.1f%% savings, $%.0f/year)",
		r.Summary.CurrentMonthlyCost, r.Summary.OptimizedMonthlyCost, r.Summary.SavingsPct, r.SavingsEstimate.TotalAnnualSavings))

	recs = append(recs, fmt.Sprintf("Stable usage: %.1f%% CPU / %.1f%% memory — %.1f cores, %.1f GB eligible for reserved instances",
		r.Summary.StableCPUPercent, r.Summary.StableMemPercent, r.Summary.TotalCPURequested*r.Summary.StableCPUPercent/100, r.Summary.TotalMemRequested*r.Summary.StableMemPercent/100))

	for _, item := range r.CommitmentPlan {
		recs = append(recs, fmt.Sprintf("%s: %s — $%.2f/mo → $%.2f/mo (save $%.0f/yr, %.0f%% off, confidence %d%%)",
			item.Type, item.Resource, item.CurrentCost, item.OptimizedCost, item.AnnualSavings, item.Discount, item.Confidence))
	}

	if r.SavingsEstimate.RightSizeSavings > 0 {
		recs = append(recs, fmt.Sprintf("Right-size opportunity: $%.2f/month savings from reducing over-provisioned resources", r.SavingsEstimate.RightSizeSavings))
	}

	if len(r.VolatileUsage) > 0 {
		recs = append(recs, fmt.Sprintf("%d volatile workload(s) (Jobs/CronJobs) — migrate to spot instances for %.0f%% savings", len(r.VolatileUsage), spotDiscount*100))
	}

	return recs
}
