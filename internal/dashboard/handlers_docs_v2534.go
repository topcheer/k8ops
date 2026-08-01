package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strconv"
	"time"
)

// v25.34 Documentation: Node KernelVersion Detail, Pod Spec Priority Class Detail, Namespace ResourceVersion Count
type NodeKernelDetailResult2534 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKernelDetail2534(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelDetailResult2534{ScannedAt: time.Now()}
	result.Summary.ByKernel = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KernelVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByKernel[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodPriorityDetailResult2534 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByPriority map[string]int `json:"byPriorityClassName"`
	} `json:"summary"`
}

func (s *Server) handlePodPriorityDetail2534(w http.ResponseWriter, r *http.Request) {
	result := PodPriorityDetailResult2534{ScannedAt: time.Now()}
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

type NSResourceVersionResult2534 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
		TotalRV int `json:"totalResourceVersions"`
	} `json:"summary"`
}

func (s *Server) handleNSResourceVersion2534(w http.ResponseWriter, r *http.Request) {
	result := NSResourceVersionResult2534{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if rv, err := strconv.ParseInt(string(ns.ResourceVersion), 10, 64); err == nil {
			result.Summary.TotalRV += int(rv)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
