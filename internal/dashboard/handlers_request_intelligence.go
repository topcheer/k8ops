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

// RequestIntelligenceResult is the resource request intelligence & right-sizing engine.
// It analyzes the gap between resource requests and actual utilization proxies to provide
// actionable right-sizing recommendations.
type RequestIntelligenceResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ReqIntelSummary     `json:"summary"`
	OverProvisioned []ReqIntelWorkload  `json:"overProvisioned"`  // wasting resources
	UnderProvisioned []ReqIntelWorkload `json:"underProvisioned"` // at risk of failure
	OptimalWorkloads int                `json:"optimalWorkloads"`
	NoRequestWorkloads int              `json:"noRequestWorkloads"`
	SavingsEstimate  SavingsEstimate    `json:"savingsEstimate"`
	RiskAssessment   RiskAssessment     `json:"riskAssessment"`
	Insights         []ReqIntelInsight  `json:"insights"`
	Recommendations  []string           `json:"recommendations"`
	PostureScore     int                `json:"postureScore"` // 0-100, higher = better right-sizing
}

// ReqIntelSummary aggregates right-sizing statistics.
type ReqIntelSummary struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	OverProvisioned  int     `json:"overProvisioned"`  // request >> usage proxy
	UnderProvisioned int     `json:"underProvisioned"` // usage proxy >> request
	Optimal          int     `json:"optimal"`          // well-sized
	NoRequests       int     `json:"noRequests"`       // no resource requests at all
	TotalCPURequest  float64 `json:"totalCPURequest"`  // total CPU requested in cores
	TotalMemRequest  float64 `json:"totalMemRequest"`  // total memory requested in GB
	EstWastedCPU     float64 `json:"estWastedCPU"`     // estimated wasted CPU cores
	EstWastedMem     float64 `json:"estWastedMem"`     // estimated wasted memory GB
}

// ReqIntelWorkload describes one workload's right-sizing analysis.
type ReqIntelWorkload struct {
	Name            string  `json:"name"`
	Namespace       string  `json:"namespace"`
	Kind            string  `json:"kind"` // Deployment, StatefulSet, DaemonSet
	Verdict         string  `json:"verdict"` // over-provisioned, under-provisioned, optimal, no-requests
	CPURequest      float64 `json:"cpuRequestMillicores"`
	MemRequest      float64 `json:"memRequestMB"`
	CPURecommend    float64 `json:"cpuRecommendMillicores,omitempty"`
	MemRecommend    float64 `json:"memRecommendMB,omitempty"`
	Confidence      string  `json:"confidence"` // high, medium, low
	Signals         []string `json:"signals"`  // evidence for the verdict
	SavingsPerMonth float64 `json:"savingsPerMonth,omitempty"` // estimated $ savings
	RiskScore       int     `json:"riskScore,omitempty"` // for under-provisioned
	PodCount        int     `json:"podCount"`
}

// SavingsEstimate quantifies potential cost savings from right-sizing.
type SavingsEstimate struct {
	MonthlyCPU     float64 `json:"monthlyCPU"`     // estimated $ / month from CPU right-sizing
	MonthlyMemory  float64 `json:"monthlyMemory"`  // estimated $ / month from memory right-sizing
	MonthlyTotal   float64 `json:"monthlyTotal"`
	NodesReduction int     `json:"nodesReduction"` // estimated nodes that could be removed
	PercentOfSpend float64 `json:"percentOfSpend"` // % of current spend that could be saved
}

// RiskAssessment quantifies risk from under-provisioning.
type RiskAssessment struct {
	HighRiskCount  int     `json:"highRiskCount"`  // workloads likely to fail from under-provisioning
	MediumRiskCount int    `json:"mediumRiskCount"`
	LowRiskCount   int     `json:"lowRiskCount"`
	EstimatedOOM   int     `json:"estimatedOOM"`   // workloads at risk of OOMKill
	EstimatedThrottle int  `json:"estimatedThrottle"` // workloads at risk of CPU throttle
}

