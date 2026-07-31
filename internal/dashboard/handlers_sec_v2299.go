package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.99 Security: Pod Seccomp Type Audit, Role Binding Subject Census, NetworkPolicy Port Catalog
type SeccompTypeResult2299 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByType    map[string]int `json:"bySeccompType"`
	} `json:"summary"`
}

func (s *Server) handleSeccompType2299(w http.ResponseWriter, r *http.Request) {
	result := SeccompTypeResult2299{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			result.Summary.ByType[string(pod.Spec.SecurityContext.SeccompProfile.Type)]++
		} else {
			result.Summary.ByType["none"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type BindingSubjectResult2299 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalBindings int            `json:"totalBindings"`
		ByKind        map[string]int `json:"bySubjectKind"`
	} `json:"summary"`
}

func (s *Server) handleBindingSubject2299(w http.ResponseWriter, r *http.Request) {
	result := BindingSubjectResult2299{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		for _, sub := range rb.Subjects {
			result.Summary.TotalBindings++
			result.Summary.ByKind[string(sub.Kind)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NetPolPortResult2299 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNetPols   int `json:"totalNetPols"`
		TotalPortRules int `json:"totalPortRules"`
	} `json:"summary"`
}

func (s *Server) handleNetPolPort2299(w http.ResponseWriter, r *http.Request) {
	result := NetPolPortResult2299{ScannedAt: time.Now()}
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNetPols++
		for _, ing := range np.Spec.Ingress {
			result.Summary.TotalPortRules += len(ing.Ports)
		}
		for _, eg := range np.Spec.Egress {
			result.Summary.TotalPortRules += len(eg.Ports)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
