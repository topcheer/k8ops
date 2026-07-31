package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v22.65 Scalability: NS Memory Request Distribution, Pod Density per Node, Cluster Endpoint Count
type NSMemReqResult2265 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		MemReqGB  float64 `json:"memReqGB"`
	} `json:"topNS"`
}

func (s *Server) handleNSMemReq2265(w http.ResponseWriter, r *http.Request) {
	result := NSMemReqResult2265{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsMem[pod.Namespace] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsMem)
	for ns, mem := range nsMem {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			MemReqGB  float64 `json:"memReqGB"`
		}{ns, mem})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReqGB > result.TopNS[j].MemReqGB })
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodDensityPerNodeResult2265 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalPods  int `json:"totalPods"`
		AvgPerNode int `json:"avgPerNode"`
	} `json:"summary"`
	TopNodes []struct {
		Node     string `json:"node"`
		PodCount int    `json:"podCount"`
	} `json:"topNodes"`
}

func (s *Server) handlePodDensityPerNode2265(w http.ResponseWriter, r *http.Request) {
	result := PodDensityPerNodeResult2265{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nodePods[pod.Spec.NodeName]++
		result.Summary.TotalPods++
	}
	result.Summary.TotalNodes = len(nodePods)
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}
	for node, count := range nodePods {
		result.TopNodes = append(result.TopNodes, struct {
			Node     string `json:"node"`
			PodCount int    `json:"podCount"`
		}{node, count})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].PodCount > result.TopNodes[j].PodCount })
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EndpointCountResult2265 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices  int `json:"totalServices"`
		TotalEndpoints int `json:"totalEndpoints"`
		WithEndpoints  int `json:"withEndpoints"`
	} `json:"summary"`
}

func (s *Server) handleEndpointCount2265(w http.ResponseWriter, r *http.Request) {
	result := EndpointCountResult2265{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	epAddrCount := make(map[string]int)
	for _, ep := range epList.Items {
		total := 0
		for _, sub := range ep.Subsets {
			total += len(sub.Addresses)
		}
		epAddrCount[ep.Namespace+"/"+ep.Name] = total
		result.Summary.TotalEndpoints += total
		if total > 0 {
			result.Summary.WithEndpoints++
		}
	}
	result.Summary.TotalServices = len(svcList.Items)
	_ = epAddrCount
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
