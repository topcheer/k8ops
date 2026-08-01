package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.82 Product: Pod InitContainer Count, Container TerminationMessagePath, Service IPFamily Policy
type InitContainerResult2482 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithInitCtnr int `json:"withInitContainers"`
		TotalInit    int `json:"totalInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleInitContainer2482(w http.ResponseWriter, r *http.Request) {
	result := InitContainerResult2482{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.InitContainers) > 0 {
			result.Summary.WithInitCtnr++
			result.Summary.TotalInit += len(pod.Spec.InitContainers)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TermMsgPathResult2482 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPath          map[string]int `json:"byTerminationMessagePath"`
	} `json:"summary"`
}

func (s *Server) handleTermMsgPath2482(w http.ResponseWriter, r *http.Request) {
	result := TermMsgPathResult2482{ScannedAt: time.Now()}
	result.Summary.ByPath = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			path := c.TerminationMessagePath
			if path == "" {
				path = "<default>"
			}
			result.Summary.ByPath[path]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IPFamilyPolicyResult2482 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byIPFamilyPolicy"`
	} `json:"summary"`
}

func (s *Server) handleIPFamilyPolicy2482(w http.ResponseWriter, r *http.Request) {
	result := IPFamilyPolicyResult2482{ScannedAt: time.Now()}
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
