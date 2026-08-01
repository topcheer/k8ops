package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.83 Scalability: Top Namespace by DaemonSet v2, Node CPU Limit vs Allocatable, Cluster EndpointSlice Total
type TopNSByDS2Result2583 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		DSCount   int    `json:"dsCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByDS2Result2583(w http.ResponseWriter, r *http.Request) {
	result := TopNSByDS2Result2583{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	nsDS := make(map[string]int)
	for _, ds := range dsList.Items {
		nsDS[ds.Namespace]++
	}
	result.Summary.TotalNS = len(nsDS)
	for ns, count := range nsDS {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			DSCount   int    `json:"dsCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].DSCount > result.TopNS[j].DSCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPULimVsAllocResult2583 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalCPUAllocatable"`
		TotalLimit float64 `json:"totalCPULimit"`
	}
}

func (s *Server) handleNodeCPULimVsAlloc2583(w http.ResponseWriter, r *http.Request) {
	result := NodeCPULimVsAllocResult2583{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLimit += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPSliceTotal2583Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices int            `json:"totalSlices"`
		ByNS        map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleEPSliceTotal2583(w http.ResponseWriter, r *http.Request) {
	result := EPSliceTotal2583Result{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	sliceList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, slice := range sliceList.Items {
		result.Summary.TotalSlices++
		result.Summary.ByNS[slice.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
