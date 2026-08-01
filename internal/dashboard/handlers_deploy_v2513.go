package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.13 Deployment: RS ReadyReplicas Ratio, STS Generation vs ObservedGen, DS NumberUnavailable Detail
type RSReadyRatioResult2513 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		TotalReady int `json:"totalReadyReplicas"`
		TotalRep   int `json:"totalReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSReadyRatio2513(w http.ResponseWriter, r *http.Request) {
	result := RSReadyRatioResult2513{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalReady += int(rs.Status.ReadyReplicas)
		if rs.Spec.Replicas != nil {
			result.Summary.TotalRep += int(*rs.Spec.Replicas)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSGenObservedResult2513 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		Mismatched int `json:"genMismatch"`
	} `json:"summary"`
}

func (s *Server) handleSTSGenObserved2513(w http.ResponseWriter, r *http.Request) {
	result := STSGenObservedResult2513{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.ObservedGeneration != sts.Generation {
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

type DSUnavailDetailResult2513 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		UnavailNodes int `json:"unavailableNodes"`
	} `json:"summary"`
}

func (s *Server) handleDSUnavailDetail2513(w http.ResponseWriter, r *http.Request) {
	result := DSUnavailDetailResult2513{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.UnavailNodes += int(ds.Status.NumberUnavailable)
	}
	score := 100
	if result.Summary.UnavailNodes > 0 {
		score = 100 - result.Summary.UnavailNodes*20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
