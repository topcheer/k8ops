package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.66 Product: Pod Priority Distribution, Container Probe Coverage, Image Pull Policy Census
type PodPriorityResult2266 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int            `json:"totalPods"`
		WithPriority    int            `json:"withPriorityClass"`
		ByPriorityClass map[string]int `json:"byPriorityClass"`
	} `json:"summary"`
}

func (s *Server) handlePodPriority2266(w http.ResponseWriter, r *http.Request) {
	result := PodPriorityResult2266{ScannedAt: time.Now()}
	result.Summary.ByPriorityClass = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.PriorityClassName != "" {
			result.Summary.WithPriority++
			result.Summary.ByPriorityClass[pod.Spec.PriorityClassName]++
		} else {
			result.Summary.ByPriorityClass["none"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ProbeCoverageResult2266 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithReadiness   int `json:"withReadinessProbe"`
		WithLiveness    int `json:"withLivenessProbe"`
		WithStartup     int `json:"withStartupProbe"`
	} `json:"summary"`
}

func (s *Server) handleProbeCoverage2266(w http.ResponseWriter, r *http.Request) {
	result := ProbeCoverageResult2266{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.ReadinessProbe != nil {
				result.Summary.WithReadiness++
			}
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
			}
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		readinessPct := result.Summary.WithReadiness * 100 / result.Summary.TotalContainers
		livenessPct := result.Summary.WithLiveness * 100 / result.Summary.TotalContainers
		score = (readinessPct + livenessPct) / 2
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PullPolicyResult2266 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPullPolicy    map[string]int `json:"byPullPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePullPolicy2266(w http.ResponseWriter, r *http.Request) {
	result := PullPolicyResult2266{ScannedAt: time.Now()}
	result.Summary.ByPullPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.ByPullPolicy[string(c.ImagePullPolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
