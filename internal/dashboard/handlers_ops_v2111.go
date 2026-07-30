package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.11 — Operations Dimension (Round 38)
// 1. Node Taint Effect Analysis
// 2. Pod Condition Health Score
// 3. Container Volume Mount Count Distribution
// ============================================================

type TaintEffResult2111 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TaintEffSummary2111 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type TaintEffSummary2111 struct {
	TotalNodes    int `json:"totalNodes"`
	NodesNoTaints int `json:"nodesWithoutTaints"`
	NodesTainted  int `json:"nodesWithTaints"`
}

func (s *Server) handleTaintEff2111(w http.ResponseWriter, r *http.Request) {
	result := TaintEffResult2111{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if len(node.Spec.Taints) > 0 {
			result.Summary.NodesTainted++
		} else {
			result.Summary.NodesNoTaints++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Condition Health
type PodCondResult2111 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodCondSummary2111 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type PodCondSummary2111 struct {
	TotalPods    int `json:"totalPods"`
	ReadyPods    int `json:"readyPods"`
	NotReadyPods int `json:"notReadyPods"`
}

func (s *Server) handlePodCond2111(w http.ResponseWriter, r *http.Request) {
	result := PodCondResult2111{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		ready := true
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				ready = false
			}
		}
		if ready {
			result.Summary.ReadyPods++
		} else {
			result.Summary.NotReadyPods++
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Volume Mount Count
type VolMountResult2111 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         VolMountSummary2111 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type VolMountSummary2111 struct {
	TotalPods      int `json:"totalPods"`
	TotalVolMounts int `json:"totalVolumeMounts"`
}

func (s *Server) handleVolMount2111(w http.ResponseWriter, r *http.Request) {
	result := VolMountResult2111{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalVolMounts += len(pod.Spec.Volumes)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
