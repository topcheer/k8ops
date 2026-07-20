package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HostPathAuditResult detects hostPath volume mounts that bypass container isolation.
type HostPathAuditResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         HostPathSummary   `json:"summary"`
	ByNamespace     []HostPathNSEntry `json:"byNamespace"`
	Violations      []HostPathEntry   `json:"violations"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type HostPathSummary struct {
	TotalPods        int `json:"totalPods"`
	PodsWithHostPath int `json:"podsWithHostPath"`
	TotalHostPaths   int `json:"totalHostPathMounts"`
	SystemPaths      int `json:"systemPathMounts"`
	WritablePaths    int `json:"writablePaths"`
	PrivilegedPaths  int `json:"privilegedPaths"` // /, /etc, /var, /proc
}

type HostPathNSEntry struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	HostPaths int    `json:"hostPaths"`
	RiskLevel string `json:"riskLevel"`
}

type HostPathEntry struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	VolumeName string `json:"volumeName"`
	HostPath   string `json:"hostPath"`
	ReadOnly   bool   `json:"readOnly"`
	Severity   string `json:"severity"`
	Reason     string `json:"reason"`
}

// privileged host paths that grant host access
var privilegedHostPaths = []string{"/", "/etc", "/var", "/proc", "/sys", "/dev", "/boot", "/root", "/lib", "/usr/lib"}

// handleHostPathAudit handles GET /api/security/hostpath-audit
func (s *Server) handleHostPathAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := HostPathAuditResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*HostPathNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		foundHostPath := false
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath == nil {
				continue
			}
			foundHostPath = true
			result.Summary.TotalHostPaths++

			entry := HostPathEntry{
				PodName:    pod.Name,
				Namespace:  pod.Namespace,
				VolumeName: vol.Name,
				HostPath:   vol.HostPath.Path,
				ReadOnly:   vol.HostPath.Type != nil && (*vol.HostPath.Type == corev1.HostPathDirectoryOrCreate || false),
			}

			// Check if path is in privileged set
			isPriv := false
			for _, p := range privilegedHostPaths {
				if entry.HostPath == p || (len(entry.HostPath) > len(p) && entry.HostPath[:len(p)] == p) {
					isPriv = true
					break
				}
			}

			// Determine severity
			switch {
			case isPriv && !entry.ReadOnly:
				entry.Severity = "critical"
				entry.Reason = "writable privileged host path - full host compromise"
				result.Summary.PrivilegedPaths++
				result.Summary.WritablePaths++
			case isPriv:
				entry.Severity = "high"
				entry.Reason = "read-only privileged host path"
				result.Summary.PrivilegedPaths++
			case !entry.ReadOnly:
				entry.Severity = "medium"
				entry.Reason = "writable host path"
				result.Summary.WritablePaths++
			default:
				entry.Severity = "low"
				entry.Reason = "read-only host path"
				result.Summary.SystemPaths++
			}

			result.Violations = append(result.Violations, entry)
		}

		if foundHostPath {
			result.Summary.PodsWithHostPath++
			if nsMap[pod.Namespace] == nil {
				nsMap[pod.Namespace] = &HostPathNSEntry{Namespace: pod.Namespace}
			}
			nsMap[pod.Namespace].PodCount++
		}
	}

	for _, e := range nsMap {
		switch {
		case e.PodCount > 5:
			e.RiskLevel = "critical"
		case e.PodCount > 2:
			e.RiskLevel = "high"
		case e.PodCount > 0:
			e.RiskLevel = "medium"
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodCount > result.ByNamespace[j].PodCount
	})

	if result.Summary.TotalPods > 0 {
		cleanPods := result.Summary.TotalPods - result.Summary.PodsWithHostPath
		result.HealthScore = cleanPods * 100 / result.Summary.TotalPods
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("HostPath 审计: %d Pod, %d 使用 hostPath, %d 挂载, %d 特权路径, %d 可写",
			result.Summary.TotalPods, result.Summary.PodsWithHostPath,
			result.Summary.TotalHostPaths, result.Summary.PrivilegedPaths,
			result.Summary.WritablePaths),
	}
	if result.Summary.PrivilegedPaths > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个特权路径挂载, 容器可逃逸到宿主机", result.Summary.PrivilegedPaths))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 移除 hostPath 挂载, 使用 PVC 或 projected volume 替代")
	}
	writeJSON(w, result)
}
