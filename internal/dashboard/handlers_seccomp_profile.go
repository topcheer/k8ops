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

// SeccompProfileResult audits seccomp profile settings across all pods.
type SeccompProfileResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         SeccompSummary   `json:"summary"`
	ByNamespace     []SeccompNSEntry `json:"byNamespace"`
	UnprotectedPods []SeccompEntry   `json:"unprotectedPods"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type SeccompSummary struct {
	TotalPods      int `json:"totalPods"`
	WithSeccomp    int `json:"withSeccompProfile"`
	DefaultProfile int `json:"defaultProfile"`
	CustomProfile  int `json:"customProfile"`
	NoProfile      int `json:"withoutSeccomp"`
	Unconfined     int `json:"unconfined"`
}

type SeccompNSEntry struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	Protected   int     `json:"protectedPods"`
	CoveragePct float64 `json:"coveragePct"`
}

type SeccompEntry struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Profile   string `json:"seccompProfile"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// handleSeccompProfileAudit handles GET /api/security/seccomp-profile-audit
func (s *Server) handleSeccompProfileAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SeccompProfileResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*SeccompNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &SeccompNSEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++

		// Check pod-level seccomp
		podSeccomp := ""
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			sp := pod.Spec.SecurityContext.SeccompProfile
			if sp.Type == corev1.SeccompProfileTypeRuntimeDefault {
				podSeccomp = "RuntimeDefault"
			} else if sp.Type == corev1.SeccompProfileTypeLocalhost {
				podSeccomp = "Localhost:" + *sp.LocalhostProfile
			} else if sp.Type == corev1.SeccompProfileTypeUnconfined {
				podSeccomp = "Unconfined"
			}
		}

		// Check container-level seccomp
		containerSeccomp := ""
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				sp := c.SecurityContext.SeccompProfile
				if sp.Type == corev1.SeccompProfileTypeRuntimeDefault {
					containerSeccomp = "RuntimeDefault"
				} else if sp.Type == corev1.SeccompProfileTypeUnconfined {
					containerSeccomp = "Unconfined"
				}
			}
		}

		profile := podSeccomp
		if profile == "" {
			profile = containerSeccomp
		}

		entry := SeccompEntry{PodName: pod.Name, Namespace: pod.Namespace, Profile: profile}

		switch {
		case profile == "RuntimeDefault":
			result.Summary.WithSeccomp++
			result.Summary.DefaultProfile++
			nsMap[pod.Namespace].Protected++
		case strings.HasPrefix(profile, "Localhost"):
			result.Summary.WithSeccomp++
			result.Summary.CustomProfile++
			nsMap[pod.Namespace].Protected++
		case profile == "Unconfined":
			result.Summary.Unconfined++
			entry.Reason = "explicitly unconfined - no syscall filtering"
			entry.Severity = "high"
			result.UnprotectedPods = append(result.UnprotectedPods, entry)
		default:
			result.Summary.NoProfile++
			entry.Reason = "no seccomp profile - inherits container runtime default"
			entry.Severity = "medium"
			result.UnprotectedPods = append(result.UnprotectedPods, entry)
		}
	}

	for _, e := range nsMap {
		if e.PodCount > 0 {
			e.CoveragePct = float64(e.Protected) / float64(e.PodCount) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	if result.Summary.TotalPods > 0 {
		result.HealthScore = result.Summary.WithSeccomp * 100 / result.Summary.TotalPods
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Seccomp 审计: %d Pod, %d 有 profile, %d 默认, %d 自定义, %d 无, %d unconfined",
			result.Summary.TotalPods, result.Summary.WithSeccomp,
			result.Summary.DefaultProfile, result.Summary.CustomProfile,
			result.Summary.NoProfile, result.Summary.Unconfined),
	}
	if result.Summary.NoProfile > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 无 seccomp profile, 建议设置 RuntimeDefault", result.Summary.NoProfile))
	}
	writeJSON(w, result)
}
