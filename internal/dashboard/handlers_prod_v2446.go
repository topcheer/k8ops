package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.46 Product: Pod HostNetwork Usage, Container WorkingDir Diversity, Service ExternalTrafficPolicy
type HostNetworkResult2446 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		HostNetwork int `json:"hostNetworkPods"`
	} `json:"summary"`
}

func (s *Server) handleHostNetwork2446(w http.ResponseWriter, r *http.Request) {
	result := HostNetworkResult2446{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostNetwork {
			result.Summary.HostNetwork++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type WorkingDirResult2446 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByDir           map[string]int `json:"byWorkingDir"`
	} `json:"summary"`
}

func (s *Server) handleWorkingDir2446(w http.ResponseWriter, r *http.Request) {
	result := WorkingDirResult2446{ScannedAt: time.Now()}
	result.Summary.ByDir = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			dir := c.WorkingDir
			if dir == "" {
				dir = "<default>"
			}
			result.Summary.ByDir[dir]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExtTrafficPolicyResult2446 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byExternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleExtTrafficPolicy2446(w http.ResponseWriter, r *http.Request) {
	result := ExtTrafficPolicyResult2446{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByPolicy[string(svc.Spec.ExternalTrafficPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
