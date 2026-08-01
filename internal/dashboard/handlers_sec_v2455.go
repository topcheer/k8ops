package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.55 Security: Pod Privileged Containers, Secret TLS Count, RoleBinding Subject Namespace
type PrivilegedCtnrResult2455 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Privileged      int `json:"privileged"`
	} `json:"summary"`
}

func (s *Server) handlePrivilegedCtnr2455(w http.ResponseWriter, r *http.Request) {
	result := PrivilegedCtnrResult2455{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Privileged++
			}
		}
	}
	score := 100
	if result.Summary.Privileged > 0 && result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.Privileged*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretTLSResult2455 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TLSCount     int `json:"tlsSecretCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretTLS2455(w http.ResponseWriter, r *http.Request) {
	result := SecretTLSResult2455{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeTLS {
			result.Summary.TLSCount++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectNSResult2455 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB int            `json:"totalRoleBindings"`
		ByNS    map[string]int `json:"bySubjectNamespace"`
	} `json:"summary"`
}

func (s *Server) handleRBSubjectNS2455(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectNSResult2455{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			ns := subj.Namespace
			if ns == "" {
				ns = "<cluster>"
			}
			result.Summary.ByNS[ns]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
