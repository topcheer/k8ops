package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.62 — Documentation Dimension (Round 46)
// 1. Node Uptime Catalog
// 2. PVC Access Mode Catalog
// 3. Pod Image Registry Distribution
// ============================================================

type NodeUptimeResult2162 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		MaxUptimeDays int `json:"maxUptimeDays"`
		MinUptimeDays int `json:"minUptimeDays"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeUptime2162(w http.ResponseWriter, r *http.Request) {
	result := NodeUptimeResult2162{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	maxD, minD := 0, 999999
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		days := int(now.Sub(node.CreationTimestamp.Time).Hours() / 24)
		if days > maxD {
			maxD = days
		}
		if days < minD {
			minD = days
		}
	}
	if minD == 999999 {
		minD = 0
	}
	result.Summary.MaxUptimeDays = maxD
	result.Summary.MinUptimeDays = minD
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Access Mode Catalog
type PVCAccessResult2162 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs    int            `json:"totalPVCs"`
		ByAccessMode map[string]int `json:"byAccessMode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCAccess2162(w http.ResponseWriter, r *http.Request) {
	result := PVCAccessResult2162{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByAccessMode = make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		for _, am := range pvc.Spec.AccessModes {
			result.Summary.ByAccessMode[string(am)]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Image Registry Distribution
type ImgRegistryResult2162 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByRegistry  map[string]int `json:"byRegistry"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgRegistry2162(w http.ResponseWriter, r *http.Request) {
	result := ImgRegistryResult2162{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByRegistry = make(map[string]int)
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			if seen[img] {
				continue
			}
			seen[img] = true
			result.Summary.TotalImages++
			reg := "docker.io"
			for i := 0; i < len(img); i++ {
				if img[i] == '/' {
					reg = img[:i]
					break
				}
			}
			result.Summary.ByRegistry[reg]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
