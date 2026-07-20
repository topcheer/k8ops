package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReadOnlyRootFSResult audits whether containers use read-only root filesystem.
type ReadOnlyRootFSResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         RORFSummary   `json:"summary"`
	ByNamespace     []RORFNsEntry `json:"byNamespace"`
	WritablePods    []RORFEntry   `json:"writableRootFsContainers"`
	HealthScore     int           `json:"healthScore"`
	Grade           string        `json:"grade"`
	Recommendations []string      `json:"recommendations"`
}

type RORFSummary struct {
	TotalContainers int `json:"totalContainers"`
	ReadOnlyRootFS  int `json:"readOnlyRootFsContainers"`
	WritableRootFS  int `json:"writableRootFsContainers"`
	WithTmpVolumes  int `json:"withTmpVolumes"`
	NoSecContext    int `json:"withoutSecurityContext"`
}

type RORFNsEntry struct {
	Namespace      string  `json:"namespace"`
	ContainerCount int     `json:"containerCount"`
	WritableCount  int     `json:"writableCount"`
	CoveragePct    float64 `json:"readOnlyPct"`
}

type RORFEntry struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	HasSecCtx    bool   `json:"hasSecurityContext"`
	ReadOnlyFS   bool   `json:"readOnlyRootFilesystem"`
	RunAsNonRoot bool   `json:"runAsNonRoot"`
	AllowPrivEsc bool   `json:"allowPrivilegeEscalation"`
	RiskLevel    string `json:"riskLevel"`
}

// handleReadOnlyRootFS handles GET /api/security/readonly-rootfs-audit
func (s *Server) handleReadOnlyRootFS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ReadOnlyRootFSResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*RORFNsEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			entry := RORFEntry{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Container: c.Name,
			}

			if c.SecurityContext == nil {
				result.Summary.NoSecContext++
				entry.HasSecCtx = false
				entry.ReadOnlyFS = false
				entry.RiskLevel = "high"
			} else {
				entry.HasSecCtx = true
				if c.SecurityContext.ReadOnlyRootFilesystem != nil {
					entry.ReadOnlyFS = *c.SecurityContext.ReadOnlyRootFilesystem
				} else {
					entry.ReadOnlyFS = false // default is writable
				}
				if c.SecurityContext.RunAsNonRoot != nil {
					entry.RunAsNonRoot = *c.SecurityContext.RunAsNonRoot
				}
				if c.SecurityContext.AllowPrivilegeEscalation != nil {
					entry.AllowPrivEsc = *c.SecurityContext.AllowPrivilegeEscalation
				}
			}

			if entry.ReadOnlyFS {
				result.Summary.ReadOnlyRootFS++
			} else {
				result.Summary.WritableRootFS++
				if entry.RiskLevel == "" {
					if entry.AllowPrivEsc {
						entry.RiskLevel = "critical"
					} else {
						entry.RiskLevel = "medium"
					}
				}
				result.WritablePods = append(result.WritablePods, entry)
			}

			// Track namespace
			if nsMap[pod.Namespace] == nil {
				nsMap[pod.Namespace] = &RORFNsEntry{Namespace: pod.Namespace}
			}
			nsMap[pod.Namespace].ContainerCount++
			if entry.ReadOnlyFS {
				// nothing
			} else {
				nsMap[pod.Namespace].WritableCount++
			}
		}
	}

	for _, e := range nsMap {
		if e.ContainerCount > 0 {
			e.CoveragePct = float64(e.ContainerCount-e.WritableCount) / float64(e.ContainerCount) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	if result.Summary.TotalContainers > 0 {
		result.HealthScore = result.Summary.ReadOnlyRootFS * 100 / result.Summary.TotalContainers
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("只读根文件系统审计: %d 容器, %d 只读, %d 可写, %d 无安全上下文",
			result.Summary.TotalContainers, result.Summary.ReadOnlyRootFS,
			result.Summary.WritableRootFS, result.Summary.NoSecContext),
	}
	if result.Summary.WritableRootFS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个容器根文件系统可写, 可被篡改植入恶意代码", result.Summary.WritableRootFS))
	}
	if result.HealthScore < 20 {
		result.Recommendations = append(result.Recommendations, "建议: 设置 readOnlyRootFilesystem: true, 配合 emptyDir tmp 挂载写入目录")
	}
	writeJSON(w, result)
}
