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
// v19.11 — Deployment Dimension (Round 5)
// 1. Image Consistency Checker
// 2. Config Reload Readiness
// 3. Deploy Freeze Status
// ============================================================

// ---------------------------------------------------------------
// 1. Image Consistency Checker — same workload, different images?
// ---------------------------------------------------------------

type ImageConsistResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ImageConsistSummary  `json:"summary"`
	Inconsistencies []ImageConsistEntry  `json:"inconsistencies"`
	ImageRegistry   []ImageRegistryEntry `json:"imageRegistry"`
	ByNamespace     []ImageConsistNS     `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type ImageConsistSummary struct {
	TotalContainers     int `json:"totalContainers"`
	UniqueImages        int `json:"uniqueImages"`
	LatestTagCount      int `json:"latestTagCount"`
	InconsistentNS      int `json:"inconsistentNamespaces"`
	DifferentRegistries int `json:"differentRegistries"`
	PinnedImages        int `json:"pinnedImages"`
}

type ImageConsistEntry struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Images    []string `json:"images"`
	Issue     string   `json:"issue"`
	RiskLevel string   `json:"riskLevel"`
}

type ImageRegistryEntry struct {
	Registry string `json:"registry"`
	Count    int    `json:"count"`
}

type ImageConsistNS struct {
	Namespace  string `json:"namespace"`
	ImageCount int    `json:"imageCount"`
	LatestTags int    `json:"latestTags"`
}

func (s *Server) handleImageConsistency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImageConsistResult{ScannedAt: time.Now()}

	imageSet := map[string]bool{}
	registryCount := map[string]int{}
	nsMap := map[string]*ImageConsistNS{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &ImageConsistNS{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}

		var images []string
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			image := c.Image
			images = append(images, image)
			imageSet[image] = true
			nsE.ImageCount++

			// Check for :latest tag
			if strings.HasSuffix(image, ":latest") || !strings.Contains(image[strings.LastIndex(image, "/")+1:], ":") {
				result.Summary.LatestTagCount++
				nsE.LatestTags++
			} else {
				result.Summary.PinnedImages++
			}

			// Extract registry
			registry := "docker.io"
			if strings.Contains(image, "/") {
				parts := strings.SplitN(image, "/", 2)
				if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
					registry = parts[0]
				}
			}
			registryCount[registry]++
		}

		// Check for inconsistent images across containers in same deployment
		uniqueImages := map[string]bool{}
		for _, img := range images {
			uniqueImages[img] = true
		}
		if len(uniqueImages) > 1 {
			result.Inconsistencies = append(result.Inconsistencies, ImageConsistEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Images:    images,
				Issue:     "multiple different images in same deployment",
				RiskLevel: "medium",
			})
		}
	}

	result.Summary.UniqueImages = len(imageSet)
	result.Summary.DifferentRegistries = len(registryCount)

	for reg, count := range registryCount {
		result.ImageRegistry = append(result.ImageRegistry, ImageRegistryEntry{
			Registry: reg, Count: count,
		})
	}
	sort.Slice(result.ImageRegistry, func(i, j int) bool {
		return result.ImageRegistry[i].Count > result.ImageRegistry[j].Count
	})

	for _, ns := range nsMap {
		if ns.LatestTags > 0 {
			result.Summary.InconsistentNS++
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	// Score: fewer latest tags = better
	if result.Summary.TotalContainers > 0 {
		pinnedPct := result.Summary.PinnedImages * 100 / result.Summary.TotalContainers
		result.HealthScore = pinnedPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildImageConsistRecs1911(&result)
	writeJSON(w, result)
}

func buildImageConsistRecs1911(r *ImageConsistResult) []string {
	recs := []string{fmt.Sprintf("Image consistency: %d containers, %d unique images, %d with :latest (%d%% pinned), %d registries",
		r.Summary.TotalContainers, r.Summary.UniqueImages,
		r.Summary.LatestTagCount,
		r.Summary.PinnedImages,
		r.Summary.DifferentRegistries)}
	if r.Summary.LatestTagCount > 0 {
		recs = append(recs, fmt.Sprintf("%d containers using :latest or unpinned tags - pin to specific versions for reproducibility", r.Summary.LatestTagCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Config Reload Readiness — ConfigMap mount type checker
// ---------------------------------------------------------------

type ConfigReloadResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ConfigReloadSummary   `json:"summary"`
	NeedsRestart    []ConfigReloadEntry   `json:"needsRestart"`
	HotReloadReady  []ConfigReloadEntry   `json:"hotReloadReady"`
	ByNS            []ConfigReloadNSEntry `json:"byNamespace"`
	Recommendations []string              `json:"recommendations"`
}

type ConfigReloadSummary struct {
	TotalCMRefs      int `json:"totalConfigMapRefs"`
	HotReloadReady   int `json:"hotReloadReady"`
	NeedsRestart     int `json:"needsRestart"`
	EnvVarMounts     int `json:"envVarMounts"`
	VolumeMounts     int `json:"volumeMounts"`
	ProjectedVolumes int `json:"projectedVolumes"`
}

type ConfigReloadEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ConfigMap string `json:"configMap"`
	MountType string `json:"mountType"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

