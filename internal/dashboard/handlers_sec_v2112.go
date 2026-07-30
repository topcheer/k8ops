package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.12 — Security Dimension (Round 38)
// 1. SA ImagePullSecret Audit
// 2. Pod runAsNonRoot Validator
// 3. Secret Immutable Flag Audit
// ============================================================

type SAPullResult2112 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SAPullSummary2112 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type SAPullSummary2112 struct {
	TotalSAs       int `json:"totalServiceAccounts"`
	WithPullSecret int `json:"withImagePullSecret"`
}

func (s *Server) handleSAPull2112(w http.ResponseWriter, r *http.Request) {
	result := SAPullResult2112{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.ImagePullSecrets) > 0 {
			result.Summary.WithPullSecret++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. runAsNonRoot Validator
type NonRootResult2112 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         NonRootSummary2112 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type NonRootSummary2112 struct {
	TotalPods   int `json:"totalPods"`
	NonRootPods int `json:"nonRootPods"`
}

func (s *Server) handleNonRoot2112(w http.ResponseWriter, r *http.Request) {
	result := NonRootResult2112{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsNonRoot != nil && *pod.Spec.SecurityContext.RunAsNonRoot {
			result.Summary.NonRootPods++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NonRootPods < result.Summary.TotalPods {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/%d pods missing runAsNonRoot", result.Summary.TotalPods-result.Summary.NonRootPods, result.Summary.TotalPods))
	}
	writeJSON(w, result)
}

// 3. Secret Immutable Flag
type SecImmutableResult2112 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         SecImmutableSummary2112 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type SecImmutableSummary2112 struct {
	TotalSecrets int `json:"totalSecrets"`
	ImmutableSet int `json:"immutableSet"`
}

func (s *Server) handleSecImmutable2112(w http.ResponseWriter, r *http.Request) {
	result := SecImmutableResult2112{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.ImmutableSet++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
