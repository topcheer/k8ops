package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.80 — Deployment Dimension (Round 33)
// 1. StatefulSet VolumeClaim Template Audit
// 2. DaemonSet Update Compliance
// 3. Deployment History Depth
// ============================================================

type VCClaimResult2080 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         VCClaimSummary2080 `json:"summary"`
	Issues          []VCClaimEntry2080 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type VCClaimSummary2080 struct {
	TotalSTS    int `json:"totalStatefulSets"`
	WithVCTempl int `json:"withVolumeClaimTemplates"`
	MissingVCT  int `json:"missingVolumeClaimTemplates"`
}

type VCClaimEntry2080 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleVCClaimAudit2080(w http.ResponseWriter, r *http.Request) {
	result := VCClaimResult2080{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			result.Summary.WithVCTempl++
		} else {
			result.Summary.MissingVCT++
			result.Issues = append(result.Issues, VCClaimEntry2080{Name: sts.Name, Namespace: sts.Namespace})
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Namespace < result.Issues[j].Namespace })

	if result.Summary.MissingVCT > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d StatefulSets without volumeClaimTemplates", result.Summary.MissingVCT))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. DaemonSet Update Compliance
// ---------------------------------------------------------------

type DSUpdateResult2080 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         DSUpdateSummary2080 `json:"summary"`
	StaleDS         []DSUpdateEntry2080 `json:"staleDaemonSets"`
	Recommendations []string            `json:"recommendations"`
}

type DSUpdateSummary2080 struct {
	TotalDS int `json:"totalDaemonSets"`
	Updated int `json:"fullyUpdated"`
	Stale   int `json:"stale"`
}

type DSUpdateEntry2080 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Desired   int32  `json:"desiredNumberScheduled"`
	Updated   int32  `json:"updatedNumberScheduled"`
}

func (s *Server) handleDSUpdate2080(w http.ResponseWriter, r *http.Request) {
	result := DSUpdateResult2080{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})

	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		desired := ds.Status.DesiredNumberScheduled
		updated := ds.Status.UpdatedNumberScheduled

		if updated >= desired && desired > 0 {
			result.Summary.Updated++
		} else if desired > 0 {
			result.Summary.Stale++
			result.StaleDS = append(result.StaleDS, DSUpdateEntry2080{
				Name: ds.Name, Namespace: ds.Namespace, Desired: desired, Updated: updated,
			})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Stale > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d DaemonSets not fully updated", result.Summary.Stale))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Deployment History Depth
// ---------------------------------------------------------------

type HistDepthResult2080 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HistDepthSummary2080 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type HistDepthSummary2080 struct {
	TotalDeploys int `json:"totalDeployments"`
	DefaultLimit int `json:"defaultRevisionHistory"`
	CustomLimit  int `json:"customRevisionHistory"`
}

func (s *Server) handleHistDepth2080(w http.ResponseWriter, r *http.Request) {
	result := HistDepthResult2080{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.RevisionHistoryLimit != nil {
			result.Summary.CustomLimit++
		} else {
			result.Summary.DefaultLimit++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// keep imports
var _ = appsv1.Deployment{}
var _ = corev1.Pod{}
