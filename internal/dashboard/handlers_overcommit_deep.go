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

// OvercommitDeepResult performs deep overcommit analysis with bin-packing efficiency scoring.
type OvercommitDeepResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         OvercommitDeepSummary `json:"summary"`
	ByWorkload      []OvercommitDeepEntry `json:"byWorkload"`
	Overcommitted   []OvercommitDeepEntry `json:"overcommittedWorkloads"`
	BinpackScore    float64               `json:"binpackScore"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type OvercommitDeepSummary struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	TotalCPUReq     float64 `json:"totalCPURequest"`
	TotalMemReqGB   float64 `json:"totalMemRequestGB"`
	TotalCPULimit   float64 `json:"totalCPULimit"`
	TotalMemLimitGB float64 `json:"totalMemLimitGB"`
	CPUReqVsLimit   float64 `json:"cpuReqVsLimitRatio"`
	MemReqVsLimit   float64 `json:"memReqVsLimitRatio"`
	OvercommitRatio float64 `json:"overcommitRatio"`
	Unbounded       int     `json:"unboundedWorkloads"`
}

type OvercommitDeepEntry struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	CPUReq     float64 `json:"cpuRequestCores"`
	CPULimit   float64 `json:"cpuLimitCores"`
	MemReqGB   float64 `json:"memRequestGB"`
	MemLimitGB float64 `json:"memLimitGB"`
	Ratio      float64 `json:"limitOverRequestRatio"`
	RiskLevel  string  `json:"riskLevel"`
}

// handleOvercommitDeep handles GET /api/scalability/overcommit-deep
func (s *Server) handleOvercommitDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := OvercommitDeepResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		var cpuReq, cpuLimit, memReq, memLimit float64
		for _, c := range dep.Spec.Template.Spec.Containers {
			if r, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuReq += r.AsApproximateFloat64()
			}
			if l, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				cpuLimit += l.AsApproximateFloat64()
			}
			if r, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq += float64(r.Value()) / (1024 * 1024 * 1024)
			}
			if l, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				memLimit += float64(l.Value()) / (1024 * 1024 * 1024)
			}
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		cpuReq *= float64(replicas)
		cpuLimit *= float64(replicas)
		memReq *= float64(replicas)
		memLimit *= float64(replicas)

		result.Summary.TotalCPUReq += cpuReq
		result.Summary.TotalCPULimit += cpuLimit
		result.Summary.TotalMemReqGB += memReq
		result.Summary.TotalMemLimitGB += memLimit

		entry := OvercommitDeepEntry{
			Name: dep.Name, Namespace: dep.Namespace,
			CPUReq: cpuReq, CPULimit: cpuLimit,
			MemReqGB: memReq, MemLimitGB: memLimit,
		}

		if cpuReq > 0 && cpuLimit > 0 {
			entry.Ratio = cpuLimit / cpuReq
		} else if cpuLimit == 0 {
			result.Summary.Unbounded++
			entry.RiskLevel = "medium"
		}

		switch {
		case entry.Ratio > 5:
			entry.RiskLevel = "critical"
			result.Overcommitted = append(result.Overcommitted, entry)
		case entry.Ratio > 3:
			entry.RiskLevel = "high"
			result.Overcommitted = append(result.Overcommitted, entry)
		case entry.Ratio > 2:
			entry.RiskLevel = "medium"
		default:
			if entry.RiskLevel == "" {
				entry.RiskLevel = "low"
			}
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Calculate ratios
	if result.Summary.TotalCPUReq > 0 {
		result.Summary.CPUReqVsLimit = result.Summary.TotalCPULimit / result.Summary.TotalCPUReq
	}
	if result.Summary.TotalMemReqGB > 0 {
		result.Summary.MemReqVsLimit = result.Summary.TotalMemLimitGB / result.Summary.TotalMemReqGB
	}
	result.Summary.OvercommitRatio = (result.Summary.CPUReqVsLimit + result.Summary.MemReqVsLimit) / 2
	result.BinpackScore = 100 - result.Summary.OvercommitRatio*10
	if result.BinpackScore < 0 {
		result.BinpackScore = 0
	}

	sort.Slice(result.Overcommitted, func(i, j int) bool {
		return result.Overcommitted[i].Ratio > result.Overcommitted[j].Ratio
	})

	result.HealthScore = int(result.BinpackScore)
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("过度提交深度审计: %d 部署, CPU 请求/限制比 %.1f, 内存比 %.1f, %d 无限制",
			result.Summary.TotalWorkloads, result.Summary.CPUReqVsLimit,
			result.Summary.MemReqVsLimit, result.Summary.Unbounded),
	}
	if len(result.Overcommitted) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署过度提交 (比率>3x)", len(result.Overcommitted)))
	}
	writeJSON(w, result)
}

var _ appsv1.DeploymentSpec
