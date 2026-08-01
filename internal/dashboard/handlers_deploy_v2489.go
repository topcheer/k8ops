package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.89 Deployment: RS Status Replicas, STS UpdateStrategy Type, DS Status DesiredCount
type RSStatusReplicasResult2489 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		TotalReady int `json:"totalReadyReplicas"`
		TotalAvail int `json:"totalAvailableReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSStatusReplicas2489(w http.ResponseWriter, r *http.Request) {
	result := RSStatusReplicasResult2489{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalReady += int(rs.Status.ReadyReplicas)
		result.Summary.TotalAvail += int(rs.Status.AvailableReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSUpdateStrategyResult2489 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByType   map[string]int `json:"byUpdateStrategyType"`
	} `json:"summary"`
}

func (s *Server) handleSTSUpdateStrategy2489(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateStrategyResult2489{ScannedAt: time.Now()}
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

type DSDesiredCountResult2489 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int `json:"totalDS"`
		Desired int `json:"totalDesiredNumberScheduled"`
		Current int `json:"totalCurrentNumberScheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSDesiredCount2489(w http.ResponseWriter, r *http.Request) {
	result := DSDesiredCountResult2489{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Desired += int(ds.Status.DesiredNumberScheduled)
		result.Summary.Current += int(ds.Status.CurrentNumberScheduled)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
