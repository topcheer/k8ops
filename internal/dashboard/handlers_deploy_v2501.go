package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.01 Deployment: RS ObservedGeneration, STS Status CollisionCount, DS UpdatedNumberScheduled
type RSObservedGenResult2501 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		TotalOG    int `json:"totalObservedGeneration"`
		Mismatched int `json:"generationMismatch"`
	} `json:"summary"`
}

func (s *Server) handleRSObservedGen2501(w http.ResponseWriter, r *http.Request) {
	result := RSObservedGenResult2501{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalOG += int(rs.Status.ObservedGeneration)
		if rs.Status.ObservedGeneration != rs.Generation {
			result.Summary.Mismatched++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSCollisionResult2501 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithColl int `json:"withCollisionCount"`
	} `json:"summary"`
}

func (s *Server) handleSTSCollision2501(w http.ResponseWriter, r *http.Request) {
	result := STSCollisionResult2501{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CollisionCount != nil && *sts.Status.CollisionCount > 0 {
			result.Summary.WithColl++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdatedNumberResult2501 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int `json:"totalDS"`
		Updated int `json:"totalUpdatedNumberScheduled"`
		Desired int `json:"totalDesiredNumberScheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdatedNumber2501(w http.ResponseWriter, r *http.Request) {
	result := DSUpdatedNumberResult2501{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Updated += int(ds.Status.UpdatedNumberScheduled)
		result.Summary.Desired += int(ds.Status.DesiredNumberScheduled)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
