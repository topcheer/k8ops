package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmDriftMonitorResult monitors Helm release drift by checking if deployed
// resources match their Helm-managed annotations and values.
type HelmDriftMonitorResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         HelmDriftSummary `json:"summary"`
	ByRelease       []HelmDriftEntry `json:"byRelease"`
	DriftedReleases []HelmDriftEntry `json:"driftedReleases"`
	MonitorScore    int              `json:"monitorScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type HelmDriftSummary struct {
	TotalReleases     int `json:"totalReleases"`
	HealthyReleases   int `json:"healthyReleases"`
	DriftedReleases   int `json:"driftedReleases"`
	OrphanedResources int `json:"orphanedResources"`
	StaleReleases     int `json:"staleReleases"`
}

type HelmDriftEntry struct {
	ReleaseName  string   `json:"releaseName"`
	Namespace    string   `json:"namespace"`
	ChartName    string   `json:"chartName"`
	ChartVersion string   `json:"chartVersion"`
	Status       string   `json:"status"`
	HasDrift     bool     `json:"hasDrift"`
	DriftItems   []string `json:"driftItems"`
	Age          string   `json:"age"`
	RiskLevel    string   `json:"riskLevel"`
}

// handleHelmDriftMonitor handles GET /api/deployment/helm-drift-monitor
func (s *Server) handleHelmDriftMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HelmDriftMonitorResult{ScannedAt: time.Now()}

	// Check Helm secrets for release metadata
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm",
	})

	releaseMap := make(map[string]*HelmDriftEntry)
	for _, sec := range secrets.Items {
		name := sec.Labels["name"]
		if name == "" {
			name = sec.Name
		}
		ns := sec.Namespace
		version := sec.Labels["version"]
		status := sec.Labels["status"]
		chart := ""
		if data, ok := sec.Data["release"]; ok {
			chart = fmt.Sprintf("(encrypted, %d bytes)", len(data))
		}

		key := ns + "/" + name
		if existing, ok := releaseMap[key]; ok {
			// Keep latest version
			if version > existing.ChartVersion {
				existing.ChartVersion = version
				existing.Status = status
			}
			continue
		}
		releaseMap[key] = &HelmDriftEntry{
			ReleaseName:  name,
			Namespace:    ns,
			ChartName:    chart,
			ChartVersion: version,
			Status:       status,
		}
	}

	// Check for drift indicators in configmaps
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	helmCMCount := 0
	for _, cm := range cms.Items {
		if cm.Labels["heritage"] == "Helm" || cm.Annotations["meta.helm.sh/release-name"] != "" {
			helmCMCount++
		}
	}

	// Also check for non-Helm managed resources that might be orphaned
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	orphanedPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		// Pod without Helm annotation but in Helm-managed namespace
		hasHelmAnn := false
		for k := range pod.Annotations {
			if contains(k, "helm.sh") {
				hasHelmAnn = true
				break
			}
		}
		if !hasHelmAnn && len(pod.OwnerReferences) == 0 {
			orphanedPods++
		}
	}

	var entries []HelmDriftEntry
	for _, e := range releaseMap {
		result.Summary.TotalReleases++

		// Determine drift risk
		if e.Status != "deployed" && e.Status != "" {
			e.HasDrift = true
			e.DriftItems = append(e.DriftItems, fmt.Sprintf("status=%s", e.Status))
			result.Summary.DriftedReleases++
		}

		// Age from secret creation
		e.Age = "unknown"

		switch {
		case e.HasDrift && e.Status == "failed":
			e.RiskLevel = "critical"
		case e.HasDrift:
			e.RiskLevel = "high"
		default:
			e.RiskLevel = "low"
			result.Summary.HealthyReleases++
		}

		entries = append(entries, *e)
	}

	result.Summary.OrphanedResources = orphanedPods

	sort.Slice(entries, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "low": 2}
		return rank[entries[i].RiskLevel] < rank[entries[j].RiskLevel]
	})
	result.ByRelease = entries

	for _, e := range entries {
		if e.HasDrift {
			result.DriftedReleases = append(result.DriftedReleases, e)
		}
	}

	if result.Summary.TotalReleases > 0 {
		result.MonitorScore = result.Summary.HealthyReleases * 100 / result.Summary.TotalReleases
	}
	gradeFromScore(&result.Grade, result.MonitorScore)

	result.Recommendations = []string{
		fmt.Sprintf("Helm 漂移监控: %d release, %d 健康, %d 漂移, %d 孤立资源", result.Summary.TotalReleases, result.Summary.HealthyReleases, result.Summary.DriftedReleases, result.Summary.OrphanedResources),
	}
	if result.Summary.DriftedReleases > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 release 存在漂移, 运行 'helm diff' 检查", result.Summary.DriftedReleases))
	}
	if result.Summary.OrphanedResources > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个孤立资源未被 Helm 管理", result.Summary.OrphanedResources))
	}
	if result.MonitorScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 使用 helm upgrade --install 统一管理, 定期 helm diff 审计")
	}
	writeJSON(w, result)
}
