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

// GrafanaHealthResult is the Grafana dashboard availability & datasource health analysis.
type GrafanaHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         GrafanaSummary     `json:"summary"`
	Dashboards      []GrafanaDashboard `json:"dashboards"`
	ByNamespace     []GrafanaNSStat    `json:"byNamespace"`
	GrafanaPods     []GrafanaPodInfo   `json:"grafanaPods"`
	Issues          []GrafanaIssue     `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// GrafanaSummary aggregates Grafana dashboard statistics.
type GrafanaSummary struct {
	GrafanaDetected    bool   `json:"grafanaDetected"`
	GrafanaVersion     string `json:"grafanaVersion,omitempty"`
	TotalDashboards    int    `json:"totalDashboards"`
	DashboardsWithData int    `json:"dashboardsWithData"`
	StaleDashboards    int    `json:"staleDashboards"`
	BrokenDashboards   int    `json:"brokenDashboards"`
	Datasources        int    `json:"datasources"`
	GrafanaPodCount    int    `json:"grafanaPodCount"`
	ReadyPods          int    `json:"readyPods"`
	NoLivenessProbe    int    `json:"noLivenessProbe"`
	NoReadinessProbe   int    `json:"noReadinessProbe"`
}

// GrafanaDashboard describes one Grafana dashboard ConfigMap.
type GrafanaDashboard struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Title         string `json:"title"`
	RefreshRate   string `json:"refreshRate"`
	PanelCount    int    `json:"panelCount"`
	DatasourceRef string `json:"datasourceRef"`
	HasTimeRange  bool   `json:"hasTimeRange"`
	IsStale       bool   `json:"isStale"`
	RiskLevel     string `json:"riskLevel"`
}

// GrafanaNSStat per-namespace dashboard stats.
type GrafanaNSStat struct {
	Namespace  string `json:"namespace"`
	Dashboards int    `json:"dashboards"`
	StaleCount int    `json:"staleCount"`
}

// GrafanaPodInfo describes a Grafana pod's health.
type GrafanaPodInfo struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restartCount"`
	HasLiveness  bool   `json:"hasLiveness"`
	HasReadiness bool   `json:"hasReadiness"`
	Image        string `json:"image"`
	Age          string `json:"age"`
}

