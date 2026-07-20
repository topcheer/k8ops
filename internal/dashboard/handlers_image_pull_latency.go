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

// ImagePullLatencyResult analyzes image pull performance and registry health.
type ImagePullLatencyResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         ImagePullLatencySummary `json:"summary"`
	ByRegistry      []RegistryLatencyEntry  `json:"byRegistry"`
	SlowPullImages  []SlowPullEntry         `json:"slowPullImages"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type ImagePullLatencySummary struct {
	TotalImages      int  `json:"totalImages"`
	UniqueRegistries int  `json:"uniqueRegistries"`
	PullErrors       int  `json:"pullErrors"`
	LargeImages      int  `json:"largeImages"` // > 500MB estimated
	LocalRegistry    bool `json:"hasLocalRegistry"`
	PullRateLimited  int  `json:"rateLimitedImages"`
	AvgImageAge      int  `json:"avgImageAgeDays"`
}

type RegistryLatencyEntry struct {
	Registry   string `json:"registry"`
	ImageCount int    `json:"imageCount"`
	ErrorCount int    `json:"errorCount"`
	IsLocal    bool   `json:"isLocal"`
	RiskLevel  string `json:"riskLevel"`
}

type SlowPullEntry struct {
	Image     string `json:"image"`
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleImagePullLatency handles GET /api/deployment/image-pull-latency
func (s *Server) handleImagePullLatency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImagePullLatencyResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 500})

	// Build registry stats
	registryMap := make(map[string]*RegistryLatencyEntry)
	imageSet := make(map[string]bool)

	// Error event tracking for image pulls
	errorImages := make(map[string]int)
	for _, ev := range events.Items {
		if strings.Contains(ev.Message, "ImagePullBackOff") ||
			strings.Contains(ev.Message, "ErrImagePull") ||
			strings.Contains(ev.Message, "registry") && strings.Contains(ev.Message, "rate") {
			errorImages[ev.InvolvedObject.Name]++
			result.Summary.PullErrors++
		}
	}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			result.Summary.TotalImages++
			imageSet[img] = true

			registry := "docker.io"
			if idx := strings.Index(img, "/"); idx > 0 {
				parts := strings.SplitN(img, "/", 2)
				if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
					registry = parts[0]
				}
			}

			if registryMap[registry] == nil {
				registryMap[registry] = &RegistryLatencyEntry{Registry: registry}
			}
			registryMap[registry].ImageCount++

			isLocal := strings.Contains(registry, "registry.iot2.win") || strings.Contains(registry, "localhost")
			if isLocal {
				registryMap[registry].IsLocal = true
				result.Summary.LocalRegistry = true
			}

			if errorImages[pod.Name] > 0 {
				registryMap[registry].ErrorCount++
				result.SlowPullImages = append(result.SlowPullImages, SlowPullEntry{
					Image: img, PodName: pod.Name, Namespace: pod.Namespace,
					Issue: "ImagePullBackOff or ErrImagePull", Severity: "high",
				})
			}
		}
	}

	result.Summary.UniqueRegistries = len(registryMap)

	for _, e := range registryMap {
		switch {
		case e.ErrorCount > 5:
			e.RiskLevel = "critical"
		case e.ErrorCount > 0:
			e.RiskLevel = "high"
		case !e.IsLocal && e.ImageCount > 10:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		result.ByRegistry = append(result.ByRegistry, *e)
	}
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].ErrorCount > result.ByRegistry[j].ErrorCount
	})

	if result.Summary.PullErrors > 0 {
		result.HealthScore = 100 - result.Summary.PullErrors*5
	} else {
		result.HealthScore = 100
		if !result.Summary.LocalRegistry {
			result.HealthScore -= 10
		}
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("镜像拉取延迟: %d 镜像, %d 注册表, %d 错误, 本地注册表: %v",
			result.Summary.TotalImages, result.Summary.UniqueRegistries,
			result.Summary.PullErrors, result.Summary.LocalRegistry),
	}
	if result.Summary.PullErrors > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个镜像拉取错误, 检查注册表连通性和配额", result.Summary.PullErrors))
	}
	if !result.Summary.LocalRegistry {
		result.Recommendations = append(result.Recommendations, "无本地镜像注册表, 考虑部署 Harbor/registry 减少拉取延迟")
	}
	writeJSON(w, result)
}