// ReqIntelInsight describes a cross-cutting finding.
type ReqIntelInsight struct {
	Type     string `json:"type"` // pattern, anomaly, optimization
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// handleRequestIntelligence analyzes resource request right-sizing using proxy signals.
// GET /api/scalability/request-intelligence
func (s *Server) handleRequestIntelligence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RequestIntelligenceResult{ScannedAt: time.Now()}

	// Collect workloads
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulSets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonSets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})

	// Build pod lookup by owner reference
	podMap := buildPodOwnerMap(pods)

	// Analyze each workload type
	var overProvisioned, underProvisioned []ReqIntelWorkload
	optimalCount := 0
	noRequestCount := 0
	totalCPUReq := 0.0
	totalMemReq := 0.0
	estWastedCPU := 0.0
	estWastedMem := 0.0
	highRisk := 0
	mediumRisk := 0
	lowRisk := 0
	estOOM := 0
	estThrottle := 0

	// Helper to analyze a workload
	analyze := func(name, namespace, kind string, podList []corev1.Pod) {
		wl := analyzeWorkloadRequests(name, namespace, kind, podList)
		if wl.CPURequest > 0 {
			totalCPUReq += wl.CPURequest / 1000 // convert millicores to cores
		}
		if wl.MemRequest > 0 {
			totalMemReq += wl.MemRequest / 1024 // convert MB to GB
		}

		switch wl.Verdict {
		case "over-provisioned":
			overProvisioned = append(overProvisioned, wl)
			// Estimate waste: 50% of over-provisioned amount
			if wl.CPURecommend > 0 && wl.CPURequest > 0 {
				estWastedCPU += (wl.CPURequest - wl.CPURecommend) * float64(wl.PodCount) / 1000 / 2
			}
			if wl.MemRecommend > 0 && wl.MemRequest > 0 {
				estWastedMem += (wl.MemRequest - wl.MemRecommend) * float64(wl.PodCount) / 1024 / 2
			}
		case "under-provisioned":
			underProvisioned = append(underProvisioned, wl)
			if wl.RiskScore >= 70 {
				highRisk++
			} else if wl.RiskScore >= 40 {
				mediumRisk++
			} else {
				lowRisk++
			}
			// Check for OOM and throttle signals regardless of risk level
			signalStr := strings.Join(wl.Signals, " ")
			if strings.Contains(signalStr, "OOM") {
				estOOM++
			}
			if strings.Contains(signalStr, "throttle") {
				estThrottle++
			}
		case "optimal":
			optimalCount++
		case "no-requests":
			noRequestCount++
		}
	}

	// Deployments
	if deployments != nil {
		for _, d := range deployments.Items {
			podList := podMap[fmt.Sprintf("Deployment/%s/%s", d.Namespace, d.Name)]
			analyze(d.Name, d.Namespace, "Deployment", podList)
		}
	}
	// StatefulSets
	if statefulSets != nil {
		for _, ss := range statefulSets.Items {
			podList := podMap[fmt.Sprintf("StatefulSet/%s/%s", ss.Namespace, ss.Name)]
			analyze(ss.Name, ss.Namespace, "StatefulSet", podList)
		}
	}
	// DaemonSets
	if daemonSets != nil {
		for _, ds := range daemonSets.Items {
			podList := podMap[fmt.Sprintf("DaemonSet/%s/%s", ds.Namespace, ds.Name)]
			analyze(ds.Name, ds.Namespace, "DaemonSet", podList)
		}
	}

	// Sort by savings (over-provisioned) and risk (under-provisioned)
	sort.Slice(overProvisioned, func(i, j int) bool {
		return overProvisioned[i].SavingsPerMonth > overProvisioned[j].SavingsPerMonth
	})
	sort.Slice(underProvisioned, func(i, j int) bool {
		return underProvisioned[i].RiskScore > underProvisioned[j].RiskScore
	})

	// Limit results
	if len(overProvisioned) > 30 {
		overProvisioned = overProvisioned[:30]
	}
	if len(underProvisioned) > 20 {
		underProvisioned = underProvisioned[:20]
	}

	result.OverProvisioned = overProvisioned
	result.UnderProvisioned = underProvisioned
	result.OptimalWorkloads = optimalCount
	result.NoRequestWorkloads = noRequestCount

	// Summary
	totalWorkloads := optimalCount + noRequestCount + len(overProvisioned) + len(underProvisioned)
	result.Summary = ReqIntelSummary{
		TotalWorkloads:   totalWorkloads,
		OverProvisioned:  len(overProvisioned),
		UnderProvisioned: len(underProvisioned),
		Optimal:          optimalCount,
		NoRequests:       noRequestCount,
		TotalCPURequest:  math.Round(totalCPUReq*100) / 100,
		TotalMemRequest:  math.Round(totalMemReq*100) / 100,
		EstWastedCPU:     math.Round(estWastedCPU*100) / 100,
		EstWastedMem:     math.Round(estWastedMem*100) / 100,
	}

	// Savings estimate ($30/core/month, $4/GB/month standard cloud pricing)
	cpuSavings := estWastedCPU * 30
	memSavings := estWastedMem * 4
	nodesReduction := 0
	if estWastedCPU > 4 {
		nodesReduction = int(estWastedCPU / 4) // assume 4 cores per node
	}
	percentSpend := 0.0
	if totalCPUReq*30+totalMemReq*4 > 0 {
		percentSpend = (cpuSavings + memSavings) / (totalCPUReq*30 + totalMemReq*4) * 100
	}
	result.SavingsEstimate = SavingsEstimate{
		MonthlyCPU:     math.Round(cpuSavings*100) / 100,
		MonthlyMemory:  math.Round(memSavings*100) / 100,
		MonthlyTotal:   math.Round((cpuSavings+memSavings)*100) / 100,
		NodesReduction: nodesReduction,
		PercentOfSpend: math.Round(percentSpend*10) / 10,
	}

	// Risk assessment
	result.RiskAssessment = RiskAssessment{
		HighRiskCount:    highRisk,
		MediumRiskCount:  mediumRisk,
		LowRiskCount:     lowRisk,
		EstimatedOOM:     estOOM,
		EstimatedThrottle: estThrottle,
	}

	// Insights
	result.Insights = generateReqIntelInsights(result)

	// Posture score
	score := 100
	if totalWorkloads > 0 {
		score -= len(overProvisioned) * 100 / totalWorkloads * 2
		score -= noRequestCount * 100 / totalWorkloads * 3
		score -= highRisk * 5
	}
	if score < 0 {
		score = 0
	}
	result.PostureScore = score

	// Recommendations
	result.Recommendations = generateReqIntelRecs(result)

	writeJSON(w, result)
}

