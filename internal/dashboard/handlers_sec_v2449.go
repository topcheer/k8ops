package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.49 Security: Pod HostPID Ratio, Secret DockerConfigJson Count, ClusterRoleBinding Subject Kind
type HostPIDResult2449 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostPID   int `json:"hostPIDPods"`
	} `json:"summary"`
}

func (s *Server) handleHostPID2449(w http.ResponseWriter, r *http.Request) {
	result := HostPIDResult2449{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostPID {
			result.Summary.HostPID++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.HostPID > 0 {
		score = 100 - (result.Summary.HostPID*100)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DockerConfigJsonResult2449 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		DockerConfig int `json:"dockerConfigJsonCount"`
	} `json:"summary"`
}

func (s *Server) handleDockerConfigJson2449(w http.ResponseWriter, r *http.Request) {
	result := DockerConfigJsonResult2449{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			result.Summary.DockerConfig++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRBSubjectKindResult2449 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRBs int            `json:"totalClusterRoleBindings"`
		ByKind    map[string]int `json:"bySubjectKind"`
	} `json:"summary"`
}

func (s *Server) handleCRBSubjectKind2449(w http.ResponseWriter, r *http.Request) {
	result := CRBSubjectKindResult2449{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		for _, subj := range crb.Subjects {
			result.Summary.ByKind[subj.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
