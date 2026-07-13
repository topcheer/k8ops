package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronScheduleResult is the CronJob schedule conflict & resource impact analysis.
type CronScheduleResult struct {
	ScannedAt         time.Time              `json:"scannedAt"`
	Summary           CronScheduleSummary    `json:"summary"`
	ByNamespace       []CronScheduleNSStat   `json:"byNamespace"`
	Jobs              []CronScheduleEntry    `json:"jobs"`
	ScheduleConflicts []CronScheduleConflict `json:"scheduleConflicts"`
	Issues            []CronScheduleIssue    `json:"issues"`
	Recommendations   []string               `json:"recommendations"`
	HealthScore       int                    `json:"healthScore"`
}

// CronScheduleSummary aggregates CronJob schedule statistics.
type CronScheduleSummary struct {
	TotalCronJobs     int `json:"totalCronJobs"`
	SuspendedJobs     int `json:"suspendedJobs"`
	NoConcurrency     int `json:"noConcurrencyLimit"`
	NoJobHistory      int `json:"noJobHistoryLimit"`
	NoResourceLimit   int `json:"noResourceLimit"`
	ScheduleConflicts int `json:"scheduleConflicts"`
	FailedLastRun     int `json:"failedLastRun"`
	UsingTimezone     int `json:"usingTimezone"`
}

// CronScheduleNSStat per-namespace CronJob schedule stats.
type CronScheduleNSStat struct {
	Namespace string `json:"namespace"`
	Total     int    `json:"total"`
	Suspended int    `json:"suspended"`
	Conflicts int    `json:"conflicts"`
}

// CronScheduleEntry describes one CronJob from a schedule perspective.
type CronScheduleEntry struct {
	Name             string     `json:"name"`
	Namespace        string     `json:"namespace"`
	Schedule         string     `json:"schedule"`
	TimeSlot         string     `json:"timeSlot"`
	Suspend          bool       `json:"suspend"`
	ConcurrencyRule  string     `json:"concurrencyRule"`
	LastScheduleTime *time.Time `json:"lastScheduleTime,omitempty"`
	ActiveJobs       int        `json:"activeJobs"`
	HasResourceLimit bool       `json:"hasResourceLimit"`
	HasJobHistory    bool       `json:"hasJobHistoryLimit"`
	Timezone         string     `json:"timezone,omitempty"`
	RiskLevel        string     `json:"riskLevel"`
}

// CronScheduleConflict describes a time slot with multiple CronJobs.
type CronScheduleConflict struct {
	TimeSlot      string   `json:"timeSlot"`
	Namespace     string   `json:"namespace"`
	CronJobs      []string `json:"cronJobs"`
	ConflictCount int      `json:"conflictCount"`
}

