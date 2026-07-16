package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DashAvailResult analyzes Grafana dashboard availability and observability UI coverage.
type DashAvailResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         DashAvailSummary    `json:"summary"`
	CoverageGaps    []DashCoverageGap   `json:"coverageGaps"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type DashAvailSummary struct {
	HasGrafana       bool   `json:"hasGrafana"`
	GrafanaNamespace string `json:"grafanaNamespace"`
	DashboardsFound  int    `json:"dashboardsFound"`
	NamespacesCovered int   `json:"namespacesCovered"`
	NamespacesBlind  int    `json:"namespacesBlind"`
	HasMetrics       bool   `json:"hasMetrics"`
	HasLogs          bool   `json:"hasLogs"`
}

type DashCoverageGap struct {
	Namespace string `json:"namespace"`
	Gap       string `json:"gap"`
	Severity  string `json:"severity"`
}

// handleDashAvail analyzes Grafana dashboard availability.
// GET /api/operations/dashboard-availability
func (s *Server) handleDashAvail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DashAvailResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Detect Grafana
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			if strings.Contains(strings.ToLower(c.Image), "grafana") {
				result.Summary.HasGrafana = true
				result.Summary.GrafanaNamespace = pod.Namespace
			}
			if strings.Contains(strings.ToLower(c.Image), "prometheus") || strings.Contains(strings.ToLower(c.Image), "victoria") {
				result.Summary.HasMetrics = true
			}
			if strings.Contains(strings.ToLower(c.Image), "loki") || strings.Contains(strings.ToLower(c.Image), "fluent") {
				result.Summary.HasLogs = true
			}
		}
	}

	// Count dashboards in ConfigMaps (Grafana dashboard JSON)
	for _, cm := range configmaps.Items {
		cmLower := strings.ToLower(cm.Name)
		if strings.Contains(cmLower, "dashboard") || strings.Contains(cmLower, "grafana") {
			for _, data := range cm.Data {
				if strings.Contains(data, "panels") || strings.Contains(data, "datasource") {
					result.Summary.DashboardsFound++
				}
			}
		}
	}

	// Check namespace coverage
	nsCount := 0
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] { continue }
		nsCount++
		// Check if namespace has any dashboard ConfigMap
		hasDash := false
		for _, cm := range configmaps.Items {
			if cm.Namespace == ns.Name && (strings.Contains(strings.ToLower(cm.Name), "dashboard") || strings.Contains(strings.ToLower(cm.Name), "grafana")) {
				hasDash = true
				break
			}
		}
		if hasDash {
			result.Summary.NamespacesCovered++
		} else {
			result.Summary.NamespacesBlind++
			result.CoverageGaps = append(result.CoverageGaps, DashCoverageGap{
				Namespace: ns.Name, Gap: "No dashboard ConfigMap found",
				Severity: "medium",
			})
		}
	}

	// Score
	score := 0
	if result.Summary.HasGrafana { score += 30 }
	if result.Summary.HasMetrics { score += 20 }
	if result.Summary.HasLogs { score += 15 }
	if nsCount > 0 { score += result.Summary.NamespacesCovered * 35 / nsCount }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.CoverageGaps, func(i, j int) bool {
		return result.CoverageGaps[i].Severity > result.CoverageGaps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Dashboard availability: %d/100 (grade %s) — Grafana:%v dashboards:%d covered:%d blind:%d", result.HealthScore, result.Grade, result.Summary.HasGrafana, result.Summary.DashboardsFound, result.Summary.NamespacesCovered, result.Summary.NamespacesBlind))
	if !result.Summary.HasGrafana { recs = append(recs, "No Grafana detected — deploy for observability dashboards") }
	if result.Summary.NamespacesBlind > 0 { recs = append(recs, fmt.Sprintf("%d namespaces without dashboards — create Grafana dashboards per namespace", result.Summary.NamespacesBlind)) }
	if len(recs) == 1 { recs = append(recs, "Observability dashboard coverage is comprehensive") }
	result.Recommendations = recs

	writeJSON(w, result)
}
