package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageFreshResult analyzes container image freshness and update tracking.
type ImageFreshResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ImageFreshSummary `json:"summary"`
	StaleImages     []StaleImageInfo  `json:"staleImages"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type ImageFreshSummary struct {
	TotalImages  int    `json:"totalImages"`
	UniqueImages int    `json:"uniqueImages"`
	FreshImages  int    `json:"freshImages"`
	StaleImages  int    `json:"staleImages"`
	AvgImageAge  string `json:"avgImageAge"`
	UnknownAge   int    `json:"unknownAge"`
	UpdateNeeded int    `json:"updateNeeded"`
}

type StaleImageInfo struct {
	Image     string `json:"image"`
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Severity  string `json:"severity"`
}

// handleImageFreshness analyzes container image freshness and update tracking.
// GET /api/deployment/image-freshness
func (s *Server) handleImageFreshness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ImageFreshResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	imageSet := map[string]bool{}
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, c := range pod.Spec.Containers {
			image := c.Image
			result.Summary.TotalImages++
			imageSet[image] = true

			status := "fresh"
			severity := "low"

			// Check for :latest
			if strings.HasSuffix(image, ":latest") || (!strings.Contains(image, ":") && !strings.Contains(image, "@")) {
				status = "latest-tag"
				severity = "high"
				result.Summary.StaleImages++
				result.StaleImages = append(result.StaleImages, StaleImageInfo{
					Image: image, Workload: pod.Name, Namespace: pod.Namespace,
					Status: status, Severity: severity,
				})
				continue
			}

			// Check for old version patterns
			lower := strings.ToLower(image)
			oldPatterns := []string{"v1", "1.0", "0.1", "old", "legacy", "deprecated"}
			for _, pat := range oldPatterns {
				if strings.Contains(lower, pat) {
					status = "possibly-stale"
					severity = "medium"
					result.Summary.UpdateNeeded++
					break
				}
			}

			if status == "fresh" {
				result.Summary.FreshImages++
			} else if status == "possibly-stale" {
				result.StaleImages = append(result.StaleImages, StaleImageInfo{
					Image: image, Workload: pod.Name, Namespace: pod.Namespace,
					Status: status, Severity: severity,
				})
			}
		}
	}

	result.Summary.UniqueImages = len(imageSet)
	result.Summary.AvgImageAge = "unknown"

	// Score
	freshRatio := 1.0
	if result.Summary.TotalImages > 0 {
		freshRatio = float64(result.Summary.FreshImages) / float64(result.Summary.TotalImages)
	}
	score := int(freshRatio * 100)
	score -= result.Summary.StaleImages * 5
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.StaleImages, func(i, j int) bool {
		return result.StaleImages[i].Severity > result.StaleImages[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Image freshness: %d/100 (grade %s) — %d total, %d unique, %d stale", result.HealthScore, result.Grade, result.Summary.TotalImages, result.Summary.UniqueImages, result.Summary.StaleImages))
	if result.Summary.StaleImages > 0 {
		recs = append(recs, fmt.Sprintf("%d images use :latest tag — pin to specific versions", result.Summary.StaleImages))
	}
	if result.Summary.UpdateNeeded > 0 {
		recs = append(recs, fmt.Sprintf("%d images may be outdated — run image update pipeline", result.Summary.UpdateNeeded))
	}
	if len(recs) == 1 {
		recs = append(recs, "All images are properly versioned and fresh")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
