package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.78 Documentation: Node ContainerRuntime Version, Pod UID, ConfigMap Age Distribution
type NodeCRTVerResult2378 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCRVer    map[string]int `json:"byContainerRuntimeVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeCRTVer2378(w http.ResponseWriter, r *http.Request) {
	result := NodeCRTVerResult2378{ScannedAt: time.Now()}
	result.Summary.ByCRVer = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByCRVer[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodUIDResult2378 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithUID   int `json:"withUID"`
	} `json:"summary"`
}

func (s *Server) handlePodUID2378(w http.ResponseWriter, r *http.Request) {
	result := PodUIDResult2378{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.UID != "" {
			result.Summary.WithUID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMAgeResult2378 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int            `json:"totalConfigMaps"`
		ByAgeBucket map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleCMAge2378(w http.ResponseWriter, r *http.Request) {
	result := CMAgeResult2378{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		age := now.Sub(cm.CreationTimestamp.Time)
		var b string
		if age < 7*24*time.Hour {
			b = "<7d"
		} else if age < 30*24*time.Hour {
			b = "7-30d"
		} else if age < 90*24*time.Hour {
			b = "30-90d"
		} else {
			b = "90d+"
		}
		result.Summary.ByAgeBucket[b]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
