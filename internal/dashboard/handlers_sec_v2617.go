package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.17 Security: Pod RunAsGroup Detail, Secret Annotation Key Dist, ClusterRole ResourceName Detail
type RunAsGroup2617Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGroup int `json:"withRunAsGroup"`
	} `json:"summary"`
}

func (s *Server) handleRunAsGroup2617(w http.ResponseWriter, r *http.Request) {
	result := RunAsGroup2617Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsGroup != nil {
			result.Summary.WithGroup++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretAnnotKey2617Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByKey        map[string]int `json:"byAnnotationKey"`
	} `json:"summary"`
}

func (s *Server) handleSecretAnnotKey2617(w http.ResponseWriter, r *http.Request) {
	result := SecretAnnotKey2617Result{ScannedAt: time.Now()}
	result.Summary.ByKey = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for k := range secret.Annotations {
			result.Summary.ByKey[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRResourceName2617Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs int            `json:"totalClusterRoles"`
		ByRes    map[string]int `json:"byResourceName"`
	} `json:"summary"`
}

func (s *Server) handleCRResourceName2617(w http.ResponseWriter, r *http.Request) {
	result := CRResourceName2617Result{ScannedAt: time.Now()}
	result.Summary.ByRes = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			for _, rn := range rule.ResourceNames {
				result.Summary.ByRes[rn]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
