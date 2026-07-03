package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronHealthStatus describes the execution health of a CronJob.
type CronHealthStatus string

const (
	CronHealthHealthy   CronHealthStatus = "healthy"   // all recent jobs succeeded
	CronHealthWarning   CronHealthStatus = "warning"   // some recent jobs failed
	CronHealthFailing   CronHealthStatus = "failing"   // last job failed or consistently failing
	CronHealthSuspended CronHealthStatus = "suspended" // cronjob is paused
	CronHealthNoRuns    CronHealthStatus = "no-runs"   // never executed
)

// CronJobHealth summarizes the execution health of a single CronJob.
type CronJobHealth struct {
	Name             string           `json:"name"`
	Namespace        string           `json:"namespace"`
	Schedule         string           `json:"schedule"`
	Suspended        bool             `json:"suspended"`
	Status           CronHealthStatus `json:"status"`
	LastScheduleTime *time.Time       `json:"lastScheduleTime,omitempty"`
	LastSuccessful   *time.Time       `json:"lastSuccessful,omitempty"`
	LastFailed       *time.Time       `json:"lastFailed,omitempty"`
	TotalJobs        int              `json:"totalJobs"`
	SuccessfulJobs   int              `json:"successfulJobs"`
	FailedJobs       int              `json:"failedJobs"`
	RunningJobs      int              `json:"runningJobs"`
	SuccessRate      float64          `json:"successRate"`
	ConsecutiveFail  int              `json:"consecutiveFailures"`
	NextExpectedRun  *time.Time       `json:"nextExpectedRun,omitempty"`
	RecentJobs       []JobSummary     `json:"recentJobs,omitempty"`
	Issues           []string         `json:"issues,omitempty"`
}

// JobSummary is a compact view of a Job execution.
type JobSummary struct {
	Name        string        `json:"name"`
	Namespace   string        `json:"namespace"`
	Status      string        `json:"status"` // succeeded, failed, running, pending
	StartTime   *time.Time    `json:"startTime,omitempty"`
	Duration    time.Duration `json:"duration"`
	Completions int           `json:"completions"`
}

// CronHealthResult is the full scan output.
type CronHealthResult struct {
	ScannedAt time.Time       `json:"scannedAt"`
	Summary   CronSummary     `json:"summary"`
	CronJobs  []CronJobHealth `json:"cronJobs"`
}

// CronSummary aggregates CronJob health statistics.
type CronSummary struct {
	TotalCronJobs int            `json:"totalCronJobs"`
	ByStatus      map[string]int `json:"byStatus"`
	TotalJobs     int            `json:"totalJobs"`
	FailedJobs    int            `json:"failedJobs"`
	Suspended     int            `json:"suspended"`
}

// handleCronJobHealth scans all CronJobs for execution health issues.
// GET /api/operations/cronjobs/health
func (s *Server) handleCronJobHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	cronList, err := rc.clientset.BatchV1().CronJobs(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	jobList, err := rc.clientset.BatchV1().Jobs(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build owner reference index: for each CronJob, find its child Jobs
	jobsByCronJob := make(map[string][]batchv1.Job)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		for _, ref := range job.OwnerReferences {
			if ref.Kind == "CronJob" {
				key := fmt.Sprintf("%s/%s", job.Namespace, ref.Name)
				jobsByCronJob[key] = append(jobsByCronJob[key], *job)
			}
		}
	}

	var cronJobs []CronJobHealth
	summary := CronSummary{ByStatus: make(map[string]int)}

	for i := range cronList.Items {
		cj := &cronList.Items[i]
		key := fmt.Sprintf("%s/%s", cj.Namespace, cj.Name)
		jobs := jobsByCronJob[key]

		// Sort jobs by creation timestamp descending (most recent first)
		sort.Slice(jobs, func(a, b int) bool {
			return jobs[a].CreationTimestamp.After(jobs[b].CreationTimestamp.Time)
		})

		health := analyzeCronJobHealth(cj, jobs)
		cronJobs = append(cronJobs, health)

		summary.TotalCronJobs++
		summary.ByStatus[string(health.Status)]++
		summary.TotalJobs += health.TotalJobs
		summary.FailedJobs += health.FailedJobs
		if health.Suspended {
			summary.Suspended++
		}
	}

	// Sort: failing first, then warning, then healthy
	sort.Slice(cronJobs, func(i, j int) bool {
		rankI := cronStatusRank(cronJobs[i].Status)
		rankJ := cronStatusRank(cronJobs[j].Status)
		if rankI != rankJ {
			return rankI < rankJ
		}
		return cronJobs[i].Namespace+"/"+cronJobs[i].Name < cronJobs[j].Namespace+"/"+cronJobs[j].Name
	})

	writeJSON(w, CronHealthResult{
		ScannedAt: time.Now(),
		Summary:   summary,
		CronJobs:  cronJobs,
	})
}

