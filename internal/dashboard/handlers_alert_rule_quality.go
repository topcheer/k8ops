package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertRuleResult analyzes alerting rule quality:
// rule coverage per workload, alerting gaps, noise potential,
// and SLO-bound alerting maturity.
type AlertRuleResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         AlertRuleSummary   `json:"summary"`
	CoverageGaps    []AlertGap         `json:"coverageGaps"`
	NoiseRisks      []NoiseRisk        `json:"noiseRisks"`
	QualityScore    int                `json:"qualityScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type AlertRuleSummary struct {
	TotalWorkloads   int  `json:"totalWorkloads"`
	HasAlertManager  bool `json:"hasAlertManager"`
	HasPrometheus    bool `json:"hasPrometheus"`
	HasGrafana       bool `json:"hasGrafana"`
	TotalRules       int  `json:"totalRules"`
	WorkloadsWithAlerts int `json:"workloadsWithAlerts"`
	WorkloadsWithoutAlerts int `json:"workloadsWithoutAlerts"`
}

type AlertGap struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	Severity  string `json:"severity"`
	Impact    string `json:"impact"`
}

type NoiseRisk struct {
	Source    string `json:"source"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

// handleAlertRuleQuality analyzes alerting rule quality and coverage.
// GET /api/operations/alert-rule-quality
func (s *Server) handleAlertRuleQuality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AlertRuleResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Detect alerting infrastructure
	infraKeywords := map[string]string{
		"prometheus": "prometheus", "alertmanager": "alertmanager",
		"grafana": "grafana", "victoria-metrics": "prometheus",
		"vmalert": "alertmanager",
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, tool := range infraKeywords {
				if strings.Contains(imgLower, kw) {
					if tool == "prometheus" {
						result.Summary.HasPrometheus = true
					}
					if tool == "alertmanager" {
						result.Summary.HasAlertManager = true
					}
					if tool == "grafana" {
						result.Summary.HasGrafana = true
					}
				}
			}
		}
	}

	// Count Prometheus rules from ConfigMaps
	ruleCount := 0
	for _, cm := range configmaps.Items {
		if strings.Contains(cm.Name, "prometheus") || strings.Contains(cm.Name, "alert") || strings.Contains(cm.Name, "rules") {
			for _, data := range cm.Data {
				ruleCount += strings.Count(data, "alert:")
			}
		}
	}
	result.Summary.TotalRules = ruleCount

	// Check workload alerting coverage
	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(0)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		// Check if workload has alerting annotations
		hasAlerting := false
		for k := range dep.Annotations {
			if strings.Contains(strings.ToLower(k), "alert") || strings.Contains(strings.ToLower(k), "prometheus.io/scrape") {
				hasAlerting = true
				break
			}
		}
		// Check if in a namespace with Prometheus rules
		if !hasAlerting && result.Summary.HasPrometheus {
			hasAlerting = true // assume generic coverage if Prometheus exists
		}

		if hasAlerting {
			result.Summary.WorkloadsWithAlerts++
		} else {
			result.Summary.WorkloadsWithoutAlerts++
			severity := "medium"
			if replicas > 3 {
				severity = "high"
			}
			result.CoverageGaps = append(result.CoverageGaps, AlertGap{
				Workload:  dep.Name,
				Namespace: dep.Namespace,
				Replicas:  int(replicas),
				Severity:  severity,
				Impact:    fmt.Sprintf("No alerting for '%s' (%d replicas) — failures go undetected", dep.Name, replicas),
			})
		}
	}

	// Noise risks
	if ruleCount > 50 {
		result.NoiseRisks = append(result.NoiseRisks, NoiseRisk{
			Source: "prometheus-rules", RiskType: "rule-explosion",
			Severity: "medium", Suggestion: "Too many alert rules — consolidate and remove redundant alerts to prevent fatigue",
		})
	}
	if !result.Summary.HasAlertManager {
		result.NoiseRisks = append(result.NoiseRisks, NoiseRisk{
			Source: "alert-routing", RiskType: "no-routing",
			Severity: "high", Suggestion: "No Alertmanager — install for alert deduplication, grouping, and routing",
		})
	}

	// Score
	score := 0
	if result.Summary.HasPrometheus {
		score += 30
	}
	if result.Summary.HasAlertManager {
		score += 25
	}
	if result.Summary.HasGrafana {
		score += 15
	}
	if result.Summary.TotalWorkloads > 0 {
		score += result.Summary.WorkloadsWithAlerts * 30 / result.Summary.TotalWorkloads
	}
	result.QualityScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.QualityScore)

	// Sort
	sort.Slice(result.CoverageGaps, func(i, j int) bool {
		return result.CoverageGaps[i].Severity > result.CoverageGaps[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Alert rule quality: %d/100 (grade %s) — %d rules, %d/%d workloads covered", result.QualityScore, result.Grade, result.Summary.TotalRules, result.Summary.WorkloadsWithAlerts, result.Summary.TotalWorkloads))
	if !result.Summary.HasPrometheus {
		recs = append(recs, "No Prometheus detected — deploy for metric-based alerting")
	}
	if !result.Summary.HasAlertManager {
		recs = append(recs, "No Alertmanager — install for alert routing and deduplication")
	}
	if result.Summary.WorkloadsWithoutAlerts > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without alerting — add recording and alerting rules", result.Summary.WorkloadsWithoutAlerts))
	}
	if len(recs) == 1 {
		recs = append(recs, "Alerting coverage is comprehensive — maintain rule quality")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
