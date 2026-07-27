package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.03 — Security Dimension (Round 20)
// 1. FSGroup Audit — pod securityContext.fsGroup compliance
// 2. Proc Mount Type — proc mount type (Default/Unmasked) audit
// 3. Kernel Param Access — sysctl configuration exposure
// ============================================================

// ---------------------------------------------------------------
// 1. FSGroup Audit
// ---------------------------------------------------------------

type FSGroupResult2003 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         FSGroupSummary2003 `json:"summary"`
	Pods            []FSGroupEntry2003 `json:"pods"`
	Recommendations []string           `json:"recommendations"`
}

type FSGroupSummary2003 struct {
	TotalPods    int `json:"totalPods"`
	WithFSGroup  int `json:"withFSGroup"`
	Without      int `json:"withoutFSGroup"`
	WithVolMount int `json:"withVolumeMounts"`
}

type FSGroupEntry2003 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	FSGroup   *int64 `json:"fsGroup"`
}

func (s *Server) handleFSGroupAudit(w http.ResponseWriter, r *http.Request) {
	result := FSGroupResult2003{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasVolMount := false
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil || vol.ConfigMap != nil || vol.Secret != nil {
				hasVolMount = true
				break
			}
		}
		if hasVolMount {
			result.Summary.WithVolMount++
		}

		entry := FSGroupEntry2003{
			Pod: pod.Name, Namespace: pod.Namespace,
		}

		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroup != nil {
			result.Summary.WithFSGroup++
			entry.FSGroup = pod.Spec.SecurityContext.FSGroup
		} else {
			result.Summary.Without++
		}

		result.Pods = append(result.Pods, entry)
	}

	if result.Summary.WithVolMount > 0 && result.Summary.WithFSGroup == 0 {
		result.Recommendations = append(result.Recommendations, "Pods with volume mounts should set fsGroup for proper file ownership")
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with fsGroup, %d without, %d with volume mounts", result.Summary.TotalPods, result.Summary.WithFSGroup, result.Summary.Without, result.Summary.WithVolMount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Proc Mount Type
// ---------------------------------------------------------------

type ProcMountResult2003 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ProcMountSummary2003 `json:"summary"`
	Issues          []ProcMountEntry2003 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type ProcMountSummary2003 struct {
	TotalPods     int `json:"totalPods"`
	WithProcMount int `json:"withProcMountType"`
	Unmasked      int `json:"unmasked"`
	Default       int `json:"defaultProc"`
}

type ProcMountEntry2003 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Type      string `json:"procMountType"`
}

func (s *Server) handleProcMountType(w http.ResponseWriter, r *http.Request) {
	result := ProcMountResult2003{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Check container-level proc mount
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.ProcMount != nil {
				result.Summary.WithProcMount++
				mountType := string(*c.SecurityContext.ProcMount)
				entry := ProcMountEntry2003{
					Pod: pod.Name, Namespace: pod.Namespace, Type: mountType,
				}

				if mountType == "Unmasked" {
					result.Summary.Unmasked++
					result.Issues = append(result.Issues, entry)
					score -= 5
				} else {
					result.Summary.Default++
				}
			}
		}
		result.Summary.TotalPods++
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with procMount, %d unmasked, %d default", result.Summary.TotalPods, result.Summary.WithProcMount, result.Summary.Unmasked, result.Summary.Default))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Kernel Param Access
// ---------------------------------------------------------------

type KernelParamResult2003 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         KernelParamSummary2003 `json:"summary"`
	Issues          []KernelParamEntry2003 `json:"issues"`
	Recommendations []string               `json:"recommendations"`
}

type KernelParamSummary2003 struct {
	TotalPods  int `json:"totalPods"`
	WithSysctl int `json:"withSysctls"`
	Dangerous  int `json:"dangerousSysctls"`
	Safe       int `json:"safeSysctls"`
}

type KernelParamEntry2003 struct {
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Sysctls   []string `json:"sysctls"`
}

var dangerousSysctls2003 = map[string]bool{
	"kernel.shm_rmid_forced":       true,
	"kernel.core_pattern":          true,
	"net.core.somaxconn":           true,
	"net.ipv4.ip_local_port_range": true,
	"net.ipv4.tcp_tw_reuse":        true,
	"vm.overcommit_memory":         true,
	"kernel.sem":                   true,
	"fs.may_detach_mounts":         true,
	"kernel.kexec_load_disabled":   true,
}

func (s *Server) handleKernelParamAccess(w http.ResponseWriter, r *http.Request) {
	result := KernelParamResult2003{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.Sysctls) > 0 {
			result.Summary.WithSysctl++
			entry := KernelParamEntry2003{
				Pod: pod.Name, Namespace: pod.Namespace,
			}

			hasDangerous := false
			for _, sysctl := range pod.Spec.SecurityContext.Sysctls {
				entry.Sysctls = append(entry.Sysctls, sysctl.Name)
				if dangerousSysctls2003[sysctl.Name] {
					hasDangerous = true
				}
			}

			if hasDangerous {
				result.Summary.Dangerous++
				result.Issues = append(result.Issues, entry)
				score -= 5
			} else {
				result.Summary.Safe++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with sysctls, %d dangerous, %d safe", result.Summary.TotalPods, result.Summary.WithSysctl, result.Summary.Dangerous, result.Summary.Safe))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
