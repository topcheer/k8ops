package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.78 Product: Container SecurityContext Nil Rate, Pod Network Policy Direction, Service ExternalName Catalog
type NilSecCtxResult2278 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		NilSecCtx       int `json:"nilSecurityContext"`
	} `json:"summary"`
}

func (s *Server) handleNilSecCtx2278(w http.ResponseWriter, r *http.Request) {
	result := NilSecCtxResult2278{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext == nil {
				result.Summary.NilSecCtx++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		nilPct := result.Summary.NilSecCtx * 100 / result.Summary.TotalContainers
		score = 100 - nilPct/2
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NetPolDirectionResult2278 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNetPols int `json:"totalNetPols"`
		WithIngress  int `json:"withIngress"`
		WithEgress   int `json:"withEgress"`
	} `json:"summary"`
}

func (s *Server) handleNetPolDirection2278(w http.ResponseWriter, r *http.Request) {
	result := NetPolDirectionResult2278{ScannedAt: time.Now()}
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNetPols++
		if len(np.Spec.Ingress) > 0 {
			result.Summary.WithIngress++
		}
		if len(np.Spec.Egress) > 0 {
			result.Summary.WithEgress++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExtNameSvcResult2278 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices   int `json:"totalServices"`
		ExternalNameSvc int `json:"externalNameServices"`
	} `json:"summary"`
}

func (s *Server) handleExtNameSvc2278(w http.ResponseWriter, r *http.Request) {
	result := ExtNameSvcResult2278{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalNameSvc++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
