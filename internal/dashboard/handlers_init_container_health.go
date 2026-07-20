package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitContainerHealthAuditResult audits init container configurations and failure patterns.
type InitContainerHealthAuditResult struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	Summary         InitContainerHealthSummary `json:"summary"`
	ByWorkload      []InitContainerHealthEntry `json:"byWorkload"`
	Issues          []InitContainerHealthIssue `json:"issues"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Recommendations []string                   `json:"recommendations"`
}

type InitContainerHealthSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithInitContainer int `json:"withInitContainers"`
	NoResourceLimit   int `json:"withoutResourceLimits"`
	NoProbe           int `json:"withoutProbes"`
	BlockingRisk      int `json:"blockingRisk"`
	HighRestartInit   int `json:"highRestartInit"`
}

type InitContainerHealthEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	InitCount    int      `json:"initContainerCount"`
	InitNames    []string `json:"initContainerNames"`
	HasResources bool     `json:"hasResourceLimits"`
	RiskLevel    string   `json:"riskLevel"`
	Issues       []string `json:"issues"`
}

type InitContainerHealthIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	InitName  string `json:"initContainer"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleInitContainerHealth handles GET /api/deployment/init-container-health
func (s *Server) handleInitContainerHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := InitContainerHealthAuditResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build init container restart map
	initRestartMap := make(map[string]int)
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.RestartCount > 0 {
				initRestartMap[pod.Name+"/"+cs.Name] = int(cs.RestartCount)
			}
		}
	}

	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		initContainers := dep.Spec.Template.Spec.InitContainers
		if len(initContainers) == 0 {
			continue
		}

		result.Summary.WithInitContainer++
		entry := InitContainerHealthEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			InitCount: len(initContainers),
		}
		var issues []string

		for _, ic := range initContainers {
			entry.InitNames = append(entry.InitNames, ic.Name)

			hasLimit := false
			if _, ok := ic.Resources.Limits[corev1.ResourceCPU]; ok {
				hasLimit = true
			}
			if !hasLimit {
				result.Summary.NoResourceLimit++
				result.Summary.BlockingRisk++
				issues = append(issues, fmt.Sprintf("init container %s has no resource limits", ic.Name))
				result.Issues = append(result.Issues, InitContainerHealthIssue{
					Name: dep.Name, Namespace: dep.Namespace, InitName: ic.Name,
					Issue: "no resource limits - can consume all node resources", Severity: "high",
				})
			} else {
				entry.HasResources = true
			}

			if ic.LivenessProbe == nil {
				result.Summary.NoProbe++
			}

			if restarts := initRestartMap[dep.Name+"/"+ic.Name]; restarts > 3 {
				result.Summary.HighRestartInit++
				issues = append(issues, fmt.Sprintf("init container %s restarted %d times", ic.Name, restarts))
				result.Issues = append(result.Issues, InitContainerHealthIssue{
					Name: dep.Name, Namespace: dep.Namespace, InitName: ic.Name,
					Issue: fmt.Sprintf("%d restarts", restarts), Severity: "medium",
				})
			}
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

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalWorkloads > 0 {
		totalInit := result.Summary.WithInitContainer
		if totalInit > 0 {
			result.HealthScore = (totalInit - result.Summary.NoResourceLimit) * 100 / totalInit
			if result.HealthScore < 0 {
				result.HealthScore = 0
			}
		} else {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Init 容器健康: %d 部署, %d 有 init, %d 无资源限制, %d 无探针, %d 高重启",
			result.Summary.TotalWorkloads, result.Summary.WithInitContainer,
			result.Summary.NoResourceLimit, result.Summary.NoProbe, result.Summary.HighRestartInit),
	}
	if result.Summary.BlockingRisk > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 init 容器无资源限制, 可阻塞 Pod 启动", result.Summary.BlockingRisk))
	}
	writeJSON(w, result)
}
