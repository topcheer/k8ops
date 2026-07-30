package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.16 — Documentation Dimension (Round 55)
// 1. Node SystemUUID Catalog
// 2. ConfigMap Mount Path Distribution
// 3. Pod Annotations Key Catalog
// ============================================================

type SysUUIDResult2216 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		UniqueUUIDs int `json:"uniqueSystemUUIDs"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSysUUID2216(w http.ResponseWriter, r *http.Request) {
	result := SysUUIDResult2216{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	uuidSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		uuidSet[node.Status.NodeInfo.SystemUUID] = true
	}
	result.Summary.UniqueUUIDs = len(uuidSet)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CM Mount Path Distribution
type CMMountPathResult2216 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs      int            `json:"totalConfigMaps"`
		TopMountPaths map[string]int `json:"topMountPaths"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMMountPath2216(w http.ResponseWriter, r *http.Request) {
	result := CMMountPathResult2216{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalCMs = len(cmList.Items)
	result.Summary.TopMountPaths = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, vm := range c.VolumeMounts {
				result.Summary.TopMountPaths[vm.MountPath]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Annotations Key Catalog
type PodAnnotKeyResult2216 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByKey     map[string]int `json:"byAnnotationKey"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePodAnnotKey2216(w http.ResponseWriter, r *http.Request) {
	result := PodAnnotKeyResult2216{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByKey = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for k := range pod.Annotations {
			result.Summary.ByKey[k]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
