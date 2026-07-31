package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.47 Security: Pod Automount SA Token Audit, Secret Type TLS, ClusterRole Verbs Census
type AutoSATokenResult2347 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		TokenMounted int `json:"tokenAutoMounted"`
	} `json:"summary"`
}

func (s *Server) handleAutoSAToken2347(w http.ResponseWriter, r *http.Request) {
	result := AutoSATokenResult2347{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
			result.Summary.TokenMounted++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		mountedPct := result.Summary.TokenMounted * 100 / result.Summary.TotalPods
		score = 100 - mountedPct/4
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretTLSResult2347 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TLSSecrets   int `json:"tlsSecrets"`
	} `json:"summary"`
}

func (s *Server) handleSecretTLS2347(w http.ResponseWriter, r *http.Request) {
	result := SecretTLSResult2347{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeTLS {
			result.Summary.TLSSecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRVerbsResult2347 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR int            `json:"totalClusterRoles"`
		ByVerb  map[string]int `json:"byVerb"`
	} `json:"summary"`
}

func (s *Server) handleCRVerbs2347(w http.ResponseWriter, r *http.Request) {
	result := CRVerbsResult2347{ScannedAt: time.Now()}
	result.Summary.ByVerb = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				result.Summary.ByVerb[verb]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
