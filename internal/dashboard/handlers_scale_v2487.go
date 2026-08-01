package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.87 Scalability: Top Node by CPU Limit, Node Memory Capacity Total, Cluster EndpointSlice Endpoint Total
type TopNodeCPULimitResult2487 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node     string  `json:"node"`
		CPULimit float64 `json:"cpuLimitCores"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeCPULimit2487(w http.ResponseWriter, r *http.Request) {
	result := TopNodeCPULimitResult2487{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		var cpu float64
		for _, c := range pod.Spec.Containers {
			cpu += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
		nodeCPU[pod.Spec.NodeName] += cpu
	}
	result.Summary.TotalNodes = len(nodeCPU)
	for node, cpu := range nodeCPU {
		result.TopNodes = append(result.TopNodes, struct {
			Node     string  `json:"node"`
			CPULimit float64 `json:"cpuLimitCores"`
		}{node, cpu})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].CPULimit > result.TopNodes[j].CPULimit })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemCapTotalResult2487 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCapGB float64 `json:"totalMemCapacityGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemCapTotal2487(w http.ResponseWriter, r *http.Request) {
	result := NodeMemCapTotalResult2487{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPSliceEndpointTotalResult2487 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices int `json:"totalEndpointSlices"`
		TotalEps    int `json:"totalEndpoints"`
		ReadyEps    int `json:"readyEndpoints"`
	} `json:"summary"`
}

func (s *Server) handleEPSliceEndpointTotal2487(w http.ResponseWriter, r *http.Request) {
	result := EPSliceEndpointTotalResult2487{ScannedAt: time.Now()}
	sliceList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, slice := range sliceList.Items {
		result.Summary.TotalSlices++
		for _, ep := range slice.Endpoints {
			result.Summary.TotalEps++
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				result.Summary.ReadyEps++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
