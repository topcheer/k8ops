package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.14 Product: Pod OS Audit, Container Resource Resize Policy, Service Publish NotReady
type PodOSResult2314 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByOS      map[string]int `json:"byOS"`
	} `json:"summary"`
}

func (s *Server) handlePodOS2314(w http.ResponseWriter, r *http.Request) {
	result := PodOSResult2314{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.ByOS[string(pod.Spec.OS.Name)]++
		} else {
			result.Summary.ByOS["<default-linux>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResizePolicyResult2314 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers  int `json:"totalContainers"`
		WithResizePolicy int `json:"withResizePolicy"`
	} `json:"summary"`
}

func (s *Server) handleResizePolicy2314(w http.ResponseWriter, r *http.Request) {
	result := ResizePolicyResult2314{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if len(c.ResizePolicy) > 0 {
				result.Summary.WithResizePolicy++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PubNotReadyResult2314 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		PubNotReady   int `json:"publishNotReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handlePubNotReady2314(w http.ResponseWriter, r *http.Request) {
	result := PubNotReadyResult2314{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PubNotReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
