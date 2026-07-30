package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.82 — Product Dimension (Round 50)
// 1. Pod DNS Config Custom Tracker
// 2. Service Session Affinity Catalog
// 3. Container Env Var Source Distribution
// ============================================================

type DNSConfigResult2182 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithDNSConfig int `json:"withDNSConfig"`
		WithDNSPolicy int `json:"withCustomDNSPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDNSConfig2182(w http.ResponseWriter, r *http.Request) {
	result := DNSConfigResult2182{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.DNSConfig != nil {
			result.Summary.WithDNSConfig++
		}
		if pod.Spec.DNSPolicy != "" && pod.Spec.DNSPolicy != corev1.DNSClusterFirst {
			result.Summary.WithDNSPolicy++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Session Affinity Catalog
type SessAffinityResult2182 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByAffinity    map[string]int `json:"bySessionAffinity"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSessAffinity2182(w http.ResponseWriter, r *http.Request) {
	result := SessAffinityResult2182{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByAffinity = make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByAffinity[string(svc.Spec.SessionAffinity)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Env Var Source Distribution
type EnvSrcResult2182 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		BySource        map[string]int `json:"byEnvVarSource"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEnvSrc2182(w http.ResponseWriter, r *http.Request) {
	result := EnvSrcResult2182{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.BySource = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, e := range c.Env {
				switch {
				case e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil:
					result.Summary.BySource["configMap"]++
				case e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil:
					result.Summary.BySource["secret"]++
				case e.ValueFrom != nil && e.ValueFrom.FieldRef != nil:
					result.Summary.BySource["fieldRef"]++
				case e.ValueFrom != nil && e.ValueFrom.ResourceFieldRef != nil:
					result.Summary.BySource["resourceFieldRef"]++
				default:
					result.Summary.BySource["literal"]++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
