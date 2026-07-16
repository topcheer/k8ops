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

// ProbeEffResult is the health probe effectiveness analyzer.
type ProbeEffResult struct {
	ScannedAt           time.Time               `json:"scannedAt"`
	Summary             ProbeEffSummary         `json:"summary"`
	ByWorkload          []ProbeEffWorkload      `json:"byWorkload"`
	NoProbeEffWorkloads []ProbeEffWorkload      `json:"noProbeEffWorkloads,omitempty"`
	Ineffective         []IneffectiveProbeEntry `json:"ineffectiveProbes,omitempty"`
	Recommendations     []string                `json:"recommendations"`
	HealthScore         int                     `json:"healthScore"`
}

// ProbeEffSummary aggregates probe effectiveness statistics.
type ProbeEffSummary struct {
	TotalContainers   int     `json:"totalContainers"`
	WithLiveness      int     `json:"withLiveness"`
	WithReadiness     int     `json:"withReadiness"`
	WithStartup       int     `json:"withStartup"`
	WithoutLiveness   int     `json:"withoutLiveness"`
	WithoutReadiness  int     `json:"withoutReadiness"`
	LivenessCoverPct  float64 `json:"livenessCoveragePct"`
	ReadinessCoverPct float64 `json:"readinessCoveragePct"`
	IneffectiveCount  int     `json:"ineffectiveCount"`
	HighRiskCount     int     `json:"highRiskCount"` // containers with probes AND high restarts
}

// ProbeEffWorkload shows probe coverage for one workload.
type ProbeEffWorkload struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Kind          string   `json:"kind"`
	Containers    int      `json:"containers"`
	Liveness      int      `json:"livenessCount"`
	Readiness     int      `json:"readinessCount"`
	Startup       int      `json:"startupCount"`
	Restarts      int      `json:"totalRestarts"`
	ProbeCoverage float64  `json:"probeCoveragePct"`
	RiskLevel     string   `json:"riskLevel"`
	Issues        []string `json:"issues,omitempty"`
}

