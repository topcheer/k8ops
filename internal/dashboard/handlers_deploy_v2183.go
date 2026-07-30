package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.83 — Deployment Dimension (Round 50)
// 1. Deployment Status Collision Tracker
// 2. StatefulSet Replicas Ready Gap
// 3. DaemonSet Node Ready Coverage
// ============================================================

type DepCollisionResult2183 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int `json:"totalDeployments"`
		WithCollision int `json:"withCollision"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDepCollision2183(w http.ResponseWriter, r *http.Request) {
	result := DepCollisionResult2183{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nameSet := make(map[string]int)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		nameSet[dep.Namespace+"/"+dep.Name]++
	}
	for _, cnt := range nameSet {
		if cnt > 1 {
			result.Summary.WithCollision++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Replicas Ready Gap
type STSReadyGapResult2183 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS     int `json:"totalStatefulSets"`
		FullyReady   int `json:"fullyReady"`
		PartialReady int `json:"partialReady"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSReadyGap2183(w http.ResponseWriter, r *http.Request) {
	result := STSReadyGapResult2183{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if sts.Status.ReadyReplicas >= replicas {
			result.Summary.FullyReady++
		} else if sts.Status.ReadyReplicas > 0 {
			result.Summary.PartialReady++
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Node Ready Coverage
type DSNodeCovResult2183 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS        int `json:"totalDaemonSets"`
		FullyScheduled int `json:"fullyScheduled"`
		NumberReady    int `json:"numberReady"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSNodeCov2183(w http.ResponseWriter, r *http.Request) {
	result := DSNodeCovResult2183{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberReady >= ds.Status.DesiredNumberScheduled {
			result.Summary.FullyScheduled++
		}
		result.Summary.NumberReady += int(ds.Status.NumberReady)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
