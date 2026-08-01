package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.38 Documentation: Node Zone Topology, Pod Finalizer List, ConfigMap Annotation Count
type NodeZoneResult2438 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByZone     map[string]int `json:"byTopologyZone"`
	} `json:"summary"`
}

func (s *Server) handleNodeZone2438(w http.ResponseWriter, r *http.Request) {
	result := NodeZoneResult2438{ScannedAt: time.Now()}
	result.Summary.ByZone = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		zone := node.Labels[corev1.LabelTopologyZone]
		if zone == "" {
			zone = "<unknown>"
		}
		result.Summary.ByZone[zone]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodFinalizerListResult2438 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int `json:"totalPods"`
		TotalFinalizers int `json:"totalFinalizers"`
	} `json:"summary"`
}

func (s *Server) handlePodFinalizerList2438(w http.ResponseWriter, r *http.Request) {
	result := PodFinalizerListResult2438{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalFinalizers += len(pod.Finalizers)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMAnnotCountResult2438 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int `json:"totalConfigMaps"`
		TotalAnnots int `json:"totalAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleCMAnnotCount2438(w http.ResponseWriter, r *http.Request) {
	result := CMAnnotCountResult2438{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.TotalAnnots += len(cm.Annotations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
