package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.01 — Deployment Dimension (Round 53)
// 1. Deployment Revision History Audit
// 2. StatefulSet Pod Management Policy Distribution
// 3. DaemonSet Update Revision Status
// ============================================================

type RevHistAuditResult2201 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithLimit    int `json:"withRevisionHistoryLimit"`
		WithoutLimit int `json:"withoutRevisionHistoryLimit"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRevHistAudit2201(w http.ResponseWriter, r *http.Request) {
	result := RevHistAuditResult2201{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.RevisionHistoryLimit != nil {
			result.Summary.WithLimit++
		} else {
			result.Summary.WithoutLimit++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Pod Mgmt Policy Distribution
type STSPodMgmtDistResult2201 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalStatefulSets"`
		ByPolicy map[string]int `json:"byPodManagementPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSPodMgmtDist2201(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtDistResult2201{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPolicy = make(map[string]int)
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByPolicy[string(sts.Spec.PodManagementPolicy)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Update Revision Status
type DSRevStatusResult2201 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int   `json:"totalDaemonSets"`
		UpdatedNum int32 `json:"updatedNumberScheduled"`
		DesiredNum int32 `json:"desiredNumberScheduled"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSRevStatus2201(w http.ResponseWriter, r *http.Request) {
	result := DSRevStatusResult2201{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.UpdatedNum += ds.Status.UpdatedNumberScheduled
		result.Summary.DesiredNum += ds.Status.DesiredNumberScheduled
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
