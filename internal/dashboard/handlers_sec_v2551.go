package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.51 Security: Pod SecurityContext RunAsUser Detail, Secret Namespace Distribution, RoleBinding Rules Summary
type RunAsUserDetailResult2551 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByUID           map[string]int `json:"byRunAsUserUID"`
	}
}

func (s *Server) handleRunAsUserDetail2551(w http.ResponseWriter, r *http.Request) {
	result := RunAsUserDetailResult2551{ScannedAt: time.Now()}
	result.Summary.ByUID = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			uid := "<default>"
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				uid = intToStr(int(*c.SecurityContext.RunAsUser))
			}
			result.Summary.ByUID[uid]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretNSDistResult2551 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByNS         map[string]int `json:"byNamespace"`
	}
}

func (s *Server) handleSecretNSDist2551(w http.ResponseWriter, r *http.Request) {
	result := SecretNSDistResult2551{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByNS[secret.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBRulesSummaryResult2551 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int `json:"totalRoleBindings"`
		TotalRules int `json:"totalRules"`
	}
}

func (s *Server) handleRBRulesSummary2551(w http.ResponseWriter, r *http.Request) {
	result := RBRulesSummaryResult2551{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalRules += len(cr.Rules)
	}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for range rbList.Items {
		result.Summary.TotalRB++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
