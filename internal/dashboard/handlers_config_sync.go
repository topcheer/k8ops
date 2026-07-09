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

// CfgSyncResult is the ConfigMap/Secret configuration sync & staleness analysis.
type CfgSyncResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CfgSyncSummary     `json:"summary"`
	StalePods       []CfgStaleEntry    `json:"stalePods"`     // pods using stale config
	SubPathMounts   []CfgSubPathEntry  `json:"subPathMounts"` // subPath mounts (no auto-update)
	NoReloader      []CfgNoReloadEntry `json:"noReloader"`    // workloads without reloader annotations
	ByConfigMap     []CfgSyncByCM      `json:"byConfigMap"`   // ConfigMaps with stale consumers
	Recommendations []string           `json:"recommendations"`
}

// CfgSyncSummary aggregates cluster-wide config sync statistics.
type CfgSyncSummary struct {
	TotalPods         int `json:"totalPods"`
	PodsWithConfigRef int `json:"podsWithConfigRef"` // pods mounting ConfigMaps/Secrets
	EnvVarRefs        int `json:"envVarRefs"`        // pods using env/envFrom (no auto-update)
	VolumeRefs        int `json:"volumeRefs"`        // pods using volume mounts (auto-update)
	SubPathRefs       int `json:"subPathRefs"`       // subPath volume mounts (no auto-update)
	StalePodCount     int `json:"stalePodCount"`     // pods with config updated after pod start
	HasReloader       int `json:"hasReloader"`       // workloads with reloader annotations
	NeedsReloader     int `json:"needsReloader"`     // workloads using env vars without reloader
	ImmutableCMs      int `json:"immutableConfigMaps"`
	ImmutableSecrets  int `json:"immutableSecrets"`
	StalenessScore    int `json:"stalenessScore"` // 0-100 (100 = no staleness)
}

// CfgStaleEntry describes a pod using stale configuration.
type CfgStaleEntry struct {
	PodName       string    `json:"podName"`
	Namespace     string    `json:"namespace"`
	ConfigType    string    `json:"configType"` // ConfigMap or Secret
	ConfigName    string    `json:"configName"`
	RefMethod     string    `json:"refMethod"` // env, envFrom, volume, subPath
	ConfigUpdated time.Time `json:"configUpdated"`
	PodStarted    time.Time `json:"podStarted"`
	StaleDuration string    `json:"staleDuration"`
	Severity      string    `json:"severity"`
}

// CfgSubPathEntry describes a subPath volume mount that won't auto-update.
type CfgSubPathEntry struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	VolumeName string `json:"volumeName"`
	MountPath  string `json:"mountPath"`
	ConfigType string `json:"configType"`
	ConfigName string `json:"configName"`
	Severity   string `json:"severity"`
}

// CfgNoReloadEntry describes a workload that needs a reloader but doesn't have one.
type CfgNoReloadEntry struct {
	WorkloadName string `json:"workloadName"`
	Namespace    string `json:"namespace"`
	WorkloadType string `json:"workloadType"`
	EnvVarRefs   int    `json:"envVarRefs"`
	VolumeRefs   int    `json:"volumeRefs"`
	Severity     string `json:"severity"`
}

// CfgSyncByCM shows ConfigMaps with stale consumers.
type CfgSyncByCM struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Type          string `json:"type"` // ConfigMap or Secret
	LastUpdated   string `json:"lastUpdated"`
	ConsumerCount int    `json:"consumerCount"`
	StaleCount    int    `json:"staleCount"`
}

