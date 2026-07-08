package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JBResult is the batch job execution health analysis.
type JBResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         JBSummary `json:"summary"`
	ByJob           []JBEntry `json:"byJob"`
	FailedJobs      []JBEntry `json:"failedJobs"`
	LongRunning     []JBEntry `json:"longRunning"` // running > expected
	SuspendedJobs   []JBEntry `json:"suspendedJobs"`
	Issues          []JBIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// JBSummary aggregates job stats.
type JBSummary struct {
	TotalJobs       int `json:"totalJobs"`
	RunningJobs     int `json:"runningJobs"`
	CompletedJobs   int `json:"completedJobs"`
	FailedJobs      int `json:"failedJobs"`
	SuspendedJobs   int `json:"suspendedJobs"`
	LongRunningJobs int `json:"longRunningJobs"` // >24h
	NoBackoffLimit  int `json:"noBackoffLimit"`
	HealthScore     int `json:"healthScore"`
}

// JBEntry describes one job's health.
type JBEntry struct {
	Name           string     `json:"name"`
	Namespace      string     `json:"namespace"`
	Status         string     `json:"status"` // Running, Complete, Failed, Suspended
	StartTime      *time.Time `json:"startTime,omitempty"`
	CompletionTime *time.Time `json:"completionTime,omitempty"`
	DurationHours  float64    `json:"durationHours"`
	Completions    int32      `json:"completions"`
	Succeeded      int32      `json:"succeeded"`
	Failed         int32      `json:"failed"`
	BackoffLimit   *int32     `json:"backoffLimit,omitempty"`
	CronJobOwner   string     `json:"cronJobOwner,omitempty"` // parent CronJob name
	RiskLevel      string     `json:"riskLevel"`
}

