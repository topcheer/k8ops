package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobOrphanResult detects orphaned or misconfigured CronJobs.
type CronJobOrphanResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         CronJobOrphanSummary `json:"summary"`
	ByCronJob       []CronJobEntry       `json:"byCronJob"`
	Issues          []CronJobIssue       `json:"issues"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type CronJobOrphanSummary struct {
	TotalCronJobs   int `json:"totalCronJobs"`
	ActiveCronJobs  int `json:"activeCronJobs"`
	SuspendedJobs   int `json:"suspendedCronJobs"`
	NoConcurrency   int `json:"withoutConcurrencyLimit"`
	NoHistoryLimit  int `json:"withoutHistoryLimit"`
	NoResourceLimit int `json:"withoutResourceLimit"`
	FailedJobs      int `json:"recentlyFailedJobs"`
}

type CronJobEntry struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Schedule     string    `json:"schedule"`
	Suspended    bool      `json:"suspended"`
	Concurrency  string    `json:"concurrencyPolicy"`
	HistoryLimit int32     `json:"successfulJobsHistoryLimit"`
	LastSchedule time.Time `json:"lastScheduleTime"`
	RiskLevel    string    `json:"riskLevel"`
}

type CronJobIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleCronJobOrphan handles GET /api/product/cronjob-orphan-audit
func (s *Server) handleCronJobOrphanAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CronJobOrphanResult{ScannedAt: time.Now()}

	cronJobs, err := rc.clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, cj := range cronJobs.Items {
		if isSystemNamespace(cj.Namespace) {
			continue
		}
		result.Summary.TotalCronJobs++

		entry := CronJobEntry{
			Name:      cj.Name,
			Namespace: cj.Namespace,
			Schedule:  cj.Spec.Schedule,
			Suspended: cj.Spec.Suspend != nil && *cj.Spec.Suspend,
		}

		if cj.Spec.ConcurrencyPolicy != "" {
			entry.Concurrency = string(cj.Spec.ConcurrencyPolicy)
		} else {
			entry.Concurrency = "Allow"
			result.Summary.NoConcurrency++
			result.Issues = append(result.Issues, CronJobIssue{
				Name: cj.Name, Namespace: cj.Namespace,
				Issue: "no concurrency policy - overlapping jobs possible", Severity: "medium",
			})
		}

		entry.HistoryLimit = 3
		if cj.Spec.SuccessfulJobsHistoryLimit != nil {
			entry.HistoryLimit = *cj.Spec.SuccessfulJobsHistoryLimit
		}
		if entry.HistoryLimit > 10 {
			result.Summary.NoHistoryLimit++
			result.Issues = append(result.Issues, CronJobIssue{
				Name: cj.Name, Namespace: cj.Namespace,
				Issue: fmt.Sprintf("history limit %d too high", entry.HistoryLimit), Severity: "low",
			})
		}

		// Check resource limits
		hasLimit := false
		for _, c := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers {
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				hasLimit = true
			}
		}
		if !hasLimit {
			result.Summary.NoResourceLimit++
			result.Issues = append(result.Issues, CronJobIssue{
				Name: cj.Name, Namespace: cj.Namespace,
				Issue: "no resource limits on job containers", Severity: "high",
			})
		}

		if entry.Suspended {
			result.Summary.SuspendedJobs++
			result.Issues = append(result.Issues, CronJobIssue{
				Name: cj.Name, Namespace: cj.Namespace,
				Issue: "cronjob is suspended", Severity: "low",
			})
		} else {
			result.Summary.ActiveCronJobs++
		}

		// Check last schedule
		if cj.Status.LastScheduleTime != nil {
			entry.LastSchedule = cj.Status.LastScheduleTime.Time
			if time.Since(entry.LastSchedule) > 24*time.Hour && !entry.Suspended {
				result.Issues = append(result.Issues, CronJobIssue{
					Name: cj.Name, Namespace: cj.Namespace,
					Issue: "not scheduled in 24h despite being active", Severity: "medium",
				})
			}
		}

		// Check for failed jobs
		if cj.Status.LastSuccessfulTime == nil && !entry.Suspended {
			result.Summary.FailedJobs++
			entry.RiskLevel = "high"
		} else {
			issueCount := 0
			for _, iss := range result.Issues {
				if iss.Name == cj.Name {
					issueCount++
				}
			}
			switch {
			case issueCount >= 2:
				entry.RiskLevel = "high"
			case issueCount >= 1:
				entry.RiskLevel = "medium"
			default:
				entry.RiskLevel = "low"
			}
		}

		result.ByCronJob = append(result.ByCronJob, entry)
	}

	sort.Slice(result.ByCronJob, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByCronJob[i].RiskLevel] < rank[result.ByCronJob[j].RiskLevel]
	})

	if result.Summary.TotalCronJobs > 0 {
		healthy := result.Summary.TotalCronJobs - result.Summary.NoResourceLimit - result.Summary.FailedJobs
		result.HealthScore = healthy * 100 / result.Summary.TotalCronJobs
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100 // no cronjobs = no risk
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("CronJob 孤儿审计: %d 总计, %d 活跃, %d 挂起, %d 无并发控制, %d 无资源限制, %d 从未成功",
			result.Summary.TotalCronJobs, result.Summary.ActiveCronJobs,
			result.Summary.SuspendedJobs, result.Summary.NoConcurrency,
			result.Summary.NoResourceLimit, result.Summary.FailedJobs),
	}
	if result.Summary.NoResourceLimit > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 CronJob 无资源限制, 可能消耗过多资源", result.Summary.NoResourceLimit))
	}
	if result.Summary.FailedJobs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 CronJob 从未成功执行", result.Summary.FailedJobs))
	}
	writeJSON(w, result)
}
