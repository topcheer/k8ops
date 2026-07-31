package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.00 Documentation: Service ClusterIP Catalog, Pod Node Name Distribution, ConfigMap Key Count
type ClusterIPResult2300 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithClusterIP int `json:"withClusterIP"`
		Headless      int `json:"headless"`
	} `json:"summary"`
}

func (s *Server) handleClusterIP2300(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPResult2300{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			result.Summary.WithClusterIP++
		} else {
			result.Summary.Headless++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodNodeDistResult2300 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByNode    map[string]int `json:"byNode"`
	} `json:"summary"`
}

func (s *Server) handlePodNodeDist2300(w http.ResponseWriter, r *http.Request) {
	result := PodNodeDistResult2300{ScannedAt: time.Now()}
	result.Summary.ByNode = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByNode[pod.Spec.NodeName]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMKeyCountResult2300 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		TotalKeys int `json:"totalKeys"`
		AvgKeys   int `json:"avgKeysPerCM"`
	} `json:"summary"`
}

func (s *Server) handleCMKeyCount2300(w http.ResponseWriter, r *http.Request) {
	result := CMKeyCountResult2300{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.TotalKeys += len(cm.Data)
	}
	if result.Summary.TotalCMs > 0 {
		result.Summary.AvgKeys = result.Summary.TotalKeys / result.Summary.TotalCMs
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
