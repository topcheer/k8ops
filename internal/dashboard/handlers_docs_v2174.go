package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.74 — Documentation Dimension (Round 48)
// 1. Node Kubelet Version Distribution
// 2. Namespace Pod Restart Policy Catalog
// 3. PVC Volume Mode Catalog
// ============================================================

type KubeletVerDistResult2174 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int            `json:"totalNodes"`
		ByKubeletVer map[string]int `json:"byKubeletVersion"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKubeletVerDist2174(w http.ResponseWriter, r *http.Request) {
	result := KubeletVerDistResult2174{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByKubeletVer = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByKubeletVer[node.Status.NodeInfo.KubeletVersion]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. NS Restart Policy Catalog
type NSRestPolResult2174 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byRestartPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSRestPol2174(w http.ResponseWriter, r *http.Request) {
	result := NSRestPolResult2174{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPolicy = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.RestartPolicy)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Volume Mode Catalog
type PVCVolModeResult2174 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs    int            `json:"totalPVCs"`
		ByVolumeMode map[string]int `json:"byVolumeMode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCVolMode2174(w http.ResponseWriter, r *http.Request) {
	result := PVCVolModeResult2174{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByVolumeMode = make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		mode := "Filesystem"
		if pvc.Spec.VolumeMode != nil {
			mode = string(*pvc.Spec.VolumeMode)
		}
		result.Summary.ByVolumeMode[mode]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
