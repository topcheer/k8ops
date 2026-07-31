package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.60 Documentation: Node Hostnam Audit, Pod Container ImagePullPolicy, PV Reclaim Policy
type NodeHostnameResult2360 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByHostname map[string]int `json:"byHostname"`
	} `json:"summary"`
}

func (s *Server) handleNodeHostname2360(w http.ResponseWriter, r *http.Request) {
	result := NodeHostnameResult2360{ScannedAt: time.Now()}
	result.Summary.ByHostname = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByHostname[node.Name]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImgPullPolResult2360 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPolicy        map[string]int `json:"byImagePullPolicy"`
	} `json:"summary"`
}

func (s *Server) handleImgPullPol2360(w http.ResponseWriter, r *http.Request) {
	result := ImgPullPolResult2360{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.ByPolicy[string(c.ImagePullPolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVReclaimResult2360 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs int            `json:"totalPVs"`
		ByPolicy map[string]int `json:"byReclaimPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePVReclaim2360(w http.ResponseWriter, r *http.Request) {
	result := PVReclaimResult2360{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		result.Summary.ByPolicy[string(pv.Spec.PersistentVolumeReclaimPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