// analyzeWorkloadRequests analyzes a single workload's resource requests using proxy signals.
func analyzeWorkloadRequests(name, namespace, kind string, pods []corev1.Pod) ReqIntelWorkload {
	wl := ReqIntelWorkload{
		Name:      name,
		Namespace: namespace,
		Kind:      kind,
		PodCount:  len(pods),
	}

	if len(pods) == 0 {
		wl.Verdict = "optimal"
		return wl
	}

	// Collect requests from first pod (representative)
	var signals []string
	totalRestarts := 0
	scheduledOnNodes := map[string]bool{}
	hasOOMKill := false
	hasHighCPU := false // proxy: high restart with no OOM = possible CPU
	unschedulable := false

	for _, pod := range pods {
		// Sum restart counts
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
			if cs.LastTerminationState.Terminated != nil {
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "OOMKilled" {
					hasOOMKill = true
				}
			}
		}

		// Track scheduling
		if pod.Spec.NodeName != "" {
			scheduledOnNodes[pod.Spec.NodeName] = true
		}
		if pod.Status.Phase == corev1.PodPending {
			for _, cond := range pod.Status.Conditions {
				if cond.Reason == "Unschedulable" {
					unschedulable = true
				}
			}
		}
	}

	// Get resource requests from first running pod's first container
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu() != nil {
				wl.CPURequest += float64(c.Resources.Requests.Cpu().MilliValue())
			}
			if c.Resources.Requests.Memory() != nil {
				wl.MemRequest += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024) // MB
			}
		}
		break
	}

	// If no requests at all
	if wl.CPURequest == 0 && wl.MemRequest == 0 {
		wl.Verdict = "no-requests"
		wl.Signals = []string{"no resource requests set"}
		return wl
	}

	// ========================================
	// Proxy signal analysis for right-sizing
	// ========================================

	// Signal 1: OOMKill → under-provisioned memory
	if hasOOMKill {
		signals = append(signals, "OOMKill detected — memory request too low")
		wl.RiskScore += 60
		// Recommend 50% increase
		wl.MemRecommend = wl.MemRequest * 1.5
	}

	// Signal 2: High restarts without OOM → possible CPU throttle or dependency issue
	if totalRestarts > 5 && !hasOOMKill {
		hasHighCPU = true
		signals = append(signals, fmt.Sprintf("high restarts (%d) without OOM — possible CPU throttle", totalRestarts))
		wl.RiskScore += 20
		wl.CPURecommend = wl.CPURequest * 1.3
	}

	// Signal 3: Unschedulable pods → request may be too high for available nodes
	if unschedulable {
		signals = append(signals, "unschedulable pods — resource request may exceed node capacity")
		wl.RiskScore += 15
	}

	// Signal 4: No restarts + no OOM + stable → possibly over-provisioned
	// Use heuristic: if workload has been running >24h with 0-1 restarts and no pressure signals,
	// it's likely over-provisioned (the safe default is to over-request)
	if totalRestarts <= 1 && !hasOOMKill && !unschedulable {
		// Check if requests are "round numbers" (e.g., 1000m, 2000m, 1Gi, 2Gi) which typically indicate
		// guesswork rather than measured sizing
		if wl.CPURequest >= 500 && isRoundNumber(wl.CPURequest) {
			signals = append(signals, "stable workload with round-number CPU request — likely over-provisioned")
			// Recommend 70% of current
			if wl.CPURecommend == 0 {
				wl.CPURecommend = wl.CPURequest * 0.7
			}
		}
		if wl.MemRequest >= 512 && isRoundNumber(wl.MemRequest) {
			signals = append(signals, "stable workload with round-number memory request — likely over-provisioned")
			if wl.MemRecommend == 0 {
				wl.MemRecommend = wl.MemRequest * 0.7
			}
		}
	}

	// Signal 5: Pod spread — if all pods on same node, might be over-provisioned
	// (not enough room on one node for all replicas, but they fit on many nodes = over-requested)
	if len(scheduledOnNodes) > 1 && len(pods) <= 3 {
		// Multiple nodes for few pods = each pod is large
		signals = append(signals, fmt.Sprintf("%d pods spread across %d nodes — each pod consumes significant resources", len(pods), len(scheduledOnNodes)))
	}

	// Determine verdict
	if len(signals) == 0 {
		wl.Verdict = "optimal"
		wl.Confidence = "medium"
	} else if hasOOMKill || hasHighCPU || unschedulable {
		wl.Verdict = "under-provisioned"
		wl.Confidence = "high"
	} else {
		// Has signals but no failure indicators → over-provisioned
		wl.Verdict = "over-provisioned"
		wl.Confidence = "medium"

		// Estimate savings
		if wl.CPURecommend > 0 && wl.CPURequest > wl.CPURecommend {
			savedCores := (wl.CPURequest - wl.CPURecommend) * float64(wl.PodCount) / 1000
			wl.SavingsPerMonth = math.Round(savedCores*30*100) / 100
		}
		if wl.MemRecommend > 0 && wl.MemRequest > wl.MemRecommend {
			savedGB := (wl.MemRequest - wl.MemRecommend) * float64(wl.PodCount) / 1024
			wl.SavingsPerMonth += math.Round(savedGB*4*100) / 100
		}
	}

	if wl.RiskScore > 100 {
		wl.RiskScore = 100
	}
	wl.Signals = signals

	return wl
}

