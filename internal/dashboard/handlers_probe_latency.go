package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProbeLatencyResult analyzes health probe latency and readiness performance:
// slow startup probes, timeout risks, probe misconfiguration patterns.
type ProbeLatencyResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ProbeLatencySummary `json:"summary"`
	SlowWorkloads   []SlowWorkload      `json:"slowWorkloads"`
	MisconfigProbes []MisconfigProbe    `json:"misconfigProbes"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type ProbeLatencySummary struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	WithStartupProbe   int `json:"withStartupProbe"`
	WithReadinessProbe int `json:"withReadinessProbe"`
	WithLivenessProbe  int `json:"withLivenessProbe"`
	MissingProbes      int `json:"missingProbes"`
	SlowStartup        int `json:"slowStartup"`
	TimeoutRisks       int `json:"timeoutRisks"`
}

type SlowWorkload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

type MisconfigProbe struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Probe     string `json:"probe"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleProbeLatency analyzes health probe latency and readiness performance.
// GET /api/operations/probe-latency
func (s *Server) handleProbeLatency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ProbeLatencyResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		result.Summary.TotalWorkloads++
		hasStartup := false
		hasReady := false
		hasLive := false

		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.StartupProbe != nil {
				hasStartup = true
				result.Summary.WithStartupProbe++
				// Check for risky configs
				if c.StartupProbe.TimeoutSeconds > 10 {
					result.Summary.TimeoutRisks++
					result.MisconfigProbes = append(result.MisconfigProbes, MisconfigProbe{
						Name: dep.Name, Namespace: dep.Namespace, Probe: "startup",
						Issue:    fmt.Sprintf("timeoutSeconds=%d (>10s risks slow detection)", c.StartupProbe.TimeoutSeconds),
						Severity: "medium",
					})
				}
				if c.StartupProbe.PeriodSeconds > 30 {
					result.Summary.SlowStartup++
					result.SlowWorkloads = append(result.SlowWorkloads, SlowWorkload{
						Name: dep.Name, Namespace: dep.Namespace,
						Issue:    fmt.Sprintf("startup probe periodSeconds=%d (>30s = slow detection)", c.StartupProbe.PeriodSeconds),
						Severity: "medium",
					})
				}
			}
			if c.ReadinessProbe != nil {
				hasReady = true
				result.Summary.WithReadinessProbe++
				if c.ReadinessProbe.InitialDelaySeconds > 60 {
					result.Summary.SlowStartup++
					result.SlowWorkloads = append(result.SlowWorkloads, SlowWorkload{
						Name: dep.Name, Namespace: dep.Namespace,
						Issue:    fmt.Sprintf("readiness initialDelaySeconds=%d (>60s delays traffic)", c.ReadinessProbe.InitialDelaySeconds),
						Severity: "high",
					})
				}
			}
			if c.LivenessProbe != nil {
				result.Summary.WithLivenessProbe++
				_ = hasLive // track for completeness
				if c.LivenessProbe.InitialDelaySeconds > 120 {
					result.MisconfigProbes = append(result.MisconfigProbes, MisconfigProbe{
						Name: dep.Name, Namespace: dep.Namespace, Probe: "liveness",
						Issue:    fmt.Sprintf("initialDelaySeconds=%d (>120s = slow restart)", c.LivenessProbe.InitialDelaySeconds),
						Severity: "medium",
					})
				}
			}
		}

		if !hasReady && !hasStartup {
			result.Summary.MissingProbes++
			result.MisconfigProbes = append(result.MisconfigProbes, MisconfigProbe{
				Name: dep.Name, Namespace: dep.Namespace, Probe: "readiness",
				Issue:    "No readiness probe — traffic sent before pod is ready",
				Severity: "high",
			})
		}
	}

	// Score
	score := 100
	if result.Summary.TotalWorkloads > 0 {
		readyRatio := float64(result.Summary.WithReadinessProbe) / float64(result.Summary.TotalWorkloads)
		score = int(readyRatio * 60)
	}
	if result.Summary.WithStartupProbe > 0 && result.Summary.TotalWorkloads > 0 {
		score += result.Summary.WithStartupProbe * 20 / result.Summary.TotalWorkloads
	}
	score -= result.Summary.SlowStartup * 5
	score -= result.Summary.MissingProbes * 3
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.SlowWorkloads, func(i, j int) bool {
		return result.SlowWorkloads[i].Severity > result.SlowWorkloads[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Probe latency health: %d/100 (grade %s) — %d workloads, %d missing probes", result.HealthScore, result.Grade, result.Summary.TotalWorkloads, result.Summary.MissingProbes))
	if result.Summary.MissingProbes > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without readiness probes — traffic may hit unready pods", result.Summary.MissingProbes))
	}
	if result.Summary.SlowStartup > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with slow probe configs — tune initialDelaySeconds and periodSeconds", result.Summary.SlowStartup))
	}
	if len(recs) == 1 {
		recs = append(recs, "Probe configuration is comprehensive — all workloads have readiness checks")
	}
	result.Recommendations = recs
	_ = strings.ToLower // keep import used
	writeJSON(w, result)
}
