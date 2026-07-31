package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.72 Documentation: Node Provider Label, Pod Env Var Count, Secret Annotation Census
type ProviderLabelResult2372 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByProvider map[string]int `json:"byProvider"`
	} `json:"summary"`
}

func (s *Server) handleProviderLabel2372(w http.ResponseWriter, r *http.Request) {
	result := ProviderLabelResult2372{ScannedAt: time.Now()}
	result.Summary.ByProvider = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		provider := "<unknown>"
		for k := range node.Labels {
			if strings.HasPrefix(k, "node.kubernetes.io/instance-type") || strings.HasPrefix(k, "beta.kubernetes.io/instance-type") {
				provider = node.Labels[k]
				break
			}
		}
		result.Summary.ByProvider[provider]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodEnvCountResult2372 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalEnvVars    int `json:"totalEnvVars"`
	} `json:"summary"`
}

func (s *Server) handlePodEnvCount2372(w http.ResponseWriter, r *http.Request) {
	result := PodEnvCountResult2372{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEnvVars += len(c.Env)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretAnnotResult2372 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets    int `json:"totalSecrets"`
		WithAnnotations int `json:"withAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleSecretAnnot2372(w http.ResponseWriter, r *http.Request) {
	result := SecretAnnotResult2372{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if len(secret.Annotations) > 0 {
			result.Summary.WithAnnotations++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