// CronScheduleIssue is a detected schedule or configuration problem.
type CronScheduleIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleCronJobSchedule audits CronJob schedule conflicts and resource configuration.
// GET /api/product/cronjob-schedule
func (s *Server) handleCronJobSchedule(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	result := &CronScheduleResult{
		ScannedAt: time.Now(),
	}

	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Map time slots for conflict detection: namespace/slot -> []cronJobName
	scheduleMap := make(map[string][]string)

	var allJobs []CronScheduleEntry
	var issues []CronScheduleIssue
	var recommendations []string

	suspendedCount := 0
	noConcurrency := 0
	noHistory := 0
	noResource := 0
	failedLast := 0
	usingTimezone := 0
	conflictCount := 0

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}

		cronJobs, err := rc.clientset.BatchV1().CronJobs(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		nsTotal := 0
		nsSuspended := 0
		nsConflicts := 0

		for _, cj := range cronJobs.Items {
			entry := CronScheduleEntry{
				Name:            cj.Name,
				Namespace:       cj.Namespace,
				Schedule:        cj.Spec.Schedule,
				Suspend:         cj.Spec.Suspend != nil && *cj.Spec.Suspend,
				ConcurrencyRule: string(cj.Spec.ConcurrencyPolicy),
				ActiveJobs:      len(cj.Status.Active),
				Timezone:        "",
			}

			if entry.ConcurrencyRule == "" {
				entry.ConcurrencyRule = "Allow"
			}

			if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
				usingTimezone++
				entry.Timezone = *cj.Spec.TimeZone
			}

			// Check job history limit
			if cj.Spec.SuccessfulJobsHistoryLimit != nil || cj.Spec.FailedJobsHistoryLimit != nil {
				entry.HasJobHistory = true
			} else {
				noHistory++
			}

			// Check resource limits in job template
			hasLimit := false
			for _, c := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers {
				if c.Resources.Limits != nil && len(c.Resources.Limits) > 0 {
					hasLimit = true
					break
				}
			}
			entry.HasResourceLimit = hasLimit
			if !hasLimit {
				noResource++
				issues = append(issues, CronScheduleIssue{
					Severity: "warning",
					Type:     "no-resource-limit",
					Resource: fmt.Sprintf("%s/%s", ns.Name, cj.Name),
					Message:  "CronJob has no resource limits — may cause resource exhaustion during execution",
				})
			}

			// Parse schedule for conflict detection
			if entry.Schedule == "" {
				issues = append(issues, CronScheduleIssue{
					Severity: "critical",
					Type:     "missing-schedule",
					Resource: fmt.Sprintf("%s/%s", ns.Name, cj.Name),
					Message:  "CronJob has no schedule defined — will never execute",
				})
			} else {
				slot := extractCronScheduleSlot(entry.Schedule)
				entry.TimeSlot = slot
				if slot != "" {
					key := fmt.Sprintf("%s/%s", ns.Name, slot)
					scheduleMap[key] = append(scheduleMap[key], fmt.Sprintf("%s/%s", ns.Name, cj.Name))
				}
			}

			// Check suspend status
			if entry.Suspend {
				suspendedCount++
				nsSuspended++
				issues = append(issues, CronScheduleIssue{
					Severity: "warning",
					Type:     "suspended",
					Resource: fmt.Sprintf("%s/%s", ns.Name, cj.Name),
					Message:  "CronJob is suspended — scheduled tasks are not running",
				})
			}

			// Check concurrency policy
			if entry.ConcurrencyRule == "Allow" {
				noConcurrency++
				issues = append(issues, CronScheduleIssue{
					Severity: "info",
					Type:     "concurrency-allow",
					Resource: fmt.Sprintf("%s/%s", ns.Name, cj.Name),
					Message:  "ConcurrencyPolicy=Allow may cause overlapping job executions — consider Forbid or Replace",
				})
			}

			// Check last schedule status
			if cj.Status.LastScheduleTime != nil {
				entry.LastScheduleTime = &cj.Status.LastScheduleTime.Time
			}

			// Check for failed last execution via status fields
			if cj.Status.LastSuccessfulTime == nil && cj.Status.LastScheduleTime != nil {
				// Never succeeded but has been scheduled — likely failing
				failedLast++
				issues = append(issues, CronScheduleIssue{
					Severity: "warning",
					Type:     "never-succeeded",
					Resource: fmt.Sprintf("%s/%s", ns.Name, cj.Name),
					Message:  "CronJob has been scheduled but never succeeded — check job execution logs",
				})
			}

			// Determine risk level
			entry.RiskLevel = assessCronScheduleRisk(entry)

			allJobs = append(allJobs, entry)
			nsTotal++
		}

		if nsTotal > 0 {
			result.ByNamespace = append(result.ByNamespace, CronScheduleNSStat{
				Namespace: ns.Name,
				Total:     nsTotal,
				Suspended: nsSuspended,
				Conflicts: nsConflicts,
			})
		}
	}

	// Detect schedule conflicts (3+ jobs at same time slot in same namespace)
	for key, jobs := range scheduleMap {
		if len(jobs) >= 3 {
			parts := strings.SplitN(key, "/", 2)
			nsName := parts[0]
			slot := parts[1]
			result.ScheduleConflicts = append(result.ScheduleConflicts, CronScheduleConflict{
				TimeSlot:      slot,
				Namespace:     nsName,
				CronJobs:      jobs,
				ConflictCount: len(jobs),
			})
			conflictCount++
			issues = append(issues, CronScheduleIssue{
				Severity: "warning",
				Type:     "schedule-conflict",
				Resource: key,
				Message:  fmt.Sprintf("%d CronJobs scheduled at same time slot %s — may cause resource spikes", len(jobs), slot),
			})
		}
	}

	// Update namespace conflict counts
	for i := range result.ByNamespace {
		for _, c := range result.ScheduleConflicts {
			if result.ByNamespace[i].Namespace == c.Namespace {
				result.ByNamespace[i].Conflicts++
			}
		}
	}

	sort.Slice(allJobs, func(i, j int) bool {
		if allJobs[i].Namespace != allJobs[j].Namespace {
			return allJobs[i].Namespace < allJobs[j].Namespace
		}
		return allJobs[i].Name < allJobs[j].Name
	})

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Namespace < result.ByNamespace[j].Namespace
	})

	sort.Slice(result.ScheduleConflicts, func(i, j int) bool {
		return result.ScheduleConflicts[i].ConflictCount > result.ScheduleConflicts[j].ConflictCount
	})

	// Generate recommendations
	if suspendedCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d CronJob(s) are suspended — review if they are still needed or should be resumed", suspendedCount))
	}
	if failedLast > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d CronJob(s) had failed last execution — check job logs and fix failures", failedLast))
	}
	if noConcurrency > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d CronJob(s) use ConcurrencyPolicy=Allow — consider Forbid/Replace to prevent overlapping executions", noConcurrency))
	}
	if noHistory > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d CronJob(s) have no job history limit — set SuccessfulJobsHistoryLimit and FailedJobsHistoryLimit to avoid job accumulation", noHistory))
	}
	if noResource > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d CronJob(s) have no resource limits — add resource requests/limits to prevent resource exhaustion", noResource))
	}
	if conflictCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d schedule conflict(s) detected — stagger CronJob schedules to avoid resource spikes", conflictCount))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "All CronJob schedules are well-distributed and configurations are healthy")
	}

	result.Jobs = allJobs
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = CronScheduleSummary{
		TotalCronJobs:     len(allJobs),
		SuspendedJobs:     suspendedCount,
		NoConcurrency:     noConcurrency,
		NoJobHistory:      noHistory,
		NoResourceLimit:   noResource,
		ScheduleConflicts: conflictCount,
		FailedLastRun:     failedLast,
		UsingTimezone:     usingTimezone,
	}
	result.HealthScore = calculateCronScheduleHealthScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// extractCronScheduleSlot extracts hour:minute from a cron schedule for conflict detection.
