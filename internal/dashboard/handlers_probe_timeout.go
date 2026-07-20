package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProbeTimeoutResult audits liveness/readiness probe timeout and interval configurations.
type ProbeTimeoutResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ProbeTimeoutSummary `json:"summary"`
	ByWorkload      []ProbeTimeoutEntry `json:"byWorkload"`
	Issues          []ProbeTimeoutIssue `json:"issues"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type ProbeTimeoutSummary struct {
	TotalContainers int `json:"totalContainers"`
	WithLiveness    int `json:"withLivenessProbe"`
	WithReadiness   int `json:"withReadinessProbe"`
	WithStartup     int `json:"withStartupProbe"`
	NoProbe         int `json:"withoutAnyProbe"`
	AggressiveProbe int `json:"aggressiveProbes"`  // timeout <2s or interval <5s
	LongTimeout     int `json:"longTimeoutProbes"` // timeout >10s
}

type ProbeTimeoutEntry struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	HasLiveness   bool     `json:"hasLiveness"`
	HasReadiness  bool     `json:"hasReadiness"`
	HasStartup    bool     `json:"hasStartup"`
	LiveTimeout   int32    `json:"livenessTimeoutSeconds"`
	ReadyTimeout  int32    `json:"readinessTimeoutSeconds"`
	LiveInterval  int32    `json:"livenessIntervalSeconds"`
	ReadyInterval int32    `json:"readinessIntervalSeconds"`
	RiskLevel     string   `json:"riskLevel"`
	Issues        []string `json:"issues"`
}

type ProbeTimeoutIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleProbeTimeoutAudit handles GET /api/deployment/probe-timeout-audit
func (s *Server) handleProbeTimeoutAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ProbeTimeoutResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			entry := ProbeTimeoutEntry{Name: dep.Name, Namespace: dep.Namespace}
			var issues []string

			if c.LivenessProbe != nil {
				entry.HasLiveness = true
				result.Summary.WithLiveness++
				if c.LivenessProbe.TimeoutSeconds != 0 {
					entry.LiveTimeout = c.LivenessProbe.TimeoutSeconds
				} else {
					entry.LiveTimeout = 1 // default
				}
				if c.LivenessProbe.PeriodSeconds != 0 {
					entry.LiveInterval = c.LivenessProbe.PeriodSeconds
				} else {
					entry.LiveInterval = 10
				}
				if entry.LiveTimeout < 2 {
					issues = append(issues, fmt.Sprintf("liveness timeout %ds too short", entry.LiveTimeout))
					result.Summary.AggressiveProbe++
				}
				if entry.LiveInterval < 5 {
					issues = append(issues, fmt.Sprintf("liveness interval %ds too aggressive", entry.LiveInterval))
					result.Summary.AggressiveProbe++
				}
				if entry.LiveTimeout > 10 {
					result.Summary.LongTimeout++
				}
			}

			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
				result.Summary.WithReadiness++
				if c.ReadinessProbe.TimeoutSeconds != 0 {
					entry.ReadyTimeout = c.ReadinessProbe.TimeoutSeconds
				} else {
					entry.ReadyTimeout = 1
				}
				if c.ReadinessProbe.PeriodSeconds != 0 {
					entry.ReadyInterval = c.ReadinessProbe.PeriodSeconds
				} else {
					entry.ReadyInterval = 10
				}
			}

			if c.StartupProbe != nil {
				entry.HasStartup = true
				result.Summary.WithStartup++
			}

			if !entry.HasLiveness && !entry.HasReadiness {
				result.Summary.NoProbe++
				issues = append(issues, "no probes defined")
				result.Issues = append(result.Issues, ProbeTimeoutIssue{
					Name: dep.Name, Namespace: dep.Namespace,
					Issue: fmt.Sprintf("container %s has no probes", c.Name), Severity: "high",
				})
			}

			entry.Issues = issues
			switch {
			case len(issues) >= 2:
				entry.RiskLevel = "high"
			case len(issues) >= 1:
				entry.RiskLevel = "medium"
			default:
				entry.RiskLevel = "low"
			}

			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalContainers > 0 {
		probed := result.Summary.WithLiveness + result.Summary.WithReadiness
		result.HealthScore = probed * 50 / result.Summary.TotalContainers
		if result.Summary.AggressiveProbe == 0 {
			result.HealthScore += 30
		}
		if result.Summary.NoProbe == 0 {
			result.HealthScore += 20
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("探针超时审计: %d 容器, %d liveness, %d readiness, %d startup, %d 无探针, %d 激进",
			result.Summary.TotalContainers, result.Summary.WithLiveness,
			result.Summary.WithReadiness, result.Summary.WithStartup,
			result.Summary.NoProbe, result.Summary.AggressiveProbe),
	}
	if result.Summary.NoProbe > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个容器无探针, 流量可能路由到未就绪 Pod", result.Summary.NoProbe))
	}
	if result.Summary.AggressiveProbe > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个探针过于激进, 可能导致误杀", result.Summary.AggressiveProbe))
	}
	writeJSON(w, result)
}

var _ appsv1.DeploymentSpec
