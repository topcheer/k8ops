package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployPipelineAuditResult audits deployment pipeline health by checking
// CI/CD integration patterns, image freshness, and rollout discipline.
type DeployPipelineAuditResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         PipelineAuditSummary `json:"summary"`
	ByWorkload      []PipelineEntry      `json:"byWorkload"`
	PipelineGaps    []PipelineGap        `json:"pipelineGaps"`
	PipelineScore   int                  `json:"pipelineScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type PipelineAuditSummary struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	FreshImages     int     `json:"freshImages"`
	StaleImages     int     `json:"staleImages"`
	WithProbes      int     `json:"withProbes"`
	WithResources   int     `json:"withResources"`
	MultiReplica    int     `json:"multiReplica"`
	RollingStrategy int     `json:"rollingStrategy"`
	AvgImageAge     float64 `json:"avgImageAgeDays"`
}

type PipelineEntry struct {
	Workload      string   `json:"workload"`
	Namespace     string   `json:"namespace"`
	Image         string   `json:"image"`
	ImageAgeDays  int      `json:"imageAgeDays"`
	Replicas      int      `json:"replicas"`
	HasProbe      bool     `json:"hasProbe"`
	HasResources  bool     `json:"hasResources"`
	Strategy      string   `json:"strategy"`
	PipelineReady bool     `json:"pipelineReady"`
	Gaps          []string `json:"gaps"`
}

type PipelineGap struct {
	Gap      string `json:"gap"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
	Fix      string `json:"fix"`
}

// handleDeployPipelineAudit handles GET /api/deployment/deploy-pipeline-audit
func (s *Server) handleDeployPipelineAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployPipelineAuditResult{ScannedAt: time.Now()}
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	totalAge := 0
	ageCount := 0

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		entry := PipelineEntry{Workload: d.Name, Namespace: d.Namespace}

		// Image analysis
		if len(d.Spec.Template.Spec.Containers) > 0 {
			entry.Image = d.Spec.Template.Spec.Containers[0].Image
		}

		// Image age from deployment creation
		age := int(now.Sub(d.CreationTimestamp.Time).Hours() / 24)
		if age < 0 {
			age = 0
		}
		entry.ImageAgeDays = age
		totalAge += age
		ageCount++
		if age < 7 {
			result.Summary.FreshImages++
		} else {
			result.Summary.StaleImages++
		}

		replicas := 1
		if d.Spec.Replicas != nil {
			replicas = int(*d.Spec.Replicas)
		}
		entry.Replicas = replicas
		if replicas >= 2 {
			result.Summary.MultiReplica++
		}

		// Probes
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				entry.HasProbe = true
				break
			}
		}
		if entry.HasProbe {
			result.Summary.WithProbes++
		}

		// Resources
		hasRes := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if len(c.Resources.Requests) == 0 {
				hasRes = false
				break
			}
		}
		entry.HasResources = hasRes
		if hasRes {
			result.Summary.WithResources++
		}

		// Strategy
		entry.Strategy = string(d.Spec.Strategy.Type)
		if d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			result.Summary.RollingStrategy++
		}

		// Pipeline readiness
		if entry.HasProbe && hasRes && replicas >= 2 && d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			entry.PipelineReady = true
		} else {
			if !entry.HasProbe {
				entry.Gaps = append(entry.Gaps, "missing-probe")
			}
			if !hasRes {
				entry.Gaps = append(entry.Gaps, "missing-resources")
			}
			if replicas < 2 {
				entry.Gaps = append(entry.Gaps, "single-replica")
			}
			if d.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
				entry.Gaps = append(entry.Gaps, "non-rolling")
			}
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if ageCount > 0 {
		result.Summary.AvgImageAge = float64(totalAge) / float64(ageCount)
	}

	// Aggregate gaps
	gapCounts := make(map[string]int)
	for _, e := range result.ByWorkload {
		for _, g := range e.Gaps {
			gapCounts[g]++
		}
	}
	gapSev := map[string]string{"missing-probe": "high", "missing-resources": "high", "single-replica": "medium", "non-rolling": "medium"}
	gapFix := map[string]string{
		"missing-probe":     "Add readiness/liveness probes",
		"missing-resources": "Set CPU/memory requests",
		"single-replica":    "Scale to >=2 replicas",
		"non-rolling":       "Switch to RollingUpdate strategy",
	}
	for gap, count := range gapCounts {
		result.PipelineGaps = append(result.PipelineGaps, PipelineGap{Gap: gap, Count: count, Severity: gapSev[gap], Fix: gapFix[gap]})
	}
	sort.Slice(result.PipelineGaps, func(i, j int) bool { return result.PipelineGaps[i].Count > result.PipelineGaps[j].Count })
	sort.Slice(result.ByWorkload, func(i, j int) bool { return len(result.ByWorkload[i].Gaps) > len(result.ByWorkload[j].Gaps) })

	// Score
	if result.Summary.TotalWorkloads > 0 {
		readyCount := 0
		for _, e := range result.ByWorkload {
			if e.PipelineReady {
				readyCount++
			}
		}
		result.PipelineScore = readyCount * 100 / result.Summary.TotalWorkloads
	}
	gradeFromScore(&result.Grade, result.PipelineScore)

	result.Recommendations = []string{
		fmt.Sprintf("流水线审计: %d 工作负载, %d 就绪 (%d%%), 平均镜像年龄 %.0f 天", result.Summary.TotalWorkloads, result.Summary.TotalWorkloads-len(gapCounts), result.PipelineScore, result.Summary.AvgImageAge),
		fmt.Sprintf("%d 新鲜镜像 (<7天), %d 过期镜像", result.Summary.FreshImages, result.Summary.StaleImages),
	}
	if len(result.PipelineGaps) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 类缺口: %s", len(result.PipelineGaps), result.PipelineGaps[0].Gap))
	}
	if result.PipelineScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 建立 CI/CD 流水线标准, 新部署必须包含探针+资源+多副本")
	}
	writeJSON(w, result)
}
