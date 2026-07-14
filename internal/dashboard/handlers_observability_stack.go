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

// ObservabilityStackResult is the observability stack integration health audit.
type ObservabilityStackResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         ObservabilitySummary  `json:"summary"`
	Pillars         []ObservabilityPillar `json:"pillars"`
	Gaps            []ObservabilityGap    `json:"gaps"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// ObservabilitySummary aggregates observability stack status.
type ObservabilitySummary struct {
	TotalPillars      int `json:"totalPillars"`      // 3: metrics, logging, tracing
	HealthyPillars    int `json:"healthyPillars"`    // pillars with active backends
	PartialPillars    int `json:"partialPillars"`    // pillars with degraded backends
	MissingPillars    int `json:"missingPillars"`    // pillars with no backends
	TotalBackends     int `json:"totalBackends"`     // total detected backends
	ReadyBackends     int `json:"readyBackends"`     // backends with all pods ready
	DegradedBackends  int `json:"degradedBackends"`  // backends with some pods not ready
	AgentCoverage     int `json:"agentCoverage"`     // % of nodes with observability agents
	NamespacesCovered int `json:"namespacesCovered"` // namespaces with at least one backend
	TotalNamespaces   int `json:"totalNamespaces"`
}

// ObservabilityPillar represents one pillar of observability.
type ObservabilityPillar struct {
	Name       string                 `json:"name"`   // metrics, logging, tracing
	Status     string                 `json:"status"` // healthy, degraded, missing
	Backends   []ObservabilityBackend `json:"backends"`
	AgentCount int                    `json:"agentCount"` // daemonset pods ready
	AgentTotal int                    `json:"agentTotal"` // daemonset desired
	Coverage   int                    `json:"coverage"`   // % coverage
}

// ObservabilityBackend describes a detected observability backend.
type ObservabilityBackend struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`   // prometheus, grafana, loki, jaeger, tempo, otel-collector, fluent-bit, fluentd, elasticsearch, etc.
	Pillar    string `json:"pillar"` // metrics, logging, tracing
	PodsReady int    `json:"podsReady"`
	PodsTotal int    `json:"podsTotal"`
	Status    string `json:"status"` // ready, degraded, down
}

