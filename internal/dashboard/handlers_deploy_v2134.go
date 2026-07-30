package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.34 — Deployment Dimension (Round 42)
// 1. Pod Scheduler Name Audit
// 2. Container StdinTTY Consistency
// 3. Deployment Replicas Min Max Range
// ============================================================

type SchedulerNameResult2134 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         SchedulerNameSummary2134 `json:"summary"`
	Recommendations []string                 `json:"recommendations"`
}

type SchedulerNameSummary2134 struct {
	TotalPods   int            `json:"totalPods"`
	ByScheduler map[string]int `json:"byScheduler"`
}

func (s *Server) handleSchedulerName2134(w http.ResponseWriter, r *http.Request) {
	result := SchedulerNameResult2134{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	bySched := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		name := pod.Spec.SchedulerName
		if name == "" {
			name = "default-scheduler"
		}
		bySched[name]++
	}
	result.Summary.ByScheduler = bySched
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. StdinTTY Consistency
type StdinTTYResult2134 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         StdinTTYSummary2134 `json:"summary"`
	Issues          []StdinTTYEntry2134 `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type StdinTTYSummary2134 struct {
	TotalContainers int `json:"totalContainers"`
	WithTTY         int `json:"withTTY"`
	WithStdin       int `json:"withStdin"`
}

type StdinTTYEntry2134 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleStdinTTY2134(w http.ResponseWriter, r *http.Request) {
	result := StdinTTYResult2134{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.TTY {
				result.Summary.WithTTY++
			}
			if c.Stdin {
				result.Summary.WithStdin++
				result.Issues = append(result.Issues, StdinTTYEntry2134{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithStdin > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with stdin enabled", result.Summary.WithStdin))
	}
	writeJSON(w, result)
}

// 3. Replicas Min Max Range
type ReplicaRangeResult2134 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ReplicaRangeSummary2134 `json:"summary"`
	HighReplica     []ReplicaRangeEntry2134 `json:"highReplicaDeployments"`
	Recommendations []string                `json:"recommendations"`
}

type ReplicaRangeSummary2134 struct {
	TotalDeploys int   `json:"totalDeployments"`
	MaxReplicas  int32 `json:"maxReplicas"`
	MinReplicas  int32 `json:"minReplicas"`
}

type ReplicaRangeEntry2134 struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func (s *Server) handleReplicaRange2134(w http.ResponseWriter, r *http.Request) {
	result := ReplicaRangeResult2134{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	maxR := int32(0)
	minR := int32(999999)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas > maxR {
			maxR = replicas
		}
		if replicas < minR {
			minR = replicas
		}
		if replicas > 20 {
			result.HighReplica = append(result.HighReplica, ReplicaRangeEntry2134{Name: dep.Name, Replicas: replicas})
		}
	}
	if minR == 999999 {
		minR = 0
	}
	result.Summary.MaxReplicas = maxR
	result.Summary.MinReplicas = minR
	sort.Slice(result.HighReplica, func(i, j int) bool { return result.HighReplica[i].Replicas > result.HighReplica[j].Replicas })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
