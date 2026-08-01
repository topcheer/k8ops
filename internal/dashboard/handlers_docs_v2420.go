package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.20 Documentation: Node OS Version, Pod Spec NodeName, ConfigMap Immutable Key Count
type NodeOSVerResult2420 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		ByOSVersion map[string]int `json:"byOSVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSVer2420(w http.ResponseWriter, r *http.Request) {
	result := NodeOSVerResult2420{ScannedAt: time.Now()}
	result.Summary.ByOSVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOSVersion[node.Status.NodeInfo.OSImage]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodNodeNameResult2420 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByNode    map[string]int `json:"byNodeName"`
	} `json:"summary"`
}

func (s *Server) handlePodNodeName2420(w http.ResponseWriter, r *http.Request) {
	result := PodNodeNameResult2420{ScannedAt: time.Now()}
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

type CMImmutableKeyResult2420 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		Immutable int `json:"immutable"`
	} `json:"summary"`
}

func (s *Server) handleCMImmutableKey2420(w http.ResponseWriter, r *http.Request) {
	result := CMImmutableKeyResult2420{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if cm.Immutable != nil && *cm.Immutable {
			result.Summary.Immutable++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