// IneffectiveProbeEntry identifies a probe that isn't preventing failures.
type IneffectiveProbeEntry struct {
	Container string `json:"container"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	ProbeType string `json:"probeType"` // liveness, readiness
	Issue     string `json:"issue"`
	Restarts  int    `json:"restarts"`
	Severity  string `json:"severity"`
}

// handleProbeEffect analyzes whether health probes effectively detect failures.
// GET /api/product/probe-effectiveness
func (s *Server) handleProbeEffect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := ProbeEffResult{ScannedAt: time.Now()}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// Aggregate per workload
	wlMap := map[string]*ProbeEffWorkload{}
	totalContainers := 0
	withLiveness := 0
	withReadiness := 0
	withStartup := 0
	var ineffectiveProbes []IneffectiveProbeEntry

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		wlName, wlKind := extractWorkloadFromPod(pod)
		if wlName == "" {
			wlName = pod.Name
			wlKind = "Pod"
		}

		wlKey := fmt.Sprintf("%s/%s", pod.Namespace, wlName)
		if wlMap[wlKey] == nil {
			wlMap[wlKey] = &ProbeEffWorkload{
				Name: wlName, Namespace: pod.Namespace, Kind: wlKind,
			}
		}

		podRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)
		}
		wlMap[wlKey].Restarts += podRestarts

		for _, c := range pod.Spec.Containers {
			totalContainers++
			wlMap[wlKey].Containers++

			hasLiveness := c.LivenessProbe != nil
			hasReadiness := c.ReadinessProbe != nil
			hasStartup := c.StartupProbe != nil

			if hasLiveness {
				withLiveness++
				wlMap[wlKey].Liveness++
			} else {
				withStartup++
			}
			if hasReadiness {
				withReadiness++
				wlMap[wlKey].Readiness++
			} else {
				wlMap[wlKey].Issues = appendUniqueProbe(wlMap[wlKey].Issues, "missing readiness probe")
			}
			if hasStartup {
				withStartup++
				wlMap[wlKey].Startup++
			}
			if !hasLiveness {
				wlMap[wlKey].Issues = appendUniqueProbe(wlMap[wlKey].Issues, "missing liveness probe")
			}

			// Check for ineffective probe patterns
			if hasLiveness {
				// Liveness with very long initialDelay = slow detection
				delay := c.LivenessProbe.InitialDelaySeconds
				if delay > 60 {
					ineffectiveProbes = append(ineffectiveProbes, IneffectiveProbeEntry{
						Container: c.Name,
						Namespace: pod.Namespace,
						Pod:       pod.Name,
						ProbeType: "liveness",
						Issue:     fmt.Sprintf("Liveness probe initialDelaySeconds=%d (>60s) — slow failure detection", delay),
						Severity:  "warning",
					})
				}

				// Liveness with very low failureThreshold = too aggressive
				threshold := c.LivenessProbe.FailureThreshold
				if threshold == 1 {
					ineffectiveProbes = append(ineffectiveProbes, IneffectiveProbeEntry{
						Container: c.Name,
						Namespace: pod.Namespace,
						Pod:       pod.Name,
						ProbeType: "liveness",
						Issue:     "Liveness probe failureThreshold=1 — single transient failure kills the pod",
						Severity:  "warning",
					})
				}

				// Liveness checking same endpoint as readiness (should differ)
				if hasReadiness && probesSameHandler(c.LivenessProbe, c.ReadinessProbe) {
					ineffectiveProbes = append(ineffectiveProbes, IneffectiveProbeEntry{
						Container: c.Name,
						Namespace: pod.Namespace,
						Pod:       pod.Name,
						ProbeType: "liveness",
						Issue:     "Liveness and readiness probes use same endpoint — cannot distinguish startup from runtime failures",
						Severity:  "info",
					})
				}
			}

			// High restarts but has liveness = probe may be ineffective
			if hasLiveness && podRestarts > 5 {
				ineffectiveProbes = append(ineffectiveProbes, IneffectiveProbeEntry{
					Container: c.Name,
					Namespace: pod.Namespace,
					Pod:       pod.Name,
					ProbeType: "liveness",
					Issue:     fmt.Sprintf("Container has liveness probe but still restarted %d times — probe may not cover failure modes", podRestarts),
					Restarts:  podRestarts,
					Severity:  "warning",
				})
				result.Summary.HighRiskCount++
			}
		}
	}

	// Build workload list
	var workloads, noProbeEffWorkloads []ProbeEffWorkload
	for _, wl := range wlMap {
		if wl.Containers > 0 {
			covered := wl.Liveness + wl.Readiness
			wl.ProbeCoverage = float64(covered) / float64(wl.Containers*2) * 100

			// Risk level
			if wl.ProbeCoverage < 50 {
				wl.RiskLevel = "high"
			} else if wl.ProbeCoverage < 80 {
				wl.RiskLevel = "medium"
			} else {
				wl.RiskLevel = "low"
			}
			if wl.Restarts > 10 {
				if wl.RiskLevel == "low" {
					wl.RiskLevel = "medium"
				}
			}
		}
		workloads = append(workloads, *wl)
		if wl.ProbeCoverage < 50 {
			noProbeEffWorkloads = append(noProbeEffWorkloads, *wl)
		}
	}
	sort.Slice(workloads, func(i, j int) bool {
		return workloads[i].ProbeCoverage < workloads[j].ProbeCoverage
	})
	result.ByWorkload = workloads
	result.NoProbeEffWorkloads = noProbeEffWorkloads

	sort.Slice(ineffectiveProbes, func(i, j int) bool {
		return ineffectiveProbes[i].Restarts > ineffectiveProbes[j].Restarts
	})
	if len(ineffectiveProbes) > 50 {
		ineffectiveProbes = ineffectiveProbes[:50]
	}
	result.Ineffective = ineffectiveProbes

	// Summary
	result.Summary.TotalContainers = totalContainers
	result.Summary.WithLiveness = withLiveness
	result.Summary.WithReadiness = withReadiness
	result.Summary.WithStartup = withStartup
	result.Summary.WithoutLiveness = totalContainers - withLiveness
	result.Summary.WithoutReadiness = totalContainers - withReadiness
	result.Summary.IneffectiveCount = len(ineffectiveProbes)
	if totalContainers > 0 {
		result.Summary.LivenessCoverPct = float64(withLiveness) / float64(totalContainers) * 100
		result.Summary.ReadinessCoverPct = float64(withReadiness) / float64(totalContainers) * 100
	}

	// Health score
	score := 100
	score -= result.Summary.WithoutLiveness * 2
	score -= result.Summary.WithoutReadiness * 3
	score -= result.Summary.HighRiskCount * 5
	score -= len(ineffectiveProbes)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	result.Recommendations = generateProbeEffectRecs(result)

	writeJSON(w, result)
}

// probesSameHandler checks if two probes use the same handler.
func probesSameHandler(a, b *corev1.Probe) bool {
	if a == nil || b == nil {
		return false
	}
	ah := getProbeHandlerDesc(a.ProbeHandler)
	bh := getProbeHandlerDesc(b.ProbeHandler)
	return ah != "" && ah == bh
}

// getProbeHandlerDesc returns a string description of the probe handler.
func getProbeHandlerDesc(h corev1.ProbeHandler) string {
	if h.HTTPGet != nil {
		return fmt.Sprintf("http:%s:%s:%d", h.HTTPGet.Path, h.HTTPGet.Port.String(), h.HTTPGet.Port.IntVal)
	}
	if h.TCPSocket != nil {
		return fmt.Sprintf("tcp:%s", h.TCPSocket.Port.String())
	}
	if h.Exec != nil {
		return fmt.Sprintf("exec:%s", strings.Join(h.Exec.Command, " "))
	}
	return ""
}

// appendUnique appends a string to a slice if not already present.
func appendUniqueProbe(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// generateProbeEffectRecs produces recommendations.
func generateProbeEffectRecs(result ProbeEffResult) []string {
	var recs []string

	if result.Summary.WithoutLiveness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) without liveness probe — pods won't be restarted on hang", result.Summary.WithoutLiveness))
	}
	if result.Summary.WithoutReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) without readiness probe — traffic routed before pod is ready", result.Summary.WithoutReadiness))
	}
	if result.Summary.HighRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have liveness probe but still restart frequently — probe may not cover actual failure modes", result.Summary.HighRiskCount))
	}
	if len(result.Ineffective) > 0 {
		recs = append(recs, fmt.Sprintf("%d ineffective probe configuration(s) detected — review thresholds and endpoints", len(result.Ineffective)))
	}
	if len(result.NoProbeEffWorkloads) > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) with <50%% probe coverage — add liveness and readiness probes", len(result.NoProbeEffWorkloads)))
	}

	livenessGap := 100 - result.Summary.LivenessCoverPct
	readinessGap := 100 - result.Summary.ReadinessCoverPct
	if livenessGap > 30 {
		recs = append(recs, fmt.Sprintf("Liveness coverage is only %.0f%% — critical gap in failure detection", result.Summary.LivenessCoverPct))
	}
	if readinessGap > 30 {
		recs = append(recs, fmt.Sprintf("Readiness coverage is only %.0f%% — traffic may reach unready pods", result.Summary.ReadinessCoverPct))
	}

	if len(recs) == 0 {
		recs = append(recs, "Probe coverage is comprehensive — all containers have liveness and readiness probes")
	}

	return recs
}
