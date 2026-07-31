package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.41 Security: Pod UIDs Range Audit, Secret DockerConfig, Role Verb Wildcard
type UIDRangeResult2341 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		RootUID         int `json:"rootUIDContainers"`
	} `json:"summary"`
}

func (s *Server) handleUIDRange2341(w http.ResponseWriter, r *http.Request) {
	result := UIDRangeResult2341{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0 {
				result.Summary.RootUID++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.RootUID*50)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DockerConfigResult2341 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets        int `json:"totalSecrets"`
		DockerConfigSecrets int `json:"dockerConfigSecrets"`
	} `json:"summary"`
}

func (s *Server) handleDockerConfig2341(w http.ResponseWriter, r *http.Request) {
	result := DockerConfigResult2341{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			result.Summary.DockerConfigSecrets++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleVerbWildcardResult2341 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles    int `json:"totalRoles"`
		WildcardVerbs int `json:"withWildcardVerbs"`
	} `json:"summary"`
}

func (s *Server) handleRoleVerbWildcard2341(w http.ResponseWriter, r *http.Request) {
	result := RoleVerbWildcardResult2341{ScannedAt: time.Now()}
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	for _, role := range roleList.Items {
		result.Summary.TotalRoles++
		for _, rule := range role.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WildcardVerbs++
					break
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalRoles > 0 && result.Summary.WildcardVerbs > 0 {
		score = 100 - (result.Summary.WildcardVerbs*30)/result.Summary.TotalRoles
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