// JBIssue is a detected job problem.
type JBIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleJobHealth analyzes batch job execution health.
// GET /api/product/job-health
func (s *Server) handleJobHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	jobs, err := rc.clientset.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	now := time.Now()
	result := JBResult{ScannedAt: now}

	for _, job := range jobs.Items {
		result.Summary.TotalJobs++

		entry := JBEntry{
			Name:      job.Name,
			Namespace: job.Namespace,
		}

		// Completions & status
		if job.Spec.Completions != nil {
			entry.Completions = *job.Spec.Completions
		}
		entry.Succeeded = job.Status.Succeeded
		entry.Failed = job.Status.Failed
		entry.BackoffLimit = job.Spec.BackoffLimit

		// Suspend check
		if job.Spec.Suspend != nil && *job.Spec.Suspend {
			entry.Status = "Suspended"
			result.Summary.SuspendedJobs++
			result.SuspendedJobs = append(result.SuspendedJobs, entry)
			result.Issues = append(result.Issues, JBIssue{
				Severity: "info", Type: "suspended-job",
				Resource: fmt.Sprintf("%s/%s", job.Namespace, job.Name),
				Message:  fmt.Sprintf("Job %s/%s is suspended — will not run until resumed", job.Namespace, job.Name),
			})
		} else {
			// Determine status from conditions
			complete := false
			failed := false
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobComplete && cond.Status == "True" {
					complete = true
				}
				if cond.Type == batchv1.JobFailed && cond.Status == "True" {
					failed = true
				}
			}

			if failed {
				entry.Status = "Failed"
				result.Summary.FailedJobs++
				result.FailedJobs = append(result.FailedJobs, entry)
				result.Issues = append(result.Issues, JBIssue{
					Severity: "warning", Type: "job-failed",
					Resource: fmt.Sprintf("%s/%s", job.Namespace, job.Name),
					Message:  fmt.Sprintf("Job %s/%s has failed (%d succeeded, %d failed) — check pod logs", job.Namespace, job.Name, entry.Succeeded, entry.Failed),
				})
			} else if complete {
				entry.Status = "Complete"
				result.Summary.CompletedJobs++
			} else if job.Status.Active > 0 {
				entry.Status = "Running"
				result.Summary.RunningJobs++
			} else {
				entry.Status = "Pending"
			}
		}

		// Duration
		if job.Status.StartTime != nil {
			entry.StartTime = &job.Status.StartTime.Time
			endTime := now
			if job.Status.CompletionTime != nil {
				entry.CompletionTime = &job.Status.CompletionTime.Time
				endTime = job.Status.CompletionTime.Time
			}
			entry.DurationHours = endTime.Sub(job.Status.StartTime.Time).Hours()
		}

		// Long running check (>24h and still active)
		if entry.Status == "Running" && entry.DurationHours > 24 {
			result.Summary.LongRunningJobs++
			result.LongRunning = append(result.LongRunning, entry)
			result.Issues = append(result.Issues, JBIssue{
				Severity: "warning", Type: "long-running-job",
				Resource: fmt.Sprintf("%s/%s", job.Namespace, job.Name),
				Message:  fmt.Sprintf("Job %s/%s has been running for %.1f hours (>24h) — may be stuck, consider adding activeDeadlineSeconds", job.Namespace, job.Name, entry.DurationHours),
			})
		}

		// No backoffLimit
		if entry.BackoffLimit == nil {
			result.Summary.NoBackoffLimit++
		}

		// Find parent CronJob
		for _, ref := range job.OwnerReferences {
			if ref.Kind == "CronJob" {
				entry.CronJobOwner = ref.Name
				break
			}
		}

		entry.RiskLevel = jbAssessRisk(entry)
		result.ByJob = append(result.ByJob, entry)
	}

	// Sort
	sort.Slice(result.ByJob, func(i, j int) bool {
		return jbRiskRank(result.ByJob[i].RiskLevel) < jbRiskRank(result.ByJob[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return jbIssueRank(result.Issues[i].Severity) < jbIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = jbScore(result.Summary)
	result.Recommendations = jbGenRecs(result.Summary, result.FailedJobs, result.LongRunning)

	writeJSON(w, result)
}

// jbAssessRisk determines risk level.
func jbAssessRisk(entry JBEntry) string {
	if entry.Status == "Failed" {
		return "high"
	}
	if entry.Status == "Running" && entry.DurationHours > 24 {
		return "high"
	}
	if entry.Status == "Suspended" {
		return "medium"
	}
	return "low"
}

// jbScore computes health score 0-100.
func jbScore(s JBSummary) int {
	if s.TotalJobs == 0 {
		return 100
	}
	score := 100
	score -= s.FailedJobs * 10
	score -= s.LongRunningJobs * 8
	score -= s.NoBackoffLimit * 2
	if score < 0 {
		score = 0
	}
	return score
}

// jbGenRecs produces actionable advice.
func jbGenRecs(s JBSummary, failed []JBEntry, longRunning []JBEntry) []string {
	var recs []string

	if s.FailedJobs > 0 {
		recs = append(recs, fmt.Sprintf("%d job(s) have failed — check pod logs and events for failure reasons", s.FailedJobs))
	}
	if s.LongRunningJobs > 0 {
		recs = append(recs, fmt.Sprintf("%d job(s) running >24h — add activeDeadlineSeconds to prevent stuck jobs from consuming resources", s.LongRunningJobs))
	}
	if s.SuspendedJobs > 0 {
		recs = append(recs, fmt.Sprintf("%d job(s) are suspended — resume or delete if no longer needed", s.SuspendedJobs))
	}
	if s.NoBackoffLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d job(s) have no backoffLimit — set explicit limit (default: 6) to control retry behavior", s.NoBackoffLimit))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Job health score is %d/100 — review batch workload configuration", s.HealthScore))
	}
	if s.FailedJobs == 0 && s.LongRunningJobs == 0 {
		recs = append(recs, fmt.Sprintf("All %d job(s) are healthy — good batch execution posture", s.TotalJobs))
	}

	return recs
}

func jbRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func jbIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
