package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.96 Product: Pod Completion Index, Container Args Catalog, Service Account Image Pull Secret
type PodCompleteResult2296 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Running   int `json:"running"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		Pending   int `json:"pending"`
	} `json:"summary"`
}

func (s *Server) handlePodComplete2296(w http.ResponseWriter, r *http.Request) {
	result := PodCompleteResult2296{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		switch pod.Status.Phase {
		case corev1.PodRunning:
			result.Summary.Running++
		case corev1.PodSucceeded:
			result.Summary.Succeeded++
		case corev1.PodFailed:
			result.Summary.Failed++
		case corev1.PodPending:
			result.Summary.Pending++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		badPct := (result.Summary.Failed + result.Summary.Pending) * 100 / result.Summary.TotalPods
		score = 100 - badPct
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ArgsCatalogResult2296 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithArgs        int `json:"withArgs"`
		WithCommand     int `json:"withCommand"`
	} `json:"summary"`
}

func (s *Server) handleArgsCatalog2296(w http.ResponseWriter, r *http.Request) {
	result := ArgsCatalogResult2296{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if len(c.Args) > 0 {
				result.Summary.WithArgs++
			}
			if len(c.Command) > 0 {
				result.Summary.WithCommand++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcAcctPullSecretResult2296 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs       int `json:"totalServiceAccounts"`
		WithPullSecret int `json:"withImagePullSecret"`
	} `json:"summary"`
}

func (s *Server) handleSvcAcctPullSecret2296(w http.ResponseWriter, r *http.Request) {
	result := SvcAcctPullSecretResult2296{ScannedAt: time.Now()}
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.ImagePullSecrets) > 0 {
			result.Summary.WithPullSecret++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
