package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.47 — Deployment Dimension (Round 11)
// 1. StatefulSet Ordinal Health — pod ordinal continuity & gaps
// 2. Job Completion Tracker — batch job success/failure patterns
// 3. CronJob Schedule Overlap — concurrent schedule collision analysis
// ============================================================

// ---------------------------------------------------------------
// 1. StatefulSet Ordinal Health
// ---------------------------------------------------------------

type STSOrdinalResult1947 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         STSOrdinalSummary1947 `json:"summary"`
	StatefulSets    []STSOrdinalEntry1947 `json:"statefulSets"`
	Issues          []STSOrdinalIssue1947 `json:"issues"`
	Recommendations []string              `json:"recommendations"`
}

type STSOrdinalSummary1947 struct {
	TotalSTS   int `json:"totalStatefulSets"`
	HealthySTS int `json:"healthyStatefulSets"`
	WithGaps   int `json:"statefulSetsWithGaps"`
	ReadySTS   int `json:"readyStatefulSets"`
	TotalPods  int `json:"totalPods"`
	ReadyPods  int `json:"readyPods"`
}

type STSOrdinalEntry1947 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	ReadyReps int32  `json:"readyReplicas"`
	PodCount  int    `json:"podCount"`
	HasGaps   bool   `json:"hasOrdinalGaps"`
	Age       string `json:"age"`
}

type STSOrdinalIssue1947 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleSTSOrdinalHealth(w http.ResponseWriter, r *http.Request) {
	result := STSOrdinalResult1947{ScannedAt: time.Now()}
	score := 100

	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		if isSystemNamespace(sts.Namespace) {
			continue
		}
		result.Summary.TotalSTS++

		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		ready := sts.Status.ReadyReplicas
		current := sts.Status.CurrentReplicas

		isHealthy := ready == replicas && current == replicas
		hasGaps := current < replicas

		entry := STSOrdinalEntry1947{
			Name: sts.Name, Namespace: sts.Namespace,
			Replicas: replicas, ReadyReps: ready,
			PodCount: int(current), HasGaps: hasGaps,
			Age: fmt.Sprintf("%.0fd", time.Since(sts.CreationTimestamp.Time).Hours()/24),
		}
		result.StatefulSets = append(result.StatefulSets, entry)

		result.Summary.TotalPods += int(current)
		result.Summary.ReadyPods += int(ready)

		if isHealthy {
			result.Summary.HealthySTS++
			result.Summary.ReadySTS++
		} else {
			if hasGaps {
				result.Summary.WithGaps++
				result.Issues = append(result.Issues, STSOrdinalIssue1947{
					Name: sts.Name, Namespace: sts.Namespace,
					IssueType: "ordinal-gap", Severity: "high",
					Detail: fmt.Sprintf("Ordinal gap: %d current vs %d desired replicas", current, replicas),
				})
				score -= 5
			}
			if ready < current {
				result.Issues = append(result.Issues, STSOrdinalIssue1947{
					Name: sts.Name, Namespace: sts.Namespace,
					IssueType: "not-ready-pods", Severity: "medium",
					Detail: fmt.Sprintf("Only %d/%d pods ready", ready, current),
				})
				score -= 3
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithGaps > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StatefulSets with ordinal gaps — check pod health", result.Summary.WithGaps))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StatefulSets, %d healthy, %d/%d pods ready",
		result.Summary.TotalSTS, result.Summary.HealthySTS, result.Summary.ReadyPods, result.Summary.TotalPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Job Completion Tracker
// ---------------------------------------------------------------

type JobCompletionResult1947 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         JobCompletionSummary1947 `json:"summary"`
	Jobs            []JobCompletionEntry1947 `json:"jobs"`
	FailedJobs      []JobFailedEntry1947     `json:"failedJobs"`
	Recommendations []string                 `json:"recommendations"`
}

type JobCompletionSummary1947 struct {
	TotalJobs     int     `json:"totalJobs"`
	CompletedJobs int     `json:"completedJobs"`
	RunningJobs   int     `json:"runningJobs"`
	FailedJobs    int     `json:"failedJobs"`
	PendingJobs   int     `json:"pendingJobs"`
	SuccessRate   float64 `json:"successRate"`
	AvgDuration   string  `json:"avgDuration"`
}

type JobCompletionEntry1947 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Succeeded int32  `json:"succeeded"`
	Failed    int32  `json:"failed"`
	Age       string `json:"age"`
}

type JobFailedEntry1947 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Failed    int32  `json:"failedCount"`
	Reason    string `json:"reason"`
}

