package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.32 Product: Pod SecurityContext SupplementalGroups, Container Termination Message Path, Service LoadBalancer Source Ranges
type SupplementalGroupsResult2332 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods           int `json:"totalPods"`
		WithSupplementalGID int `json:"withSupplementalGroups"`
	} `json:"summary"`
}

func (s *Server) handleSupplementalGroups2332(w http.ResponseWriter, r *http.Request) {
	result := SupplementalGroupsResult2332{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.SupplementalGroups) > 0 {
			result.Summary.WithSupplementalGID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TermMsgPathResult2332 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByTermMsgPolicy map[string]int `json:"byTerminationMessagePolicy"`
	} `json:"summary"`
}

func (s *Server) handleTermMsgPath2332(w http.ResponseWriter, r *http.Request) {
	result := TermMsgPathResult2332{ScannedAt: time.Now()}
	result.Summary.ByTermMsgPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.ByTermMsgPolicy[string(c.TerminationMessagePolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBSourceRangeResult2332 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLBSvc      int `json:"totalLoadBalancerServices"`
		WithSourceRange int `json:"withSourceRangeRestriction"`
	} `json:"summary"`
}

func (s *Server) handleLBSourceRange2332(w http.ResponseWriter, r *http.Request) {
	result := LBSourceRangeResult2332{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLBSvc++
		if len(svc.Spec.LoadBalancerSourceRanges) > 0 {
			result.Summary.WithSourceRange++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
