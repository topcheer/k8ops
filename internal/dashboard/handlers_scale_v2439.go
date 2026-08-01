package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.39 Scalability: Top Node by CPU Request, Node Pod Capacity Usage, Cluster ServiceAccount Total
type TopNodeCPUReqResult2439 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node   string  `json:"node"`
		CPUReq float64 `json:"cpuReq"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeCPUReq2439(w http.ResponseWriter, r *http.Request) {
	result := TopNodeCPUReqResult2439{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nodeCPU[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNodes = len(nodeCPU)
	for node, cpu := range nodeCPU {
		result.TopNodes = append(result.TopNodes, struct {
			Node   string  `json:"node"`
			CPUReq float64 `json:"cpuReq"`
		}{node, cpu})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].CPUReq > result.TopNodes[j].CPUReq })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodCapUsageResult2439 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		TotalCapPods int `json:"totalPodCapacity"`
		TotalPods    int `json:"totalRunningPods"`
	} `json:"summary"`
}

func (s *Server) handleNodePodCapUsage2439(w http.ResponseWriter, r *http.Request) {
	result := NodePodCapUsageResult2439{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapPods += int(node.Status.Allocatable.Pods().Value())
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SATotalResult2439 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs int            `json:"totalServiceAccounts"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleSATotal2439(w http.ResponseWriter, r *http.Request) {
	result := SATotalResult2439{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		result.Summary.ByNS[sa.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
