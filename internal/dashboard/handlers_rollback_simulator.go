package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RollbackSimulatorResult simulates the impact of rolling back each deployment
// to its previous revision. It identifies rollback risks, data loss potential,
// and provides a safe rollback checklist.
type RollbackSimulatorResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         RollbackSimSummary `json:"summary"`
	Simulations     []RollbackSimEntry `json:"simulations"`
	RiskyRollbacks  []RollbackSimEntry `json:"riskyRollbacks"`
	SafeRollbacks   []RollbackSimEntry `json:"safeRollbacks"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type RollbackSimSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithHistory      int `json:"withRollbackHistory"`
	RiskyRollbacks   int `json:"riskyRollbacks"`
	SafeRollbacks    int `json:"safeRollbacks"`
	NoHistory        int `json:"noHistoryAvailable"`
}

type RollbackSimEntry struct {
	Workload      string   `json:"workload"`
	Namespace     string   `json:"namespace"`
	RevisionCount int      `json:"revisionCount"`
	ImageChanged  bool     `json:"imageChanged"`
	RiskLevel     string   `json:"riskLevel"`
	RiskScore     int      `json:"riskScore"`
	Risks         []string `json:"risks"`
	HasPVC        bool     `json:"hasPVC"`
	HasHPA        bool     `json:"hasHPA"`
	Command       string   `json:"rollbackCommand"`
}

// handleRollbackSimulator handles GET /api/deployment/rollback-simulator
func (s *Server) handleRollbackSimulator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RollbackSimulatorResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	replicaSets, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// Build maps
	rsByOwner := make(map[string]int) // ns/name -> RS count
	hpaMap := make(map[string]bool)
	for _, hpa := range hpas.Items {
		hpaMap[hpa.Namespace+"/"+hpa.Spec.ScaleTargetRef.Name] = true
	}
	for _, rs := range replicaSets.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" {
				rsByOwner[rs.Namespace+"/"+ref.Name]++
			}
		}
	}

	var sims []RollbackSimEntry
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		key := d.Namespace + "/" + d.Name
		revCount := rsByOwner[key]

		entry := RollbackSimEntry{
			Workload: d.Name, Namespace: d.Namespace,
			RevisionCount: revCount,
			Command:       fmt.Sprintf("kubectl rollout undo deployment/%s -n %s", d.Name, d.Namespace),
		}

		// Check for PVCs (data loss risk)
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				entry.HasPVC = true
				entry.Risks = append(entry.Risks, "PVC 数据可能在版本回滚后不兼容")
				break
			}
		}

		// Check for HPA (scale mismatch risk)
		if hpaMap[key] {
			entry.HasHPA = true
			entry.Risks = append(entry.Risks, "HPA 可能与新版本副本数不匹配")
		}

		// Check revision history
		if revCount < 2 {
			entry.RiskLevel = "unknown"
			entry.Risks = append(entry.Risks, "无历史版本可供回滚")
			result.Summary.NoHistory++
		} else {
			result.Summary.WithHistory++

			// Risk scoring
			riskScore := 0
			if entry.HasPVC {
				riskScore += 40
			}
			if entry.HasHPA {
				riskScore += 20
			}
			if revCount > 10 {
				riskScore += 10 // Too many revisions, rollback target unclear
				entry.Risks = append(entry.Risks, "版本历史过多，回滚目标不确定")
			}

			// Check if image changed between revisions (heuristic: check annotations)
			revAnnotation := d.Annotations["deployment.kubernetes.io/revision"]
			if revAnnotation != "" {
				entry.ImageChanged = true // Assume changed if annotated
			}

			entry.RiskScore = riskScore

			switch {
			case riskScore >= 40:
				entry.RiskLevel = "high"
				result.Summary.RiskyRollbacks++
				result.RiskyRollbacks = append(result.RiskyRollbacks, entry)
			case riskScore >= 20:
				entry.RiskLevel = "medium"
			default:
				entry.RiskLevel = "low"
				result.Summary.SafeRollbacks++
				result.SafeRollbacks = append(result.SafeRollbacks, entry)
			}
		}

		sims = append(sims, entry)
	}

	sort.Slice(sims, func(i, j int) bool {
		return sims[i].RiskScore > sims[j].RiskScore
	})
	result.Simulations = sims

	// Score
	if result.Summary.TotalDeployments > 0 {
		safePct := result.Summary.SafeRollbacks * 100 / result.Summary.TotalDeployments
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildRollbackSimRecs(&result)
	writeJSON(w, result)
}

func buildRollbackSimRecs(r *RollbackSimulatorResult) []string {
	recs := []string{
		fmt.Sprintf("回滚就绪: %d 安全, %d 高风险, %d 无历史",
			r.Summary.SafeRollbacks, r.Summary.RiskyRollbacks, r.Summary.NoHistory),
	}
	if r.Summary.RiskyRollbacks > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载回滚有数据丢失风险（PVC）", r.Summary.RiskyRollbacks))
	}
	if r.Summary.NoHistory > 5 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载无回滚历史，建议设置 revisionHistoryLimit >= 2", r.Summary.NoHistory))
	}
	if len(r.RiskyRollbacks) > 0 {
		top := r.RiskyRollbacks[0]
		recs = append(recs, fmt.Sprintf("最高风险: %s/%s (score=%d)", top.Namespace, top.Workload, top.RiskScore))
	}
	return recs
}

var _ appsv1.DeploymentList
