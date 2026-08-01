package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.29 Scalability: Top Namespace by Ingress, Node CPU Allocatable vs Limit, Cluster CronJob Total
type TopNSByIngressResult2529 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		IngCount  int    `json:"ingressCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByIngress2529(w http.ResponseWriter, r *http.Request) {
	result := TopNSByIngressResult2529{ScannedAt: time.Now()}
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	nsIng := make(map[string]int)
	for _, ing := range ingList.Items {
		nsIng[ing.Namespace]++
	}
	result.Summary.TotalNS = len(nsIng)
	for ns, count := range nsIng {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			IngCount  int    `json:"ingressCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].IngCount > result.TopNS[j].IngCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUAllocVsLimitResult2529 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalCPUAllocatable"`
		TotalLimit float64 `json:"totalCPULimit"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUAllocVsLimit2529(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocVsLimitResult2529{ScannedAt: time.Now()}
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

type CronJobTotalResult2529 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		ByNS          map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleCronJobTotal2529(w http.ResponseWriter, r *http.Request) {
	result := CronJobTotalResult2529{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cron := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.ByNS[cron.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
