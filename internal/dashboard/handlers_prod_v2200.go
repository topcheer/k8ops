package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.00 — Product Dimension (Round 53)
// 1. Pod IP Family Policy Catalog
// 2. Container Startup Probe Type Distribution
// 3. Service Session Affinity Timeout
// ============================================================

type IPFamPolicyResult2200 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByIPFamily map[string]int `json:"byIPFamilyPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleIPFamPolicy2200(w http.ResponseWriter, r *http.Request) {
	result := IPFamPolicyResult2200{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByIPFamily = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		policy := string(corev1.IPFamilyPolicySingleStack)
		if pod.Spec.HostNetwork {
			policy = "hostNetwork"
		} else {
			policy = "default"
		}
		result.Summary.ByIPFamily[policy]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Startup Probe Type
type StartupProbeTypeResult2200 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		WithStartup     int            `json:"withStartupProbe"`
		ByType          map[string]int `json:"byProbeType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleStartupProbeType2200(w http.ResponseWriter, r *http.Request) {
	result := StartupProbeTypeResult2200{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByType = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
				switch {
				case c.StartupProbe.HTTPGet != nil:
					result.Summary.ByType["httpGet"]++
				case c.StartupProbe.TCPSocket != nil:
					result.Summary.ByType["tcpSocket"]++
				case c.StartupProbe.Exec != nil:
					result.Summary.ByType["exec"]++
				default:
					result.Summary.ByType["other"]++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Session Affinity Timeout
type SessTimeoutResult2200 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithTimeout   int `json:"withClientIPTimeout"`
		MaxTimeout    int `json:"maxTimeoutSeconds"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSessTimeout2200(w http.ResponseWriter, r *http.Request) {
	result := SessTimeoutResult2200{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	maxT := 0
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.SessionAffinityConfig != nil && svc.Spec.SessionAffinityConfig.ClientIP != nil {
			timeout := int(*svc.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds)
			result.Summary.WithTimeout++
			if timeout > maxT {
				maxT = timeout
			}
		}
	}
	result.Summary.MaxTimeout = maxT
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
