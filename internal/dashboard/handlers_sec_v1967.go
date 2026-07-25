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
// v19.67 — Security Dimension (Round 14)
// 1. Run-As-Non-Root Audit — containers running as root user
// 2. Host PID/IPC Audit — host namespace sharing exposure
// 3. Image Digest Pinning — image tag vs digest immutability
// ============================================================

// ---------------------------------------------------------------
// 1. Run-As-Non-Root Audit
// ---------------------------------------------------------------

type RunAsNonRootResult1967 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RunAsNonRootSummary1967 `json:"summary"`
	Violations      []RunAsNonRootEntry1967 `json:"violations"`
	Recommendations []string                `json:"recommendations"`
}

type RunAsNonRootSummary1967 struct {
	TotalContainers  int `json:"totalContainers"`
	RunAsRoot        int `json:"runningAsRoot"`
	WithRunAsNonRoot int `json:"withRunAsNonRoot"`
	WithRunAsUser    int `json:"withExplicitUserID"`
	WithFSGroup      int `json:"withFSGroup"`
	NoSecurityCtx    int `json:"withoutSecurityContext"`
}

type RunAsNonRootEntry1967 struct {
	Container string `json:"container"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	RunAsUser int64  `json:"runAsUser"`
	IsRoot    bool   `json:"isRoot"`
	Issue     string `json:"issue"`
}

func (s *Server) handleRunAsNonRootAudit(w http.ResponseWriter, r *http.Request) {
	result := RunAsNonRootResult1967{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Pod-level security context
		podSC := pod.Spec.SecurityContext
		podRunAsNonRoot := false
		podRunAsUser := int64(-1)
		if podSC != nil {
			if podSC.RunAsNonRoot != nil && *podSC.RunAsNonRoot {
				podRunAsNonRoot = true
			}
			if podSC.RunAsUser != nil {
				podRunAsUser = *podSC.RunAsUser
			}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			cRunAsNonRoot := podRunAsNonRoot
			cRunAsUser := podRunAsUser
			hasSC := c.SecurityContext != nil

			if hasSC {
				if c.SecurityContext.RunAsNonRoot != nil {
					cRunAsNonRoot = *c.SecurityContext.RunAsNonRoot
				}
				if c.SecurityContext.RunAsUser != nil {
					cRunAsUser = *c.SecurityContext.RunAsUser
					result.Summary.WithRunAsUser++
				}
			} else {
				result.Summary.NoSecurityCtx++
			}

			if cRunAsNonRoot {
				result.Summary.WithRunAsNonRoot++
			} else {
				// Running without runAsNonRoot=true
				isRoot := false
				issue := "missing runAsNonRoot constraint"

				if cRunAsUser == 0 {
					isRoot = true
					issue = "explicitly running as UID 0 (root)"
				} else if cRunAsUser < 0 && !cRunAsNonRoot {
					isRoot = true
					issue = "no runAsUser specified — defaults to root"
				}

				if isRoot {
					result.Summary.RunAsRoot++
					result.Violations = append(result.Violations, RunAsNonRootEntry1967{
						Container: c.Name, Pod: pod.Name, Namespace: pod.Namespace,
						RunAsUser: cRunAsUser, IsRoot: true, Issue: issue,
					})
					if cRunAsUser == 0 {
						score -= 5
					} else {
						score -= 2
					}
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d runAsNonRoot, %d running as root", result.Summary.TotalContainers, result.Summary.WithRunAsNonRoot, result.Summary.RunAsRoot))
	if result.Summary.RunAsRoot > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers running as root — set runAsNonRoot: true or specify non-zero UID", result.Summary.RunAsRoot))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Host PID/IPC Audit
// ---------------------------------------------------------------

type HostPIDIPCResult1967 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         HostPIDIPCSummary1967 `json:"summary"`
	Violations      []HostPIDIPCEntry1967 `json:"violations"`
	Recommendations []string              `json:"recommendations"`
}

type HostPIDIPCSummary1967 struct {
	TotalPods       int `json:"totalPods"`
	HostPIDPods     int `json:"hostPIDPods"`
	HostIPCPods     int `json:"hostIPCPods"`
	HostNetworkPods int `json:"hostNetworkPods"`
	ShareProcessNS  int `json:"shareProcessNamespace"`
}

type HostPIDIPCEntry1967 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

func (s *Server) handleHostPIDIPCAudit(w http.ResponseWriter, r *http.Request) {
	result := HostPIDIPCResult1967{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		violations := []HostPIDIPCEntry1967{}

		if pod.Spec.HostPID {
			result.Summary.HostPIDPods++
			violations = append(violations, HostPIDIPCEntry1967{
				Name: pod.Name, Namespace: pod.Namespace,
				Issue:    "hostPID: true — container can see host processes",
				Severity: "high",
			})
			score -= 5
		}

		if pod.Spec.HostIPC {
			result.Summary.HostIPCPods++
			violations = append(violations, HostPIDIPCEntry1967{
				Name: pod.Name, Namespace: pod.Namespace,
				Issue:    "hostIPC: true — container shares IPC namespace with host",
				Severity: "high",
			})
			score -= 5
		}

		if pod.Spec.HostNetwork {
			result.Summary.HostNetworkPods++
			violations = append(violations, HostPIDIPCEntry1967{
				Name: pod.Name, Namespace: pod.Namespace,
				Issue:    "hostNetwork: true — container uses host network stack",
				Severity: "medium",
			})
			score -= 2
		}

		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.ShareProcessNS++
			violations = append(violations, HostPIDIPCEntry1967{
				Name: pod.Name, Namespace: pod.Namespace,
				Issue:    "shareProcessNamespace: true — all containers share PID namespace",
				Severity: "low",
			})
		}

		result.Violations = append(result.Violations, violations...)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	totalViolations := result.Summary.HostPIDPods + result.Summary.HostIPCPods + result.Summary.HostNetworkPods
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods scanned, %d with host namespace sharing", result.Summary.TotalPods, totalViolations))
	if totalViolations > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods using hostPID/hostIPC/hostNetwork — remove unless explicitly required", totalViolations))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Image Digest Pinning
// ---------------------------------------------------------------

type ImageDigestResult1967 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ImageDigestSummary1967 `json:"summary"`
	UnpinnedImages  []ImageDigestEntry1967 `json:"unpinnedImages"`
	PinnedImages    []ImageDigestEntry1967 `json:"pinnedImages"`
	Recommendations []string               `json:"recommendations"`
}

type ImageDigestSummary1967 struct {
	TotalImages      int `json:"totalUniqueImages"`
	PinnedByDigest   int `json:"pinnedByDigest"`
	UsingLatest      int `json:"usingLatestTag"`
	UsingFloatingTag int `json:"usingFloatingTag"`
	TotalContainers  int `json:"totalContainers"`
}

type ImageDigestEntry1967 struct {
	Image    string `json:"image"`
	IsDigest bool   `json:"isDigestPinned"`
	IsLatest bool   `json:"isLatest"`
	Tag      string `json:"tag"`
	UseCount int    `json:"useCount"`
}

func (s *Server) handleImageDigestPinning(w http.ResponseWriter, r *http.Request) {
	result := ImageDigestResult1967{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imageMap := make(map[string]*ImageDigestEntry1967)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			img := c.Image

			entry, ok := imageMap[img]
			if !ok {
				entry = &ImageDigestEntry1967{Image: img}
				imageMap[img] = entry

				// Parse image reference
				// Check if pinned by digest (contains @sha256:)
				if strings.Contains(img, "@sha256:") {
					entry.IsDigest = true
					entry.Tag = "digest"
				} else {
					// Extract tag
					parts := strings.Split(img, ":")
					if len(parts) > 1 {
						tag := parts[len(parts)-1]
						entry.Tag = tag
						if tag == "latest" {
							entry.IsLatest = true
						}
					} else {
						entry.Tag = "latest" // implicit latest
						entry.IsLatest = true
					}
				}
			}
			entry.UseCount++
		}
	}

	for _, entry := range imageMap {
		result.Summary.TotalImages++

		if entry.IsDigest {
			result.Summary.PinnedByDigest++
			result.PinnedImages = append(result.PinnedImages, *entry)
		} else {
			if entry.IsLatest {
				result.Summary.UsingLatest++
				score -= 3
			} else {
				result.Summary.UsingFloatingTag++
				score -= 1
			}
			result.UnpinnedImages = append(result.UnpinnedImages, *entry)
		}
	}

	sort.Slice(result.UnpinnedImages, func(i, j int) bool {
		return result.UnpinnedImages[i].UseCount > result.UnpinnedImages[j].UseCount
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unique images: %d digest-pinned, %d latest, %d floating tags", result.Summary.TotalImages, result.Summary.PinnedByDigest, result.Summary.UsingLatest, result.Summary.UsingFloatingTag))
	if result.Summary.UsingLatest > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images using :latest — pin to specific version or digest", result.Summary.UsingLatest))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
