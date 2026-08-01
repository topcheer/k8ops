package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.07 Deployment: RS Template Generation, STS Replicas vs Ready, DS NumberUnavailable
type RSTemplateGenResult2507 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		TotalGen int `json:"totalGeneration"`
	} `json:"summary"`
}

func (s *Server) handleRSTemplateGen2507(w http.ResponseWriter, r *http.Request) {
	result := RSTemplateGenResult2507{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalGen += int(rs.Generation)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSReplicasVsReadyResult2507 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int `json:"totalSTS"`
		TotalReplicas int `json:"totalDesiredReplicas"`
		TotalReady    int `json:"totalReadyReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSReplicasVsReady2507(w http.ResponseWriter, r *http.Request) {
	result := STSReplicasVsReadyResult2507{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReplicas += int(*sts.Spec.Replicas)
		}
		result.Summary.TotalReady += int(sts.Status.ReadyReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNumUnavailableResult2507 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		Unavailable  int `json:"totalNumberUnavailable"`
		Misscheduled int `json:"totalNumberMisscheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSNumUnavailable2507(w http.ResponseWriter, r *http.Request) {
	result := DSNumUnavailableResult2507{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Unavailable += int(ds.Status.NumberUnavailable)
		result.Summary.Misscheduled += int(ds.Status.NumberMisscheduled)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
