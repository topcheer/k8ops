package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SLOBurnRateResult calculates SLO error budget burn rates using SRE methodology.
// Fast burn rate detects acute incidents; slow burn rate detects chronic issues.
type SLOBurnRateResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	WindowHours     int            `json:"windowHours"`
	SLOTarget       float64        `json:"sloTarget"`
	ErrorBudget     float64        `json:"errorBudgetPct"`
	Summary         SLOBurnSummary `json:"summary"`
	BurnEntries     []SLOBurnEntry `json:"burnEntries"`
	CriticalBurns   []SLOBurnEntry `json:"criticalBurns"`
	BurnScore       int            `json:"burnScore"`
	Grade           string         `json:"grade"`
	Recommendations []string       `json:"recommendations"`
}

type SLOBurnSummary struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	BudgetRemaining float64 `json:"avgBudgetRemainingPct"`
	FastBurnCount   int     `json:"fastBurnCount"`
	SlowBurnCount   int     `json:"slowBurnCount"`
	BudgetExhausted int     `json:"budgetExhaustedCount"`
	HealthySLO      int     `json:"healthySLOCCount"`
}

type SLOBurnEntry struct {
	Workload        string  `json:"workload"`
	Namespace       string  `json:"namespace"`
	ErrorRate       float64 `json:"errorRatePct"`
	FastBurnRate    float64 `json:"fastBurnRate"`
	SlowBurnRate    float64 `json:"slowBurnRate"`
	BudgetRemaining float64 `json:"budgetRemainingPct"`
	Status          string  `json:"status"`
	ETAHours        float64 `json:"etaToExhaustionHours"`
}

// handleSLOBurnRate handles GET /api/operations/slo-burn-rate
func (s *Server) handleSLOBurnRate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	sloTarget := 99.9
	errorBudget := 100.0 - sloTarget // 0.1%
	fastWindow := 1.0                // 1 hour
	slowWindow := 72.0               // 72 hours (3 days)

	result := SLOBurnRateResult{
		ScannedAt:   time.Now(),
		WindowHours: int(slowWindow),
		SLOTarget:   sloTarget,
		ErrorBudget: errorBudget,
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Aggregate per-workload error indicators
	wlMap := make(map[string]*SLOBurnEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
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
			wlMap[key] = &SLOBurnEntry{Workload: wlName, Namespace: pod.Namespace}
		}
		entry := wlMap[key]

		// Estimate error rate from pod health indicators
		// Each restart or non-ready container contributes to error rate
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				entry.ErrorRate += 0.5 // 0.5% per unready container
			}
		}
		if pod.Status.Phase != corev1.PodRunning {
			entry.ErrorRate += 1.0 // 1% for non-running pod
		}
		totalR := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalR += int(cs.RestartCount)
		}
		if totalR > 0 {
			entry.ErrorRate += float64(totalR) * 0.2
		}
	}

	var entries []SLOBurnEntry
	totalBudget := 0.0
	healthyCount := 0

	for _, e := range wlMap {
		if e.ErrorRate < 0.01 {
			e.ErrorRate = 0.01 // Floor at 0.01%
		}

		// Burn rate = actual error rate / error budget
		burnRate := e.ErrorRate / errorBudget
		e.FastBurnRate = burnRate * (fastWindow / slowWindow) * 14.4 // Multiplier for 1h window
		e.SlowBurnRate = burnRate

		// Budget remaining (simplified: based on burn rate over window)
		consumed := e.ErrorRate * slowWindow / 100.0
		e.BudgetRemaining = 100.0 - consumed*10
		if e.BudgetRemaining < 0 {
			e.BudgetRemaining = 0
		}

		// Classify status
		switch {
		case e.FastBurnRate > 14.4: // 14.4x = exhaust budget in 1h
			e.Status = "critical-fast"
			result.Summary.FastBurnCount++
		case e.SlowBurnRate > 1.0: // 1x = exhaust budget in window
			e.Status = "warning-slow"
			result.Summary.SlowBurnCount++
		case e.BudgetRemaining < 10:
			e.Status = "budget-low"
			result.Summary.SlowBurnCount++
		default:
			e.Status = "healthy"
			healthyCount++
		}

		// ETA to budget exhaustion
		if burnRate > 0 && e.BudgetRemaining > 0 {
			e.ETAHours = e.BudgetRemaining / (burnRate * 100 / slowWindow)
		}

		totalBudget += e.BudgetRemaining
		entries = append(entries, *e)
	}

	result.Summary.TotalWorkloads = len(entries)
	result.Summary.HealthySLO = healthyCount
	if len(entries) > 0 {
		result.Summary.BudgetRemaining = totalBudget / float64(len(entries))
	}

	// Sort by budget remaining ascending (worst first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BudgetRemaining < entries[j].BudgetRemaining
	})
	result.BurnEntries = entries

	// Collect critical burns
	for _, e := range entries {
		if e.Status == "critical-fast" || e.Status == "warning-slow" {
			result.CriticalBurns = append(result.CriticalBurns, e)
			if e.BudgetRemaining == 0 {
				result.Summary.BudgetExhausted++
			}
		}
	}

	// Burn score: based on budget health
	result.BurnScore = int(result.Summary.BudgetRemaining)
	switch {
	case result.BurnScore >= 80:
		result.Grade = "A"
	case result.BurnScore >= 60:
		result.Grade = "B"
	case result.BurnScore >= 40:
		result.Grade = "C"
	case result.BurnScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildSLOBurnRecs(&result)
	writeJSON(w, result)
}

func buildSLOBurnRecs(r *SLOBurnRateResult) []string {
	recs := []string{
		fmt.Sprintf("SLO 目标: %.1f%% (错误预算 %.2f%%)", r.SLOTarget, r.ErrorBudget),
	}
	if r.Summary.FastBurnCount > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个工作负载快速燃烧错误预算（1h窗口内 >14.4x）", r.Summary.FastBurnCount))
	}
	if r.Summary.BudgetExhausted > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载错误预算已耗尽", r.Summary.BudgetExhausted))
	}
	if r.Summary.BudgetRemaining < 50 {
		recs = append(recs, fmt.Sprintf("平均预算剩余仅 %.1f%%，需立即关注", r.Summary.BudgetRemaining))
	}
	if r.Summary.HealthySLO == r.Summary.TotalWorkloads && r.Summary.TotalWorkloads > 0 {
		recs = append(recs, "所有工作负载 SLO 状态健康")
	}
	return recs
}