type ConfigReloadNSEntry struct {
	Namespace    string `json:"namespace"`
	CMRefs       int    `json:"configMapRefs"`
	NeedsRestart int    `json:"needsRestart"`
}

func (s *Server) handleConfigReloadReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ConfigReloadResult{ScannedAt: time.Now()}

	nsMap := map[string]*ConfigReloadNSEntry{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &ConfigReloadNSEntry{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}

		// Check volume-based ConfigMap mounts (these support hot reload)
		for _, vol := range dep.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil {
				result.Summary.TotalCMRefs++
				result.Summary.VolumeMounts++
				nsE.CMRefs++
				result.HotReloadReady = append(result.HotReloadReady, ConfigReloadEntry{
					Name: dep.Name, Namespace: dep.Namespace,
					ConfigMap: vol.ConfigMap.Name,
					MountType: "volume", RiskLevel: "low",
					Issue: "volume mount supports hot reload (with default update)",
				})
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						result.Summary.TotalCMRefs++
						result.Summary.ProjectedVolumes++
						nsE.CMRefs++
						result.HotReloadReady = append(result.HotReloadReady, ConfigReloadEntry{
							Name: dep.Name, Namespace: dep.Namespace,
							ConfigMap: src.ConfigMap.Name,
							MountType: "projected", RiskLevel: "low",
							Issue: "projected volume mount",
						})
					}
				}
			}
		}

		// Check env var-based ConfigMap refs (these need pod restart)
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					result.Summary.TotalCMRefs++
					result.Summary.EnvVarMounts++
					result.Summary.NeedsRestart++
					nsE.NeedsRestart++
					result.NeedsRestart = append(result.NeedsRestart, ConfigReloadEntry{
						Name: dep.Name, Namespace: dep.Namespace,
						ConfigMap: env.ValueFrom.ConfigMapKeyRef.Name,
						MountType: "env-var", RiskLevel: "medium",
						Issue: "env var ConfigMap ref requires pod restart to update",
					})
				}
			}
		}
	}

	for _, ns := range nsMap {
		result.ByNS = append(result.ByNS, *ns)
	}
	sort.Slice(result.ByNS, func(i, j int) bool {
		return result.ByNS[i].NeedsRestart > result.ByNS[j].NeedsRestart
	})

	// Score: more hot-reload ready = better
	if result.Summary.TotalCMRefs > 0 {
		hotPct := (result.Summary.VolumeMounts + result.Summary.ProjectedVolumes) * 100 / result.Summary.TotalCMRefs
		result.HealthScore = hotPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildConfigReloadRecs1911(&result)
	writeJSON(w, result)
}

func buildConfigReloadRecs1911(r *ConfigReloadResult) []string {
	recs := []string{fmt.Sprintf("Config reload readiness: %d CM refs (%d volume mount, %d env-var mount), %d need restart",
		r.Summary.TotalCMRefs, r.Summary.VolumeMounts+r.Summary.ProjectedVolumes,
		r.Summary.EnvVarMounts, r.Summary.NeedsRestart)}
	if r.Summary.NeedsRestart > 0 {
		recs = append(recs, fmt.Sprintf("%d ConfigMaps mounted as env vars - switch to volume mounts for hot reload", r.Summary.NeedsRestart))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Deploy Freeze Status — check for maintenance windows / freezes
// ---------------------------------------------------------------

type DeployFreezeResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DeployFreezeSummary  `json:"summary"`
	FreezeWindows   []DeployFreezeWindow `json:"freezeWindows"`
	ActiveFreezes   []DeployFreezeWindow `json:"activeFreezes"`
	RecentChanges   []DeployFreezeChange `json:"recentChanges"`
	Recommendations []string             `json:"recommendations"`
}

type DeployFreezeSummary struct {
	TotalWorkloads    int  `json:"totalWorkloads"`
	CurrentlyFrozen   bool `json:"currentlyFrozen"`
	ActiveFreezeCount int  `json:"activeFreezeCount"`
	ChangesInWindow   int  `json:"changesInWindow"`
	ChangesBlocked    int  `json:"changesBlocked"`
	SafeToDeploy      bool `json:"safeToDeploy"`
	NextFreezeHours   int  `json:"nextFreezeHours"`
}

type DeployFreezeWindow struct {
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Type      string `json:"type"`
	Active    bool   `json:"active"`
	Namespace string `json:"namespace"`
}

type DeployFreezeChange struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
	EventType string `json:"eventType"`
}

