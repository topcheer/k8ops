package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.95 Scalability: Top Namespace by Job, Node Memory Allocatable vs Limit, Cluster NetworkPolicy Total v2
type TopNSByJob2Result2595 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		JobCount  int    `json:"jobCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByJob2Result2595(w http.ResponseWriter, r *http.Request) {
	result := TopNSByJob2Result2595{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	nsJobs := make(map[string]int)
	for _, job := range jobList.Items {
		nsJobs[job.Namespace]++
	}
	result.Summary.TotalNS = len(nsJobs)
	for ns, count := range nsJobs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			JobCount  int    `json:"jobCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].JobCount > result.TopNS[j].JobCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemAllocVsLimResult2595 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocMemGB"`
		TotalLim   float64 `json:"totalLimitMemGB"`
	}
}

func (s *Server) handleNodeMemAllocVsLim2595(w http.ResponseWriter, r *http.Request) {
	result := NodeMemAllocVsLimResult2595{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLim += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NetPolicyTotal2Result2595 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP int            `json:"totalNetworkPolicies"`
		ByNS    map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleNetPolicyTotal2Result2595(w http.ResponseWriter, r *http.Request) {
	result := NetPolicyTotal2Result2595{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		result.Summary.ByNS[np.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
