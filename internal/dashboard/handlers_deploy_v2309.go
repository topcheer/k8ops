package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.09 Deployment: Deployment Observed Generation, RS Template Hash Distribution, CronJob Concurrency
type ObservedGenResult2309 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		InSync       int `json:"generationInSync"`
		OutOfSync    int `json:"generationOutOfSync"`
	} `json:"summary"`
}

func (s *Server) handleObservedGen2309(w http.ResponseWriter, r *http.Request) {
	result := ObservedGenResult2309{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Generation == dep.Status.ObservedGeneration {
			result.Summary.InSync++
		} else {
			result.Summary.OutOfSync++
		}
	}
	score := 100
	if result.Summary.TotalDeploys > 0 {
		score = result.Summary.InSync * 100 / result.Summary.TotalDeploys
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RSTemplateHashResult2309 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS      int            `json:"totalRS"`
		ByHashBucket map[string]int `json:"byHashBucket"`
	} `json:"summary"`
}

func (s *Server) handleRSTemplateHash2309(w http.ResponseWriter, r *http.Request) {
	result := RSTemplateHashResult2309{ScannedAt: time.Now()}
	result.Summary.ByHashBucket = make(map[string]int)
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 {
			continue
		}
		result.Summary.TotalRS++
		hash := rs.Labels["pod-template-hash"]
		if hash == "" {
			hash = "<none>"
		}
		result.Summary.ByHashBucket[hash]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobConcurResult2309 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		ByConcurrency map[string]int `json:"byConcurrencyPolicy"`
	} `json:"summary"`
}

func (s *Server) handleCronJobConcur2309(w http.ResponseWriter, r *http.Request) {
	result := CronJobConcurResult2309{ScannedAt: time.Now()}
	result.Summary.ByConcurrency = make(map[string]int)
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.ByConcurrency[string(cj.Spec.ConcurrencyPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
