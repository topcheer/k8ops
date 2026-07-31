package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.08 Product: Pod GMSA Audit, Container Startup Probe Type, Service Internal Traffic Policy
type GMSAResult2308 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGMSA  int `json:"withGMSA"`
	} `json:"summary"`
}

func (s *Server) handleGMSA2308(w http.ResponseWriter, r *http.Request) {
	result := GMSAResult2308{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.WindowsOptions != nil && pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != nil && *pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != "" {
			result.Summary.WithGMSA++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StartupProbeTypeResult2308 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalWithStartup int            `json:"totalWithStartupProbe"`
		ByProbeType      map[string]int `json:"byProbeType"`
	} `json:"summary"`
}

func (s *Server) handleStartupProbeType2308(w http.ResponseWriter, r *http.Request) {
	result := StartupProbeTypeResult2308{ScannedAt: time.Now()}
	result.Summary.ByProbeType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.StartupProbe != nil {
				result.Summary.TotalWithStartup++
				if c.StartupProbe.HTTPGet != nil {
					result.Summary.ByProbeType["httpGet"]++
				} else if c.StartupProbe.TCPSocket != nil {
					result.Summary.ByProbeType["tcpSocket"]++
				} else if c.StartupProbe.Exec != nil {
					result.Summary.ByProbeType["exec"]++
				} else if c.StartupProbe.GRPC != nil {
					result.Summary.ByProbeType["grpc"]++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IntTrafficResult2308 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByPolicy      map[string]int `json:"byInternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleIntTraffic2308(w http.ResponseWriter, r *http.Request) {
	result := IntTrafficResult2308{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.InternalTrafficPolicy != nil {
			result.Summary.ByPolicy[string(*svc.Spec.InternalTrafficPolicy)]++
		} else {
			result.Summary.ByPolicy["<default>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