// ObservabilityGap describes a gap in the observability stack.
type ObservabilityGap struct {
	Pillar   string `json:"pillar,omitempty"`
	Backend  string `json:"backend,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleObservabilityStack audits the full observability stack integration health.
// GET /api/operations/observability-stack
func (s *Server) handleObservabilityStack(w http.ResponseWriter, r *http.Request) {
	result := ObservabilityStackResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Known observability backends: name pattern → (type, pillar)
	backendPatterns := []struct {
		keyword string
		bType   string
		pillar  string
	}{
		// Metrics
		{"prometheus", "prometheus", "metrics"},
		{"vmsingle", "victoria-metrics", "metrics"},
		{"vmagent", "victoria-metrics", "metrics"},
		{"thanos", "thanos", "metrics"},
		{"mimir", "grafana-mimir", "metrics"},
		{" cortex", "cortex", "metrics"},
		// Logging
		{"loki", "loki", "logging"},
		{"elasticsearch", "elasticsearch", "logging"},
		{"fluent-bit", "fluent-bit", "logging"},
		{"fluentbit", "fluent-bit", "logging"},
		{"fluentd", "fluentd", "logging"},
		{"vector", "vector", "logging"},
		{"filebeat", "filebeat", "logging"},
		{"promtail", "promtail", "logging"},
		// Tracing
		{"jaeger", "jaeger", "tracing"},
		{"tempo", "grafana-tempo", "tracing"},
		{"zipkin", "zipkin", "tracing"},
		{"otel-collector", "opentelemetry", "tracing"},
		{"otelcol", "opentelemetry", "tracing"},
		{"opentelemetry", "opentelemetry", "tracing"},
	}

	// Known observability namespaces
	obsNamespaces := map[string]bool{
		"monitoring":     true,
		"observability":  true,
		"logging":        true,
		"tracing":        true,
		"telemetry":      true,
		"opentelemetry":  true,
		"grafana":        true,
		"prometheus":     true,
		"kube-system":    true,
		"elastic-system": true,
		"loki-stack":     true,
		"jaeger-system":  true,
	}

	pillarMap := map[string]*ObservabilityPillar{
		"metrics": {Name: "metrics", Status: "missing"},
		"logging": {Name: "logging", Status: "missing"},
		"tracing": {Name: "tracing", Status: "missing"},
	}

	// Known observability agent DaemonSets (by name pattern)
	agentKeywords := map[string]string{
		"node-exporter":  "metrics",
		"fluent-bit":     "logging",
		"fluentd":        "logging",
		"promtail":       "logging",
		"filebeat":       "logging",
		"otel-collector": "tracing",
		"otelcol":        "tracing",
		"vector":         "logging",
	}

	// 1. Detect observability backends from pods
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		seenBackends := map[string]*ObservabilityBackend{}
		agentPods := map[string]int{}  // pillar → ready count
		agentTotal := map[string]int{} // pillar → total count

		for _, pod := range pods.Items {
			podLower := strings.ToLower(pod.Name)
			nsLower := strings.ToLower(pod.Namespace)

			// Check if this is an observability agent (DaemonSet-style)
			for keyword, pillar := range agentKeywords {
				if strings.Contains(podLower, keyword) {
					if pod.Status.Phase == corev1.PodRunning {
						agentPods[pillar]++
					}
					agentTotal[pillar]++
				}
			}

			// Check if this is an observability backend
			if !obsNamespaces[nsLower] && !strings.Contains(podLower, "prometheus") &&
				!strings.Contains(podLower, "grafana") && !strings.Contains(podLower, "jaeger") {
				// Also check labels for observability indicators
				isObs := false
				for k, v := range pod.Labels {
					labelLower := strings.ToLower(k + "=" + v)
					if strings.Contains(labelLower, "prometheus") || strings.Contains(labelLower, "grafana") ||
						strings.Contains(labelLower, "jaeger") || strings.Contains(labelLower, "loki") ||
						strings.Contains(labelLower, "tempo") || strings.Contains(labelLower, "otel") {
						isObs = true
						break
					}
				}
				if !isObs {
					continue
				}
			}

			for _, bp := range backendPatterns {
				if strings.Contains(podLower, bp.keyword) {
					key := fmt.Sprintf("%s/%s", pod.Namespace, bp.bType)
					if seenBackends[key] == nil {
						seenBackends[key] = &ObservabilityBackend{
							Name:      bp.bType,
							Namespace: pod.Namespace,
							Type:      bp.bType,
							Pillar:    bp.pillar,
							Status:    "ready",
						}
					}
					seenBackends[key].PodsTotal++
					if pod.Status.Phase == corev1.PodRunning {
						isReady := true
						for _, cs := range pod.Status.ContainerStatuses {
							if !cs.Ready {
								isReady = false
								break
							}
						}
						if isReady {
							seenBackends[key].PodsReady++
						}
					}
					break
				}
			}
		}

		// 2. Build pillars from detected backends
		for _, backend := range seenBackends {
			pillar := pillarMap[backend.Pillar]
			if pillar == nil {
				continue
			}
			pillar.Backends = append(pillar.Backends, *backend)

			// Update backend status
			if backend.PodsReady == 0 {
				backend.Status = "down"
				result.Gaps = append(result.Gaps, ObservabilityGap{
					Pillar:   backend.Pillar,
					Backend:  backend.Name,
					Issue:    fmt.Sprintf("%s in %s has no ready pods", backend.Name, backend.Namespace),
					Severity: "critical",
				})
			} else if backend.PodsReady < backend.PodsTotal {
				backend.Status = "degraded"
				result.Gaps = append(result.Gaps, ObservabilityGap{
					Pillar:  backend.Pillar,
					Backend: backend.Name,
					Issue: fmt.Sprintf("%s in %s: %d/%d pods ready",
						backend.Name, backend.Namespace, backend.PodsReady, backend.PodsTotal),
					Severity: "warning",
				})
			}

			result.Summary.TotalBackends++
			if backend.PodsReady > 0 {
				result.Summary.ReadyBackends++
			}
			if backend.PodsReady < backend.PodsTotal {
				result.Summary.DegradedBackends++
			}
		}

		// 3. Update pillar statuses and agent coverage
		nodeCount := 0
		for _, pillar := range pillarMap {
			if len(pillar.Backends) > 0 {
				pillar.AgentCount = agentPods[pillar.Name]
				pillar.AgentTotal = agentTotal[pillar.Name]
				if pillar.AgentTotal > 0 {
					pillar.Coverage = pillar.AgentCount * 100 / pillar.AgentTotal
				} else {
					pillar.Coverage = 0
					result.Gaps = append(result.Gaps, ObservabilityGap{
						Pillar:   pillar.Name,
						Issue:    fmt.Sprintf("No %s agent DaemonSet detected", pillar.Name),
						Severity: "warning",
					})
				}

				if pillar.Backends != nil && len(pillar.Backends) > 0 {
					allReady := true
					for _, b := range pillar.Backends {
						if b.PodsReady == 0 {
							allReady = false
							break
						}
					}
					if allReady {
						pillar.Status = "healthy"
						result.Summary.HealthyPillars++
					} else {
						pillar.Status = "degraded"
						result.Summary.PartialPillars++
					}
				}
			} else {
				result.Summary.MissingPillars++
				result.Gaps = append(result.Gaps, ObservabilityGap{
					Pillar:   pillar.Name,
					Issue:    fmt.Sprintf("No %s backend detected", pillar.Name),
					Severity: "critical",
				})
			}
			result.Pillars = append(result.Pillars, *pillar)
		}

		// 4. Calculate namespace coverage
		nsWithBackends := map[string]bool{}
		for _, backend := range seenBackends {
			nsWithBackends[backend.Namespace] = true
		}
		namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
		if err == nil {
			result.Summary.TotalNamespaces = len(namespaces.Items)
			result.Summary.NamespacesCovered = len(nsWithBackends)
		}

		// 5. Calculate node count for agent coverage
		nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
		if err == nil {
			nodeCount = len(nodes.Items)
		}

		// Agent coverage = average of per-pillar agent coverage
		if nodeCount > 0 {
			totalCov := 0
			countedPillars := 0
			for _, pillar := range pillarMap {
				if pillar.AgentTotal > 0 {
					totalCov += pillar.Coverage
					countedPillars++
				}
			}
			if countedPillars > 0 {
				result.Summary.AgentCoverage = totalCov / countedPillars
			}
		}
	}

	// Sort pillars for deterministic output
	sort.Slice(result.Pillars, func(i, j int) bool {
		return result.Pillars[i].Name < result.Pillars[j].Name
	})

	// 6. Calculate health score
	score := 100
	// Missing pillars are critical
	score -= result.Summary.MissingPillars * 25
	// Partial pillars are moderate
	score -= result.Summary.PartialPillars * 10
	// Degraded backends
	score -= result.Summary.DegradedBackends * 5
	// Low agent coverage
	if result.Summary.AgentCoverage < 50 && result.Summary.TotalBackends > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	if result.Summary.MissingPillars > 0 {
		result.Recommendations = append(result.Recommendations,
			"Deploy missing observability pillars for full visibility (metrics, logging, tracing)")
	}
	if result.Summary.DegradedBackends > 0 {
		result.Recommendations = append(result.Recommendations,
			"Investigate degraded backends — some pods are not ready")
	}
	if result.Summary.AgentCoverage < 100 && result.Summary.TotalBackends > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Observability agent coverage is %d%% — ensure DaemonSet agents run on all nodes", result.Summary.AgentCoverage))
	}
	if result.Summary.NamespacesCovered < result.Summary.TotalNamespaces/2 && result.Summary.TotalNamespaces > 0 {
		result.Recommendations = append(result.Recommendations,
			"Observability backends are concentrated in few namespaces — consider multi-namespace deployment for HA")
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Observability stack is healthy — all three pillars (metrics, logging, tracing) are operational")
	}

	writeJSON(w, result)
}
