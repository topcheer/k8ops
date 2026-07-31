package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.77 Security: Pod RunAsNonRoot, Secret Type Census, Role Binding Kind
type RunAsNonRootResult2377 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		NonRoot   int `json:"runAsNonRoot"`
	} `json:"summary"`
}

func (s *Server) handleRunAsNonRoot2377(w http.ResponseWriter, r *http.Request) {
	result := RunAsNonRootResult2377{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsNonRoot != nil && *pod.Spec.SecurityContext.RunAsNonRoot {
			result.Summary.NonRoot++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = result.Summary.NonRoot * 100 / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretTypeCensusResult2377 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"bySecretType"`
	} `json:"summary"`
}

func (s *Server) handleSecretTypeCensus2377(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeCensusResult2377{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleBindingKindResult2377 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalBindings int            `json:"totalBindings"`
		ByRoleKind    map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
}

func (s *Server) handleRoleBindingKind2377(w http.ResponseWriter, r *http.Request) {
	result := RoleBindingKindResult2377{ScannedAt: time.Now()}
	result.Summary.ByRoleKind = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalBindings++
		result.Summary.ByRoleKind[rb.RoleRef.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
