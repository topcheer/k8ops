package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.78 Operations: Node ContainerRuntime Version Check, Pod Phase Distribution, Container Probe Summary
type NodeRuntimeCheckResult2478 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byContainerRuntime"`
	} `json:"summary"`
}

func (s *Server) handleNodeRuntimeCheck2478(w http.ResponseWriter, r *http.Request) {
	result := NodeRuntimeCheckResult2478{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		rt := node.Status.NodeInfo.ContainerRuntimeVersion
		if rt == "" {
			rt = "<unknown>"
		}
		result.Summary.ByRuntime[rt]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodPhaseDistResult2478 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePodPhaseDist2478(w http.ResponseWriter, r *http.Request) {
	result := PodPhaseDistResult2478{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		result.Summary.ByPhase[string(pod.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ProbeSummaryResult2478 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLiveness    int `json:"withLivenessProbe"`
		WithReadiness   int `json:"withReadinessProbe"`
		WithStartup     int `json:"withStartupProbe"`
	} `json:"summary"`
}

func (s *Server) handleProbeSummary2478(w http.ResponseWriter, r *http.Request) {
	result := ProbeSummaryResult2478{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
			}
			if c.ReadinessProbe != nil {
				result.Summary.WithReadiness++
			}
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		readinessRatio := result.Summary.WithReadiness * 100 / result.Summary.TotalContainers
		if readinessRatio < score {
			score = readinessRatio
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
