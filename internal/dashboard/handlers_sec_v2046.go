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
// v20.46 — Security Dimension (Round 27)
// 1. Root FS Writable Audit — containers with writable root filesystem
// 2. Host Path Mount Audit — hostPath volume exposure
// 3. Token Secret Rotation — long-lived service account token secrets
// ============================================================

// ---------------------------------------------------------------
// 1. Root FS Writable Audit
// ---------------------------------------------------------------

type RootFSResult2046 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         RootFSSummary2046 `json:"summary"`
	WritableRootFS  []RootFSEntry2046 `json:"writableRootFS"`
	Recommendations []string          `json:"recommendations"`
}

type RootFSSummary2046 struct {
	TotalContainers int `json:"totalContainers"`
	ReadOnlyRootFS  int `json:"readOnlyRootFS"`
	WritableRootFS  int `json:"writableRootFS"`
}

type RootFSEntry2046 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleRootFSAudit(w http.ResponseWriter, r *http.Request) {
	result := RootFSResult2046{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			isReadOnly := false
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil {
				isReadOnly = *c.SecurityContext.ReadOnlyRootFilesystem
			}

			if isReadOnly {
				result.Summary.ReadOnlyRootFS++
			} else {
				result.Summary.WritableRootFS++
				result.WritableRootFS = append(result.WritableRootFS, RootFSEntry2046{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.WritableRootFS, func(i, j int) bool {
		return result.WritableRootFS[i].Namespace < result.WritableRootFS[j].Namespace
	})

	if result.Summary.WritableRootFS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have writable root filesystem — set readOnlyRootFilesystem: true", result.Summary.WritableRootFS))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Host Path Mount Audit
// ---------------------------------------------------------------

type HostPathResult2046 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HostPathSummary2046 `json:"summary"`
	HostPathMounts  []HostPathEntry2046 `json:"hostPathMounts"`
	Recommendations []string            `json:"recommendations"`
}

type HostPathSummary2046 struct {
	TotalPods        int `json:"totalPods"`
	PodsWithHostPath int `json:"podsWithHostPath"`
	TotalMounts      int `json:"totalMounts"`
}

type HostPathEntry2046 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	HostPath  string `json:"hostPath"`
	MountPath string `json:"mountPath"`
}

func (s *Server) handleHostPathAudit2046(w http.ResponseWriter, r *http.Request) {
	result := HostPathResult2046{ScannedAt: time.Now()}
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
				result.Summary.TotalMounts++
				hasHostPath = true
				result.HostPathMounts = append(result.HostPathMounts, HostPathEntry2046{
					Pod: pod.Name, Namespace: pod.Namespace,
					HostPath: vol.HostPath.Path,
				})
				score -= 3
			}
		}

		if hasHostPath {
			result.Summary.PodsWithHostPath++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HostPathMounts, func(i, j int) bool {
		return result.HostPathMounts[i].Namespace < result.HostPathMounts[j].Namespace
	})

	if result.Summary.PodsWithHostPath > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods mount hostPath volumes — security risk, use PVCs instead", result.Summary.PodsWithHostPath))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Token Secret Rotation
// ---------------------------------------------------------------

type TokenRotResult2046 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TokenRotSummary2046 `json:"summary"`
	OldTokens       []TokenRotEntry2046 `json:"oldTokens"`
	Recommendations []string            `json:"recommendations"`
}

type TokenRotSummary2046 struct {
	TotalTokens     int `json:"totalTokenSecrets"`
	OldTokens       int `json:"oldTokens"`
	ManuallyCreated int `json:"manuallyCreated"`
}

type TokenRotEntry2046 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
}

func (s *Server) handleTokenRotAudit(w http.ResponseWriter, r *http.Request) {
	result := TokenRotResult2046{ScannedAt: time.Now()}
	score := 100

	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, secret := range secretList.Items {
		if secret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}
		result.Summary.TotalTokens++

		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)

		if ageDays > 90 {
			result.Summary.OldTokens++
			result.OldTokens = append(result.OldTokens, TokenRotEntry2046{
				Name: secret.Name, Namespace: secret.Namespace, AgeDays: ageDays,
			})
			score -= 2
		}

		// Check if manually created (has annotation but no auto-generated label)
		if secret.Annotations["kubernetes.io/service-account.name"] != "" &&
			secret.Labels == nil {
			result.Summary.ManuallyCreated++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OldTokens, func(i, j int) bool {
		return result.OldTokens[i].AgeDays > result.OldTokens[j].AgeDays
	})

	if result.Summary.OldTokens > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d SA token secrets older than 90 days — review rotation policy", result.Summary.OldTokens))
	}

	writeJSON(w, result)
}
