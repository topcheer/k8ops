package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.26 Product: Pod Runtime Class Audit, Container Stdin Once, Service Allocation CIDR
type RuntimeClassResult2326 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int            `json:"totalPods"`
		ByRuntimeClass map[string]int `json:"byRuntimeClass"`
	} `json:"summary"`
}

func (s *Server) handleRuntimeClass2326(w http.ResponseWriter, r *http.Request) {
	result := RuntimeClassResult2326{ScannedAt: time.Now()}
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

type StdinOnceResult2326 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdinOnce   int `json:"withStdinOnce"`
	} `json:"summary"`
}

func (s *Server) handleStdinOnce2326(w http.ResponseWriter, r *http.Request) {
	result := StdinOnceResult2326{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StdinOnce {
				result.Summary.WithStdinOnce++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type AllocCIDRResult2326 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByCIDRPrefix  map[string]int `json:"byClusterIPCidr"`
	} `json:"summary"`
}

func (s *Server) handleAllocCIDR2326(w http.ResponseWriter, r *http.Request) {
	result := AllocCIDRResult2326{ScannedAt: time.Now()}
	result.Summary.ByCIDRPrefix = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
			continue
		}
		result.Summary.TotalServices++
		parts := strings.SplitN(svc.Spec.ClusterIP, ".", 2)
		if len(parts) >= 2 {
			result.Summary.ByCIDRPrefix[parts[0]+"."+parts[1]+"."]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
