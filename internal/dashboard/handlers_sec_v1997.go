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

// ============================================================
// v19.97 — Security Dimension (Round 19)
// 1. Pod HostPath Audit — hostPath volume exposure tracking
// 2. Container ReadOnlyRootFS — read-only filesystem compliance
// 3. SA Token Age — service account token staleness estimator
// ============================================================

// ---------------------------------------------------------------
// 1. Pod HostPath Audit
// ---------------------------------------------------------------

type HostPathResult1997 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HostPathSummary1997 `json:"summary"`
	Violations      []HostPathEntry1997 `json:"violations"`
	Recommendations []string            `json:"recommendations"`
}

type HostPathSummary1997 struct {
	TotalPods      int `json:"totalPods"`
	WithHostPath   int `json:"withHostPathVolumes"`
	TotalMounts    int `json:"totalHostPathMounts"`
	WritableMounts int `json:"writableMounts"`
}

type HostPathEntry1997 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	HostPath  string `json:"hostPath"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

func (s *Server) handleHostPathAuditV2(w http.ResponseWriter, r *http.Request) {
	result := HostPathResult1997{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasHostPath := false
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				hasHostPath = true
				result.Summary.TotalMounts++

				// Check if writable (default is false in newer K8s)
				ro := false
				if vol.HostPath.Type != nil {
					t := *vol.HostPath.Type
					if t == corev1.HostPathDirectoryOrCreate || t == corev1.HostPathFileOrCreate {
						ro = false
					}
				}

				entry := HostPathEntry1997{
					Pod: pod.Name, Namespace: pod.Namespace,
					HostPath:  vol.HostPath.Path,
					MountPath: vol.Name, ReadOnly: ro,
				}

				// Check for dangerous paths
				dangerous := false
				for _, p := range []string{"/etc", "/proc", "/sys", "/var/run", "/var/lib/kubelet", "/var/lib/docker"} {
					if strings.HasPrefix(vol.HostPath.Path, p) {
						dangerous = true
						break
					}
				}
				if dangerous {
					score -= 5
				}
				if !ro {
					result.Summary.WritableMounts++
				}

				result.Violations = append(result.Violations, entry)
			}
		}

		if hasHostPath {
			result.Summary.WithHostPath++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with hostPath, %d mounts (%d writable)", result.Summary.TotalPods, result.Summary.WithHostPath, result.Summary.TotalMounts, result.Summary.WritableMounts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container ReadOnlyRootFS
// ---------------------------------------------------------------

type ReadOnlyFSResult1997 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ReadOnlyFSSummary1997 `json:"summary"`
	Issues          []ReadOnlyFSEntry1997 `json:"issues"`
	Recommendations []string              `json:"recommendations"`
}

type ReadOnlyFSSummary1997 struct {
	TotalContainers int `json:"totalContainers"`
	ReadOnlyRootFS  int `json:"withReadOnlyRootFS"`
	WritableRootFS  int `json:"withWritableRootFS"`
	NotSet          int `json:"notSet"`
}

type ReadOnlyFSEntry1997 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleReadOnlyRootFSV2(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyFSResult1997{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil {
				if *c.SecurityContext.ReadOnlyRootFilesystem {
					result.Summary.ReadOnlyRootFS++
				} else {
					result.Summary.WritableRootFS++
					result.Issues = append(result.Issues, ReadOnlyFSEntry1997{
						Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					})
					score -= 1
				}
			} else {
				result.Summary.NotSet++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d readonly, %d writable, %d not set", result.Summary.TotalContainers, result.Summary.ReadOnlyRootFS, result.Summary.WritableRootFS, result.Summary.NotSet))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. SA Token Age
// ---------------------------------------------------------------

type SATokenAgeResult1997 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenAgeSummary1997 `json:"summary"`
	OldTokens       []SATokenAgeEntry1997 `json:"oldTokens"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenAgeSummary1997 struct {
	TotalSAs      int     `json:"totalServiceAccounts"`
	WithAutoMount int     `json:"withAutoMount"`
	OldSAs        int     `json:"oldServiceAccounts90d"`
	AvgAgeDays    float64 `json:"avgAgeDays"`
}

type SATokenAgeEntry1997 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	AgeDays   float64 `json:"ageDays"`
}

func (s *Server) handleSATokenAgeV3(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult1997{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	var totalAge float64
	var count int

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++

		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			result.Summary.WithAutoMount++
		}

		if sa.CreationTimestamp.IsZero() {
			continue
		}

		ageDays := time.Since(sa.CreationTimestamp.Time).Hours() / 24
		totalAge += ageDays
		count++

		if ageDays > 90 {
			result.Summary.OldSAs++
			result.OldTokens = append(result.OldTokens, SATokenAgeEntry1997{
				Name: sa.Name, Namespace: sa.Namespace, AgeDays: ageDays,
			})
		}
	}

	if count > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(count)
	}

	sort.Slice(result.OldTokens, func(i, j int) bool {
		return result.OldTokens[i].AgeDays > result.OldTokens[j].AgeDays
	})
	if len(result.OldTokens) > 20 {
		result.OldTokens = result.OldTokens[:20]
	}

	if result.Summary.OldSAs > 20 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs: %d automount, %d old (>90d), avg %.0fd", result.Summary.TotalSAs, result.Summary.WithAutoMount, result.Summary.OldSAs, result.Summary.AvgAgeDays))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
