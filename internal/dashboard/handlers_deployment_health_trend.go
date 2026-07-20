package dashboard

import (
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeploymentHealthTrendResult analyzes deployment health trends over recent events.
type DeploymentHealthTrendResult struct {
	ScannedAt         time.Time                `json:"scannedAt"`
	Summary           DeployHealthTrendSummary `json:"summary"`
	ByWorkload        []DeployHealthEntry      `json:"byWorkload"`
	DegradedWorkloads []DeployHealthEntry      `json:"degradedWorkloads"`
	HealthScore       int                      `json:"healthScore"`
	Grade             string                   `json:"grade"`
	Recommendations   []string                 `json:"recommendations"`
}

type DeployHealthTrendSummary struct {
	TotalDeployments    int `json:"totalDeployments"`
	HealthyDeployments  int `json:"healthyDeployments"`
	DegradedDeployments int `json:"degradedDeployments"`
	NotReadyDeployments int `json:"notReadyDeployments"`
	UpdatedRecently     int `json:"updatedRecently"`
	RollingUpdate       int `json:"inRollingUpdate"`
}

type DeployHealthEntry struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	DesiredReplicas   int32    `json:"desiredReplicas"`
	ReadyReplicas     int32    `json:"readyReplicas"`
	UpdatedReplicas   int32    `json:"updatedReplicas"`
	AvailableReplicas int32    `json:"availableReplicas"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel"`
	Issues            []string `json:"issues"`
}

// handleDeploymentHealthTrend handles GET /api/operations/deployment-health-trend
func (s *Server) handleDeploymentHealthTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DeploymentHealthTrendResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod restart count map by deployment
	depRestartMap := make(map[string]int) // ns/name -> total restarts
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		depName := getOwnerDeployName(&pod)
		if depName == "" {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			depRestartMap[pod.Namespace+"/"+depName] += int(cs.RestartCount)
		}
	}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := DeployHealthEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.DesiredReplicas = replicas
		entry.ReadyReplicas = dep.Status.ReadyReplicas
		entry.UpdatedReplicas = dep.Status.UpdatedReplicas
		entry.AvailableReplicas = dep.Status.AvailableReplicas

		var issues []string

		switch {
		case entry.UpdatedReplicas < entry.DesiredReplicas:
			entry.Status = "rolling-out"
			result.Summary.RollingUpdate++
			issues = append(issues, fmt.Sprintf("rollout in progress: %d/%d updated", entry.UpdatedReplicas, entry.DesiredReplicas))
		case entry.ReadyReplicas < entry.DesiredReplicas:
			entry.Status = "degraded"
			result.Summary.DegradedDeployments++
			issues = append(issues, fmt.Sprintf("not enough ready: %d/%d", entry.ReadyReplicas, entry.DesiredReplicas))
		case entry.AvailableReplicas < entry.DesiredReplicas:
			entry.Status = "partially-available"
			result.Summary.NotReadyDeployments++
			issues = append(issues, fmt.Sprintf("availability gap: %d/%d available", entry.AvailableReplicas, entry.DesiredReplicas))
		default:
			entry.Status = "healthy"
			result.Summary.HealthyDeployments++
		}

		// Check restart rate
		restarts := depRestartMap[dep.Namespace+"/"+dep.Name]
		if restarts > 10 {
			issues = append(issues, fmt.Sprintf("%d total restarts", restarts))
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
			result.DegradedWorkloads = append(result.DegradedWorkloads, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if result.Summary.TotalDeployments > 0 {
		result.HealthScore = result.Summary.HealthyDeployments * 100 / result.Summary.TotalDeployments
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("部署健康趋势: %d 部署, %d 健康, %d 降级, %d 未就绪, %d 滚动更新中",
			result.Summary.TotalDeployments, result.Summary.HealthyDeployments,
			result.Summary.DegradedDeployments, result.Summary.NotReadyDeployments,
			result.Summary.RollingUpdate),
	}
	if result.Summary.DegradedDeployments > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署副本不足, 影响可用性", result.Summary.DegradedDeployments))
	}
	writeJSON(w, result)
}

func getOwnerDeployName(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" {
			name := ref.Name
			if idx := lastIdx(name, "-"); idx > 0 {
				return name[:idx]
			}
			return name
		}
		if ref.Kind == "Deployment" {
			return ref.Name
		}
	}
	return ""
}

var _ appsv1.DeploymentStatus
