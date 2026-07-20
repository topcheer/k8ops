package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaxSurgeAuditResult analyzes rolling update maxSurge/maxUnavailable configuration.
type MaxSurgeAuditResult struct {
	ScannedAt        time.Time       `json:"scannedAt"`
	Summary          MaxSurgeSummary `json:"summary"`
	ByWorkload       []MaxSurgeEntry `json:"byWorkload"`
	RiskyDeployments []MaxSurgeEntry `json:"riskyDeployments"`
	HealthScore      int             `json:"healthScore"`
	Grade            string          `json:"grade"`
	Recommendations  []string        `json:"recommendations"`
}

type MaxSurgeSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	RollingUpdate    int `json:"rollingUpdateStrategy"`
	Recreate         int `json:"recreateStrategy"`
	DefaultSurge     int `json:"usingDefaultSurge"`
	HighSurge        int `json:"highSurgeDeployments"`
	ZeroUnavailable  int `json:"zeroMaxUnavailable"`
	CustomConfig     int `json:"customConfigured"`
}

type MaxSurgeEntry struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Strategy       string   `json:"strategy"`
	MaxSurge       string   `json:"maxSurge"`
	MaxUnavailable string   `json:"maxUnavailable"`
	Replicas       int32    `json:"replicas"`
	RiskLevel      string   `json:"riskLevel"`
	Issues         []string `json:"issues"`
}

// handleMaxSurgeAudit handles GET /api/deployment/max-surge-audit
func (s *Server) handleMaxSurgeAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := MaxSurgeAuditResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := MaxSurgeEntry{
			Name: dep.Name, Namespace: dep.Namespace,
			Strategy: string(dep.Spec.Strategy.Type),
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.Replicas = replicas

		var issues []string

		if dep.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			result.Summary.RollingUpdate++

			// Check rolling update params
			ru := dep.Spec.Strategy.RollingUpdate
			if ru != nil {
				if ru.MaxSurge != nil {
					entry.MaxSurge = ru.MaxSurge.String()
					result.Summary.CustomConfig++
					// High surge check
					if ru.MaxSurge.IntVal > 1 || (ru.MaxSurge.Type == 1 && ru.MaxSurge.IntVal > int32(replicas)) {
						result.Summary.HighSurge++
						issues = append(issues, fmt.Sprintf("high maxSurge=%s", entry.MaxSurge))
					}
				} else {
					entry.MaxSurge = "25% (default)"
					result.Summary.DefaultSurge++
				}

				if ru.MaxUnavailable != nil {
					entry.MaxUnavailable = ru.MaxUnavailable.String()
					if ru.MaxUnavailable.IntVal == 0 {
						result.Summary.ZeroUnavailable++
					}
				} else {
					entry.MaxUnavailable = "25% (default)"
				}
			}
		} else if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			result.Summary.Recreate++
			issues = append(issues, "Recreate strategy - downtime during deploy")
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 2:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.RiskyDeployments = append(result.RiskyDeployments, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.RiskyDeployments, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.RiskyDeployments[i].RiskLevel] < rank[result.RiskyDeployments[j].RiskLevel]
	})

	if result.Summary.TotalDeployments > 0 {
		configured := result.Summary.CustomConfig
		result.HealthScore = configured * 100 / result.Summary.TotalDeployments
		result.HealthScore += (result.Summary.RollingUpdate - result.Summary.Recreate) * 100 / result.Summary.TotalDeployments
		result.HealthScore = result.HealthScore / 2
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("MaxSurge 审计: %d 部署, %d RollingUpdate, %d Recreate, %d 高 surge, %d 零不可用",
			result.Summary.TotalDeployments, result.Summary.RollingUpdate,
			result.Summary.Recreate, result.Summary.HighSurge,
			result.Summary.ZeroUnavailable),
	}
	if result.Summary.Recreate > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署使用 Recreate 策略, 部署期间有停机", result.Summary.Recreate))
	}
	writeJSON(w, result)
}
