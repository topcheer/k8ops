package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.84 Documentation: Node KubeProxyVer, Pod Container Image Size, PVC Access Mode
type KPVerResult2384 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	} `json:"summary"`
}

func (s *Server) handleKPVer2384(w http.ResponseWriter, r *http.Request) {
	result := KPVerResult2384{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVersion[node.Status.NodeInfo.KubeProxyVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImgSizeResult2384 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages     int `json:"totalImages"`
		TotalContainers int `json:"totalContainers"`
	} `json:"summary"`
}

func (s *Server) handleImgSize2384(w http.ResponseWriter, r *http.Request) {
	result := ImgSizeResult2384{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.TotalImages++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCAccessModeResult2384 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByMode    map[string]int `json:"byAccessMode"`
	} `json:"summary"`
}

func (s *Server) handlePVCAccessMode2384(w http.ResponseWriter, r *http.Request) {
	result := PVCAccessModeResult2384{ScannedAt: time.Now()}
	result.Summary.ByMode = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		for _, am := range pvc.Spec.AccessModes {
			result.Summary.ByMode[string(am)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