// GrafanaIssue is a detected Grafana problem.
type GrafanaIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleGrafanaHealth audits Grafana dashboard availability and datasource health.
// GET /api/operations/grafana-health
func (s *Server) handleGrafanaHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	result := &GrafanaHealthResult{
		ScannedAt: time.Now(),
	}

	// List all pods to find Grafana instances
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: "",
	})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var grafanaPods []GrafanaPodInfo
	grafanaDetected := false
	var grafanaImage string

	for i := range pods.Items {
		pod := &pods.Items[i]
		isGrafanaPod := false
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			name := strings.ToLower(pod.Name)
			if strings.Contains(img, "grafana") || strings.Contains(name, "grafana") {
				isGrafanaPod = true
				grafanaDetected = true
				if grafanaImage == "" {
					grafanaImage = c.Image
				}
			}
		}
		if !isGrafanaPod {
			continue
		}

		// Check pod health
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		// Check probes
		hasLiveness := false
		hasReadiness := false
		for _, c := range pod.Spec.Containers {
			if c.LivenessProbe != nil {
				hasLiveness = true
			}
			if c.ReadinessProbe != nil {
				hasReadiness = true
			}
		}

		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}

		age := time.Since(pod.CreationTimestamp.Time).Round(time.Hour).String()

		grafanaPods = append(grafanaPods, GrafanaPodInfo{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			Ready:        ready,
			RestartCount: restarts,
			HasLiveness:  hasLiveness,
			HasReadiness: hasReadiness,
			Image:        grafanaImage,
			Age:          age,
		})
	}

	noLiveness := 0
	noReadiness := 0
	readyCount := 0
	for _, gp := range grafanaPods {
		if !gp.HasLiveness {
			noLiveness++
		}
		if !gp.HasReadiness {
			noReadiness++
		}
		if gp.Ready {
			readyCount++
		}
		if !gp.Ready {
			result.Issues = append(result.Issues, GrafanaIssue{
				Severity: "critical",
				Type:     "grafana-pod-not-ready",
				Resource: fmt.Sprintf("%s/%s", gp.Namespace, gp.Name),
				Message:  "Grafana pod is not ready — dashboards may be unavailable",
			})
		}
		if gp.RestartCount > 3 {
			result.Issues = append(result.Issues, GrafanaIssue{
				Severity: "warning",
				Type:     "grafana-high-restarts",
				Resource: fmt.Sprintf("%s/%s", gp.Namespace, gp.Name),
				Message:  fmt.Sprintf("Grafana pod has %d restarts — may indicate instability", gp.RestartCount),
			})
		}
	}

	// Scan ConfigMaps for Grafana dashboards
	configMaps, err := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var allDashboards []GrafanaDashboard
	staleCount := 0
	brokenCount := 0
	dashWithData := 0
	nsStats := make(map[string]*GrafanaNSStat)

	for i := range configMaps.Items {
		cm := &configMaps.Items[i]

		// Detect Grafana dashboard ConfigMaps by labels/annotations
		isDashboard := false
		if cm.Labels != nil {
			if v, ok := cm.Labels["grafana_dashboard"]; ok && v != "" {
				isDashboard = true
			}
			if _, ok := cm.Labels["app.kubernetes.io/name"]; ok && strings.Contains(strings.ToLower(cm.Labels["app.kubernetes.io/name"]), "grafana") {
				isDashboard = true
			}
		}
		if cm.Annotations != nil {
			if _, ok := cm.Annotations["grafana_dashboard"]; ok {
				isDashboard = true
			}
		}
		// Also detect by name pattern
		if strings.Contains(strings.ToLower(cm.Name), "grafana") && strings.Contains(strings.ToLower(cm.Name), "dashboard") {
			isDashboard = true
		}

		if !isDashboard {
			continue
		}

		// Analyze dashboard content
		dash := GrafanaDashboard{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		}

		// Parse dashboard JSON from ConfigMap data
		for key, value := range cm.Data {
			if strings.Contains(key, "dashboard") || strings.Contains(key, ".json") || key == "data" {
				dash.Title = extractJSONValue(value, "title")
				dash.RefreshRate = extractJSONValue(value, "refresh")
				dash.PanelCount = countPanels(value)
				dash.DatasourceRef = extractJSONValue(value, "datasource")
				dash.HasTimeRange = strings.Contains(value, "\"time\"") || strings.Contains(value, "\"from\"")
				break
			}
		}

		if dash.Title == "" {
			dash.Title = cm.Name
		}

		// Determine if stale (no refresh rate or very long refresh)
		if dash.RefreshRate == "" {
			dash.IsStale = true
			staleCount++
		} else if strings.Contains(dash.RefreshRate, "1d") || strings.Contains(dash.RefreshRate, "7d") || strings.Contains(dash.RefreshRate, "30d") {
			dash.IsStale = true
			staleCount++
		}

		// Check for broken dashboards (no datasource reference)
		if dash.DatasourceRef == "" && dash.PanelCount > 0 {
			brokenCount++
			result.Issues = append(result.Issues, GrafanaIssue{
				Severity: "warning",
				Type:     "dashboard-no-datasource",
				Resource: fmt.Sprintf("%s/%s", cm.Namespace, cm.Name),
				Message:  fmt.Sprintf("Dashboard '%s' has %d panels but no datasource reference — panels will show no data", dash.Title, dash.PanelCount),
			})
		} else if dash.PanelCount > 0 {
			dashWithData++
		}

		// Risk level
		if !dash.HasTimeRange && dash.PanelCount > 0 {
			result.Issues = append(result.Issues, GrafanaIssue{
				Severity: "info",
				Type:     "dashboard-no-time-range",
				Resource: fmt.Sprintf("%s/%s", cm.Namespace, cm.Name),
				Message:  fmt.Sprintf("Dashboard '%s' has no explicit time range — users may see unexpected data windows", dash.Title),
			})
		}

		dash.RiskLevel = assessGrafanaDashRisk(dash)

		allDashboards = append(allDashboards, dash)

		// Update namespace stats
		if _, ok := nsStats[cm.Namespace]; !ok {
			nsStats[cm.Namespace] = &GrafanaNSStat{Namespace: cm.Namespace}
		}
		nsStats[cm.Namespace].Dashboards++
		if dash.IsStale {
			nsStats[cm.Namespace].StaleCount++
		}
	}

	// Convert namespace stats to slice
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Dashboards > result.ByNamespace[j].Dashboards
	})

	// Sort dashboards by risk level
	sort.Slice(allDashboards, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[allDashboards[i].RiskLevel] < riskOrder[allDashboards[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if !grafanaDetected {
		recommendations = append(recommendations, "Grafana is not detected — install Grafana for metrics visualization and operational dashboards")
	}
	if len(grafanaPods) > 0 && readyCount == 0 {
		recommendations = append(recommendations, "All Grafana pods are not ready — check pod logs and configuration")
	}
	if noLiveness > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d Grafana pod(s) have no liveness probe — add liveness probes for automatic recovery", noLiveness))
	}
	if noReadiness > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d Grafana pod(s) have no readiness probe — add readiness probes for proper traffic routing", noReadiness))
	}
	if staleCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d dashboard(s) have stale or missing refresh rates — set auto-refresh for real-time monitoring", staleCount))
	}
	if brokenCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d dashboard(s) have no datasource reference — configure datasource to display data", brokenCount))
	}
	if len(allDashboards) == 0 && grafanaDetected {
		recommendations = append(recommendations, "Grafana is running but no dashboards detected — import community dashboards for cluster observability")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Grafana is healthy — all dashboards have proper datasources and refresh rates")
	}

	result.Dashboards = allDashboards
	result.GrafanaPods = grafanaPods
	result.Recommendations = recommendations
	result.Summary = GrafanaSummary{
		GrafanaDetected:    grafanaDetected,
		GrafanaVersion:     extractGrafanaVersion(grafanaImage),
		TotalDashboards:    len(allDashboards),
		DashboardsWithData: dashWithData,
		StaleDashboards:    staleCount,
		BrokenDashboards:   brokenCount,
		Datasources:        countUniqueDatasources(allDashboards),
		GrafanaPodCount:    len(grafanaPods),
		ReadyPods:          readyCount,
		NoLivenessProbe:    noLiveness,
		NoReadinessProbe:   noReadiness,
	}
	result.HealthScore = computeGrafanaHealthScore(result.Summary, len(result.Issues))

	writeJSON(w, result)
}

