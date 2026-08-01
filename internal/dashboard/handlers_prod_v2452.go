package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.52 Product: Pod HostIPC Usage, Container Image PullPolicy Distribution, Service LoadBalancerIP Count
type HostIPCResult2452 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostIPC   int `json:"hostIPC"`
	} `json:"summary"`
}

func (s *Server) handleHostIPC2452(w http.ResponseWriter, r *http.Request) {
	result := HostIPCResult2452{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PullPolicyDistResult2452 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPolicy        map[string]int `json:"byPullPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePullPolicyDist2452(w http.ResponseWriter, r *http.Request) {
	result := PullPolicyDistResult2452{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.ByPolicy[string(c.ImagePullPolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBIPCountResult2452 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		WithLBIP  int `json:"withLoadBalancerIP"`
	} `json:"summary"`
}

func (s *Server) handleLBIPCount2452(w http.ResponseWriter, r *http.Request) {
	result := LBIPCountResult2452{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer && svc.Spec.LoadBalancerIP != "" {
			result.Summary.WithLBIP++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
