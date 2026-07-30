package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.71 — Deployment Dimension (Round 48)
// 1. Deployment Available Replicas Ratio
// 2. StatefulSet Partition Update Status
// 3. DaemonSet Rollout Status
// ============================================================

type AvailReplicaResult2171 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys   int `json:"totalDeployments"`
		FullyAvailable int `json:"fullyAvailable"`
		Partial        int `json:"partiallyAvailable"`
		Unavailable    int `json:"unavailable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAvailReplica2171(w http.ResponseWriter, r *http.Request) {
	result := AvailReplicaResult2171{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if dep.Status.AvailableReplicas >= replicas && replicas > 0 {
			result.Summary.FullyAvailable++
		} else if dep.Status.AvailableReplicas > 0 {
			result.Summary.Partial++
			score -= 2
		} else {
			result.Summary.Unavailable++
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Partition Update
type STSPartitionResult2171 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int `json:"totalStatefulSets"`
		WithPartition int `json:"withPartition"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSPartition2171(w http.ResponseWriter, r *http.Request) {
	result := STSPartitionResult2171{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			result.Summary.WithPartition++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Rollout Status
type DSRolloutResult2171 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS            int `json:"totalDaemonSets"`
		FullyScheduled     int `json:"fullyScheduled"`
		PartiallyScheduled int `json:"partiallyScheduled"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSRollout2171(w http.ResponseWriter, r *http.Request) {
	result := DSRolloutResult2171{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Status.DesiredNumberScheduled > 0 {
			if ds.Status.CurrentNumberScheduled >= ds.Status.DesiredNumberScheduled {
				result.Summary.FullyScheduled++
			} else {
				result.Summary.PartiallyScheduled++
				score -= 5
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
