package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.13 — Deployment Dimension (Round 55)
// 1. Deployment MaxUnavailable Audit
// 2. StatefulSet Update Strategy Distribution
// 3. ReplicaSet Replicas vs Ready Gap
// ============================================================

type MaxUnavailResult2213 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithCustom   int `json:"withCustomMaxUnavailable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMaxUnavail2213(w http.ResponseWriter, r *http.Request) {
	result := MaxUnavailResult2213{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Update Strategy
type STSUpdStratDistResult2213 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int            `json:"totalStatefulSets"`
		ByStrategy map[string]int `json:"byUpdateStrategy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSUpdStratDist2213(w http.ResponseWriter, r *http.Request) {
	result := STSUpdStratDistResult2213{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByStrategy = make(map[string]int)
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByStrategy[string(sts.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RS Replicas vs Ready
type RSReadyGapResult2213 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS      int   `json:"totalReplicaSets"`
		TotalDesired int32 `json:"totalDesiredReplicas"`
		TotalReady   int32 `json:"totalReadyReplicas"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSReadyGap2213(w http.ResponseWriter, r *http.Request) {
	result := RSReadyGapResult2213{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas == nil || *rs.Spec.Replicas == 0 {
			continue
		}
		result.Summary.TotalRS++
		result.Summary.TotalDesired += *rs.Spec.Replicas
		result.Summary.TotalReady += rs.Status.ReadyReplicas
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
