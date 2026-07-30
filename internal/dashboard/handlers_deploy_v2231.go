package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.31 — Deployment Dimension (Round 58)
// 1. Deployment Selector Complexity Score
// 2. StatefulSet Persistent Volume Retain Policy
// 3. ReplicaSet Template Generation Lag
// ============================================================

type SelectorComplexityResult2231 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		AvgLabels    int `json:"avgSelectorLabels"`
		MaxLabels    int `json:"maxSelectorLabels"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSelectorComplexity2231(w http.ResponseWriter, r *http.Request) {
	result := SelectorComplexityResult2231{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	totalLabels := 0
	maxLabels := 0
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		cnt := len(dep.Spec.Selector.MatchLabels)
		totalLabels += cnt
		if cnt > maxLabels {
			maxLabels = cnt
		}
	}
	if result.Summary.TotalDeploys > 0 {
		result.Summary.AvgLabels = totalLabels / result.Summary.TotalDeploys
	}
	result.Summary.MaxLabels = maxLabels
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS PV Retain Policy
type STSPVRetainResult2231 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalStatefulSets"`
		ByPolicy map[string]int `json:"byPersistentVolumeRetentionPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSPVRetain2231(w http.ResponseWriter, r *http.Request) {
	result := STSPVRetainResult2231{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPolicy = make(map[string]int)
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.PersistentVolumeClaimRetentionPolicy != nil {
			result.Summary.ByPolicy[string(sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted)]++
		} else {
			result.Summary.ByPolicy["default"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RS Template Gen Lag
type RSTmplLagResult2231 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS       int   `json:"totalReplicaSets"`
		TotalObserved int64 `json:"totalObservedGeneration"`
		WithLag       int   `json:"withGenerationLag"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSTmplLag2231(w http.ResponseWriter, r *http.Request) {
	result := RSTmplLagResult2231{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalObserved += rs.Status.ObservedGeneration
		if rs.Generation != rs.Status.ObservedGeneration {
			result.Summary.WithLag++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
