package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.91 — Security Dimension (Round 51)
// 1. Pod SELinux Level Distribution
// 2. Secret Annotation Compliance
// 3. ClusterRoleBinding RoleRef Kind Audit
// ============================================================

type SELinuxLevelResult2191 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithLevel int `json:"withSELinuxLevel"`
		WithRole  int `json:"withSELinuxRole"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSELinuxLevel2191(w http.ResponseWriter, r *http.Request) {
	result := SELinuxLevelResult2191{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SELinuxOptions != nil {
			if pod.Spec.SecurityContext.SELinuxOptions.Level != "" {
				result.Summary.WithLevel++
			}
			if pod.Spec.SecurityContext.SELinuxOptions.Role != "" {
				result.Summary.WithRole++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Annotation Compliance
type SecAnnotResult2191 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		WithAnnot    map[string]int `json:"byAnnotation"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSecAnnot2191(w http.ResponseWriter, r *http.Request) {
	result := SecAnnotResult2191{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.WithAnnot = make(map[string]int)
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for k := range secret.Annotations {
			if containsStr2039(k, "kubernetes.io") {
				result.Summary.WithAnnot[k]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CRB RoleRef Kind
type CRBRoleRefResult2191 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRB   int            `json:"totalClusterRoleBindings"`
		ByRoleKind map[string]int `json:"byRoleRefKind"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRBRoleRef2191(w http.ResponseWriter, r *http.Request) {
	result := CRBRoleRefResult2191{ScannedAt: time.Now()}
	score := 100
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByRoleKind = make(map[string]int)
	for _, crb := range crbList.Items {
		result.Summary.TotalCRB++
		result.Summary.ByRoleKind[crb.RoleRef.Kind]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
