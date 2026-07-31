package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.40 — Documentation Dimension (Round 59)
// 1. Node Kubelet Version vs Proxy Version Match
// 2. PVC Finalizer Catalog
// 3. Pod Deletion Grace Period Catalog
// ============================================================

type KubeletProxyMatchResult2240 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		Matched    int `json:"kubeletProxyVersionMatch"`
		Mismatched int `json:"versionMismatch"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKubeletProxyMatch2240(w http.ResponseWriter, r *http.Request) {
	result := KubeletProxyMatchResult2240{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Status.NodeInfo.KubeletVersion == node.Status.NodeInfo.KubeProxyVersion {
			result.Summary.Matched++
		} else {
			result.Summary.Mismatched++
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

// 2. PVC Finalizer Catalog
type PVCFinalizerResult2240 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs     int            `json:"totalPVCs"`
		WithFinalizer int            `json:"withFinalizer"`
		ByFinalizer   map[string]int `json:"byFinalizer"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCFinalizer2240(w http.ResponseWriter, r *http.Request) {
	result := PVCFinalizerResult2240{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByFinalizer = make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if len(pvc.Finalizers) > 0 {
			result.Summary.WithFinalizer++
			for _, f := range pvc.Finalizers {
				result.Summary.ByFinalizer[f]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Deletion Grace Period
type DelGraceResult2240 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithCustom   int `json:"withCustomDeletionGrace"`
		DefaultGrace int `json:"usingDefaultGrace"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDelGrace2240(w http.ResponseWriter, r *http.Request) {
	result := DelGraceResult2240{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.TerminationGracePeriodSeconds != nil && *pod.Spec.TerminationGracePeriodSeconds != 30 {
			result.Summary.WithCustom++
		} else {
			result.Summary.DefaultGrace++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
