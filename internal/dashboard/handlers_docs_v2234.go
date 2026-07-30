package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.34 — Documentation Dimension (Round 58)
// 1. Node Container Runtime ID Hash Catalog
// 2. ConfigMap Env From Distribution
// 3. Pod Scheduler Hint Catalog
// ============================================================

type CRHashResult2234 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCRHash   map[string]int `json:"byContainerRuntimeHash"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRHash2234(w http.ResponseWriter, r *http.Request) {
	result := CRHashResult2234{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByCRHash = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cr := node.Status.NodeInfo.ContainerRuntimeVersion
		if len(cr) > 15 {
			cr = cr[:15]
		}
		result.Summary.ByCRHash[cr]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. ConfigMap Env From Distribution
type CMEnvFromResult2234 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithEnvFrom int `json:"withConfigMapEnvFrom"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMEnvFrom2234(w http.ResponseWriter, r *http.Request) {
	result := CMEnvFromResult2234{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					result.Summary.WithEnvFrom++
					goto nextPod
				}
			}
		}
	nextPod:
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Scheduler Hint Catalog
type SchedHintResult2234 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithHints int `json:"withSchedulerHints"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSchedHint2234(w http.ResponseWriter, r *http.Request) {
	result := SchedHintResult2234{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.SchedulingGates) > 0 {
			result.Summary.WithHints++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
