package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.02 Documentation: Node Label Count, Pod Volume Count, ConfigMap Data Key Size
type NodeLabelCountResult2402 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalLabels int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleNodeLabelCount2402(w http.ResponseWriter, r *http.Request) {
	result := NodeLabelCountResult2402{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalLabels += len(node.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodVolumeCountResult2402 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		TotalVolumes int `json:"totalVolumes"`
	} `json:"summary"`
}

func (s *Server) handlePodVolumeCount2402(w http.ResponseWriter, r *http.Request) {
	result := PodVolumeCountResult2402{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalVolumes += len(pod.Spec.Volumes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMDataKeySizeResult2402 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		TotalKeys int `json:"totalDataKeys"`
	} `json:"summary"`
}

func (s *Server) handleCMDataKeySize2402(w http.ResponseWriter, r *http.Request) {
	result := CMDataKeySizeResult2402{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.TotalKeys += len(cm.Data)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
