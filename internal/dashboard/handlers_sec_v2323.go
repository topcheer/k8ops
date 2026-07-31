package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.23 Security: Pod AppArmor Audit, ConfigMap Immutable Mark, Secret Rotation Risk
type AppArmorResult2323 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByProfile map[string]int `json:"byAppArmorProfile"`
	} `json:"summary"`
}

func (s *Server) handleAppArmor2323(w http.ResponseWriter, r *http.Request) {
	result := AppArmorResult2323{ScannedAt: time.Now()}
	result.Summary.ByProfile = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Annotations != nil {
			if aa, ok := pod.Annotations["container.apparmor.security.beta.kubernetes.io/"]; ok {
				result.Summary.ByProfile[aa]++
			} else {
				result.Summary.ByProfile["<default>"]++
			}
		} else {
			result.Summary.ByProfile["<default>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMImmutableResult2323 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		Immutable int `json:"immutable"`
	} `json:"summary"`
}

func (s *Server) handleCMImmutable2323(w http.ResponseWriter, r *http.Request) {
	result := CMImmutableResult2323{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if cm.Immutable != nil && *cm.Immutable {
			result.Summary.Immutable++
		}
	}
	score := 100
	if result.Summary.TotalCMs > 0 {
		immutablePct := result.Summary.Immutable * 100 / result.Summary.TotalCMs
		score = 50 + immutablePct/2
		if score > 100 {
			score = 100
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretRotationResult2323 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		StaleSecrets int `json:"staleSecrets"`
	} `json:"summary"`
}

func (s *Server) handleSecretRotation2323(w http.ResponseWriter, r *http.Request) {
	result := SecretRotationResult2323{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.CreationTimestamp.Time.Before(now.AddDate(0, -6, 0)) {
			result.Summary.StaleSecrets++
		}
	}
	score := 100
	if result.Summary.TotalSecrets > 0 {
		stalePct := result.Summary.StaleSecrets * 100 / result.Summary.TotalSecrets
		score = 100 - stalePct/3
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
