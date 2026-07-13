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

// ConfigMountResult is the ConfigMap & Secret mount injection risk audit.
type ConfigMountResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         ConfigMountSummary `json:"summary"`
	ByNamespace     []ConfigMountNS    `json:"byNamespace"`
	VolumeMounts    []ConfigMountEntry `json:"volumeMounts"`
	Issues          []ConfigMountIssue `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// ConfigMountSummary aggregates ConfigMap/Secret mount statistics.
type ConfigMountSummary struct {
	TotalPods       int `json:"totalPods"`
	ConfigMapMounts int `json:"configMapMounts"`
	SecretMounts    int `json:"secretMounts"`
	EnvFromCM       int `json:"envFromConfigMaps"`
	EnvFromSecrets  int `json:"envFromSecrets"`
	LargeConfigMaps int `json:"largeConfigMaps"`
	NoOptionalFlag  int `json:"noOptionalFlag"`
	SubPathMounts   int `json:"subPathMounts"`
	MissingCMRefs   int `json:"missingCMRefs"`
	MissingSecrets  int `json:"missingSecrets"`
}

// ConfigMountNS per-namespace stats.
type ConfigMountNS struct {
	Namespace  string `json:"namespace"`
	PodCount   int    `json:"podCount"`
	MountCount int    `json:"mountCount"`
	RiskCount  int    `json:"riskCount"`
}

// ConfigMountEntry describes one mount risk.
type ConfigMountEntry struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	VolumeName string `json:"volumeName"`
	SourceType string `json:"sourceType"` // ConfigMap or Secret
	SourceName string `json:"sourceName"`
	MountPath  string `json:"mountPath"`
	IsOptional bool   `json:"isOptional"`
	HasSubPath bool   `json:"hasSubPath"`
	IsLarge    bool   `json:"isLarge"`
	RiskLevel  string `json:"riskLevel"`
}

// ConfigMountIssue is a detected mount risk.
type ConfigMountIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleConfigMountRisk audits ConfigMap & Secret mount injection risks.
// GET /api/product/config-mount-risk
func (s *Server) handleConfigMountRisk(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &ConfigMountResult{
		ScannedAt: time.Now(),
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	configMaps, err := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build CM size map
	cmSizeMap := make(map[string]int) // ns/name -> byte size
	for i := range configMaps.Items {
		cm := &configMaps.Items[i]
		key := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
		size := 0
		for _, v := range cm.Data {
			size += len(v)
		}
		for _, v := range cm.BinaryData {
			size += len(v)
		}
		cmSizeMap[key] = size
	}

	var entries []ConfigMountEntry
	var issues []ConfigMountIssue
	nsStats := make(map[string]*ConfigMountNS)

	totalPods := 0
	cmMounts := 0
	secretMounts := 0
	envFromCM := 0
	envFromSecrets := 0
	largeCMs := 0
	noOptional := 0
	subPathMounts := 0
	missingCMRefs := 0
	missingSecrets := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		totalPods++

		// Build volume source map
		volSources := make(map[string]corev1.Volume)
		for _, vol := range pod.Spec.Volumes {
			volSources[vol.Name] = vol
		}

		for _, c := range pod.Spec.Containers {
			// Check volume mounts
			for _, vm := range c.VolumeMounts {
				vol, ok := volSources[vm.Name]
				if !ok {
					continue
				}

				entry := ConfigMountEntry{
					PodName:    pod.Name,
					Namespace:  pod.Namespace,
					Container:  c.Name,
					VolumeName: vm.Name,
					MountPath:  vm.MountPath,
					IsOptional: true,
				}

				hasSubPath := vm.SubPath != ""
				entry.HasSubPath = hasSubPath
				if hasSubPath {
					subPathMounts++
				}

				// Check ConfigMap volume
				if vol.ConfigMap != nil {
					entry.SourceType = "ConfigMap"
					entry.SourceName = vol.ConfigMap.Name
					cmMounts++

					if vol.ConfigMap.Optional != nil {
						entry.IsOptional = *vol.ConfigMap.Optional
					} else {
						entry.IsOptional = false
						noOptional++
						issues = append(issues, ConfigMountIssue{
							Severity: "info",
							Type:     "cm-not-optional",
							Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, vol.ConfigMap.Name),
							Message:  fmt.Sprintf("ConfigMap '%s' mount is not marked optional — pod will fail if ConfigMap is deleted", vol.ConfigMap.Name),
						})
					}

					// Check if CM exists and is large
					cmKey := fmt.Sprintf("%s/%s", pod.Namespace, vol.ConfigMap.Name)
					if size, ok := cmSizeMap[cmKey]; ok {
						if size > 500*1024 { // >500KB
							entry.IsLarge = true
							largeCMs++
							issues = append(issues, ConfigMountIssue{
								Severity: "warning",
								Type:     "large-configmap",
								Resource: cmKey,
								Message:  fmt.Sprintf("ConfigMap '%s' is %dKB — large ConfigMaps slow down pod startup and consume memory", vol.ConfigMap.Name, size/1024),
							})
						}
					} else {
						// CM doesn't exist
						missingCMRefs++
						if !entry.IsOptional {
							issues = append(issues, ConfigMountIssue{
								Severity: "critical",
								Type:     "missing-configmap",
								Resource: cmKey,
								Message:  fmt.Sprintf("ConfigMap '%s' referenced by pod but not found — pod will fail to start", vol.ConfigMap.Name),
							})
						}
					}
				}

				// Check Secret volume
				if vol.Secret != nil {
					entry.SourceType = "Secret"
					entry.SourceName = vol.Secret.SecretName
					secretMounts++

					if vol.Secret.Optional != nil {
						entry.IsOptional = *vol.Secret.Optional
					} else {
						entry.IsOptional = false
						noOptional++
					}
				}

				// Check projected volume
				if vol.Projected != nil {
					for _, src := range vol.Projected.Sources {
						if src.ConfigMap != nil {
							entry.SourceType = "ConfigMap"
							entry.SourceName = src.ConfigMap.Name
							cmMounts++
							if src.ConfigMap.Optional != nil {
								entry.IsOptional = *src.ConfigMap.Optional
							}
						}
						if src.Secret != nil {
							entry.SourceType = "Secret"
							entry.SourceName = src.Secret.Name
							secretMounts++
							if src.Secret.Optional != nil {
								entry.IsOptional = *src.Secret.Optional
							}
						}
					}
				}

				if entry.SourceType != "" {
					entry.RiskLevel = assessConfigMountRisk(entry)
					if entry.RiskLevel != "healthy" {
						entries = append(entries, entry)
					}

					if _, ok := nsStats[pod.Namespace]; !ok {
						nsStats[pod.Namespace] = &ConfigMountNS{Namespace: pod.Namespace}
					}
					nsStats[pod.Namespace].PodCount = totalPods
					nsStats[pod.Namespace].MountCount++
					if entry.RiskLevel == "warning" || entry.RiskLevel == "critical" {
						nsStats[pod.Namespace].RiskCount++
					}
				}
			}

			// Check envFrom
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					envFromCM++
					if ef.ConfigMapRef.Optional == nil || !*ef.ConfigMapRef.Optional {
						noOptional++
					}
				}
				if ef.SecretRef != nil {
					envFromSecrets++
					if ef.SecretRef.Optional == nil || !*ef.SecretRef.Optional {
						noOptional++
					}
				}
			}
		}
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskCount > result.ByNamespace[j].RiskCount
	})

	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})
	if len(entries) > 50 {
		entries = entries[:50]
	}

	// Recommendations
	var recommendations []string
	if missingCMRefs > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d ConfigMap reference(s) are missing — pods will fail to start", missingCMRefs))
	}
	if largeCMs > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d large ConfigMap(s) (>500KB) — split data or use separate volumes", largeCMs))
	}
	if noOptional > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d mount(s) not marked optional — pods fail if source is deleted, consider optional: true", noOptional))
	}
	if subPathMounts > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d subPath mount(s) — subPath prevents ConfigMap/Secret hot-reload", subPathMounts))
	}
	if envFromSecrets > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d envFrom Secret injection(s) — consider mounted volumes for better rotation support", envFromSecrets))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "ConfigMap and Secret mounts are properly configured")
	}

	result.VolumeMounts = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = ConfigMountSummary{
		TotalPods:       totalPods,
		ConfigMapMounts: cmMounts,
		SecretMounts:    secretMounts,
		EnvFromCM:       envFromCM,
		EnvFromSecrets:  envFromSecrets,
		LargeConfigMaps: largeCMs,
		NoOptionalFlag:  noOptional,
		SubPathMounts:   subPathMounts,
		MissingCMRefs:   missingCMRefs,
		MissingSecrets:  missingSecrets,
	}
	result.HealthScore = computeConfigMountScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessConfigMountRisk determines risk level.
func assessConfigMountRisk(entry ConfigMountEntry) string {
	risk := 0
	if !entry.IsOptional && entry.SourceType == "ConfigMap" {
		risk += 1
	}
	if entry.IsLarge {
		risk += 2
	}
	if entry.HasSubPath {
		risk += 1
	}
	switch {
	case risk >= 3:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computeConfigMountScore computes a 0-100 health score.
func computeConfigMountScore(s ConfigMountSummary, issueCount int) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.MissingCMRefs * 15
	score -= s.LargeConfigMaps * 5
	score -= s.NoOptionalFlag * 1
	score -= s.SubPathMounts * 1
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
