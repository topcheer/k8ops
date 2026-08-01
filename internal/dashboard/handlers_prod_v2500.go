package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.00 Product: Pod RuntimeClass, Container Resource Request Summary, Service AllocateLoadBalancerNodePorts
type RuntimeClassResult2500 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int            `json:"totalPods"`
		ByRuntimeClass map[string]int `json:"byRuntimeClassName"`
	} `json:"summary"`
}

func (s *Server) handleRuntimeClass2500(w http.ResponseWriter, r *http.Request) {
	result := RuntimeClassResult2500{ScannedAt: time.Now()}
	result.Summary.ByRuntimeClass = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		rc := pod.Spec.RuntimeClassName
		if rc == nil || *rc == "" {
			result.Summary.ByRuntimeClass["<default>"]++
		} else {
			result.Summary.ByRuntimeClass[*rc]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceReqSummaryResult2500 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPURequestCores"`
		TotalMemReqMB   float64 `json:"totalMemRequestMB"`
	} `json:"summary"`
}

func (s *Server) handleResourceReqSummary2500(w http.ResponseWriter, r *http.Request) {
	result := ResourceReqSummaryResult2500{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalMemReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type AllocLBNodePortsResult2500 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB  int `json:"totalLoadBalancerServices"`
		AllocNPs int `json:"allocateLoadBalancerNodePorts"`
	} `json:"summary"`
}

func (s *Server) handleAllocLBNodePorts2500(w http.ResponseWriter, r *http.Request) {
	result := AllocLBNodePortsResult2500{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if svc.Spec.AllocateLoadBalancerNodePorts == nil || *svc.Spec.AllocateLoadBalancerNodePorts {
			result.Summary.AllocNPs++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
