package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.27 Security: Pod ProcMount, Secret Age Distribution, RoleBinding Verbs Total
type ProcMountResult2527 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByProcMount     map[string]int `json:"byProcMount"`
	} `json:"summary"`
}

func (s *Server) handleProcMount2527(w http.ResponseWriter, r *http.Request) {
	result := ProcMountResult2527{ScannedAt: time.Now()}
	result.Summary.ByProcMount = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			pm := "Default"
			if c.SecurityContext != nil && c.SecurityContext.ProcMount != nil {
				pm = string(*c.SecurityContext.ProcMount)
			}
			result.Summary.ByProcMount[pm]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretAgeResult2527 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int   `json:"totalSecrets"`
		AvgAgeDays   int64 `json:"avgAgeDays"`
	} `json:"summary"`
}

func (s *Server) handleSecretAge2527(w http.ResponseWriter, r *http.Request) {
	result := SecretAgeResult2527{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	var totalAge float64
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		totalAge += now.Sub(secret.CreationTimestamp.Time).Hours()
	}
	if result.Summary.TotalSecrets > 0 {
		result.Summary.AvgAgeDays = int64(totalAge / float64(result.Summary.TotalSecrets) / 24)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBVerbsTotalResult2527 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB    int `json:"totalRoleBindings"`
		TotalVerbs int `json:"totalVerbs"`
	} `json:"summary"`
}

func (s *Server) handleRBVerbsTotal2527(w http.ResponseWriter, r *http.Request) {
	result := RBVerbsTotalResult2527{ScannedAt: time.Now()}
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for range rbList.Items {
		result.Summary.TotalRB++
	}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		for _, rule := range cr.Rules {
			result.Summary.TotalVerbs += len(rule.Verbs)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
