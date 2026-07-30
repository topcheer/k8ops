package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.04 — Deployment Dimension (Round 37)
// 1. Deployment Strategy Validator
// 2. StatefulSet Service Binding Audit
// 3. DaemonSet Toleration Coverage
// ============================================================

type StrategyValResult2104 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         StrategyValSummary2104 `json:"summary"`
	RiskyStrat      []StrategyValEntry2104 `json:"riskyStrategies"`
	Recommendations []string               `json:"recommendations"`
}

type StrategyValSummary2104 struct {
	TotalDeploys  int `json:"totalDeployments"`
	RollingUpdate int `json:"rollingUpdate"`
	Recreate      int `json:"recreate"`
}

type StrategyValEntry2104 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleStrategyVal2104(w http.ResponseWriter, r *http.Request) {
	result := StrategyValResult2104{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			result.Summary.Recreate++
			result.RiskyStrat = append(result.RiskyStrat, StrategyValEntry2104{Name: dep.Name, Namespace: dep.Namespace})
			score -= 2
		} else {
			result.Summary.RollingUpdate++
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.RiskyStrat, func(i, j int) bool { return result.RiskyStrat[i].Namespace < result.RiskyStrat[j].Namespace })

	if result.Summary.Recreate > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments use Recreate — causes downtime", result.Summary.Recreate))
	}
	writeJSON(w, result)
}

// 2. STS Service Binding
type STSSvcResult2104 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         STSSvcSummary2104 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type STSSvcSummary2104 struct {
	TotalSTS int `json:"totalStatefulSets"`
	WithSvc  int `json:"withHeadlessService"`
}

func (s *Server) handleSTSSvc2104(w http.ResponseWriter, r *http.Request) {
	result := STSSvcResult2104{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName != "" {
			result.Summary.WithSvc++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DaemonSet Toleration Coverage
type DSTolResult2104 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         DSTolSummary2104 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type DSTolSummary2104 struct {
	TotalDS int `json:"totalDaemonSets"`
	WithTol int `json:"withTolerations"`
}

func (s *Server) handleDSTol2104(w http.ResponseWriter, r *http.Request) {
	result := DSTolResult2104{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})

	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if len(ds.Spec.Template.Spec.Tolerations) > 0 {
			result.Summary.WithTol++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
