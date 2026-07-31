package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.45 — Security Dimension (Round 60)
// 1. Pod AppArmor Profile Distribution
// 2. Secret Immutable Status Audit
// 3. RoleBinding RoleRef Kind Distribution
// ============================================================

type AppArmorResult2245 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByProfile map[string]int `json:"byAppArmorProfile"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAppArmor2245(w http.ResponseWriter, r *http.Request) {
	result := AppArmorResult2245{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByProfile = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		profile := "default"
		if pod.Annotations["container.apparmor.security.beta.kubernetes.io"] != "" {
			profile = pod.Annotations["container.apparmor.security.beta.kubernetes.io"]
		}
		result.Summary.ByProfile[profile]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Immutable Audit
type SecretImmutableResult2245 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutable"`
		Mutable      int `json:"mutable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecretImmutable2245(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutableResult2245{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.Immutable++
		} else {
			result.Summary.Mutable++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RB RoleRef Kind
type RBRoleRefKindResult2245 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int            `json:"totalRoleBindings"`
		ByRoleKind map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRBRoleRefKind2245(w http.ResponseWriter, r *http.Request) {
	result := RBRoleRefKindResult2245{ScannedAt: time.Now()}
	score := 100
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByRoleKind = make(map[string]int)
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.ByRoleKind[rb.RoleRef.Kind]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
