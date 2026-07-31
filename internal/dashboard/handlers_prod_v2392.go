package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.92 Product: Pod Affinity Rule Count, Container Env ValueFrom, Service LoadBalancer Class
type AffinityRuleResult2392 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithAffinity int `json:"withAffinityRules"`
	} `json:"summary"`
}

func (s *Server) handleAffinityRule2392(w http.ResponseWriter, r *http.Request) {
	result := AffinityRuleResult2392{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil {
			result.Summary.WithAffinity++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvValueFromResult2392 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEnvVars  int `json:"totalEnvVars"`
		WithValueFrom int `json:"withValueFrom"`
	} `json:"summary"`
}

func (s *Server) handleEnvValueFrom2392(w http.ResponseWriter, r *http.Request) {
	result := EnvValueFromResult2392{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, e := range c.Env {
				result.Summary.TotalEnvVars++
				if e.ValueFrom != nil {
					result.Summary.WithValueFrom++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBClassResult2392 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLBSvc int            `json:"totalLoadBalancerServices"`
		ByClass    map[string]int `json:"byLoadBalancerClass"`
	} `json:"summary"`
}

func (s *Server) handleLBClass2392(w http.ResponseWriter, r *http.Request) {
	result := LBClassResult2392{ScannedAt: time.Now()}
	result.Summary.ByClass = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLBSvc++
		cls := "<default>"
		if svc.Spec.LoadBalancerClass != nil {
			cls = *svc.Spec.LoadBalancerClass
		}
		result.Summary.ByClass[cls]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
