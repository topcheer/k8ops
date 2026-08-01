package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.20 Product: Pod PreemptionPolicy, Container Mem Request Detail, Service AllocateLoadBalancerNodePorts
type PreemptionPolicy2620Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byPreemptionPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePreemptionPolicy2620(w http.ResponseWriter, r *http.Request) {
	result := PreemptionPolicy2620Result{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		p := "<none>"
		if pod.Spec.PreemptionPolicy != nil {
			p = string(*pod.Spec.PreemptionPolicy)
		}
		result.Summary.ByPolicy[p]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MemReqDetail2620Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemReq     float64 `json:"totalMemReqMB"`
	} `json:"summary"`
}

func (s *Server) handleMemReqDetail2620(w http.ResponseWriter, r *http.Request) {
	result := MemReqDetail2620Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemReq += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcAllocLBNP2620Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB   int `json:"totalLoadBalancer"`
		WithAlloc int `json:"withAllocNodePorts"`
	} `json:"summary"`
}

func (s *Server) handleSvcAllocLBNP2620(w http.ResponseWriter, r *http.Request) {
	result := SvcAllocLBNP2620Result{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if svc.Spec.AllocateLoadBalancerNodePorts != nil && *svc.Spec.AllocateLoadBalancerNodePorts {
			result.Summary.WithAlloc++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
