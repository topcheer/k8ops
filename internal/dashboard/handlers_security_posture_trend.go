package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecurityPostureTrendResult tracks security posture changes over time using events.
type SecurityPostureTrendResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         SecPostureTrendSummary `json:"summary"`
	Trend           []SecTrendPoint        `json:"trend"`
	ByCategory      []SecTrendCategory     `json:"byCategory"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type SecPostureTrendSummary struct {
	TotalPods       int     `json:"totalPods"`
	RunningAsRoot   int     `json:"runningAsRoot"`
	NoSecurityCtx   int     `json:"withoutSecurityContext"`
	PrivilegedCount int     `json:"privilegedContainers"`
	RootFsWritable  int     `json:"writableRootFs"`
	SecIncidents    int     `json:"recentSecurityEvents"`
	PostureScore    float64 `json:"postureScore"`
}

type SecTrendPoint struct {
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Count     int    `json:"count"`
	Severity  string `json:"severity"`
}

type SecTrendCategory struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
	TrendDir string `json:"trendDirection"`
}

// handleSecurityPostureTrend handles GET /api/docs/security-posture-trend
func (s *Server) handleSecurityPostureTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SecurityPostureTrendResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 500})

	catMap := make(map[string]*SecTrendCategory)
	addCat := func(cat, severity string, count int) {
		if catMap[cat] == nil {
			catMap[cat] = &SecTrendCategory{Category: cat, TrendDir: "stable"}
		}
		catMap[cat].Count += count
		if severity == "critical" {
			catMap[cat].Severity = "critical"
		} else if catMap[cat].Severity == "" {
			catMap[cat].Severity = severity
		}
	}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			sc := c.SecurityContext
			if sc == nil {
				result.Summary.NoSecurityCtx++
				addCat("no-security-context", "high", 1)
				result.Trend = append(result.Trend, SecTrendPoint{
					Namespace: pod.Namespace, Issue: "no securityContext",
					Count: 1, Severity: "high",
				})
				continue
			}

			if sc.Privileged != nil && *sc.Privileged {
				result.Summary.PrivilegedCount++
				addCat("privileged", "critical", 1)
			}
			if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				result.Summary.RunningAsRoot++
				addCat("runs-as-root", "high", 1)
			}
			if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				result.Summary.RootFsWritable++
				addCat("writable-rootfs", "medium", 1)
			}
			if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
				addCat("privilege-escalation", "critical", 1)
			}
		}
	}

	// Count security-related events
	for _, ev := range events.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		if ev.Type == corev1.EventTypeWarning {
			msg := ev.Message
			if containsStr1876(msg, "security") || containsStr1876(msg, "forbidden") ||
				containsStr1876(msg, "denied") || containsStr1876(msg, "RBAC") {
				result.Summary.SecIncidents++
				addCat("security-events", "medium", 1)
			}
		}
	}

	for _, c := range catMap {
		result.ByCategory = append(result.ByCategory, *c)
	}
	sort.Slice(result.ByCategory, func(i, j int) bool {
		return result.ByCategory[i].Count > result.ByCategory[j].Count
	})

	// Calculate posture score
	totalContainers := result.Summary.TotalPods
	if totalContainers > 0 {
		issues := result.Summary.PrivilegedCount*2 + result.Summary.RunningAsRoot + result.Summary.NoSecurityCtx
		result.Summary.PostureScore = (1 - float64(issues)/float64(totalContainers*2)) * 100
		if result.Summary.PostureScore < 0 {
			result.Summary.PostureScore = 0
		}
	}
	result.HealthScore = int(result.Summary.PostureScore)
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("安全态势趋势: %d Pod, %d root, %d 无安全上下文, %d 特权, %d 可写根FS, %d 安全事件",
			result.Summary.TotalPods, result.Summary.RunningAsRoot,
			result.Summary.NoSecurityCtx, result.Summary.PrivilegedCount,
			result.Summary.RootFsWritable, result.Summary.SecIncidents),
	}
	if result.Summary.PrivilegedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个特权容器需要立即修复", result.Summary.PrivilegedCount))
	}
	if result.HealthScore < 30 {
		result.Recommendations = append(result.Recommendations, "安全态势严重不足, 建议实施 Pod Security Standards")
	}
	writeJSON(w, result)
}
