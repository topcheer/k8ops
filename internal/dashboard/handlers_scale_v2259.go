package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v22.59 Scalability: NS CPU Request Distribution, Node Memory Fragmentation, Cluster Service Health
type NSCPUReqResult2259 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuReq"`
	} `json:"topNS"`
}

func (s *Server) handleNSCPUReq2259(w http.ResponseWriter, r *http.Request) {
	result := NSCPUReqResult2259{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns, cpu := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuReq"`
		}{ns, cpu})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemFragResult2259 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalAllocGB float64 `json:"totalAllocGB"`
		TotalReqGB   float64 `json:"totalReqGB"`
		FragPct      int     `json:"fragPct"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemFrag2259(w http.ResponseWriter, r *http.Request) {
	result := NodeMemFragResult2259{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.TotalAllocGB > 0 {
		result.Summary.FragPct = int((1 - result.Summary.TotalReqGB/result.Summary.TotalAllocGB) * 100)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcHealthResult2259 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		Healthy       int `json:"healthy"`
	} `json:"summary"`
}

func (s *Server) handleSvcHealth2259(w http.ResponseWriter, r *http.Request) {
	result := SvcHealthResult2259{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	epSet := make(map[string]bool)
	for _, ep := range epList.Items {
		totalAddr := 0
		for _, sub := range ep.Subsets {
			totalAddr += len(sub.Addresses)
		}
		if totalAddr > 0 {
			epSet[ep.Namespace+"/"+ep.Name] = true
		}
	}
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if epSet[svc.Namespace+"/"+svc.Name] {
			result.Summary.Healthy++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
