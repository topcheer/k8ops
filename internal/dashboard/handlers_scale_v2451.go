package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.51 Scalability: Top Namespace by CPU Request, Node CPU Allocatable Total, Cluster ConfigMap Total
type TopNSByCPUReqResult2451 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuReqCores"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByCPUReq2451(w http.ResponseWriter, r *http.Request) {
	result := TopNSByCPUReqResult2451{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		var cpu float64
		for _, c := range pod.Spec.Containers {
			cpu += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
		nsCPU[pod.Namespace] += cpu
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns, cpu := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuReqCores"`
		}{ns, cpu})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUAllocTotalResult2451 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCPUCore float64 `json:"totalCPUAllocatableCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUAllocTotal2451(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocTotalResult2451{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPUCore += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMTotalResult2451 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs int            `json:"totalConfigMaps"`
		ByNS     map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleCMTotal2451(w http.ResponseWriter, r *http.Request) {
	result := CMTotalResult2451{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		result.Summary.ByNS[cm.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
