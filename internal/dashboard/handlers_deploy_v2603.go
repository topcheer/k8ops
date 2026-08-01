package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.03 Deployment: RS Spec Replicas vs Status, STS Spec UpdateStrategy Detail, DS Spec MinReadySeconds
type RSSpecVsStatus2603Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		Mismatched int `json:"specVsStatusMismatch"`
	} `json:"summary"`
}

func (s *Server) handleRSSpecVsStatus2603(w http.ResponseWriter, r *http.Request) {
	result := RSSpecVsStatus2603Result{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		specRep := int32(0)
		if rs.Spec.Replicas != nil {
			specRep = *rs.Spec.Replicas
		}
		if specRep != rs.Status.Replicas {
			result.Summary.Mismatched++
		}
	}
	score := 100
	if result.Summary.TotalRS > 0 {
		score = 100 - (result.Summary.Mismatched*100)/result.Summary.TotalRS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSStrategyDetail2603Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByType   map[string]int `json:"byStrategyType"`
	} `json:"summary"`
}

func (s *Server) handleSTSStrategyDetail2603(w http.ResponseWriter, r *http.Request) {
	result := STSStrategyDetail2603Result{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByType[string(sts.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSMinReady2603Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		CustomReady int `json:"customMinReady"`
	} `json:"summary"`
}

func (s *Server) handleDSMinReady2603(w http.ResponseWriter, r *http.Request) {
	result := DSMinReady2603Result{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.MinReadySeconds > 0 {
			result.Summary.CustomReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
