package dashboard

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.31 Security: Pod ProcMount Unmasked, Secret Data Count, ClusterRole Verb Create
type ProcMountResult2431 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		UnmaskedProc    int `json:"unmaskedProcMount"`
	} `json:"summary"`
}

func (s *Server) handleProcMount2431(w http.ResponseWriter, r *http.Request) {
	result := ProcMountResult2431{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ProcMount != nil && *c.SecurityContext.ProcMount == corev1.UnmaskedProcMount {
				result.Summary.UnmaskedProc++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.UnmaskedProc > 0 {
		score = 100 - (result.Summary.UnmaskedProc*80)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretDataCountResult2431 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByDataCount  map[string]int `json:"byDataKeyCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretDataCount2431(w http.ResponseWriter, r *http.Request) {
	result := SecretDataCountResult2431{ScannedAt: time.Now()}
	result.Summary.ByDataCount = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByDataCount[fmt.Sprintf("%d keys", len(secret.Data))]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRVerbCreateResult2431 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR   int `json:"totalClusterRoles"`
		CanCreate int `json:"withCreateVerb"`
	} `json:"summary"`
}

func (s *Server) handleCRVerbCreate2431(w http.ResponseWriter, r *http.Request) {
	result := CRVerbCreateResult2431{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "create" {
					result.Summary.CanCreate++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