func (s *Server) handleDeployFreezeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DeployFreezeResult{ScannedAt: time.Now()}

	now := time.Now()

	// Check for freeze annotations on namespaces
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}

		// Check for freeze annotations
		freezeAnnotation := ""
		if ns.Annotations != nil {
			for k, v := range ns.Annotations {
				if strings.Contains(strings.ToLower(k), "freeze") || strings.Contains(strings.ToLower(k), "maintenance") {
					freezeAnnotation = k + "=" + v
					break
				}
			}
		}

		if freezeAnnotation != "" {
			freezeWindow := DeployFreezeWindow{
				Name: ns.Name, Namespace: ns.Name,
				Type: "namespace-annotation", Active: true,
				StartTime: ns.CreationTimestamp.Format(time.RFC3339),
			}
			result.ActiveFreezes = append(result.ActiveFreezes, freezeWindow)
			result.FreezeWindows = append(result.FreezeWindows, freezeWindow)
			result.Summary.ActiveFreezeCount++
		}
	}

	// Check for typical freeze windows (weekends, holidays)
	weekday := now.Weekday()
	hour := now.Hour()
	result.Summary.NextFreezeHours = 0

	// Friday evening freeze check
	if weekday == time.Friday && hour >= 17 {
		result.FreezeWindows = append(result.FreezeWindows, DeployFreezeWindow{
			Name: "weekend-freeze", Type: "recurring",
			StartTime: "Friday 17:00", EndTime: "Monday 09:00",
			Active: true,
		})
	}

	// Check recent deployment events in last 1h
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	cutoff := now.Add(-1 * time.Hour)
	for _, evt := range events.Items {
		if isSystemNamespace(evt.Namespace) || evt.LastTimestamp.Time.Before(cutoff) {
			continue
		}
		reasonLower := strings.ToLower(evt.Reason)
		if strings.Contains(reasonLower, "scal") || strings.Contains(reasonLower, "rollout") ||
			strings.Contains(reasonLower, "creat") || strings.Contains(reasonLower, "updat") {
			result.Summary.ChangesInWindow++
			result.RecentChanges = append(result.RecentChanges, DeployFreezeChange{
				Name: evt.InvolvedObject.Name, Namespace: evt.Namespace,
				Timestamp: evt.LastTimestamp.Format(time.RFC3339),
				EventType: classifyChangeEvent(reasonLower, evt.Message),
			})
		}
	}

	// Count total workloads
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			result.Summary.TotalWorkloads++
		}
	}

	// Determine freeze status
	result.Summary.CurrentlyFrozen = result.Summary.ActiveFreezeCount > 0
	result.Summary.SafeToDeploy = !result.Summary.CurrentlyFrozen && result.Summary.ChangesInWindow < 10

	// Estimate next freeze (weekend)
	if weekday >= time.Monday && weekday <= time.Thursday {
		daysUntilFriday := int(time.Friday - weekday)
		result.Summary.NextFreezeHours = daysUntilFriday*24 - hour + 17
	} else if weekday == time.Friday && hour < 17 {
		result.Summary.NextFreezeHours = 17 - hour
	}

	// Score: safe to deploy = high score
	if result.Summary.SafeToDeploy {
		result.HealthScore = 90
	} else if result.Summary.CurrentlyFrozen {
		result.HealthScore = 30
	} else {
		result.HealthScore = 60
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildDeployFreezeRecs1911(&result)
	writeJSON(w, result)
}

func buildDeployFreezeRecs1911(r *DeployFreezeResult) []string {
	status := "clear"
	if r.Summary.CurrentlyFrozen {
		status = "FROZEN"
	}
	recs := []string{fmt.Sprintf("Deploy freeze: status=%s, %d active freezes, %d changes in last hour, next freeze in %dh",
		status, r.Summary.ActiveFreezeCount, r.Summary.ChangesInWindow, r.Summary.NextFreezeHours)}
	if r.Summary.CurrentlyFrozen {
		recs = append(recs, "Deploy freeze active - halt non-emergency deployments")
	} else if r.Summary.ChangesInWindow > 10 {
		recs = append(recs, fmt.Sprintf("%d changes in last hour - high deployment activity, consider staggering", r.Summary.ChangesInWindow))
	}
	return recs
}

// corev1 is used for ResourceCPU reference - keep import
var _ corev1.ResourceName
