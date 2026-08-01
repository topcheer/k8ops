package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.43 Deployment: RS Replicas vs Ready, STS Status UpdateRevision, DS ObservedGeneration vs Generation
type RSReplicasVsReadyResult2543 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		Mismatched int `json:"replicasMismatch"`
	} `json:"summary"`
}

func (s *Server) handleRSReplicasVsReady2543(w http.ResponseWriter, r *http.Request) {
	result := RSReplicasVsReadyResult2543{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if int(rs.Status.Replicas) != int(rs.Status.ReadyReplicas) {
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

type STSUpdateRevResult2543 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithRev  int `json:"withUpdateRevision"`
	} `json:"summary"`
}

func (s *Server) handleSTSUpdateRev2543(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateRevResult2543{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.UpdateRevision != "" {
			result.Summary.WithRev++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSObsVsGenResult2543 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		Mismatched int `json:"genMismatch"`
	} `json:"summary"`
}

func (s *Server) handleDSObsVsGen2543(w http.ResponseWriter, r *http.Request) {
	result := DSObsVsGenResult2543{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Status.ObservedGeneration != ds.Generation {
			result.Summary.Mismatched++
		}
	}
	score := 100
	if result.Summary.TotalDS > 0 {
		score = 100 - (result.Summary.Mismatched*100)/result.Summary.TotalDS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
