package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v22.54 Product: Pod SetHostnameAsFQDN, Container Env Var Count, Service IPFamily Distribution
type HostnameFQDNResult2254 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithFQDN  int `json:"withFQDN"`
	} `json:"summary"`
}

func (s *Server) handleHostnameFQDN2254(w http.ResponseWriter, r *http.Request) {
	result := HostnameFQDNResult2254{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != "Running" {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SetHostnameAsFQDN != nil && *pod.Spec.SetHostnameAsFQDN {
			result.Summary.WithFQDN++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvVarCountResult2254 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalEnvVars    int `json:"totalEnvVars"`
		AvgPerContainer int `json:"avgPerContainer"`
	} `json:"summary"`
}

func (s *Server) handleEnvVarCount2254(w http.ResponseWriter, r *http.Request) {
	result := EnvVarCountResult2254{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != "Running" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEnvVars += len(c.Env)
		}
	}
	if result.Summary.TotalContainers > 0 {
		result.Summary.AvgPerContainer = result.Summary.TotalEnvVars / result.Summary.TotalContainers
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcIPFamResult2254 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByIPFamily    map[string]int `json:"byIPFamily"`
	} `json:"summary"`
}

func (s *Server) handleSvcIPFam2254(w http.ResponseWriter, r *http.Request) {
	result := SvcIPFamResult2254{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByIPFamily = make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if len(svc.Spec.IPFamilies) > 0 {
			for _, f := range svc.Spec.IPFamilies {
				result.Summary.ByIPFamily[string(f)]++
			}
		} else {
			result.Summary.ByIPFamily["default"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
