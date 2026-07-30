package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.77 — Deployment Dimension (Round 49)
// 1. Deployment Updated Replicas Gap
// 2. StatefulSet VolumeClaim Template Audit
// 3. ReplicaSet Generation Gap
// ============================================================

type UpdatedReplicaResult2177 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int `json:"totalDeployments"`
		FullyUpdated  int `json:"fullyUpdated"`
		PartialUpdate int `json:"partiallyUpdated"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleUpdatedReplica2177(w http.ResponseWriter, r *http.Request) {
	result := UpdatedReplicaResult2177{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if dep.Status.UpdatedReplicas >= replicas {
			result.Summary.FullyUpdated++
		} else if dep.Status.UpdatedReplicas > 0 {
			result.Summary.PartialUpdate++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS VolumeClaim Template
type STSVolClaimResult2177 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS     int `json:"totalStatefulSets"`
		WithVolClaim int `json:"withVolumeClaimTemplate"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSVolClaim2177(w http.ResponseWriter, r *http.Request) {
	result := STSVolClaimResult2177{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			result.Summary.WithVolClaim++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ReplicaSet Generation Gap
type RSGenerationResult2177 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS       int   `json:"totalReplicaSets"`
		MaxGeneration int64 `json:"maxGeneration"`
		StaleRS       int   `json:"staleReplicaSets"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSGeneration2177(w http.ResponseWriter, r *http.Request) {
	result := RSGenerationResult2177{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Generation > result.Summary.MaxGeneration {
			result.Summary.MaxGeneration = rs.Generation
		}
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 && rs.Status.Replicas == 0 {
			result.Summary.StaleRS++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