// handleConfigSync analyzes ConfigMap/Secret configuration sync and staleness.
// GET /api/deployment/config-sync
func (s *Server) handleConfigSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Fetch ConfigMaps, Secrets, and Pods
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build last-updated maps: ns/name -> last modified time
	cmUpdated := map[string]time.Time{}
	for _, cm := range cms.Items {
		cmUpdated[cm.Namespace+"/"+cm.Name] = cmUpdatedTime(&cm)
	}

	secretUpdated := map[string]time.Time{}
	immutableSecrets := 0
	for _, sec := range secrets.Items {
		secretUpdated[sec.Namespace+"/"+sec.Name] = secretUpdatedTime(&sec)
		if sec.Immutable != nil && *sec.Immutable {
			immutableSecrets++
		}
	}

	immutableCMs := 0
	for _, cm := range cms.Items {
		if cm.Immutable != nil && *cm.Immutable {
			immutableCMs++
		}
	}

	now := time.Now()
	result := CfgSyncResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)
	result.Summary.ImmutableCMs = immutableCMs
	result.Summary.ImmutableSecrets = immutableSecrets

	// Track CM/Secret consumer counts
	cmConsumers := map[string]int{} // ns/name -> count
	cmStaleConsumers := map[string]int{}

	wlNeedsReloader := map[string]*CfgNoReloadEntry{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		podStart := pod.CreationTimestamp.Time
		if pod.Status.StartTime != nil {
			podStart = pod.Status.StartTime.Time
		}

		hasConfigRef := false

		envVarCount := 0
		volCount := 0

		// Check env var references (ConfigMapKeyRef / SecretKeyRef)
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil && env.ValueFrom.ConfigMapKeyRef.Name != "" {
						hasConfigRef = true
						envVarCount++
						cmKey := fmt.Sprintf("%s/%s", pod.Namespace, env.ValueFrom.ConfigMapKeyRef.Name)
						cmConsumers[cmKey]++
						result.Summary.EnvVarRefs++

						if cmTime, ok := cmUpdated[cmKey]; ok && cmTime.After(podStart) {
							result.Summary.StalePodCount++
							result.StalePods = append(result.StalePods, makeStaleEntry(&pod, "ConfigMap", env.ValueFrom.ConfigMapKeyRef.Name, "env", cmTime, podStart, now))
							cmStaleConsumers[cmKey]++
						}
					}
					if env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
						hasConfigRef = true
						envVarCount++
						secKey := fmt.Sprintf("%s/%s", pod.Namespace, env.ValueFrom.SecretKeyRef.Name)
						cmConsumers[secKey]++
						result.Summary.EnvVarRefs++

						if secTime, ok := secretUpdated[secKey]; ok && secTime.After(podStart) {
							result.Summary.StalePodCount++
							result.StalePods = append(result.StalePods, makeStaleEntry(&pod, "Secret", env.ValueFrom.SecretKeyRef.Name, "env", secTime, podStart, now))
							cmStaleConsumers[secKey]++
						}
					}
				}
			}

			// Check envFrom (whole ConfigMap/Secret as env vars)
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name != "" {
					hasConfigRef = true
					envVarCount++
					cmKey := fmt.Sprintf("%s/%s", pod.Namespace, ef.ConfigMapRef.Name)
					cmConsumers[cmKey]++
					result.Summary.EnvVarRefs++

					if cmTime, ok := cmUpdated[cmKey]; ok && cmTime.After(podStart) {
						result.Summary.StalePodCount++
						result.StalePods = append(result.StalePods, makeStaleEntry(&pod, "ConfigMap", ef.ConfigMapRef.Name, "envFrom", cmTime, podStart, now))
						cmStaleConsumers[cmKey]++
					}
				}
				if ef.SecretRef != nil && ef.SecretRef.Name != "" {
					hasConfigRef = true
					envVarCount++
					secKey := fmt.Sprintf("%s/%s", pod.Namespace, ef.SecretRef.Name)
					cmConsumers[secKey]++
					result.Summary.EnvVarRefs++

					if secTime, ok := secretUpdated[secKey]; ok && secTime.After(podStart) {
						result.Summary.StalePodCount++
						result.StalePods = append(result.StalePods, makeStaleEntry(&pod, "Secret", ef.SecretRef.Name, "envFrom", secTime, podStart, now))
						cmStaleConsumers[secKey]++
					}
				}
			}
		}

		// Check volume mounts
		for _, vol := range pod.Spec.Volumes {
			cfgType := ""
			cfgName := ""
			if vol.ConfigMap != nil {
				cfgType = "ConfigMap"
				cfgName = vol.ConfigMap.Name
			} else if vol.Secret != nil {
				cfgType = "Secret"
				cfgName = vol.Secret.SecretName
			} else if vol.Projected != nil {
				// Projected volumes can contain ConfigMap/Secret sources
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						cfgType = "ConfigMap"
						cfgName = src.ConfigMap.Name
						break
					}
					if src.Secret != nil {
						cfgType = "Secret"
						cfgName = src.Secret.Name
						break
					}
				}
			}

			if cfgType == "" || cfgName == "" {
				continue
			}

			hasConfigRef = true
			volCount++
			result.Summary.VolumeRefs++
			cfgKey := fmt.Sprintf("%s/%s", pod.Namespace, cfgName)
			cmConsumers[cfgKey]++

			// Check for subPath mounts (no auto-update)
			hasSubPath := false
			for _, c := range pod.Spec.Containers {
				for _, vm := range c.VolumeMounts {
					if vm.Name == vol.Name && vm.SubPath != "" {
						hasSubPath = true
						result.Summary.SubPathRefs++
						result.SubPathMounts = append(result.SubPathMounts, CfgSubPathEntry{
							PodName:    pod.Name,
							Namespace:  pod.Namespace,
							VolumeName: vol.Name,
							MountPath:  vm.MountPath,
							ConfigType: cfgType,
							ConfigName: cfgName,
							Severity:   "medium",
						})
					}
				}
			}

			// For env var refs, check if config was updated after pod start
			// Volume mounts DO auto-update (except subPath), so only flag env var stale
			// But subPath volumes are also stale
			if hasSubPath {
				var updateTime time.Time
				var ok bool
				if cfgType == "ConfigMap" {
					updateTime, ok = cmUpdated[cfgKey]
				} else {
					updateTime, ok = secretUpdated[cfgKey]
				}
				if ok && updateTime.After(podStart) {
					result.Summary.StalePodCount++
					result.StalePods = append(result.StalePods, makeStaleEntry(&pod, cfgType, cfgName, "subPath", updateTime, podStart, now))
					cmStaleConsumers[cfgKey]++
				}
			}
		}

		if hasConfigRef {
			result.Summary.PodsWithConfigRef++
		}

		// Check for reloader annotations on the pod (inherited from workload)
		hasReloader := hasReloaderAnnotation(&pod)
		if hasReloader {
			result.Summary.HasReloader++
		} else if envVarCount > 0 && !hasReloader {
			// Workload uses env vars but has no reloader — needs one
			wlName := getWorkloadName(&pod)
			if wlName != "" {
				key := fmt.Sprintf("%s/%s", pod.Namespace, wlName)
				if _, exists := wlNeedsReloader[key]; !exists {
					wlNeedsReloader[key] = &CfgNoReloadEntry{
						WorkloadName: wlName,
						Namespace:    pod.Namespace,
						WorkloadType: inferWorkloadTypeFromPod(&pod),
						Severity:     "high",
					}
				}
				wlNeedsReloader[key].EnvVarRefs += envVarCount
				wlNeedsReloader[key].VolumeRefs += volCount
			}
		}
	}

	// Build needsReloader list
	for _, entry := range wlNeedsReloader {
		result.NoReloader = append(result.NoReloader, *entry)
		result.Summary.NeedsReloader++
	}
	sort.Slice(result.NoReloader, func(i, j int) bool {
		return result.NoReloader[i].EnvVarRefs > result.NoReloader[j].EnvVarRefs
	})

	// Build byConfigMap list
	for key, staleCount := range cmStaleConsumers {
		if staleCount == 0 {
			continue
		}
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		cmType := "ConfigMap"
		var updateTime time.Time
		if t, ok := cmUpdated[key]; ok {
			updateTime = t
		} else if t, ok := secretUpdated[key]; ok {
			cmType = "Secret"
			updateTime = t
		}
		result.ByConfigMap = append(result.ByConfigMap, CfgSyncByCM{
			Name:          parts[1],
			Namespace:     parts[0],
			Type:          cmType,
			LastUpdated:   updateTime.Format(time.RFC3339),
			ConsumerCount: cmConsumers[key],
			StaleCount:    staleCount,
		})
	}
	sort.Slice(result.ByConfigMap, func(i, j int) bool {
		return result.ByConfigMap[i].StaleCount > result.ByConfigMap[j].StaleCount
	})

	// Sort stale pods by staleness duration (longest first)
	sort.Slice(result.StalePods, func(i, j int) bool {
		return result.StalePods[i].ConfigUpdated.After(result.StalePods[j].ConfigUpdated)
	})
	if len(result.StalePods) > 50 {
		result.StalePods = result.StalePods[:50]
	}
	if len(result.SubPathMounts) > 30 {
		result.SubPathMounts = result.SubPathMounts[:30]
	}

	result.Summary.StalenessScore = cfgSyncScore(result.Summary)
	result.Recommendations = cfgSyncRecommendations(&result)

	writeJSON(w, result)
}

