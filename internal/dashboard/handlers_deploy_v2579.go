package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.79 Deployment: RS Template Hashes, STS Status CurrentReplicas vs Replicas, DS UpdateStrategy Type
type RSTemplateHashResult2579 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		WithHash int `json:"withTemplateHash"`
	}
}

func (s *Server) handleRSTemplateHash2579(w http.ResponseWriter, r *http.Request) {
	result := RSTemplateHashResult2579{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Status.TemplateHash != "" || rs.Labels["pod-template-hash"] != "" {
			result.Summary.WithHash++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSCurVsRepResult2579 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		Mismatched int `json:"mismatched"`
	}
}

func (s *Server) handleSTSCurVsRep2579(w http.ResponseWriter, r *http.Request) {
	result := STSCurVsRepResult2579{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CurrentReplicas != sts.Status.Replicas {
			result.Summary.Mismatched++
		}
	}
	score := 100
	if result.Summary.TotalSTS > 0 {
		score = 100 - (result.Summary.Mismatched*100)/result.Summary.TotalSTS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DSUpdateStrategyResult2579 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int            `json:"totalDS"`
		ByType  map[string]int `json:"byUpdateStrategyType"`
	}
}

func (s *Server) handleDSUpdateStrategy2579(w http.ResponseWriter, r *http.Request) {
	result := DSUpdateStrategyResult2579{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.ByType[string(ds.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
