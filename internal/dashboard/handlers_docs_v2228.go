package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.28 — Documentation Dimension (Round 57)
// 1. Node OS Kernel Boot Time Catalog
// 2. ConfigMap Namespace Distribution
// 3. Pod Owner Reference API Version Catalog
// ============================================================

type KernelBootResult2228 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int            `json:"totalNodes"`
		ByKernelBoot map[string]int `json:"byKernelBootTime"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKernelBoot2228(w http.ResponseWriter, r *http.Request) {
	result := KernelBootResult2228{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByKernelBoot = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				bootDay := cond.LastHeartbeatTime.Format("2006-01-02")
				result.Summary.ByKernelBoot[bootDay]++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CM Namespace Distribution
type CMNSDistResult2228 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int            `json:"totalConfigMaps"`
		ByNamespace map[string]int `json:"byNamespace"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMNSDist2228(w http.ResponseWriter, r *http.Request) {
	result := CMNSDistResult2228{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByNamespace = make(map[string]int)
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.ByNamespace[cm.Namespace]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Owner Ref API Version
type OwnerRefAPIVerResult2228 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int            `json:"totalPods"`
		ByAPIVersion map[string]int `json:"byOwnerAPIVersion"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOwnerRefAPIVer2228(w http.ResponseWriter, r *http.Request) {
	result := OwnerRefAPIVerResult2228{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByAPIVersion = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.OwnerReferences) > 0 {
			result.Summary.ByAPIVersion[pod.OwnerReferences[0].APIVersion]++
		} else {
			result.Summary.ByAPIVersion["standalone"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
