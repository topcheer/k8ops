package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.30 Product: Pod Spec TerminationGracePeriod Distribution, Container Resource EphemeralStorage Limit, Service IPFamilyPolicy Detail
type TermGraceDistResult2530 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Default   int `json:"defaultGrace"`
		Custom    int `json:"customGrace"`
	} `json:"summary"`
}

func (s *Server) handleTermGraceDist2530(w http.ResponseWriter, r *http.Request) {
	result := TermGraceDistResult2530{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.TerminationGracePeriodSeconds == nil || *pod.Spec.TerminationGracePeriodSeconds == 30 {
			result.Summary.Default++
		} else {
			result.Summary.Custom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EphemeralLimitResult2530 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEphemeral   int `json:"withEphemeralLimit"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralLimit2530(w http.ResponseWriter, r *http.Request) {
	result := EphemeralLimitResult2530{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if _, ok := c.Resources.Limits[corev1.ResourceEphemeralStorage]; ok {
				result.Summary.WithEphemeral++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IPFamilyPolicyDetailResult2530 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byIPFamilyPolicy"`
	} `json:"summary"`
}

func (s *Server) handleIPFamilyPolicyDetail2530(w http.ResponseWriter, r *http.Request) {
	result := IPFamilyPolicyDetailResult2530{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		policy := "<none>"
		if svc.Spec.IPFamilyPolicy != nil {
			policy = string(*svc.Spec.IPFamilyPolicy)
		}
		result.Summary.ByPolicy[policy]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
