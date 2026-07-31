package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.56 Product: Pod Hostname Audit, Container Stderr True, Service IP Family
type HostnameResult2356 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithHostname int `json:"withCustomHostname"`
	} `json:"summary"`
}

func (s *Server) handleHostname2356(w http.ResponseWriter, r *http.Request) {
	result := HostnameResult2356{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Hostname != "" {
			result.Summary.WithHostname++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrStdinResult2356 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdin       int `json:"withStdin"`
	} `json:"summary"`
}

func (s *Server) handleCtnrStdin2356(w http.ResponseWriter, r *http.Request) {
	result := CtnrStdinResult2356{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStdin++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcIPFamilyResult2356 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByIPFamily    map[string]int `json:"byIPFamily"`
	} `json:"summary"`
}

func (s *Server) handleSvcIPFamily2356(w http.ResponseWriter, r *http.Request) {
	result := SvcIPFamilyResult2356{ScannedAt: time.Now()}
	result.Summary.ByIPFamily = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, fam := range svc.Spec.IPFamilies {
			result.Summary.ByIPFamily[string(fam)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
