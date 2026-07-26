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
// v19.90 — Operations Dimension (Round 18)
// 1. Pod Grace Period — terminationGracePeriodSeconds compliance
// 2. Resource Limit Ratio — request-to-limit gap analysis
// 3. CronJob Execution Health — schedule adherence & failure tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Grace Period
// ---------------------------------------------------------------

type GracePeriodResult1990 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         GracePeriodSummary1990 `json:"summary"`
	Issues          []GracePeriodEntry1990 `json:"issues"`
	Recommendations []string               `json:"recommendations"`
}

type GracePeriodSummary1990 struct {
	TotalPods    int `json:"totalPods"`
	WithCustom   int `json:"withCustomGracePeriod"`
	DefaultGrace int `json:"usingDefault30s"`
	TooShort     int `json:"tooShortGracePeriod"`
	TooLong      int `json:"tooLongGracePeriod"`
}

type GracePeriodEntry1990 struct {
	Pod         string `json:"pod"`
	Namespace   string `json:"namespace"`
	GracePeriod int64  `json:"gracePeriodSeconds"`
	Issue       string `json:"issue"`
}

func (s *Server) handlePodGracePeriod(w http.ResponseWriter, r *http.Request) {
	result := GracePeriodResult1990{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		gp := int64(30)
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			gp = *pod.Spec.TerminationGracePeriodSeconds
			result.Summary.WithCustom++
		} else {
			result.Summary.DefaultGrace++
		}

		if gp < 5 && gp > 0 {
			result.Summary.TooShort++
			result.Issues = append(result.Issues, GracePeriodEntry1990{
				Pod: pod.Name, Namespace: pod.Namespace, GracePeriod: gp,
				Issue: fmt.Sprintf("Grace period %ds too short — may not finish cleanup", gp),
			})
			score -= 2
		}
		if gp > 300 {
			result.Summary.TooLong++
			result.Issues = append(result.Issues, GracePeriodEntry1990{
				Pod: pod.Name, Namespace: pod.Namespace, GracePeriod: gp,
				Issue: fmt.Sprintf("Grace period %ds very long — slows node drain", gp),
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d custom grace, %d default 30s, %d too short, %d too long", result.Summary.TotalPods, result.Summary.WithCustom, result.Summary.DefaultGrace, result.Summary.TooShort, result.Summary.TooLong))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Resource Limit Ratio
// ---------------------------------------------------------------

type LimitRatioResult1990 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         LimitRatioSummary1990 `json:"summary"`
	Containers      []LimitRatioEntry1990 `json:"containers"`
	Recommendations []string              `json:"recommendations"`
}

type LimitRatioSummary1990 struct {
	TotalContainers int     `json:"totalContainers"`
	WithBoth        int     `json:"withReqAndLimit"`
	AvgCPURatio     float64 `json:"avgCPURatio"`
	AvgMemRatio     float64 `json:"avgMemRatio"`
	OvercommitCPU   int     `json:"cpuOvercommitContainers"`
	OvercommitMem   int     `json:"memOvercommitContainers"`
}

type LimitRatioEntry1990 struct {
	Pod       string  `json:"pod"`
	Container string  `json:"container"`
	CPURatio  float64 `json:"cpuRatio"`
	MemRatio  float64 `json:"memRatio"`
}

func (s *Server) handleResourceLimitRatio(w http.ResponseWriter, r *http.Request) {
	result := LimitRatioResult1990{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalCPURatio, totalMemRatio float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			reqCPU := c.Resources.Requests.Cpu().AsApproximateFloat64()
			limCPU := c.Resources.Limits.Cpu().AsApproximateFloat64()
			reqMem := float64(c.Resources.Requests.Memory().Value())
			limMem := float64(c.Resources.Limits.Memory().Value())

			if reqCPU > 0 && limCPU > 0 {
				cpuRatio := reqCPU / limCPU
				totalCPURatio += cpuRatio
				count++

				if cpuRatio > 1.0 {
					result.Summary.OvercommitCPU++
					score -= 2
				}

				entry := LimitRatioEntry1990{
					Pod: pod.Name, Container: c.Name, CPURatio: cpuRatio,
				}
				if reqMem > 0 && limMem > 0 {
					memRatio := reqMem / limMem
					entry.MemRatio = memRatio
					totalMemRatio += memRatio
					if memRatio > 1.0 {
						result.Summary.OvercommitMem++
					}
				}
				result.Containers = append(result.Containers, entry)
				result.Summary.WithBoth++
			}
		}
	}

	if count > 0 {
		result.Summary.AvgCPURatio = totalCPURatio / float64(count)
		result.Summary.AvgMemRatio = totalMemRatio / float64(count)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with req+limit: avg CPU ratio %.2f, mem ratio %.2f, %d CPU overcommit", result.Summary.WithBoth, result.Summary.AvgCPURatio, result.Summary.AvgMemRatio, result.Summary.OvercommitCPU))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. CronJob Execution Health
// ---------------------------------------------------------------

type CronJobHealthResult1990 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         CronJobHealthSummary1990 `json:"summary"`
	CronJobs        []CronJobHealthEntry1990 `json:"cronJobs"`
	Recommendations []string                 `json:"recommendations"`
}

type CronJobHealthSummary1990 struct {
	TotalCronJobs        int    `json:"totalCronJobs"`
	Suspended            int    `json:"suspendedCronJobs"`
	ActiveJobs           int    `json:"activeJobs"`
	LastSchedule         string `json:"lastScheduleAge"`
	WithConcurrencyLimit int    `json:"withConcurrencyLimit"`
}

type CronJobHealthEntry1990 struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Schedule      string `json:"schedule"`
	Suspended     bool   `json:"suspended"`
	Active        int    `json:"activeJobs"`
	LastScheduled string `json:"lastScheduled"`
}

func (s *Server) handleCronJobExecHealth(w http.ResponseWriter, r *http.Request) {
	result := CronJobHealthResult1990{ScannedAt: time.Now()}
	score := 100

	cjList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})

	var newestSchedule time.Time
	for _, cj := range cjList.Items {
		result.Summary.TotalCronJobs++

		entry := CronJobHealthEntry1990{
			Name: cj.Name, Namespace: cj.Namespace,
			Schedule:  cj.Spec.Schedule,
			Suspended: cj.Spec.Suspend != nil && *cj.Spec.Suspend,
			Active:    len(cj.Status.Active),
		}

		if entry.Suspended {
			result.Summary.Suspended++
			score -= 2
		}
		if entry.Active > 0 {
			result.Summary.ActiveJobs += entry.Active
		}
		if cj.Spec.ConcurrencyPolicy != "" && cj.Spec.ConcurrencyPolicy != batchv1.AllowConcurrent {
			result.Summary.WithConcurrencyLimit++
		}

		if !cj.Status.LastScheduleTime.IsZero() {
			entry.LastScheduled = fmt.Sprintf("%.0fh ago", time.Since(cj.Status.LastScheduleTime.Time).Hours())
			if cj.Status.LastScheduleTime.Time.After(newestSchedule) {
				newestSchedule = cj.Status.LastScheduleTime.Time
			}
		} else {
			entry.LastScheduled = "never"
		}

		result.CronJobs = append(result.CronJobs, entry)
	}

	if !newestSchedule.IsZero() {
		result.Summary.LastSchedule = fmt.Sprintf("%.0fh", time.Since(newestSchedule).Hours())
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d CronJobs: %d suspended, %d active, %d with concurrency limit", result.Summary.TotalCronJobs, result.Summary.Suspended, result.Summary.ActiveJobs, result.Summary.WithConcurrencyLimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
