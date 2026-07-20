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

// ImageBaselineResult detects image drift from a known-good baseline by tracking image digests.
type ImageBaselineResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         ImageBaselineSummary `json:"summary"`
	ByImage         []ImageBaselineEntry `json:"byImage"`
	DriftedImages   []ImageBaselineEntry `json:"driftedImages"`
	LatestTagImages []ImageBaselineEntry `json:"latestTagImages"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type ImageBaselineSummary struct {
	TotalImages       int `json:"totalImages"`
	UniqueImages      int `json:"uniqueImages"`
	UsingLatestTag    int `json:"usingLatestTag"`
	UsingDigest       int `json:"usingDigest"` // using @sha256
	DriftedImages     int `json:"driftedImages"`
	OldImages         int `json:"oldImages"`
	UnversionedImages int `json:"unversionedImages"`
}

type ImageBaselineEntry struct {
	Image       string   `json:"image"`
	ShortName   string   `json:"shortName"`
	Registry    string   `json:"registry"`
	UsedBy      int      `json:"usedByPods"`
	Namespaces  []string `json:"namespaces"`
	HasVersion  bool     `json:"hasVersionTag"`
	IsLatest    bool     `json:"isLatest"`
	IsDigest    bool     `json:"isDigestPinned"`
	RiskLevel   string   `json:"riskLevel"`
	RiskFactors []string `json:"riskFactors"`
}

// handleImageBaseline handles GET /api/security/image-baseline-drift
func (s *Server) handleImageBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImageBaselineResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	imageMap := make(map[string]*ImageBaselineEntry)

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			result.Summary.TotalImages++

			if imageMap[img] == nil {
				entry := &ImageBaselineEntry{Image: img}

				// Parse registry
				if idx := strings.Index(img, "/"); idx > 0 {
					parts := strings.SplitN(img, "/", 2)
					entry.Registry = parts[0]
					entry.ShortName = parts[1]
				} else {
					entry.ShortName = img
					entry.Registry = "docker.io"
				}

				// Check for digest pinning
				entry.IsDigest = strings.Contains(img, "@sha256")
				if entry.IsDigest {
					result.Summary.UsingDigest++
				}

				// Check for version tag
				if !entry.IsDigest {
					tagPart := img
					if idx := strings.LastIndex(img, ":"); idx > strings.LastIndex(img, "/") {
						tagPart = img[idx+1:]
						entry.HasVersion = true
						if tagPart == "latest" {
							entry.IsLatest = true
							result.Summary.UsingLatestTag++
						}
					} else {
						entry.HasVersion = false
						result.Summary.UnversionedImages++
					}
				}

				imageMap[img] = entry
			}

			entry := imageMap[img]
			entry.UsedBy++
			found := false
			for _, ns := range entry.Namespaces {
				if ns == pod.Namespace {
					found = true
					break
				}
			}
			if !found {
				entry.Namespaces = append(entry.Namespaces, pod.Namespace)
			}
		}
	}

	result.Summary.UniqueImages = len(imageMap)

	// Risk assessment
	for _, entry := range imageMap {
		var risks []string
		if entry.IsLatest {
			risks = append(risks, "uses-latest-tag")
		}
		if !entry.HasVersion && !entry.IsDigest {
			risks = append(risks, "no-version-tag")
		}
		if !entry.IsDigest {
			risks = append(risks, "not-digest-pinned")
		}
		entry.RiskFactors = risks

		switch {
		case len(risks) >= 3:
			entry.RiskLevel = "critical"
			result.Summary.DriftedImages++
			result.DriftedImages = append(result.DriftedImages, *entry)
		case len(risks) >= 2:
			entry.RiskLevel = "high"
			result.Summary.DriftedImages++
			result.DriftedImages = append(result.DriftedImages, *entry)
		case len(risks) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.IsLatest {
			result.LatestTagImages = append(result.LatestTagImages, *entry)
		}
		result.ByImage = append(result.ByImage, *entry)
	}

	sort.Slice(result.ByImage, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByImage[i].RiskLevel] < rank[result.ByImage[j].RiskLevel]
	})

	if result.Summary.UniqueImages > 0 {
		pinned := result.Summary.UsingDigest
		result.HealthScore = pinned * 100 / result.Summary.UniqueImages
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("镜像基线漂移: %d 镜像引用 (%d 唯一), %d 使用 latest, %d digest 锁定, %d 漂移",
			result.Summary.TotalImages, result.Summary.UniqueImages,
			result.Summary.UsingLatestTag, result.Summary.UsingDigest, result.Summary.DriftedImages),
	}
	if result.Summary.UsingLatestTag > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个镜像使用 :latest 标签, 无法保证版本一致性", result.Summary.UsingLatestTag))
	}
	if result.Summary.UsingDigest == 0 {
		result.Recommendations = append(result.Recommendations, "没有任何镜像使用 digest 锁定 (@sha256), 存在供应链安全风险")
	}
	if result.HealthScore < 20 {
		result.Recommendations = append(result.Recommendations, "建议: 使用 image digest 替代 tag, 实施 image policy admission controller")
	}
	writeJSON(w, result)
}
