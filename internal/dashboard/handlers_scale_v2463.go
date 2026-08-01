package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.63 Scalability: Top Node by Pod Count, Node CPU Capacity Total, Cluster PV Total
type TopNodeByPodResult2463 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node     string `json:"node"`
		PodCount int    `json:"podCount"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeByPod2463(w http.ResponseWriter, r *http.Request) {
	result := TopNodeByPodResult2463{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nodePods[pod.Spec.NodeName]++
		}
	}
	result.Summary.TotalNodes = len(nodePods)
	for node, count := range nodePods {
		result.TopNodes = append(result.TopNodes, struct {
			Node     string `json:"node"`
			PodCount int    `json:"podCount"`
		}{node, count})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].PodCount > result.TopNodes[j].PodCount })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUCapTotalResult2463 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int     `json:"totalNodes"`
		TotalCPUCap float64 `json:"totalCPUCapacityCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUCapTotal2463(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUCapTotalResult2463{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPUCap += node.Status.Capacity.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVTotalResult2463 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVs int            `json:"totalPVs"`
		ByPhase  map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePVTotal2463(w http.ResponseWriter, r *http.Request) {
	result := PVTotalResult2463{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++
		result.Summary.ByPhase[string(pv.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
