package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.92 — Documentation Dimension (Round 51)
// 1. Node OS Architecture Catalog
// 2. ConfigMap Binary Data Tracker
// 3. Pod Image ID Reference Catalog
// ============================================================

type NodeOSArchResult2192 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSArch   map[string]int `json:"byOSArchitecture"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeOSArch2192(w http.ResponseWriter, r *http.Request) {
	result := NodeOSArchResult2192{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByOSArch = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOSArch[node.Status.NodeInfo.OperatingSystem+"/"+node.Status.NodeInfo.Architecture]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CM Binary Data Tracker
type CMBinaryResult2192 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs        int `json:"totalConfigMaps"`
		WithBinaryData  int `json:"withBinaryData"`
		TotalBinaryKeys int `json:"totalBinaryKeys"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMBinary2192(w http.ResponseWriter, r *http.Request) {
	result := CMBinaryResult2192{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if len(cm.BinaryData) > 0 {
			result.Summary.WithBinaryData++
			result.Summary.TotalBinaryKeys += len(cm.BinaryData)
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Image ID Reference
type ImgIDRefResult2192 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithImageID int `json:"withImageID"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgIDRef2192(w http.ResponseWriter, r *http.Request) {
	result := ImgIDRefResult2192{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.ImageID != "" {
				result.Summary.WithImageID++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