func extractCronScheduleSlot(schedule string) string {
	parts := strings.Fields(schedule)
	if len(parts) < 2 {
		return ""
	}
	minute := parts[0]
	hour := parts[1]
	// Skip wildcards and step values
	if strings.Contains(minute, "*") || strings.Contains(hour, "*") {
		return ""
	}
	// Handle ranges and lists — use the first value
	if strings.Contains(minute, ",") {
		minute = strings.Split(minute, ",")[0]
	}
	if strings.Contains(hour, ",") {
		hour = strings.Split(hour, ",")[0]
	}
	if strings.Contains(minute, "/") {
		minute = strings.Split(minute, "/")[0]
	}
	if strings.Contains(hour, "/") {
		hour = strings.Split(hour, "/")[0]
	}
	if strings.Contains(minute, "-") {
		minute = strings.Split(minute, "-")[0]
	}
	if strings.Contains(hour, "-") {
		hour = strings.Split(hour, "-")[0]
	}
	return fmt.Sprintf("%s:%s", hour, minute)
}

// assessCronScheduleRisk determines the risk level of a CronJob schedule configuration.
func assessCronScheduleRisk(entry CronScheduleEntry) string {
	risk := 0
	if entry.Suspend {
		risk += 2
	}
	if !entry.HasResourceLimit {
		risk += 1
	}
	if entry.ConcurrencyRule == "Allow" {
		risk += 1
	}
	if entry.ActiveJobs > 3 {
		risk += 2
	}
	switch {
	case risk >= 4:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// calculateCronScheduleHealthScore computes a 0-100 health score.
func calculateCronScheduleHealthScore(summary CronScheduleSummary, issueCount int) int {
	if summary.TotalCronJobs == 0 {
		return 100
	}
	score := 100
	score -= summary.FailedLastRun * 10
	score -= summary.SuspendedJobs * 5
	score -= summary.ScheduleConflicts * 5
	score -= summary.NoResourceLimit * 2
	score -= summary.NoConcurrency * 2
	score -= summary.NoJobHistory * 1
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