func (s *Server) handleJobCompletion(w http.ResponseWriter, r *http.Request) {
	result := JobCompletionResult1947{ScannedAt: time.Now()}
	score := 100

	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})

	var totalDuration float64
	var durationCount int

	for _, job := range jobList.Items {
		if isSystemNamespace(job.Namespace) {
			continue
		}
		result.Summary.TotalJobs++

		status := "pending"
		succeeded := job.Status.Succeeded
		failed := job.Status.Failed

		if job.Status.Failed > 0 && job.Status.Succeeded == 0 {
			status = "failed"
			result.Summary.FailedJobs++
			score -= 3
			result.FailedJobs = append(result.FailedJobs, JobFailedEntry1947{
				Name: job.Name, Namespace: job.Namespace,
				Failed: failed, Reason: "Job failed with 0 successes",
			})
		} else if succeeded > 0 {
			status = "completed"
			result.Summary.CompletedJobs++
			// Calculate duration
			if !job.Status.StartTime.IsZero() && !job.Status.CompletionTime.IsZero() {
				dur := job.Status.CompletionTime.Sub(job.Status.StartTime.Time).Minutes()
				totalDuration += dur
				durationCount++
			}
		} else if job.Status.Active > 0 {
			status = "running"
			result.Summary.RunningJobs++
		} else {
			result.Summary.PendingJobs++
		}

		entry := JobCompletionEntry1947{
			Name: job.Name, Namespace: job.Namespace,
			Status: status, Succeeded: succeeded, Failed: failed,
			Age: fmt.Sprintf("%.0fd", time.Since(job.CreationTimestamp.Time).Hours()/24),
		}
		result.Jobs = append(result.Jobs, entry)
	}

	if result.Summary.TotalJobs > 0 {
		result.Summary.SuccessRate = float64(result.Summary.CompletedJobs) * 100 / float64(result.Summary.TotalJobs)
	}
	if durationCount > 0 {
		result.Summary.AvgDuration = fmt.Sprintf("%.1fmin", totalDuration/float64(durationCount))
	}

	if result.Summary.SuccessRate < 80 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.FailedJobs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d failed jobs — check container logs and resource limits", result.Summary.FailedJobs))
	}
	if result.Summary.SuccessRate < 80 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Success rate %.0f%% — below 80%% threshold", result.Summary.SuccessRate))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d jobs (%d completed, %d failed, %.0f%% success)",
		result.Summary.TotalJobs, result.Summary.CompletedJobs, result.Summary.FailedJobs, result.Summary.SuccessRate))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. CronJob Schedule Overlap
// ---------------------------------------------------------------

type CronOverlapResult1947 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CronOverlapSummary1947 `json:"summary"`
	CronJobs        []CronOverlapEntry1947 `json:"cronJobs"`
	Collisions      []CronCollision1947    `json:"collisions"`
	Recommendations []string               `json:"recommendations"`
}

type CronOverlapSummary1947 struct {
	TotalCronJobs     int `json:"totalCronJobs"`
	SuspendedJobs     int `json:"suspendedJobs"`
	AllowConcurrent   int `json:"allowConcurrent"`
	ForbidConcurrent  int `json:"forbidConcurrent"`
	ReplaceConcurrent int `json:"replaceConcurrent"`
	CollisionCount    int `json:"collisionCount"`
	ActiveJobs        int `json:"activeJobs"`
}

type CronOverlapEntry1947 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Schedule        string `json:"schedule"`
	ConcurrencyRule string `json:"concurrencyPolicy"`
	Suspended       bool   `json:"suspended"`
	LastSchedule    string `json:"lastSchedule"`
}

type CronCollision1947 struct {
	JobA      string `json:"jobA"`
	JobB      string `json:"jobB"`
	Namespace string `json:"namespace"`
	Detail    string `json:"detail"`
}

func (s *Server) handleCronOverlap(w http.ResponseWriter, r *http.Request) {
	result := CronOverlapResult1947{ScannedAt: time.Now()}
	score := 100

	cjList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})

	// Track schedules per namespace for collision detection
	nsSchedules := make(map[string][]struct{ name, schedule string })

	for _, cj := range cjList.Items {
		if isSystemNamespace(cj.Namespace) {
			continue
		}
		result.Summary.TotalCronJobs++

		schedule := cj.Spec.Schedule
		concurrency := string(cj.Spec.ConcurrencyPolicy)
		if concurrency == "" {
			concurrency = "Allow"
		}
		suspended := cj.Spec.Suspend != nil && *cj.Spec.Suspend

		lastSchedule := "never"
		if cj.Status.LastScheduleTime != nil {
			lastSchedule = fmt.Sprintf("%.0fh ago", time.Since(cj.Status.LastScheduleTime.Time).Hours())
		}

		entry := CronOverlapEntry1947{
			Name: cj.Name, Namespace: cj.Namespace,
			Schedule: schedule, ConcurrencyRule: concurrency,
			Suspended: suspended, LastSchedule: lastSchedule,
		}
		result.CronJobs = append(result.CronJobs, entry)

		if suspended {
			result.Summary.SuspendedJobs++
		} else {
			result.Summary.ActiveJobs++
		}

		switch concurrency {
		case "Forbid":
			result.Summary.ForbidConcurrent++
		case "Replace":
			result.Summary.ReplaceConcurrent++
		default:
			result.Summary.AllowConcurrent++
		}

		// Track for collision detection
		nsSchedules[cj.Namespace] = append(nsSchedules[cj.Namespace], struct{ name, schedule string }{cj.Name, schedule})
	}

	// Detect schedule collisions: same schedule in same namespace
	for ns, jobs := range nsSchedules {
		for i := 0; i < len(jobs); i++ {
			for j := i + 1; j < len(jobs); j++ {
				if jobs[i].schedule == jobs[j].schedule {
					result.Summary.CollisionCount++
					result.Collisions = append(result.Collisions, CronCollision1947{
						JobA: jobs[i].name, JobB: jobs[j].name, Namespace: ns,
						Detail: fmt.Sprintf("Same schedule '%s' — potential resource contention", jobs[i].schedule),
					})
					score -= 2
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.AllowConcurrent > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d CronJobs allow concurrent runs — set Forbid for safety", result.Summary.AllowConcurrent))
	}
	if result.Summary.CollisionCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d schedule collisions — stagger execution times", result.Summary.CollisionCount))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d CronJobs (%d active, %d suspended)", result.Summary.TotalCronJobs, result.Summary.ActiveJobs, result.Summary.SuspendedJobs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// Suppress unused import
var _ appsv1.StatefulSet = appsv1.StatefulSet{}
