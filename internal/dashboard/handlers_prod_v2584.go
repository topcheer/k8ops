package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.84 Product: Pod Spec Affinity PodAntiAffinity, Container Env Var Count, Service LoadBalancer Ingress
type PodAntiAffinityResult2584 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithAnti  int `json:"withPodAntiAffinity"`
	}
}

func (s *Server) handlePodAntiAffinity2584(w http.ResponseWriter, r *http.Request) {
	result := PodAntiAffinityResult2584{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil && pod.Spec.Affinity.PodAntiAffinity != nil {
			result.Summary.WithAnti++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvVarCountResult2584 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalEnvVars    int `json:"totalEnvVars"`
	}
}

func (s *Server) handleEnvVarCount2584(w http.ResponseWriter, r *http.Request) {
	result := EnvVarCountResult2584{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEnvVars += len(c.Env)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBIngressResult2584 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB     int `json:"totalLoadBalancer"`
		WithIngress int `json:"withIngress"`
	}
}

func (s *Server) handleLBIngress2584(w http.ResponseWriter, r *http.Request) {
	result := LBIngressResult2584{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			result.Summary.WithIngress++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
