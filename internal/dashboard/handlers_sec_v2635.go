package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.35 Security: Pod FSGroupChangePolicy, Secret Immutable Count v2, RoleBinding Subject APIGroup Detail
type FSGroupChange2635Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byFSGroupChangePolicy"`
	} `json:"summary"`
}

func (s *Server) handleFSGroupChange2635(w http.ResponseWriter, r *http.Request) {
	result := FSGroupChange2635Result{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		policy := "<default>"
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroupChangePolicy != nil {
			policy = string(*pod.Spec.SecurityContext.FSGroupChangePolicy)
		}
		result.Summary.ByPolicy[policy]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretImmutable2Result2635 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		Immutable    int `json:"immutableCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretImmutable2Result2635(w http.ResponseWriter, r *http.Request) {
	result := SecretImmutable2Result2635{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Immutable != nil && *secret.Immutable {
			result.Summary.Immutable++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectAPIGroup2635Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int            `json:"totalRoleBindings"`
		ByAPIGroup map[string]int `json:"bySubjectAPIGroup"`
	} `json:"summary"`
}

func (s *Server) handleRBSubjectAPIGroup2635(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectAPIGroup2635Result{ScannedAt: time.Now()}
	result.Summary.ByAPIGroup = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			result.Summary.ByAPIGroup[subj.APIGroup]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
