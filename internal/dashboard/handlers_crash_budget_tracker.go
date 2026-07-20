package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CrashBudgetTrackerResult tracks crash budget consumption - how many
// crashes are "allowed" before action is required, similar to error budgets.
type CrashBudgetTrackerResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CrashBudgetSummary `json:"summary"`
	ByWorkload      []CrashBudgetEntry `json:"byWorkload"`
	BudgetConsumed  []CrashBudgetEntry `json:"budgetConsumed"`
	BudgetScore     int                `json:"budgetScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type CrashBudgetSummary struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	CrashFreeWls   int     `json:"crashFreeWorkloads"`
	CrashingWls    int     `json:"crashingWorkloads"`
	TotalCrashes   int     `json:"totalCrashes"`
	MonthlyBudget  int     `json:"monthlyCrashBudget"`
	BudgetUsed     int     `json:"budgetUsedPct"`
	AvgCrashRate   float64 `json:"avgCrashRatePerDay"`
}

type CrashBudgetEntry struct {
	Workload     string  `json:"workload"`
	Namespace    string  `json:"namespace"`
	Restarts     int     `json:"restarts"`
	DailyRate    float64 `json:"dailyCrashRate"`
	BudgetUsed   float64 `json:"budgetUsedPct"`
	Status       string  `json:"status"`
	ActionNeeded string  `json:"actionNeeded"`
}

// handleCrashBudgetTracker handles GET /api/operations/crash-budget-tracker
func (s *Server) handleCrashBudgetTracker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	monthlyBudget := 10 // 10 crashes per month per workload is the budget
	result := CrashBudgetTrackerResult{ScannedAt: time.Now()}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	wlMap := make(map[string]*CrashBudgetEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
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
			wlMap[key] = &CrashBudgetEntry{Workload: wlName, Namespace: pod.Namespace}
		}
		entry := wlMap[key]
		for _, cs := range pod.Status.ContainerStatuses {
			entry.Restarts += int(cs.RestartCount)
		}
	}

	result.Summary.MonthlyBudget = monthlyBudget
	var entries []CrashBudgetEntry
	totalCrashes := 0

	for _, e := range wlMap {
		result.Summary.TotalWorkloads++
		totalCrashes += e.Restarts

		// Daily rate (estimate: restarts over ~30 days)
		e.DailyRate = float64(e.Restarts) / 30.0

		// Budget consumption (monthly budget per workload)
		e.BudgetUsed = float64(e.Restarts) / float64(monthlyBudget) * 100
		if e.BudgetUsed > 100 {
			e.BudgetUsed = 100
		}

		// Status
		switch {
		case e.Restarts == 0:
			e.Status = "crash-free"
			e.ActionNeeded = "none"
			result.Summary.CrashFreeWls++
		case e.BudgetUsed < 50:
			e.Status = "within-budget"
			e.ActionNeeded = "monitor"
			result.Summary.CrashFreeWls++
		case e.BudgetUsed < 100:
			e.Status = "near-limit"
			e.ActionNeeded = "investigate"
			result.Summary.CrashingWls++
		default:
			e.Status = "budget-exhausted"
			e.ActionNeeded = "fix-required"
			result.Summary.CrashingWls++
			result.BudgetConsumed = append(result.BudgetConsumed, *e)
		}
		entries = append(entries, *e)
	}

	result.Summary.TotalCrashes = totalCrashes
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgCrashRate = float64(totalCrashes) / float64(result.Summary.TotalWorkloads) / 30.0
		result.Summary.BudgetUsed = totalCrashes * 100 / (result.Summary.TotalWorkloads * monthlyBudget)
		if result.Summary.BudgetUsed > 100 {
			result.Summary.BudgetUsed = 100
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].BudgetUsed > entries[j].BudgetUsed })
	result.ByWorkload = entries

	if result.Summary.TotalWorkloads > 0 {
		result.BudgetScore = result.Summary.CrashFreeWls * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.BudgetScore)

	result.Recommendations = []string{
		fmt.Sprintf("崩溃预算: %d/%d 工作负载无崩溃 (%d%%), 月预算 %d/工作负载", result.Summary.CrashFreeWls, result.Summary.TotalWorkloads, result.BudgetScore, monthlyBudget),
		fmt.Sprintf("总崩溃: %d, 平均 %.2f 次/天", totalCrashes, result.Summary.AvgCrashRate),
	}
	if len(result.BudgetConsumed) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作负载预算已耗尽, 需要修复", len(result.BudgetConsumed)))
	}
	if result.BudgetScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 设置 liveness probe, 检查 OOM 和应用错误日志")
	}
	writeJSON(w, result)
}
