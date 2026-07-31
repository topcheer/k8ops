package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.49 — Deployment Dimension (Round 61)
// 1. Deployment Condition Status Tracker
// 2. StatefulSet MinReady Seconds Audit
// 3. DaemonSet Deprecated Field Detector
// ============================================================

type DepCondStatusResult2249 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Progressing  int `json:"progressing"`
		Available    int `json:"available"`
		ReplicaFail  int `json:"replicaFailure"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDepCondStatus2249(w http.ResponseWriter, r *http.Request) {
	result := DepCondStatusResult2249{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		for _, cond := range dep.Status.Conditions {
			if cond.Status == "True" {
				switch string(cond.Type) {
				case "Progressing":
					result.Summary.Progressing++
				case "Available":
					result.Summary.Available++
				case "ReplicaFailure":
					result.Summary.ReplicaFail++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS MinReady Seconds
type STSMinReadyResult2249 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalStatefulSets"`
		WithCustom int `json:"withCustomMinReadySeconds"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSMinReady2249(w http.ResponseWriter, r *http.Request) {
	result := STSMinReadyResult2249{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.MinReadySeconds > 0 {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Deprecated Field Detector
type DSDeprecatedResult2249 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS        int `json:"totalDaemonSets"`
		WithDeprecated int `json:"withDeprecatedFields"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSDeprecated2249(w http.ResponseWriter, r *http.Request) {
	result := DSDeprecatedResult2249{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.UpdateStrategy.Type == "OnDelete" {
			result.Summary.WithDeprecated++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
