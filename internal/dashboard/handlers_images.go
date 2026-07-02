package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageInfo represents a container image deployed in the cluster.
type ImageInfo struct {
	Image       string   `json:"image"`
	Registry    string   `json:"registry"`
	Repo        string   `json:"repo"`
	Tag         string   `json:"tag"`
	PullPolicy  string   `json:"pullPolicy"`
	UsedByCount int      `json:"usedByCount"`
	Namespaces  []string `json:"namespaces"`
	Workloads   []string `json:"workloads"`
	HasLimits   bool     `json:"hasLimits"`
	HasRequests bool     `json:"hasRequests"`
}

// handleImageInventory returns all container images in the cluster.
// GET /api/images
func (s *Server) handleImageInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Aggregate images
	imageMap := map[string]*ImageInfo{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		workload := fmt.Sprintf("%s/%s", pod.Kind, pod.Name)
		if pod.OwnerReferences != nil && len(pod.OwnerReferences) > 0 {
			workload = fmt.Sprintf("%s/%s", pod.OwnerReferences[0].Kind, pod.Name)
		}

		for _, c := range pod.Spec.Containers {
			img := c.Image
			entry, ok := imageMap[img]
			if !ok {
				entry = parseImageRef(img)
				imageMap[img] = entry
			}
			entry.UsedByCount++

			// Track namespaces
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

			// Track workloads
			wlFound := false
			for _, wl := range entry.Workloads {
				if wl == workload {
					wlFound = true
					break
				}
			}
			if !wlFound {
				entry.Workloads = append(entry.Workloads, workload)
			}

			// Check resource limits/requests
			if !c.Resources.Limits.Cpu().IsZero() || !c.Resources.Limits.Memory().IsZero() {
				entry.HasLimits = true
			}
			if !c.Resources.Requests.Cpu().IsZero() || !c.Resources.Requests.Memory().IsZero() {
				entry.HasRequests = true
			}

			// Track pull policy (first one wins for display)
			if entry.PullPolicy == "" {
				entry.PullPolicy = string(c.ImagePullPolicy)
			}
		}
	}

	// Convert to slice
	items := make([]ImageInfo, 0, len(imageMap))
	for _, info := range imageMap {
		items = append(items, *info)
	}

	// Sort by usage count (most used first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].UsedByCount > items[j].UsedByCount
	})

	// Build summary
	totalImages := len(items)
	withoutLimits := 0
	withoutRequests := 0
	latestTagCount := 0
	duplicateRegistries := map[string]int{}

	for _, img := range items {
		if !img.HasLimits {
			withoutLimits++
		}
		if !img.HasRequests {
			withoutRequests++
		}
		if img.Tag == "latest" {
			latestTagCount++
		}
		duplicateRegistries[img.Registry]++
	}

	writeJSON(w, map[string]any{
		"count": totalImages,
		"summary": map[string]any{
			"totalImages":      totalImages,
			"withoutLimits":    withoutLimits,
			"withoutRequests":  withoutRequests,
			"usingLatestTag":   latestTagCount,
			"uniqueRegistries": len(duplicateRegistries),
		},
		"items": items,
	})
}

// parseImageRef parses a container image reference into components.
// Examples:
//
//	nginx                    → registry: docker.io, repo: library/nginx, tag: latest
//	nginx:1.21               → registry: docker.io, repo: library/nginx, tag: 1.21
//	myrepo/app:v2            → registry: docker.io, repo: myrepo/app, tag: v2
//	registry.io/app:v2       → registry: registry.io, repo: app, tag: v2
//	registry.io:5000/app:v2  → registry: registry.io:5000, repo: app, tag: v2
//	nginx@sha256:abc...      → registry: docker.io, repo: library/nginx, tag: @sha256:abc...
func parseImageRef(image string) *ImageInfo {
	info := &ImageInfo{
		Image:    image,
		Registry: "docker.io",
		Repo:     image,
		Tag:      "latest",
	}

	// Handle digest
	if idx := strings.Index(image, "@"); idx >= 0 {
		info.Tag = image[idx:]
		image = image[:idx]
	}

	// Split registry from repo
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		// Check if first part is a registry (contains . or :)
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			info.Registry = parts[0]
			image = parts[1]
		}
	}

	// Split tag
	if idx := strings.LastIndex(image, ":"); idx >= 0 {
		info.Tag = image[idx+1:]
		image = image[:idx]
	}

	// Handle docker.io library/ prefix
	if info.Registry == "docker.io" && !strings.Contains(image, "/") {
		image = "library/" + image
	}

	info.Repo = image
	return info
}
