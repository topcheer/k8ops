package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.37 Deployment: RS Label Selector, STS Status CurrentRevision, DS Status UpdatedNumber vs DesiredNumber
type RSLabelSelectorResult2537 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS      int `json:"totalRS"`
		WithSelector int `json:"withLabelSelector"`
	} `json:"summary"`
}

func (s *Server) handleRSLabelSelector2537(w http.ResponseWriter, r *http.Request) {
	result := RSLabelSelectorResult2537{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.Spec.Selector.MatchLabels) > 0 {
			result.Summary.WithSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSCurrentRevResult2537 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithRev  int `json:"withCurrentRevision"`
	} `json:"summary"`
}

func (s *Server) handleSTSCurrentRev2537(w http.ResponseWriter, r *http.Request) {
	result := STSCurrentRevResult2537{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CurrentRevision != "" {
			result.Summary.WithRev++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdatedVsDesiredResult2537 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int `json:"totalDS"`
		Updated int `json:"totalUpdated"`
		Desired int `json:"totalDesired"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdatedVsDesired2537(w http.ResponseWriter, r *http.Request) {
	result := DSUpdatedVsDesiredResult2537{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Updated += int(ds.Status.UpdatedNumberScheduled)
		result.Summary.Desired += int(ds.Status.DesiredNumberScheduled)
	}
	score := 100
	if result.Summary.Desired > 0 && result.Summary.Updated < result.Summary.Desired {
		score = result.Summary.Updated * 100 / result.Summary.Desired
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
