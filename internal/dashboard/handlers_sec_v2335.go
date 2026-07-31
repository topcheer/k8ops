package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.35 Security: Pod fsGroupChangePolicy Always, ServiceAccount Automount Default, Secret Immutable Mark
type FSGroupAlwaysResult2335 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		Always         int `json:"alwaysPolicy"`
		OnRootMismatch int `json:"onRootMismatch"`
	} `json:"summary"`
}

func (s *Server) handleFSGroupAlways2335(w http.ResponseWriter, r *http.Request) {
	result := FSGroupAlwaysResult2335{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroupChangePolicy != nil {
			if string(*pod.Spec.SecurityContext.FSGroupChangePolicy) == "Always" {
				result.Summary.Always++
			} else if string(*pod.Spec.SecurityContext.FSGroupChangePolicy) == "OnRootMismatch" {
				result.Summary.OnRootMismatch++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SAAutomountResult2335 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs     int `json:"totalServiceAccounts"`
		AutoDisabled int `json:"automountDisabled"`
	} `json:"summary"`
}

func (s *Server) handleSAAutomount2335(w http.ResponseWriter, r *http.Request) {
	result := SAAutomountResult2335{ScannedAt: time.Now()}
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if sa.AutomountServiceAccountToken != nil && !*sa.AutomountServiceAccountToken {
			result.Summary.AutoDisabled++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretImmutableResult2335 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutable"`
	} `json:"summary"`
}

func (s *Server) handleSecretImmutable2335(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutableResult2335{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.Immutable++
		}
	}
	score := 100
	if result.Summary.TotalSecrets > 0 {
		imPct := result.Summary.Immutable * 100 / result.Summary.TotalSecrets
		score = 50 + imPct/2
		if score > 100 {
			score = 100
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
