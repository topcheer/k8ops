package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.59 Security: Pod RunAsGroup Audit, Secret SSHKey, Role API Groups Census
type RunAsGroupResult2359 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithGroupSet int `json:"withRunAsGroup"`
	} `json:"summary"`
}

func (s *Server) handleRunAsGroup2359(w http.ResponseWriter, r *http.Request) {
	result := RunAsGroupResult2359{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsGroup != nil {
			result.Summary.WithGroupSet++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SSHKeyResult2359 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		SSHKeys      int `json:"sshKeySecrets"`
	} `json:"summary"`
}

func (s *Server) handleSSHKey2359(w http.ResponseWriter, r *http.Request) {
	result := SSHKeyResult2359{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeSSHAuth {
			result.Summary.SSHKeys++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RoleAPIGroupsResult2359 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRoles int            `json:"totalRoles"`
		ByGroup    map[string]int `json:"byAPIGroup"`
	} `json:"summary"`
}

func (s *Server) handleRoleAPIGroups2359(w http.ResponseWriter, r *http.Request) {
	result := RoleAPIGroupsResult2359{ScannedAt: time.Now()}
	result.Summary.ByGroup = make(map[string]int)
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	for _, role := range roleList.Items {
		result.Summary.TotalRoles++
		for _, rule := range role.Rules {
			for _, grp := range rule.APIGroups {
				result.Summary.ByGroup[grp]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
