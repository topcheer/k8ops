package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.95 Security: Pod Capabilities Audit, Secret Data Size, RoleBinding Namespace
type CapabilitiesResult2395 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCaps        int `json:"withCapabilities"`
	} `json:"summary"`
}

func (s *Server) handleCapabilities2395(w http.ResponseWriter, r *http.Request) {
	result := CapabilitiesResult2395{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				result.Summary.WithCaps++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.WithCaps > 0 {
		score = 100 - (result.Summary.WithCaps*30)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretDataSizeResult2395 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets  int `json:"totalSecrets"`
		TotalDataSize int `json:"totalDataSizeBytes"`
	} `json:"summary"`
}

func (s *Server) handleSecretDataSize2395(w http.ResponseWriter, r *http.Request) {
	result := SecretDataSizeResult2395{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for _, v := range secret.Data {
			result.Summary.TotalDataSize += len(v)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBNSResult2395 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB int            `json:"totalRoleBindings"`
		ByNS    map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleRBNS2395(w http.ResponseWriter, r *http.Request) {
	result := RBNSResult2395{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		result.Summary.ByNS[rb.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