// makeStaleEntry creates a CfgStaleEntry from pod and config info.
func makeStaleEntry(pod *corev1.Pod, cfgType, cfgName, refMethod string, cfgUpdated, podStart, now time.Time) CfgStaleEntry {
	staleDur := now.Sub(cfgUpdated)
	severity := "low"
	if staleDur > 24*time.Hour {
		severity = "high"
	} else if staleDur > 1*time.Hour {
		severity = "medium"
	}

	return CfgStaleEntry{
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		ConfigType:    cfgType,
		ConfigName:    cfgName,
		RefMethod:     refMethod,
		ConfigUpdated: cfgUpdated,
		PodStarted:    podStart,
		StaleDuration: formatDuration(staleDur),
		Severity:      severity,
	}
}

// cmUpdatedTime returns the last modification time for a ConfigMap.
func cmUpdatedTime(cm *corev1.ConfigMap) time.Time {
	// Use ResourceVersion as a proxy for modification order if CreationTimestamp is all we have
	// In practice, ManagedFields or annotations may have better timestamps
	// Fall back to CreationTimestamp which is always available
	t := cm.CreationTimestamp.Time
	// Check if any data key was modified more recently via annotations
	for _, key := range []string{"kubectl.kubernetes.io/last-applied-configuration"} {
		if cm.Annotations != nil {
			if _, ok := cm.Annotations[key]; ok {
				// The resource was managed by kubectl — creation time is a reasonable proxy
			}
		}
	}
	return t
}

