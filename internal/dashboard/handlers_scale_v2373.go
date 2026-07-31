package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.73 Scalability: Top Node by Pod, Namespace HPA Coverage, Cluster Endpoint Service Ratio
type TopNodePodResult2373 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node string `json:"node"`
		Pods int    `json:"podCount"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodePod2373(w http.ResponseWriter, r *http.Request) {
	result := TopNodePodResult2373{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nodePods[pod.Spec.NodeName]++
		}
	}
	result.Summary.TotalNodes = len(nodePods)
	for node, pods := range nodePods {
		result.TopNodes = append(result.TopNodes, struct {
			Node string `json:"node"`
			Pods int    `json:"podCount"`
		}{node, pods})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].Pods > result.TopNodes[j].Pods })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSHPACoverageResult2373 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNS"`
		ByNS    map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNSHPACoverage2373(w http.ResponseWriter, r *http.Request) {
	result := NSHPACoverageResult2373{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	for _, hpa := range hpaList.Items {
		result.Summary.ByNS[hpa.Namespace]++
		result.Summary.TotalNS++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPSvcRatioResult2373 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices  int `json:"totalServices"`
		TotalEndpoints int `json:"totalEndpoints"`
		Ratio          int `json:"endpointServiceRatio"`
	} `json:"summary"`
}

func (s *Server) handleEPSvcRatio2373(w http.ResponseWriter, r *http.Request) {
	result := EPSvcRatioResult2373{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalServices = len(svcList.Items)
	result.Summary.TotalEndpoints = len(epList.Items)
	if result.Summary.TotalServices > 0 {
		result.Summary.Ratio = result.Summary.TotalEndpoints * 100 / result.Summary.TotalServices
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
