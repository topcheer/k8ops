package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.67 Security: Pod SeccompProfile, Secret Data Key Count, ClusterRole Verbs Total
type SeccompProfileResult2467 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByProfile       map[string]int `json:"bySeccompProfileType"`
	} `json:"summary"`
}

func (s *Server) handleSeccompProfile2467(w http.ResponseWriter, r *http.Request) {
	result := SeccompProfileResult2467{ScannedAt: time.Now()}
	result.Summary.ByProfile = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			pt := "<none>"
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				pt = string(c.SecurityContext.SeccompProfile.Type)
			}
			result.Summary.ByProfile[pt]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretKeyCountResult2467 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalKeys    int `json:"totalDataKeys"`
	} `json:"summary"`
}

func (s *Server) handleSecretKeyCount2467(w http.ResponseWriter, r *http.Request) {
	result := SecretKeyCountResult2467{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalKeys += len(secret.Data)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRVerbsTotalResult2467 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int `json:"totalClusterRoles"`
		TotalVerbs int `json:"totalVerbs"`
	} `json:"summary"`
}

func (s *Server) handleCRVerbsTotal2467(w http.ResponseWriter, r *http.Request) {
	result := CRVerbsTotalResult2467{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			result.Summary.TotalVerbs += len(rule.Verbs)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