// secretUpdatedTime returns the last modification time for a Secret.
func secretUpdatedTime(sec *corev1.Secret) time.Time {
	return sec.CreationTimestamp.Time
}

// hasReloaderAnnotation checks if a pod has reloader-related annotations.
func hasReloaderAnnotation(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	for key := range pod.Annotations {
		if strings.Contains(key, "reloader") || strings.Contains(key, "configmap.reloader.stakater.com") || strings.Contains(key, "secret.reloader.stakater.com") {
			return true
		}
	}
	return false
}

// getWorkloadName extracts the owning workload name from pod owner references.
func getWorkloadName(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Deployment" || ref.Kind == "StatefulSet" || ref.Kind == "DaemonSet" {
			return ref.Name
		}
	}
	// For ReplicaSet, try to find parent Deployment name
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" {
			// Strip hash suffix if present (e.g., app-xxx -> app)
			return strings.Split(ref.Name, "-")[0]
		}
	}
	return ""
}

// cfgSyncScore computes a 0-100 staleness score (100 = no staleness).
func cfgSyncScore(s CfgSyncSummary) int {
	if s.PodsWithConfigRef == 0 {
		return 100
	}

	score := 100

	// Penalize stale pods
	staleRatio := float64(s.StalePodCount) / float64(s.PodsWithConfigRef)
	score -= int(staleRatio * 35)

	// Penalize env var refs (inherently stale-prone)
	envRatio := 0.0
	totalRefs := s.EnvVarRefs + s.VolumeRefs
	if totalRefs > 0 {
		envRatio = float64(s.EnvVarRefs) / float64(totalRefs)
	}
	score -= int(envRatio * 20)

	// Penalize subPath mounts
	if s.SubPathRefs > 0 {
		score -= min(15, s.SubPathRefs*3)
	}

	// Penalize workloads needing reloader
	if s.NeedsReloader > 0 {
		score -= min(20, s.NeedsReloader*5)
	}

	// Penalize no immutable ConfigMaps/Secrets (immutable = safer)
	if s.ImmutableCMs == 0 && s.PodsWithConfigRef > 5 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	return score
}

// cfgSyncRecommendations generates actionable recommendations.
func cfgSyncRecommendations(r *CfgSyncResult) []string {
	var recs []string

	if r.Summary.StalePodCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) are running with stale configuration — restart them to pick up updated ConfigMaps/Secrets",
			r.Summary.StalePodCount,
		))
	}

	if r.Summary.EnvVarRefs > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d ConfigMap/Secret references use environment variables — these do NOT auto-update on config changes. Consider switching to volume mounts or installing Reloader",
			r.Summary.EnvVarRefs,
		))
	}

	if r.Summary.SubPathRefs > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d subPath volume mount(s) detected — subPath mounts do NOT auto-update when ConfigMap/Secret changes. Restart pods after config updates",
			r.Summary.SubPathRefs,
		))
	}

	if r.Summary.NeedsReloader > 0 {
		recs = addReloaderRec(recs, r.Summary.NeedsReloader)
	}

	if r.Summary.ImmutableCMs == 0 && r.Summary.PodsWithConfigRef > 0 {
		recs = append(recs, "No immutable ConfigMaps detected — set immutable: true for ConfigMaps that rarely change to improve cluster performance and prevent accidental modifications")
	}

	if r.Summary.ImmutableSecrets == 0 && r.Summary.PodsWithConfigRef > 0 {
		recs = append(recs, "No immutable Secrets detected — set immutable: true for Secrets that rarely change to improve cluster performance and prevent accidental modifications")
	}

	if len(recs) == 0 {
		recs = append(recs, "Configuration sync is healthy — no stale configs or risky mount patterns detected")
	}

	return recs
}

// addReloaderRec adds the reloader recommendation to recs.
func addReloaderRec(recs []string, count int) []string {
	return append(recs, fmt.Sprintf(
		"%d workload(s) need Reloader (Stakater Reloader or similar) — install it to automatically restart pods when ConfigMaps/Secrets are updated",
		count,
	))
}
