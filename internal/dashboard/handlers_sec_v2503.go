package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.03 Security: Pod SELinux, Secret Owner Reference, ClusterRole AggregationRule Verbs
type SELinuxResult2503 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithSELinux     int `json:"withSELinux"`
	} `json:"summary"`
}

func (s *Server) handleSELinux2503(w http.ResponseWriter, r *http.Request) {
	result := SELinuxResult2503{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.SELinuxOptions != nil {
				result.Summary.WithSELinux++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretOwnerRefResult2503 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByOwnerKind  map[string]int `json:"byOwnerKind"`
	} `json:"summary"`
}

func (s *Server) handleSecretOwnerRef2503(w http.ResponseWriter, r *http.Request) {
	result := SecretOwnerRefResult2503{ScannedAt: time.Now()}
	result.Summary.ByOwnerKind = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if len(secret.OwnerReferences) == 0 {
			result.Summary.ByOwnerKind["<none>"]++
		}
		for _, ref := range secret.OwnerReferences {
			result.Summary.ByOwnerKind[ref.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRAggVerbsResult2503 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int `json:"totalClusterRoles"`
		WithAgg    int `json:"withAggregationRule"`
		TotalVerbs int `json:"totalAggregatedVerbs"`
	} `json:"summary"`
}

func (s *Server) handleCRAggVerbs2503(w http.ResponseWriter, r *http.Request) {
	result := CRAggVerbsResult2503{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		if cr.AggregationRule != nil {
			result.Summary.WithAgg++
		}
		for _, rule := range cr.Rules {
			result.Summary.TotalVerbs += len(rule.Verbs)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
