package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.29 Security: Pod Sysctl Audit, ConfigMap Projected Volume, Role Binding User Audit
type SysctlResult2329 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithSysctl int `json:"withSysctl"`
	} `json:"summary"`
}

func (s *Server) handleSysctl2329(w http.ResponseWriter, r *http.Request) {
	result := SysctlResult2329{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.Sysctls) > 0 {
			result.Summary.WithSysctl++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMProjectedVolResult2329 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithProjected int `json:"withProjectedVolume"`
	} `json:"summary"`
}

func (s *Server) handleCMProjectedVol2329(w http.ResponseWriter, r *http.Request) {
	result := CMProjectedVolResult2329{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.Projected != nil {
				result.Summary.WithProjected++
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleBindingUserResult2329 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalBindings int            `json:"totalBindings"`
		BySubjectKind map[string]int `json:"bySubjectKind"`
	} `json:"summary"`
}

func (s *Server) handleRoleBindingUser2329(w http.ResponseWriter, r *http.Request) {
	result := RoleBindingUserResult2329{ScannedAt: time.Now()}
	result.Summary.BySubjectKind = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		for _, sub := range crb.Subjects {
			result.Summary.TotalBindings++
			result.Summary.BySubjectKind[string(sub.Kind)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
