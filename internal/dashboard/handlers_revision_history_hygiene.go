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

// RevisionHistoryResult audits deployment revision history limit and cleanup status.
type RevisionHistoryResult struct {
	ScannedAt           time.Time              `json:"scannedAt"`
	Summary             RevisionHistorySummary `json:"summary"`
	ByWorkload          []RevisionHistEntry    `json:"byWorkload"`
	WastefulDeployments []RevisionHistEntry    `json:"wastefulDeployments"`
	HealthScore         int                    `json:"healthScore"`
	Grade               string                 `json:"grade"`
	Recommendations     []string               `json:"recommendations"`
}

type RevisionHistorySummary struct {
	TotalDeployments  int `json:"totalDeployments"`
	DefaultLimit      int `json:"defaultHistoryLimit"`
	HighHistoryDeploy int `json:"highHistoryDeployments"`
	NoHistoryLimit    int `json:"noHistoryLimit"`
	TotalReplicaSets  int `json:"totalReplicaSets"`
	OrphanedRS        int `json:"orphanedReplicaSets"`
}

type RevisionHistEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	HistoryLimit    int32  `json:"revisionHistoryLimit"`
	ReplicaSetCount int    `json:"replicaSetCount"`
	OldReplicaSets  int    `json:"oldReplicaSets"`
	WastedRS        int    `json:"wastedReplicaSets"`
	RiskLevel       string `json:"riskLevel"`
}

// handleRevisionHistoryHygiene handles GET /api/deployment/revision-history-hygiene
func (s *Server) handleRevisionHistoryHygiene(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RevisionHistoryResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	rss, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})

	// Build RS count by deployment
	rsByDeploy := make(map[string]int) // ns/deploy -> count of old RS
	for _, rs := range rss.Items {
		if rs.Status.Replicas == 0 && rs.Status.ReadyReplicas == 0 {
			// Old RS, check owner
			for _, ref := range rs.OwnerReferences {
				if ref.Kind == "Deployment" {
					key := rs.Namespace + "/" + ref.Name
					rsByDeploy[key]++
					result.Summary.TotalReplicaSets++
				}
			}
		}
	}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		limit := int32(10) // default
		if dep.Spec.RevisionHistoryLimit != nil {
			limit = *dep.Spec.RevisionHistoryLimit
		}

		key := dep.Namespace + "/" + dep.Name
		oldRS := rsByDeploy[key]

		entry := RevisionHistEntry{
			Name:            dep.Name,
			Namespace:       dep.Namespace,
			HistoryLimit:    limit,
			ReplicaSetCount: oldRS,
			OldReplicaSets:  oldRS,
			WastedRS:        0,
		}

		if limit > 10 {
			result.Summary.HighHistoryDeploy++
			entry.WastedRS = oldRS - 10
			if entry.WastedRS < 0 {
				entry.WastedRS = 0
			}
		}
		if limit == 0 {
			result.Summary.NoHistoryLimit++
		}

		// Default revision limit in result
		if result.Summary.DefaultLimit == 0 {
			result.Summary.DefaultLimit = int(limit)
		}

		switch {
		case limit == 0:
			entry.RiskLevel = "critical"
			result.Summary.OrphanedRS += oldRS
		case limit > 20:
			entry.RiskLevel = "high"
			result.Summary.OrphanedRS += entry.WastedRS
		case limit > 10:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.WastefulDeployments = append(result.WastefulDeployments, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalDeployments > 0 {
		clean := result.Summary.TotalDeployments - len(result.WastefulDeployments)
		result.HealthScore = clean * 100 / result.Summary.TotalDeployments
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("修订历史审计: %d 部署, %d 高历史限制, %d 无限制, %d 旧 ReplicaSet",
			result.Summary.TotalDeployments, result.Summary.HighHistoryDeploy,
			result.Summary.NoHistoryLimit, result.Summary.TotalReplicaSets),
	}
	if result.Summary.OrphanedRS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个旧 ReplicaSet 可清理", result.Summary.OrphanedRS))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 设置 revisionHistoryLimit=10, 清理旧 ReplicaSet")
	}
	writeJSON(w, result)
}

// Used to avoid unused import
var _ appsv1.DeploymentSpec
var _ corev1.PodSpec
