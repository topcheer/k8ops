package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EphemeralStorageQuotaResult audits ephemeral storage usage and limits.
type EphemeralStorageQuotaResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         EphemeralSummary    `json:"summary"`
	ByNamespace     []EphemeralNSEntry  `json:"byNamespace"`
	AtRiskPods      []EphemeralPodEntry `json:"atRiskPods"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type EphemeralSummary struct {
	TotalPods          int     `json:"totalPods"`
	WithEphemeralLimit int     `json:"withEphemeralLimit"`
	WithEmptyDir       int     `json:"withEmptyDirVolumes"`
	WithLogVolume      int     `json:"largeLogVolumePods"`
	TotalEphemeralGB   float64 `json:"totalEphemeralGB"`
	UnboundedPods      int     `json:"unboundedPods"`
}

type EphemeralNSEntry struct {
	Namespace  string  `json:"namespace"`
	PodCount   int     `json:"podCount"`
	Unbounded  int     `json:"unboundedPods"`
	EstUsageGB float64 `json:"estimatedUsageGB"`
	RiskLevel  string  `json:"riskLevel"`
}

type EphemeralPodEntry struct {
	PodName    string  `json:"podName"`
	Namespace  string  `json:"namespace"`
	HasLimit   bool    `json:"hasEphemeralLimit"`
	EmptyDirs  int     `json:"emptyDirCount"`
	EstUsageGB float64 `json:"estimatedUsageGB"`
	RiskLevel  string  `json:"riskLevel"`
}

// handleEphemeralStorageQuota handles GET /api/deployment/ephemeral-storage-quota
func (s *Server) handleEphemeralStorageQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EphemeralStorageQuotaResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*EphemeralNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		entry := EphemeralPodEntry{PodName: pod.Name, Namespace: pod.Namespace}
		var estUsage float64 // estimated GB

		// Check ephemeral storage limits
		for _, c := range pod.Spec.Containers {
			if lim, ok := c.Resources.Limits[corev1.ResourceEphemeralStorage]; ok {
				entry.HasLimit = true
				estUsage += float64(lim.Value()) / (1024 * 1024 * 1024)
			} else {
				// No limit - assume 1GB default usage
				estUsage += 1.0
			}
		}

		// Count emptyDir volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.EmptyDir != nil {
				entry.EmptyDirs++
				result.Summary.WithEmptyDir++
				if vol.EmptyDir.SizeLimit != nil {
					estUsage += float64(vol.EmptyDir.SizeLimit.Value()) / (1024 * 1024 * 1024)
				} else {
					estUsage += 0.5 // assume 500MB per unbounded emptyDir
				}
			}
		}

		entry.EstUsageGB = estUsage
		result.Summary.TotalEphemeralGB += estUsage

		if entry.HasLimit {
			result.Summary.WithEphemeralLimit++
		} else {
			result.Summary.UnboundedPods++
			entry.RiskLevel = "medium"
			result.AtRiskPods = append(result.AtRiskPods, entry)
		}

		// Namespace tracking
		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &EphemeralNSEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++
		nsMap[pod.Namespace].EstUsageGB += estUsage
		if !entry.HasLimit {
			nsMap[pod.Namespace].Unbounded++
		}
	}

	for _, e := range nsMap {
		ratio := float64(e.Unbounded) / float64(e.PodCount) * 100
		switch {
		case ratio > 80:
			e.RiskLevel = "critical"
		case ratio > 50:
			e.RiskLevel = "high"
		case ratio > 20:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Unbounded > result.ByNamespace[j].Unbounded
	})

	if result.Summary.TotalPods > 0 {
		result.HealthScore = result.Summary.WithEphemeralLimit * 100 / result.Summary.TotalPods
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("临时存储审计: %d Pod, %d 有限制, %d 无限制, %d emptyDir, 总计 %.1fGB",
			result.Summary.TotalPods, result.Summary.WithEphemeralLimit,
			result.Summary.UnboundedPods, result.Summary.WithEmptyDir,
			result.Summary.TotalEphemeralGB),
	}
	if result.Summary.UnboundedPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 无临时存储限制, 磁盘耗尽风险", result.Summary.UnboundedPods))
	}
	if result.HealthScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 设置 resources.limits.ephemeral-storage 和 emptyDir.sizeLimit")
	}
	writeJSON(w, result)
}