// analyzeCronJobHealth evaluates the health of a single CronJob based on its Jobs.
func analyzeCronJobHealth(cj *batchv1.CronJob, jobs []batchv1.Job) CronJobHealth {
	health := CronJobHealth{
		Name:      cj.Name,
		Namespace: cj.Namespace,
		Schedule:  cj.Spec.Schedule,
	}

	// Check suspension
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		health.Status = CronHealthSuspended
		health.Suspended = true
		health.Issues = append(health.Issues, "CronJob is suspended — no new jobs will be created")
		return health
	}

	// Last schedule time
	if cj.Status.LastScheduleTime != nil {
		t := cj.Status.LastScheduleTime.Time
		health.LastScheduleTime = &t
	}

	// Last successful time
	if cj.Status.LastSuccessfulTime != nil {
		t := cj.Status.LastSuccessfulTime.Time
		health.LastSuccessful = &t
	}

	// Analyze recent jobs (up to 10 most recent)
	recentCount := len(jobs)
	if recentCount > 10 {
		recentCount = 10
	}

	health.TotalJobs = len(jobs)
	consecutiveFail := 0
	for i, job := range jobs[:recentCount] {
		js := jobStatusString(job)
		summary := JobSummary{
			Name:        job.Name,
			Namespace:   job.Namespace,
			Status:      js,
			Completions: int(ptrInt32Value(job.Spec.Completions)),
		}
		if job.Status.StartTime != nil {
			t := job.Status.StartTime.Time
			summary.StartTime = &t
			if job.Status.CompletionTime != nil {
				summary.Duration = job.Status.CompletionTime.Sub(job.Status.StartTime.Time)
			}
		}
		health.RecentJobs = append(health.RecentJobs, summary)

		switch js {
		case "succeeded":
			health.SuccessfulJobs++
			if consecutiveFail == 0 && i == 0 {
				// first job succeeded
			}
		case "failed":
			health.FailedJobs++
			if i == consecutiveFail {
				consecutiveFail++
			}
			if health.LastFailed == nil {
				if job.Status.CompletionTime != nil {
					t := job.Status.CompletionTime.Time
					health.LastFailed = &t
				}
			}
		case "running":
			health.RunningJobs++
		}
	}

	// Also count total successes/failures across ALL jobs
	for _, job := range jobs {
		js := jobStatusString(job)
		if js == "succeeded" {
			// already counted above for recent only, now full count
		}
	}

	health.ConsecutiveFail = consecutiveFail

	// Compute success rate from recent jobs
	if health.TotalJobs > 0 {
		health.SuccessRate = float64(health.SuccessfulJobs) / float64(health.TotalJobs) * 100
	}

	// Determine overall status
	if health.TotalJobs == 0 {
		health.Status = CronHealthNoRuns
		health.Issues = append(health.Issues, "CronJob has never executed — verify schedule and timezone configuration")
		return health
	}

	switch {
	case consecutiveFail >= 3:
		health.Status = CronHealthFailing
		health.Issues = append(health.Issues, fmt.Sprintf("%d consecutive failures — CronJob is consistently failing", consecutiveFail))
	case consecutiveFail >= 1:
		health.Status = CronHealthWarning
		health.Issues = append(health.Issues, fmt.Sprintf("Last %d job(s) failed — monitor for recovery", consecutiveFail))
	case health.SuccessRate < 50 && health.TotalJobs >= 3:
		health.Status = CronHealthWarning
		health.Issues = append(health.Issues, fmt.Sprintf("Low success rate %.0f%% across %d jobs", health.SuccessRate, health.TotalJobs))
	default:
		health.Status = CronHealthHealthy
	}

	// Check for stale CronJob (no execution in a long time)
	if health.LastScheduleTime != nil {
		hoursSinceLast := time.Since(*health.LastScheduleTime).Hours()
		scheduleInterval := estimateScheduleIntervalHours(cj.Spec.Schedule)
		// For sub-hour schedules, use 24h as minimum threshold
		if scheduleInterval == 0 {
			scheduleInterval = 1 // assume at least hourly
		}
		if hoursSinceLast > scheduleInterval*3 {
			health.Issues = append(health.Issues, fmt.Sprintf("No execution in %.0f hours (expected every %.0fh)", hoursSinceLast, scheduleInterval))
			if health.Status == CronHealthHealthy {
				health.Status = CronHealthWarning
			}
		}
	}

	return health
}

// jobStatusString returns a human-readable status for a Job.
func jobStatusString(job batchv1.Job) string {
	// Check active (running)
	if job.Status.Active > 0 {
		return "running"
	}
	// Check succeeded
	if job.Status.Succeeded > 0 {
		return "succeeded"
	}
	// Check failed
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return "failed"
		}
		if cond.Type == batchv1.JobSuspended && cond.Status == corev1.ConditionTrue {
			return "suspended"
		}
	}
	// Check if there are completions set and not yet met
	if job.Spec.Completions != nil {
		if job.Status.Succeeded < int32(*job.Spec.Completions) {
			return "pending"
		}
	}
	return "unknown"
}

// estimateScheduleIntervalHours roughly estimates the interval between runs.
func estimateScheduleIntervalHours(schedule string) float64 {
	// Very basic cron schedule parsing — handles common patterns
	// Format: "*/N * * * *" or "M H * * *"
	fields := splitFields(schedule)
	if len(fields) != 5 {
		return 0
	}

	// Every N minutes
	if len(fields[0]) > 2 && fields[0][:2] == "*/" {
		return 0 // sub-hour, not useful for staleness check
	}
	// Every N hours
	if len(fields[1]) > 2 && fields[1][:2] == "*/" {
		var n int
		fmt.Sscanf(fields[1][2:], "%d", &n)
		if n > 0 {
			return float64(n)
		}
	}
	// Weekly (day of week specified)
	if fields[4] != "*" {
		return 168
	}
	// Daily at specific hour
	if fields[1] != "*" && fields[2] == "*" {
		return 24
	}

	return 0
}

// splitFields splits a string by whitespace.
func splitFields(s string) []string {
	var fields []string
	current := ""
	for _, ch := range s {
		if ch == ' ' || ch == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

// cronStatusRank returns a sort rank for health status.
func cronStatusRank(s CronHealthStatus) int {
	switch s {
	case CronHealthFailing:
		return 0
	case CronHealthWarning:
		return 1
	case CronHealthNoRuns:
		return 2
	case CronHealthSuspended:
		return 3
	case CronHealthHealthy:
		return 4
	}
	return 9
}

// ptrInt32Value safely dereferences a *int32.
func ptrInt32Value(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