// buildPodOwnerMap creates a map of owner kind/ns/name → pods.
func buildPodOwnerMap(pods *corev1.PodList) map[string][]corev1.Pod {
	m := map[string][]corev1.Pod{}
	if pods == nil {
		return m
	}
	for _, pod := range pods.Items {
		for _, ref := range pod.OwnerReferences {
			key := fmt.Sprintf("%s/%s/%s", ref.Kind, pod.Namespace, ref.Name)
			m[key] = append(m[key], pod)
		}
	}
	return m
}

// isRoundNumber checks if a value is a "round" request (e.g., 1000m, 2000m, 512Mi, 1Gi).
func isRoundNumber(val float64) bool {
	// Exact matches against common round values (typical guesswork)
	roundValues := []float64{100, 200, 500, 1000, 2000, 4000, 8000,
		128, 256, 512, 1024, 2048, 4096, 8192}
	for _, rv := range roundValues {
		if val == rv {
			return true
		}
	}
	// Check multiples of 1000 for CPU (e.g., 3000m, 6000m)
	if math.Mod(val, 1000) == 0 && val >= 1000 {
		return true
	}
	// Check multiples of 1024 for memory (e.g., 3072Mi)
	if math.Mod(val, 1024) == 0 && val >= 1024 {
		return true
	}
	return false
}

