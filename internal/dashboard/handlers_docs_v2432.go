package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.32 Documentation: Node Taint Effect, Pod Container Args Count, PV Status Phase
type TaintEffectResult2432 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByEffect   map[string]int `json:"byTaintEffect"`
	} `json:"summary"`
}

func (s *Server) handleTaintEffect2432(w http.ResponseWriter, r *http.Request) {
	result := TaintEffectResult2432{ScannedAt: time.Now()}
	result.Summary.ByEffect = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, taint := range node.Spec.Taints {
			result.Summary.ByEffect[string(taint.Effect)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrArgsCountResult2432 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalArgs       int `json:"totalArgs"`
	} `json:"summary"`
}

func (s *Server) handleCtnrArgsCount2432(w http.ResponseWriter, r *http.Request) {
	result := CtnrArgsCountResult2432{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalArgs += len(c.Args)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVStatusResult2432 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs int            `json:"totalPVs"`
		ByPhase  map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePVStatus2432(w http.ResponseWriter, r *http.Request) {
	result := PVStatusResult2432{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		result.Summary.ByPhase[string(pv.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
