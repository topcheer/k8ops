package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.93 Security: Pod ReadOnlyRootFilesystem, Secret OwnerRef UID, RoleBinding Subject Name Detail
type ReadOnlyRootFSResult2593 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnly        int `json:"readOnlyRootFilesystem"`
	}
}

func (s *Server) handleReadOnlyRootFS2593(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyRootFSResult2593{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnly++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.ReadOnly * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretOwnerUIDResult2593 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		WithOwnerUID int `json:"withOwnerUID"`
	}
}

func (s *Server) handleSecretOwnerUID2593(w http.ResponseWriter, r *http.Request) {
	result := SecretOwnerUIDResult2593{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for _, ref := range secret.OwnerReferences {
			if ref.UID != "" {
				result.Summary.WithOwnerUID++
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RBSubjectNameResult2593 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRB int            `json:"totalRoleBindings"`
		ByName  map[string]int `json:"bySubjectName"`
	}
}

func (s *Server) handleRBSubjectName2593(w http.ResponseWriter, r *http.Request) {
	result := RBSubjectNameResult2593{ScannedAt: time.Now()}
	result.Summary.ByName = make(map[string]int)
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.TotalRB++
		for _, subj := range rb.Subjects {
			result.Summary.ByName[subj.Name]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
