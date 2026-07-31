package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.80 Product: Pod DNSSearchDomains, Container Liveness Probe Audit, Service Type Distribution
type DNSSearchResult2380 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByDNSPolicy map[string]int `json:"byDNSPolicy"`
	} `json:"summary"`
}

func (s *Server) handleDNSSearch2380(w http.ResponseWriter, r *http.Request) {
	result := DNSSearchResult2380{ScannedAt: time.Now()}
	result.Summary.ByDNSPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByDNSPolicy[string(pod.Spec.DNSPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LivenessProbeResult2380 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLiveness    int `json:"withLivenessProbe"`
	} `json:"summary"`
}

func (s *Server) handleLivenessProbe2380(w http.ResponseWriter, r *http.Request) {
	result := LivenessProbeResult2380{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.WithLiveness * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SvcTypeDistResult2380 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByType        map[string]int `json:"byServiceType"`
	} `json:"summary"`
}

func (s *Server) handleSvcTypeDist2380(w http.ResponseWriter, r *http.Request) {
	result := SvcTypeDistResult2380{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByType[string(svc.Spec.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
