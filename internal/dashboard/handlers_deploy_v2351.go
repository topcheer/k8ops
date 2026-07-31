package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.51 Deployment: Deployment Updated Replicas, STS Current Replicas, RS Full Status
type DepUpdatedRepsResult2351 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int   `json:"totalDeployments"`
		TotalReps    int32 `json:"totalReplicas"`
		TotalUpdated int32 `json:"totalUpdated"`
	} `json:"summary"`
}

func (s *Server) handleDepUpdatedReps2351(w http.ResponseWriter, r *http.Request) {
	result := DepUpdatedRepsResult2351{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.TotalReps += dep.Status.Replicas
		result.Summary.TotalUpdated += dep.Status.UpdatedReplicas
	}
	score := 100
	if result.Summary.TotalReps > 0 {
		score = int(result.Summary.TotalUpdated * 100 / result.Summary.TotalReps)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSCurrentRepsResult2351 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int   `json:"totalSTS"`
		TotalReps  int32 `json:"totalCurrentReplicas"`
		TotalReady int32 `json:"totalReadyReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSCurrentReps2351(w http.ResponseWriter, r *http.Request) {
	result := STSCurrentRepsResult2351{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalReps += sts.Status.Replicas
		result.Summary.TotalReady += sts.Status.ReadyReplicas
	}
	score := 100
	if result.Summary.TotalReps > 0 {
		score = int(result.Summary.TotalReady * 100 / result.Summary.TotalReps)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RSFullStatusResult2351 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int   `json:"totalRS"`
		TotalReps  int32 `json:"totalReplicas"`
		TotalReady int32 `json:"totalReady"`
	} `json:"summary"`
}

func (s *Server) handleRSFullStatus2351(w http.ResponseWriter, r *http.Request) {
	result := RSFullStatusResult2351{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 && len(rs.OwnerReferences) > 0 {
			continue
		}
		result.Summary.TotalRS++
		result.Summary.TotalReps += rs.Status.Replicas
		result.Summary.TotalReady += rs.Status.ReadyReplicas
	}
	score := 100
	if result.Summary.TotalReps > 0 {
		score = int(result.Summary.TotalReady * 100 / result.Summary.TotalReps)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
