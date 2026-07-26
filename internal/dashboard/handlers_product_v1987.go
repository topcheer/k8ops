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

// ============================================================
// v19.87 — Product Dimension (Round 17)
// 1. StatefulSet Health — ordering & readiness compliance
// 2. DaemonSet Coverage — node coverage gap analysis
// 3. Job Completion Rate — batch job success/failure tracking
// ============================================================

// ---------------------------------------------------------------
// 1. StatefulSet Health
// ---------------------------------------------------------------

type StsHealthResult1987 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         StsHealthSummary1987 `json:"summary"`
	Issues          []StsHealthEntry1987 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type StsHealthSummary1987 struct {
	TotalSTS       int `json:"totalStatefulSets"`
	FullyReady     int `json:"fullyReady"`
	PartiallyReady int `json:"partiallyReady"`
	NotReady       int `json:"notReady"`
	ZeroReplicas   int `json:"zeroReplicas"`
}

type StsHealthEntry1987 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Desired   int    `json:"desiredReplicas"`
	Ready     int    `json:"readyReplicas"`
	Status    string `json:"status"`
}

func (s *Server) handleStatefulSetHealthV2(w http.ResponseWriter, r *http.Request) {
	result := StsHealthResult1987{ScannedAt: time.Now()}
	score := 100

	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++

		desired := 1
		if sts.Spec.Replicas != nil {
			desired = int(*sts.Spec.Replicas)
		}
		ready := int(sts.Status.ReadyReplicas)

		entry := StsHealthEntry1987{
			Name: sts.Name, Namespace: sts.Namespace,
			Desired: desired, Ready: ready,
		}

		if desired == 0 {
			entry.Status = "zero"
			result.Summary.ZeroReplicas++
		} else if ready == desired {
			entry.Status = "ready"
			result.Summary.FullyReady++
		} else if ready > 0 {
			entry.Status = "partial"
			result.Summary.PartiallyReady++
			result.Issues = append(result.Issues, entry)
			score -= 3
		} else {
			entry.Status = "not-ready"
			result.Summary.NotReady++
			result.Issues = append(result.Issues, entry)
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d STS: %d ready, %d partial, %d not-ready", result.Summary.TotalSTS, result.Summary.FullyReady, result.Summary.PartiallyReady, result.Summary.NotReady))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. DaemonSet Coverage
// ---------------------------------------------------------------

type DSCoverageResult1987 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         DSCoverageSummary1987 `json:"summary"`
	Issues          []DSCoverageEntry1987 `json:"issues"`
	Recommendations []string              `json:"recommendations"`
}

type DSCoverageSummary1987 struct {
	TotalDS            int `json:"totalDaemonSets"`
	FullyScheduled     int `json:"fullyScheduled"`
	PartiallyScheduled int `json:"partiallyScheduled"`
	NotScheduled       int `json:"notScheduled"`
	TotalNodes         int `json:"totalNodes"`
}

type DSCoverageEntry1987 struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	DesiredScheduled int    `json:"desiredScheduled"`
	CurrentScheduled int    `json:"currentScheduled"`
	Ready            int    `json:"numberReady"`
}

func (s *Server) handleDaemonSetCoverage(w http.ResponseWriter, r *http.Request) {
	result := DSCoverageResult1987{ScannedAt: time.Now()}
	score := 100

	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNodes = len(nodeList.Items)

	for _, ds := range dsList.Items {
		result.Summary.TotalDS++

		desired := int(ds.Status.DesiredNumberScheduled)
		current := int(ds.Status.CurrentNumberScheduled)
		ready := int(ds.Status.NumberReady)

		entry := DSCoverageEntry1987{
			Name: ds.Name, Namespace: ds.Namespace,
			DesiredScheduled: desired, CurrentScheduled: current, Ready: ready,
		}

		if desired > 0 && current == desired && ready >= desired {
			result.Summary.FullyScheduled++
		} else if current > 0 {
			result.Summary.PartiallyScheduled++
			result.Issues = append(result.Issues, entry)
			score -= 3
		} else {
			result.Summary.NotScheduled++
			result.Issues = append(result.Issues, entry)
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d DS across %d nodes: %d full, %d partial, %d not-scheduled", result.Summary.TotalDS, result.Summary.TotalNodes, result.Summary.FullyScheduled, result.Summary.PartiallyScheduled, result.Summary.NotScheduled))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Job Completion Rate
// ---------------------------------------------------------------

type JobCompResult1987 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         JobCompSummary1987 `json:"summary"`
	Jobs            []JobCompEntry1987 `json:"jobs"`
	Recommendations []string           `json:"recommendations"`
}

type JobCompSummary1987 struct {
	TotalJobs      int     `json:"totalJobs"`
	Succeeded      int     `json:"succeeded"`
	Failed         int     `json:"failed"`
	Running        int     `json:"running"`
	Pending        int     `json:"pending"`
	CompletionRate float64 `json:"completionRatePct"`
}

type JobCompEntry1987 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
}

func (s *Server) handleJobCompletionRate(w http.ResponseWriter, r *http.Request) {
	result := JobCompResult1987{ScannedAt: time.Now()}
	score := 100

	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})

	completed := 0
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++

		entry := JobCompEntry1987{
			Name: job.Name, Namespace: job.Namespace,
		}

		if job.Status.Succeeded > 0 && job.Status.Failed == 0 {
			entry.Status = "succeeded"
			entry.Succeeded = int(job.Status.Succeeded)
			result.Summary.Succeeded++
			completed++
		} else if job.Status.Failed > 0 {
			entry.Status = "failed"
			entry.Failed = int(job.Status.Failed)
			result.Summary.Failed++
			completed++
			score -= 3
		} else if job.Status.Active > 0 {
			entry.Status = "running"
			result.Summary.Running++
		} else {
			entry.Status = "pending"
			result.Summary.Pending++
		}

		// Check for completion
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				entry.Status = "complete"
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				entry.Status = "failed"
			}
		}

		result.Jobs = append(result.Jobs, entry)
	}

	if result.Summary.TotalJobs > 0 {
		result.Summary.CompletionRate = float64(completed) / float64(result.Summary.TotalJobs) * 100
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d jobs: %d succeeded, %d failed, %d running, %d pending (%.0f%% completion)", result.Summary.TotalJobs, result.Summary.Succeeded, result.Summary.Failed, result.Summary.Running, result.Summary.Pending, result.Summary.CompletionRate))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
