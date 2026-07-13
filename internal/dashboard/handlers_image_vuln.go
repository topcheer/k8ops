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

// ImageVulnResult is the container image vulnerability & patch lag audit.
type ImageVulnResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ImageVulnSummary  `json:"summary"`
	ByImage         []ImageVulnEntry  `json:"byImage"`
	ByNamespace     []ImageVulnNSStat `json:"byNamespace"`
	StaleImages     []StaleImageEntry `json:"staleImages"`
	Recommendations []string          `json:"recommendations"`
}

// ImageVulnSummary aggregates image vulnerability statistics.
type ImageVulnSummary struct {
	TotalImages   int `json:"totalImages"`
	LatestTag     int `json:"latestTag"`     // using :latest tag
	OldImages     int `json:"oldImages"`     // no tag or very old tag
	NoDigest      int `json:"noDigest"`      // no image digest (not pinned)
	UniqueImages  int `json:"uniqueImages"`  // distinct image:tag combos
	DuplicateTags int `json:"duplicateTags"` // same image with different tags
	HealthScore   int `json:"healthScore"`
}

// ImageVulnEntry describes one image's usage.
type ImageVulnEntry struct {
	Image      string   `json:"image"`
	Tag        string   `json:"tag"`
	PodCount   int      `json:"podCount"`
	Namespaces []string `json:"namespaces,omitempty"`
	HasDigest  bool     `json:"hasDigest"`
	IsLatest   bool     `json:"isLatest"`
	RiskLevel  string   `json:"riskLevel"`
}

// ImageVulnNSStat shows image stats per namespace.
type ImageVulnNSStat struct {
	Namespace   string `json:"namespace"`
	TotalImages int    `json:"totalImages"`
	LatestCount int    `json:"latestCount"`
	OldCount    int    `json:"oldCount"`
}

// StaleImageEntry describes a stale/outdated image.
type StaleImageEntry struct {
	Image     string `json:"image"`
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// imageVulnAuditCore performs the audit on pods (testable).
func imageVulnAuditCore(pods []corev1.Pod) ImageVulnResult {
	result := ImageVulnResult{
		ScannedAt: time.Now(),
	}

	imageMap := make(map[string]*ImageVulnEntry)
	nsStats := make(map[string]*ImageVulnNSStat)

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &ImageVulnNSStat{Namespace: ns}
		}

		for _, c := range pod.Spec.Containers {
			image := c.Image
			result.Summary.TotalImages++
			nsStats[ns].TotalImages++

			// Parse image:tag
			tag := "latest"
			hasDigest := strings.Contains(image, "@sha256:")
			imageRef := image
			if hasDigest {
				imageRef = strings.Split(image, "@")[0]
			}

			if idx := strings.LastIndex(imageRef, ":"); idx > 0 {
				// Check if the part after : is a tag (not a port in registry:port/repo)
				possibleTag := imageRef[idx+1:]
				if !strings.Contains(possibleTag, "/") {
					tag = possibleTag
					imageRef = imageRef[:idx]
				}
			}

			isLatest := tag == "latest"
			isOld := tag == "" || tag == "latest" || strings.Contains(tag, "old") || strings.Contains(tag, "legacy")

			key := fmt.Sprintf("%s:%s", imageRef, tag)
			if hasDigest {
				key = image // use full image with digest as key
			}

			if _, ok := imageMap[key]; !ok {
				imageMap[key] = &ImageVulnEntry{
					Image:     imageRef,
					Tag:       tag,
					HasDigest: hasDigest,
					IsLatest:  isLatest,
				}
				result.Summary.UniqueImages++
			}
			imageMap[key].PodCount++
			imageMap[key].Namespaces = append(imageMap[key].Namespaces, ns)

			if isLatest {
				result.Summary.LatestTag++
				nsStats[ns].LatestCount++
			}
			if isOld && !isLatest {
				result.Summary.OldImages++
				nsStats[ns].OldCount++
			} else if isLatest {
				result.Summary.OldImages++
				nsStats[ns].OldCount++
			}
			if !hasDigest {
				result.Summary.NoDigest++
			}

			// Flag stale images
			if isLatest {
				result.StaleImages = append(result.StaleImages, StaleImageEntry{
					Image: image, PodName: pod.Name, Namespace: ns,
					Reason:   "using :latest tag — image version is non-reproducible",
					Severity: "medium",
				})
			}
			if !hasDigest && !isLatest {
				result.StaleImages = append(result.StaleImages, StaleImageEntry{
					Image: image, PodName: pod.Name, Namespace: ns,
					Reason:   "image not pinned by digest — cannot guarantee reproducibility",
					Severity: "low",
				})
			}
		}
	}

	// Build image entries
	for _, entry := range imageMap {
		entry.RiskLevel = "low"
		if entry.IsLatest {
			entry.RiskLevel = "medium"
		}
		if !entry.HasDigest {
			if entry.RiskLevel == "low" {
				entry.RiskLevel = "medium"
			}
		}
		result.ByImage = append(result.ByImage, *entry)
	}
	sort.Slice(result.ByImage, func(i, j int) bool {
		return result.ByImage[i].PodCount > result.ByImage[j].PodCount
	})

	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].LatestCount > result.ByNamespace[j].LatestCount
	})

	sort.Slice(result.StaleImages, func(i, j int) bool {
		return result.StaleImages[i].Severity > result.StaleImages[j].Severity
	})

	result.Summary.HealthScore = imageVulnScore(result.Summary)
	result.Recommendations = imageVulnRecommendations(result.Summary)

	return result
}

// imageVulnScore calculates health score.
func imageVulnScore(s ImageVulnSummary) int {
	if s.TotalImages == 0 {
		return 100
	}
	base := 100
	base -= s.LatestTag * 5
	base -= s.NoDigest * 2
	if base < 0 {
		base = 0
	}
	return base
}

// imageVulnRecommendations generates recommendations.
func imageVulnRecommendations(s ImageVulnSummary) []string {
	var recs []string
	if s.LatestTag > 0 {
		recs = append(recs, fmt.Sprintf("%d images use :latest tag — pin to specific version tags for reproducibility", s.LatestTag))
	}
	if s.NoDigest > 0 {
		recs = append(recs, fmt.Sprintf("%d images are not pinned by digest — use @sha256 digests for supply chain security", s.NoDigest))
	}
	if s.UniqueImages < s.TotalImages {
		recs = append(recs, fmt.Sprintf("%d unique images across %d total — standardize on fewer base images to reduce attack surface", s.UniqueImages, s.TotalImages))
	}
	if s.LatestTag == 0 && s.NoDigest == 0 {
		recs = append(recs, "image supply chain is well-managed — all images pinned with specific tags or digests")
	}
	return recs
}

// handleImageVuln audits container image vulnerability and patch lag.
// GET /api/security/image-vuln
func (s *Server) handleImageVuln(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := imageVulnAuditCore(pods.Items)
	writeJSON(w, result)
}
