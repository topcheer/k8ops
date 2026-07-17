package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RevisionDriftResult detects configuration drift between ReplicaSets
// of the same Deployment. When multiple ReplicaSets retain old pod templates,
// it indicates rollout history accumulation or failed rollbacks.
type RevisionDriftResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         DriftSummary      `json:"summary"`
	ByDeployment    []DriftDeployment `json:"byDeployment"`
	OldRevisions    []DriftRevision   `json:"oldRevisions"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type DriftSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithMultipleRS   int `json:"withMultipleRevisions"`
	MaxRevisions     int `json:"maxRevisions"`
	OldReplicaSets   int `json:"oldReplicaSets"`
	PodsInOldRevs    int `json:"podsInOldRevisions"`
	DriftDetected    int `json:"driftDetected"`
}

type DriftDeployment struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	RevisionCount int    `json:"revisionCount"`
	ActiveRS      string `json:"activeRevision"`
	OldRSCount    int    `json:"oldRevisionCount"`
	PodsInOld     int    `json:"podsInOldRevisions"`
	HasDrift      bool   `json:"hasDrift"`
}

type DriftRevision struct {
	Deployment string `json:"deployment"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Pods       int    `json:"podCount"`
	Age        string `json:"age"`
}

// handleRevisionDrift handles GET /api/deployment/revision-drift
func (s *Server) handleRevisionDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	replicaSets, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})

	// Group ReplicaSets by owner deployment
	rsByOwner := make(map[string][]appsv1.ReplicaSet)
	for _, rs := range replicaSets.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" {
				key := rs.Namespace + "/" + ref.Name
				rsByOwner[key] = append(rsByOwner[key], rs)
			}
		}
	}

	result := RevisionDriftResult{ScannedAt: time.Now()}
	var oldRevs []DriftRevision

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		key := d.Namespace + "/" + d.Name
		rsList := rsByOwner[key]

		if len(rsList) <= 1 {
			continue
		}

		result.Summary.WithMultipleRS++
		if len(rsList) > result.Summary.MaxRevisions {
			result.Summary.MaxRevisions = len(rsList)
		}

		entry := DriftDeployment{
			Name: d.Name, Namespace: d.Namespace,
			RevisionCount: len(rsList),
		}

		// Find active (has replicas) and old ReplicaSets
		activeRev := ""
		podsInOld := 0
		oldCount := 0
		hasDrift := false

		for _, rs := range rsList {
			rsPods := int(ptrInt32(rs.Spec.Replicas))
			rev := rs.Annotations["deployment.kubernetes.io/revision"]
			if rev == "" {
				rev = fmt.Sprintf("rs-%s", rs.Name)
			}
			if rsPods > 0 {
				activeRev = rev
			} else {
				oldCount++
				result.Summary.OldReplicaSets++

				// Check if old RS has pods running (indicates drift)
				readyPods := int(rs.Status.ReadyReplicas)
				if readyPods > 0 {
					podsInOld += readyPods
					result.Summary.PodsInOldRevs += readyPods
					hasDrift = true
				}

				oldRevs = append(oldRevs, DriftRevision{
					Deployment: d.Name, Namespace: d.Namespace,
					Revision: rev, Pods: readyPods,
					Age: svcAge(rs.CreationTimestamp.Time),
				})
			}
		}

		entry.ActiveRS = activeRev
		entry.OldRSCount = oldCount
		entry.PodsInOld = podsInOld
		entry.HasDrift = hasDrift

		if hasDrift {
			result.Summary.DriftDetected++
		}

		result.ByDeployment = append(result.ByDeployment, entry)
	}

	sort.Slice(result.ByDeployment, func(i, j int) bool {
		return result.ByDeployment[i].RevisionCount > result.ByDeployment[j].RevisionCount
	})

	sort.Slice(oldRevs, func(i, j int) bool {
		return oldRevs[i].Pods > oldRevs[i].Pods
	})
	result.OldRevisions = oldRevs

	// Score
	if result.Summary.TotalDeployments > 0 {
		cleanDeployments := result.Summary.TotalDeployments - result.Summary.DriftDetected
		result.HealthScore = cleanDeployments * 100 / result.Summary.TotalDeployments
		// Penalty for too many old ReplicaSets
		if result.Summary.OldReplicaSets > result.Summary.TotalDeployments*2 {
			result.HealthScore -= 10
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 90:
		result.Grade = "A"
	case result.HealthScore >= 75:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildDriftRecs(&result)
	writeJSON(w, result)
}

func buildDriftRecs(r *RevisionDriftResult) []string {
	recs := []string{}
	if r.Summary.DriftDetected > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Deployment 存在版本漂移（旧 RS 仍有 Pod 运行）", r.Summary.DriftDetected))
	}
	if r.Summary.OldReplicaSets > 20 {
		recs = append(recs, fmt.Sprintf("%d 个旧 ReplicaSet，建议设置 revisionHistoryLimit=5", r.Summary.OldReplicaSets))
	}
	if r.Summary.PodsInOldRevs > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 运行在旧版本上，可能需要回滚或前进", r.Summary.PodsInOldRevs))
	}
	if r.Summary.MaxRevisions > 15 {
		recs = append(recs, fmt.Sprintf("单个 Deployment 最多有 %d 个版本历史", r.Summary.MaxRevisions))
	}
	if len(recs) == 0 {
		recs = append(recs, "版本历史管理良好，无漂移")
	}
	return recs
}