// extractJSONValue extracts a string field value from a JSON string.
func extractJSONValue(json, field string) string {
	searchKey := fmt.Sprintf("\"%s\"", field)
	idx := strings.Index(json, searchKey)
	if idx == -1 {
		return ""
	}
	// Find the colon after the key
	rest := json[idx+len(searchKey):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx == -1 {
		return ""
	}
	rest = rest[colonIdx+1:]
	// Skip whitespace
	rest = strings.TrimLeft(rest, " \t\n\r")
	// Check if value is a string (starts with quote)
	if len(rest) > 0 && rest[0] == '"' {
		endIdx := strings.Index(rest[1:], "\"")
		if endIdx == -1 {
			return ""
		}
		return rest[1 : endIdx+1]
	}
	// Return non-string value up to comma or closing brace
	endChars := []string{",", "}", "\n"}
	endIdx := len(rest)
	for _, c := range endChars {
		if i := strings.Index(rest, c); i != -1 && i < endIdx {
			endIdx = i
		}
	}
	return strings.TrimSpace(rest[:endIdx])
}

// countPanels counts the number of panels in a dashboard JSON.
func countPanels(json string) int {
	// Count occurrences of "type" within panels array
	count := strings.Count(json, "\"type\"")
	if count > 100 {
		count = 100 // sanity cap
	}
	return count
}

// extractGrafanaVersion extracts version from image string.
func extractGrafanaVersion(image string) string {
	if image == "" {
		return ""
	}
	// e.g. grafana/grafana:10.2.0 → 10.2.0
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return ""
	}
	ver := parts[len(parts)-1]
	// Remove suffixes like -ubuntu
	if idx := strings.Index(ver, "-"); idx != -1 {
		ver = ver[:idx]
	}
	return ver
}

// countUniqueDatasources counts unique datasource references across dashboards.
func countUniqueDatasources(dashboards []GrafanaDashboard) int {
	seen := make(map[string]bool)
	for _, d := range dashboards {
		if d.DatasourceRef != "" {
			seen[d.DatasourceRef] = true
		}
	}
	return len(seen)
}

// assessGrafanaDashRisk determines the risk level of a dashboard.
func assessGrafanaDashRisk(dash GrafanaDashboard) string {
	risk := 0
	if dash.IsStale {
		risk += 1
	}
	if dash.DatasourceRef == "" && dash.PanelCount > 0 {
		risk += 2
	}
	if !dash.HasTimeRange && dash.PanelCount > 0 {
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

// computeGrafanaHealthScore computes a 0-100 health score.
func computeGrafanaHealthScore(summary GrafanaSummary, issueCount int) int {
	if !summary.GrafanaDetected {
		return 50 // neutral — Grafana not installed
	}
	score := 100
	score -= (summary.GrafanaPodCount - summary.ReadyPods) * 15
	score -= summary.NoReadinessProbe * 3
	score -= summary.NoLivenessProbe * 2
	score -= summary.BrokenDashboards * 5
	score -= summary.StaleDashboards * 2
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
