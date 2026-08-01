package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.94 Product: Pod EphemeralStorage Request, Container Image ID Summary, Service InternalTrafficPolicy
type EphemeralStorageResult2494 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEphemeral   int `json:"withEphemeralStorageRequest"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralStorage2494(w http.ResponseWriter, r *http.Request) {
	result := EphemeralStorageResult2494{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if _, ok := c.Resources.Requests[corev1.ResourceEphemeralStorage]; ok {
				result.Summary.WithEphemeral++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageIDSummaryResult2494 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithImageID     int `json:"withImageID"`
	} `json:"summary"`
}

func (s *Server) handleImageIDSummary2494(w http.ResponseWriter, r *http.Request) {
	result := ImageIDSummaryResult2494{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.ImageID != "" {
				result.Summary.WithImageID++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type InternalTrafficResult2494 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byInternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleInternalTraffic2494(w http.ResponseWriter, r *http.Request) {
	result := InternalTrafficResult2494{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		policy := "<none>"
		if svc.Spec.InternalTrafficPolicy != nil {
			policy = string(*svc.Spec.InternalTrafficPolicy)
		}
		result.Summary.ByPolicy[policy]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
