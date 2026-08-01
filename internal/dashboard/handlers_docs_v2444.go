package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.44 Documentation: Node InstanceType Distribution, Pod Priority Distribution, EndpointSlice Count
type NodeInstanceTypeResult2444 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int            `json:"totalNodes"`
		ByInstanceType map[string]int `json:"byInstanceType"`
	} `json:"summary"`
}

func (s *Server) handleNodeInstanceType2444(w http.ResponseWriter, r *http.Request) {
	result := NodeInstanceTypeResult2444{ScannedAt: time.Now()}
	result.Summary.ByInstanceType = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		it := node.Labels[corev1.LabelInstanceType]
		if it == "" {
			it = "<unknown>"
		}
		result.Summary.ByInstanceType[it]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodPriorityDistResult2444 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByPriority map[string]int `json:"byPriorityClass"`
	} `json:"summary"`
}

func (s *Server) handlePodPriorityDist2444(w http.ResponseWriter, r *http.Request) {
	result := PodPriorityDistResult2444{ScannedAt: time.Now()}
	result.Summary.ByPriority = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "<none>"
		}
		result.Summary.ByPriority[pc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EndpointSliceCountResult2444 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices    int `json:"totalEndpointSlices"`
		TotalEndpoints int `json:"totalEndpoints"`
	} `json:"summary"`
}

func (s *Server) handleEndpointSliceCount2444(w http.ResponseWriter, r *http.Request) {
	result := EndpointSliceCountResult2444{ScannedAt: time.Now()}
	sliceList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, slice := range sliceList.Items {
		result.Summary.TotalSlices++
		result.Summary.TotalEndpoints += len(slice.Endpoints)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