// generateReqIntelInsights produces cross-cutting findings.
func generateReqIntelInsights(result RequestIntelligenceResult) []ReqIntelInsight {
	var insights []ReqIntelInsight

	if result.Summary.OverProvisioned > 0 && result.SavingsEstimate.MonthlyTotal > 0 {
		insights = append(insights, ReqIntelInsight{
			Type:     "optimization",
			Severity: "info",
			Title:    fmt.Sprintf("Right-sizing can save $%.0f/month", result.SavingsEstimate.MonthlyTotal),
			Detail:   fmt.Sprintf("%d over-provisioned workload(s) — reducing requests could free %.1f CPU cores and %.1f GB memory", result.Summary.OverProvisioned, result.Summary.EstWastedCPU, result.Summary.EstWastedMem),
		})
	}

	if result.RiskAssessment.HighRiskCount > 0 {
		insights = append(insights, ReqIntelInsight{
			Type:     "anomaly",
			Severity: "critical",
			Title:    fmt.Sprintf("%d workload(s) at risk of failure from under-provisioning", result.RiskAssessment.HighRiskCount),
			Detail:   fmt.Sprintf("%d at risk of OOMKill, %d at risk of CPU throttle — increase resource requests", result.RiskAssessment.EstimatedOOM, result.RiskAssessment.EstimatedThrottle),
		})
	}

	if result.NoRequestWorkloads > 0 {
		insights = append(insights, ReqIntelInsight{
			Type:     "pattern",
			Severity: "high",
			Title:    fmt.Sprintf("%d workload(s) with no resource requests", result.NoRequestWorkloads),
			Detail:   "Without resource requests, the scheduler cannot make informed placement decisions — workloads may cause node pressure",
		})
	}

	if result.SavingsEstimate.NodesReduction > 0 {
		insights = append(insights, ReqIntelInsight{
			Type:     "optimization",
			Severity: "info",
			Title:    fmt.Sprintf("Consolidation could remove %d node(s)", result.SavingsEstimate.NodesReduction),
			Detail:   fmt.Sprintf("After right-sizing, freed resources could allow removing %d node(s) — saving additional infrastructure cost", result.SavingsEstimate.NodesReduction),
		})
	}

	if result.PostureScore >= 80 {
		insights = append(insights, ReqIntelInsight{
			Type:     "pattern",
			Severity: "info",
			Title:    "Resource request posture is well-optimized",
			Detail:   fmt.Sprintf("Posture score %d/100 — most workloads are well-sized", result.PostureScore),
		})
	}

	return insights
}

// generateReqIntelRecs produces actionable recommendations.
func generateReqIntelRecs(result RequestIntelligenceResult) []string {
	var recs []string

	if result.RiskAssessment.HighRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("URGENT: %d workload(s) at high risk — increase resource requests to prevent OOM/throttle failures", result.RiskAssessment.HighRiskCount))
	}

	if result.NoRequestWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have no resource requests — set CPU/memory requests for proper scheduling", result.NoRequestWorkloads))
	}

	if result.Summary.OverProvisioned > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are over-provisioned — right-sizing could save $%.0f/month (%.1f%% of spend)", result.Summary.OverProvisioned, result.SavingsEstimate.MonthlyTotal, result.SavingsEstimate.PercentOfSpend))
	}

	if result.SavingsEstimate.NodesReduction > 0 {
		recs = append(recs, fmt.Sprintf("After right-sizing, consider removing %d node(s) for additional infrastructure savings", result.SavingsEstimate.NodesReduction))
	}

	if result.Summary.EstWastedCPU > 0 {
		recs = append(recs, fmt.Sprintf("Estimated %.1f CPU cores and %.1f GB memory wasted from over-provisioning — use VPA for automated right-sizing", result.Summary.EstWastedCPU, result.Summary.EstWastedMem))
	}

	if result.PostureScore >= 80 && result.RiskAssessment.HighRiskCount == 0 {
		recs = append(recs, fmt.Sprintf("Resource posture score %d/100 — cluster is well-sized with minimal risk", result.PostureScore))
	}

	if len(recs) == 0 {
		recs = append(recs, "All workloads have appropriate resource requests — no right-sizing needed")
	}

	return recs
}
