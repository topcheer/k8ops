package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.61 Deployment: RS Paused Status, STS Partition, DS DeletionTimestamp
type RSPausedResult2561 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS int `json:"totalRS"`
		Paused  int `json:"pausedRS"`
	}
}

func (s *Server) handleRSPaused2561(w http.ResponseWriter, r *http.Request) {
	result := RSPausedResult2561{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Spec.Paused {
			result.Summary.Paused++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPartitionResult2561 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithPart int `json:"withPartition"`
	}
}

func (s *Server) handleSTSPartition2561(w http.ResponseWriter, r *http.Request) {
	result := STSPartitionResult2561{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			if *sts.Spec.UpdateStrategy.RollingUpdate.Partition > 0 {
				result.Summary.WithPart++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSDeletionResult2561 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS  int `json:"totalDS"`
		Deleting int `json:"deletingDS"`
	}
}

func (s *Server) handleDSDeletion2561(w http.ResponseWriter, r *http.Request) {
	result := DSDeletionResult2561{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.DeletionTimestamp != nil {
			result.Summary.Deleting++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
