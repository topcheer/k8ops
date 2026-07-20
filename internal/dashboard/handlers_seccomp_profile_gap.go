package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SeccompProfileGapResult analyzes which workloads lack seccomp profiles,
// leaving them vulnerable to unnecessary kernel syscall access.
type SeccompProfileGapResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         SeccompGapSummary `json:"summary"`
	ByWorkload      []SeccompGapEntry `json:"byWorkload"`
	Unprotected     []SeccompGapEntry `json:"unprotected"`
	GapScore        int               `json:"gapScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type SeccompGapSummary struct {
	TotalPods      int `json:"totalPods"`
	WithSeccomp    int `json:"withSeccomp"`
	WithoutSeccomp int `json:"withoutSeccomp"`
	RuntimeDefault int `json:"runtimeDefault"`
	Localhost      int `json:"localhostProfile"`
	Unconfined     int `json:"unconfined"`
}

type SeccompGapEntry struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	Workload   string `json:"workload"`
	Container  string `json:"container"`
	HasSeccomp bool   `json:"hasSeccomp"`
	Profile    string `json:"profileType"`
	RiskLevel  string `json:"riskLevel"`
}

// handleSeccompProfileGap handles GET /api/security/seccomp-profile-gap
func (s *Server) handleSeccompProfileGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SeccompProfileGapResult{ScannedAt: time.Now()}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}

		// Check pod-level seccomp
		podSeccomp := ""
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			sp := pod.Spec.SecurityContext.SeccompProfile
			if sp.Type == corev1.SeccompProfileTypeRuntimeDefault {
				podSeccomp = "RuntimeDefault"
			} else if sp.Type == corev1.SeccompProfileTypeLocalhost {
				podSeccomp = "Localhost"
			} else if sp.Type == corev1.SeccompProfileTypeUnconfined {
				podSeccomp = "Unconfined"
			}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalPods++
			entry := SeccompGapEntry{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Workload:  wlName,
				Container: c.Name,
			}

			// Check container-level seccomp (overrides pod-level)
			profile := podSeccomp
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				sp := c.SecurityContext.SeccompProfile
				if sp.Type == corev1.SeccompProfileTypeRuntimeDefault {
					profile = "RuntimeDefault"
				} else if sp.Type == corev1.SeccompProfileTypeLocalhost {
					profile = "Localhost"
				} else if sp.Type == corev1.SeccompProfileTypeUnconfined {
					profile = "Unconfined"
				}
			}

			if profile != "" {
				entry.HasSeccomp = true
				entry.Profile = profile
				result.Summary.WithSeccomp++
				switch profile {
				case "RuntimeDefault":
					result.Summary.RuntimeDefault++
				case "Localhost":
					result.Summary.Localhost++
				case "Unconfined":
					result.Summary.Unconfined++
					entry.HasSeccomp = false // unconfined = no protection
					result.Summary.WithoutSeccomp++
					entry.RiskLevel = "high"
					result.Unprotected = append(result.Unprotected, entry)
				}
			} else {
				entry.HasSeccomp = false
				entry.Profile = "none"
				entry.RiskLevel = "medium"
				result.Summary.WithoutSeccomp++
				result.Unprotected = append(result.Unprotected, entry)
			}

			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	// Sort unprotected by risk
	sort.Slice(result.Unprotected, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.Unprotected[i].RiskLevel] < rank[result.Unprotected[j].RiskLevel]
	})

	if result.Summary.TotalPods > 0 {
		protectedRatio := float64(result.Summary.WithSeccomp) / float64(result.Summary.TotalPods)
		result.GapScore = int(protectedRatio * 100)
	}
	gradeFromScore(&result.Grade, result.GapScore)

	result.Recommendations = []string{
		fmt.Sprintf("Seccomp 覆盖: %d/%d 容器受保护 (%d%%)", result.Summary.WithSeccomp, result.Summary.TotalPods, result.GapScore),
		fmt.Sprintf("RuntimeDefault: %d, Localhost: %d, Unconfined: %d, None: %d",
			result.Summary.RuntimeDefault, result.Summary.Localhost, result.Summary.Unconfined,
			result.Summary.WithoutSeccomp-result.Summary.Unconfined),
	}
	if len(result.Unprotected) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个容器缺少 seccomp 保护", len(result.Unprotected)))
	}
	if result.GapScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 全局设置 seccompProfile.type=RuntimeDefault, 通过 PSA enforce")
	}
	writeJSON(w, result)
}
