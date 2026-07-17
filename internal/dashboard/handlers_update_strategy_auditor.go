package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateStrategyAuditorResult evaluates deployment update strategies and
// rollout configurations. Identifies risky Recreate strategies, missing
// surge/unavailable controls, and suboptimal rollback readiness.
type UpdateStrategyAuditorResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         StrategyAuditSummary `json:"summary"`
	Findings        []StrategyFinding    `json:"findings"`
	ByStrategy      []StrategyStat       `json:"byStrategy"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type StrategyAuditSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	RollingUpdate    int `json:"rollingUpdate"`
	Recreate         int `json:"recreate"`
	RiskyStrategy    int `json:"riskyStrategy"`
	NoMaxSurge       int `json:"noMaxSurge"`
	NoMaxUnavailable int `json:"noMaxUnavailable"`
	OldRevisionLimit int `json:"oldRevisionLimit"`
}

type StrategyFinding struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
	Current   string `json:"current"`
	Suggested string `json:"suggested"`
	PatchCmd  string `json:"patchCmd"`
}

type StrategyStat struct {
	Strategy string `json:"strategy"`
	Count    int    `json:"count"`
	Percent  int    `json:"percent"`
}

// handleUpdateStrategyAuditor handles GET /api/deployment/update-strategy-auditor
func (s *Server) handleUpdateStrategyAuditor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := UpdateStrategyAuditorResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	stratCounts := make(map[string]int)
	var findings []StrategyFinding

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++
		stratCounts[string(d.Spec.Strategy.Type)]++
		replicas := int(ptrInt32(d.Spec.Replicas))

		// Check strategy type
		if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType && replicas > 1 {
			result.Summary.Recreate++
			result.Summary.RiskyStrategy++
			findings = append(findings, StrategyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue:     "使用 Recreate 策略，多副本部署更新时会全部停止",
				Severity:  "high",
				Current:   "Recreate",
				Suggested: "RollingUpdate",
				PatchCmd: fmt.Sprintf(
					"kubectl patch deployment %s -n %s -p '{\"spec\":{\"strategy\":{\"type\":\"RollingUpdate\"}}}'",
					d.Name, d.Namespace),
			})
		}

		// Check RollingUpdate params
		if d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType || d.Spec.Strategy.Type == "" {
			result.Summary.RollingUpdate++

			ru := d.Spec.Strategy.RollingUpdate
			if ru == nil {
				result.Summary.NoMaxSurge++
				findings = append(findings, StrategyFinding{
					Workload: d.Name, Namespace: d.Namespace,
					Issue:     "未设置 maxSurge/maxUnavailable",
					Severity:  "medium",
					Current:   "默认值",
					Suggested: "maxSurge=1, maxUnavailable=0",
					PatchCmd: fmt.Sprintf(
						"kubectl patch deployment %s -n %s -type=strategic -p "+
							"'{\"spec\":{\"strategy\":{\"rollingUpdate\":{\"maxSurge\":1,\"maxUnavailable\":0}}}}'",
						d.Name, d.Namespace),
				})
			} else {
				// Check if maxUnavailable is too high
				if ru.MaxUnavailable != nil {
					val := ru.MaxUnavailable.IntValue()
					if replicas > 1 && val >= replicas {
						findings = append(findings, StrategyFinding{
							Workload: d.Name, Namespace: d.Namespace,
							Issue:     fmt.Sprintf("maxUnavailable=%d 过高，可能导致全部不可用", val),
							Severity:  "medium",
							Current:   fmt.Sprintf("maxUnavailable=%d", val),
							Suggested: "maxUnavailable < replicas",
						})
					}
				}
			}
		}

		// Check revision history limit
		if d.Spec.RevisionHistoryLimit == nil || *d.Spec.RevisionHistoryLimit > 20 {
			result.Summary.OldRevisionLimit++
			findings = append(findings, StrategyFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Issue:    "RevisionHistoryLimit 过大或未设置，旧 ReplicaSet 占用资源",
				Severity: "low",
				Current: func() string {
					if d.Spec.RevisionHistoryLimit != nil {
						return fmt.Sprintf("revisionHistoryLimit=%d", *d.Spec.RevisionHistoryLimit)
					}
					return "默认(10)"
				}(),
				Suggested: "revisionHistoryLimit=5",
			})
		}
	}

	// Strategy stats
	total := result.Summary.TotalDeployments
	for strat, count := range stratCounts {
		pct := 0
		if total > 0 {
			pct = count * 100 / total
		}
		result.ByStrategy = append(result.ByStrategy, StrategyStat{
			Strategy: strat, Count: count, Percent: pct,
		})
	}
	sort.Slice(result.ByStrategy, func(i, j int) bool {
		return result.ByStrategy[i].Count > result.ByStrategy[j].Count
	})

	sort.Slice(findings, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[findings[i].Severity] < sevOrder[findings[j].Severity]
	})
	result.Findings = findings

	// Score
	if total > 0 {
		result.HealthScore = (total - result.Summary.RiskyStrategy) * 100 / total
		result.HealthScore -= result.Summary.NoMaxSurge * 3
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
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

	result.Recommendations = buildStrategyAuditRecs(&result)
	writeJSON(w, result)
}

func buildStrategyAuditRecs(r *UpdateStrategyAuditorResult) []string {
	recs := []string{}
	if r.Summary.RiskyStrategy > 0 {
		recs = append(recs, fmt.Sprintf("%d 个多副本 Deployment 使用 Recreate，更新时会中断服务", r.Summary.RiskyStrategy))
	}
	if r.Summary.NoMaxSurge > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Deployment 未设置 RollingUpdate 参数", r.Summary.NoMaxSurge))
	}
	if r.Summary.OldRevisionLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Deployment 保留了过多旧版本，建议设置 revisionHistoryLimit=5", r.Summary.OldRevisionLimit))
	}
	if len(recs) == 0 {
		recs = append(recs, "更新策略配置良好")
	}
	return recs
}
