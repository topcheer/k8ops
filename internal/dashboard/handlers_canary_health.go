package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CanaryHealthResult analyzes canary and blue-green deployment health.
type CanaryHealthResult struct {
	ScannedAt         time.Time           `json:"scannedAt"`
	Summary           CanaryHealthSummary `json:"summary"`
	Deployments       []CanaryDeployEntry `json:"deployments"`
	AtRiskDeployments []CanaryDeployEntry `json:"atRiskDeployments"`
	HealthScore       int                 `json:"healthScore"`
	Grade             string              `json:"grade"`
	Recommendations   []string            `json:"recommendations"`
}

type CanaryHealthSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithCanary       int `json:"withCanaryAnnotations"`
	WithStrategy     int `json:"withProgressiveStrategy"`
	HealthyRollouts  int `json:"healthyRollouts"`
	StalledRollouts  int `json:"stalledRollouts"`
	MaxUnavailable   int `json:"maxUnavailablePods"`
}

type CanaryDeployEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Strategy     string   `json:"strategy"`
	Desired      int      `json:"desiredReplicas"`
	Ready        int      `json:"readyReplicas"`
	Updated      int      `json:"updatedReplicas"`
	Available    int      `json:"availableReplicas"`
	HasCanaryAnn bool     `json:"hasCanaryAnnotations"`
	RolloutStat  string   `json:"rolloutStatus"`
	RiskLevel    string   `json:"riskLevel"`
	Issues       []string `json:"issues"`
}

// handleCanaryHealth handles GET /api/product/canary-health
func (s *Server) handleCanaryHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CanaryHealthResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := CanaryDeployEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Strategy:  string(dep.Spec.Strategy.Type),
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.Desired = int(replicas)
		entry.Updated = int(dep.Status.UpdatedReplicas)
		entry.Ready = int(dep.Status.ReadyReplicas)
		entry.Available = int(dep.Status.AvailableReplicas)

		// Check for canary/argo annotations
		for k := range dep.Annotations {
			if strings.Contains(k, "canary") || strings.Contains(k, "argoproj.io") ||
				strings.Contains(k, "rollout") {
				entry.HasCanaryAnn = true
				break
			}
		}

		// Determine strategy
		if entry.HasCanaryAnn {
			result.Summary.WithCanary++
		}
		if dep.Spec.Strategy.Type == "RollingUpdate" {
			result.Summary.WithStrategy++
		}

		// Assess rollout status
		var issues []string
		switch {
		case entry.Updated < entry.Desired:
			entry.RolloutStat = "in-progress"
			issues = append(issues, fmt.Sprintf("%d/%d updated", entry.Updated, entry.Desired))
			result.Summary.StalledRollouts++
		case entry.Ready < entry.Desired:
			entry.RolloutStat = "degraded"
			issues = append(issues, fmt.Sprintf("%d/%d ready", entry.Ready, entry.Desired))
			unavailable := entry.Desired - entry.Available
			if unavailable > result.Summary.MaxUnavailable {
				result.Summary.MaxUnavailable = unavailable
			}
		case entry.Available < entry.Desired:
			entry.RolloutStat = "partially-available"
			issues = append(issues, fmt.Sprintf("%d/%d available", entry.Available, entry.Desired))
		default:
			entry.RolloutStat = "healthy"
			result.Summary.HealthyRollouts++
		}

		// Check pod restart ratio
		restartCount := 0
		podCount := 0
		for _, pod := range pods.Items {
			if pod.Namespace != dep.Namespace || pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for k, v := range dep.Spec.Selector.MatchLabels {
				if pod.Labels[k] != v {
					goto nextPod
				}
			}
			podCount++
			for _, cs := range pod.Status.ContainerStatuses {
				restartCount += int(cs.RestartCount)
			}
		nextPod:
		}
		if podCount > 0 && restartCount > podCount*2 {
			issues = append(issues, fmt.Sprintf("high restart ratio: %d/%d", restartCount, podCount))
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 3:
			entry.RiskLevel = "critical"
		case len(issues) >= 2:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.AtRiskDeployments = append(result.AtRiskDeployments, entry)
		}
		result.Deployments = append(result.Deployments, entry)
	}

	sort.Slice(result.Deployments, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.Deployments[i].RiskLevel] < rank[result.Deployments[j].RiskLevel]
	})

	if result.Summary.TotalDeployments > 0 {
		result.HealthScore = result.Summary.HealthyRollouts * 100 / result.Summary.TotalDeployments
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("金丝雀健康: %d 部署, %d 有渐进策略, %d 健康, %d 异常",
			result.Summary.TotalDeployments, result.Summary.WithStrategy,
			result.Summary.HealthyRollouts, result.Summary.StalledRollouts),
	}
	if result.Summary.StalledRollouts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署滚动更新停滞", result.Summary.StalledRollouts))
	}
	if result.Summary.MaxUnavailable > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("最多 %d 个 Pod 不可用", result.Summary.MaxUnavailable))
	}
	writeJSON(w, result)
}
