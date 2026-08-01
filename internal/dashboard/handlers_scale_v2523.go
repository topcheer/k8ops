package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.23 Scalability: Top Node by Memory Request, Node Pods Allocatable Ratio, Cluster Job Total
type TopNodeMemReqResult2523 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
	} `json:"summary"`
	TopNodes []struct {
		Node     string  `json:"node"`
		MemReqMB float64 `json:"memReqMB"`
	} `json:"topNodes"`
}

func (s *Server) handleTopNodeMemReq2523(w http.ResponseWriter, r *http.Request) {
	result := TopNodeMemReqResult2523{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		var mem float64
		for _, c := range pod.Spec.Containers {
			mem += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
		nodeMem[pod.Spec.NodeName] += mem
	}
	result.Summary.TotalNodes = len(nodeMem)
	for node, mem := range nodeMem {
		result.TopNodes = append(result.TopNodes, struct {
			Node     string  `json:"node"`
			MemReqMB float64 `json:"memReqMB"`
		}{node, mem})
	}
	sort.Slice(result.TopNodes, func(i, j int) bool { return result.TopNodes[i].MemReqMB > result.TopNodes[j].MemReqMB })
	if len(result.TopNodes) > 10 {
		result.TopNodes = result.TopNodes[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodePodsAllocRatioResult2523 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgRatio   float64 `json:"avgPodsAllocatableRatio"`
	} `json:"summary"`
}

func (s *Server) handleNodePodsAllocRatio2523(w http.ResponseWriter, r *http.Request) {
	result := NodePodsAllocRatioResult2523{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodePods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nodePods[pod.Spec.NodeName]++
		}
	}
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Allocatable.Pods().Value()
		if cap > 0 {
			totalRatio += float64(nodePods[node.Name]) / float64(cap)
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgRatio = totalRatio / float64(result.Summary.TotalNodes) * 100
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobTotalResult2523 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int            `json:"totalJobs"`
		ByNS      map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleJobTotal2523(w http.ResponseWriter, r *http.Request) {
	result := JobTotalResult2523{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		result.Summary.ByNS[job.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
