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

// SignalCorrelationResult proactively correlates signals from multiple dimensions
// to detect emerging issues before they become incidents. Unlike incident-correlation
// (which triages existing incidents), this engine looks for anomaly patterns across
// restarts, resource pressure, event storms, node conditions, and pod health trends.
type SignalCorrelationResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CorrelationSummary  `json:"summary"`
	Correlations    []CorrelatedSignal  `json:"correlations"`
	Hotspots        []SignalHotspot     `json:"hotspots"`
	EmergingRisks   []EmergingRisk      `json:"emergingRisks"`
	SignalMatrix    []SignalMatrixEntry `json:"signalMatrix"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// CorrelationSummary aggregates correlation statistics.
type CorrelationSummary struct {
	TotalSignalsAnalyzed int `json:"totalSignalsAnalyzed"`
	CorrelationsFound    int `json:"correlationsFound"`
	HighRiskCorrelations int `json:"highRiskCorrelations"`
	HotspotsIdentified   int `json:"hotspotsIdentified"`
	EmergingRisks        int `json:"emergingRisks"`
	SignalSources        int `json:"signalSources"`
}

// CorrelatedSignal describes a detected signal correlation.
type CorrelatedSignal struct {
	ID          string   `json:"id"`
	Signals     []string `json:"signals"` // e.g., ["restart-spike", "cpu-pressure"]
	Scope       string   `json:"scope"`   // namespace/workload/node
	ScopeName   string   `json:"scopeName"`
	RiskLevel   string   `json:"riskLevel"`  // critical, high, medium, low
	Confidence  int      `json:"confidence"` // 0-100
	Description string   `json:"description"`
	Evidence    []string `json:"evidence"`
	ETA         string   `json:"eta"` // estimated time to impact
}

// SignalHotspot describes a cluster area with anomalous signal density.
type SignalHotspot struct {
	Scope       string   `json:"scope"`
	Name        string   `json:"name"`
	SignalCount int      `json:"signalCount"`
	SignalTypes []string `json:"signalTypes"`
	HeatScore   int      `json:"heatScore"` // 0-100
	Severity    string   `json:"severity"`
}

// EmergingRisk describes a risk that hasn't materialized yet.
type EmergingRisk struct {
	Type        string `json:"type"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
	Probability int    `json:"probability"` // 0-100
	Impact      string `json:"impact"`
	Mitigation  string `json:"mitigation"`
}

// SignalMatrixEntry is one signal source's status.
type SignalMatrixEntry struct {
	Source       string `json:"source"`
	Status       string `json:"status"`
	AnomalyCount int    `json:"anomalyCount"`
	Detail       string `json:"detail"`
}

// handleSignalCorrelation handles GET /api/operations/signal-correlation
func (s *Server) handleSignalCorrelation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SignalCorrelationResult{ScannedAt: time.Now()}
	now := time.Now()

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})

	// Collect signals per namespace
	nsSignals := map[string]map[string]int{} // ns -> signal type -> count
	nsPods := map[string][]corev1.Pod{}
	nodeSignals := map[string]map[string]int{}

	// Signal: pod restart patterns
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		ns := pod.Namespace
		if nsSignals[ns] == nil {
			nsSignals[ns] = map[string]int{}
		}
		nsPods[ns] = append(nsPods[ns], pod)

		// Restart count signal
		totalRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
		}
		if totalRestarts > 0 {
			nsSignals[ns]["restart-activity"] += totalRestarts
		}
		if totalRestarts > 5 {
			nsSignals[ns]["restart-spike"]++
		}

		// Pod phase signals
		switch pod.Status.Phase {
		case corev1.PodPending:
			nsSignals[ns]["pending-pods"]++
		case corev1.PodFailed:
			nsSignals[ns]["failed-pods"]++
		}

		// CrashLoop signal
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				nsSignals[ns]["crashloop"]++
			}
		}

		// Resource pressure signals
		for _, st := range pod.Status.ContainerStatuses {
			if st.State.Terminated != nil && st.State.Terminated.Reason == "OOMKilled" {
				nsSignals[ns]["oom-kills"]++
			}
		}

		// CPU/Memory request signals
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests == nil || len(c.Resources.Requests) == 0 {
				nsSignals[ns]["missing-requests"]++
			}
			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
				nsSignals[ns]["missing-limits"]++
			}
		}
	}

	// Signal: node pressure
	for _, node := range nodes.Items {
		nodeName := node.Name
		if nodeSignals[nodeName] == nil {
			nodeSignals[nodeName] = map[string]int{}
		}
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				nodeSignals[nodeName]["memory-pressure"]++
			case corev1.NodeDiskPressure:
				nodeSignals[nodeName]["disk-pressure"]++
			case corev1.NodePIDPressure:
				nodeSignals[nodeName]["pid-pressure"]++
			case corev1.NodeNetworkUnavailable:
				nodeSignals[nodeName]["network-unavailable"]++
			}
		}
	}

	// Signal: event storms
	for _, evt := range events.Items {
		if evt.Type != "Warning" {
			continue
		}
		age := now.Sub(evt.CreationTimestamp.Time)
		if age > 30*time.Minute {
			continue
		}
		ns := evt.Namespace
		if ns == "" {
			continue
		}
		if nsSignals[ns] == nil {
			nsSignals[ns] = map[string]int{}
		}
		nsSignals[ns]["warning-events"]++

		// Classify specific warning types
		reason := strings.ToLower(evt.Reason)
		if strings.Contains(reason, "failed") || strings.Contains(reason, "backoff") {
			nsSignals[ns]["failed-events"]++
		}
		if strings.Contains(reason, "unhealthy") || strings.Contains(reason, "unready") {
			nsSignals[ns]["unhealthy-events"]++
		}
	}

	// Analyze correlations per namespace
	corrID := 1
	for ns, signals := range nsSignals {
		// Correlation: restart-spike + crashloop + warning-events
		if signals["restart-spike"] > 0 && signals["crashloop"] > 0 {
			conf := 90
			risk := "critical"
			result.Correlations = append(result.Correlations, CorrelatedSignal{
				ID:          fmt.Sprintf("CORR-%d", corrID),
				Signals:     []string{"restart-spike", "crashloop"},
				Scope:       "namespace",
				ScopeName:   ns,
				RiskLevel:   risk,
				Confidence:  conf,
				Description: fmt.Sprintf("Namespace %s has %d crash-loop pods with %d restart-spike pods — active failure pattern", ns, signals["crashloop"], signals["restart-spike"]),
				Evidence: []string{
					fmt.Sprintf("crashloop pods: %d", signals["crashloop"]),
					fmt.Sprintf("restart spikes: %d", signals["restart-spike"]),
				},
				ETA: "<1h",
			})
			corrID++
		}

		// Correlation: pending-pods + missing-requests → scheduling failure
		if signals["pending-pods"] > 0 && signals["missing-requests"] > 0 {
			conf := 75
			risk := "high"
			result.Correlations = append(result.Correlations, CorrelatedSignal{
				ID:          fmt.Sprintf("CORR-%d", corrID),
				Signals:     []string{"pending-pods", "missing-requests"},
				Scope:       "namespace",
				ScopeName:   ns,
				RiskLevel:   risk,
				Confidence:  conf,
				Description: fmt.Sprintf("Namespace %s has %d pending pods and %d containers without resource requests — likely scheduling failure", ns, signals["pending-pods"], signals["missing-requests"]),
				Evidence: []string{
					fmt.Sprintf("pending pods: %d", signals["pending-pods"]),
					fmt.Sprintf("missing requests: %d", signals["missing-requests"]),
				},
				ETA: "1-4h",
			})
			corrID++
		}

		// Correlation: oom-kills + missing-limits → memory exhaustion
		if signals["oom-kills"] > 0 && signals["missing-limits"] > 0 {
			conf := 80
			risk := "high"
			result.Correlations = append(result.Correlations, CorrelatedSignal{
				ID:          fmt.Sprintf("CORR-%d", corrID),
				Signals:     []string{"oom-kills", "missing-limits"},
				Scope:       "namespace",
				ScopeName:   ns,
				RiskLevel:   risk,
				Confidence:  conf,
				Description: fmt.Sprintf("Namespace %s has %d OOM kills and %d containers without memory limits — unbounded memory consumption", ns, signals["oom-kills"], signals["missing-limits"]),
				Evidence: []string{
					fmt.Sprintf("OOM kills: %d", signals["oom-kills"]),
					fmt.Sprintf("missing limits: %d", signals["missing-limits"]),
				},
				ETA: "4-24h",
			})
			corrID++
		}

		// Correlation: warning-events + failed-events → cascading failure risk
		if signals["warning-events"] > 5 && signals["failed-events"] > 2 {
			conf := 70
			risk := "medium"
			result.Correlations = append(result.Correlations, CorrelatedSignal{
				ID:          fmt.Sprintf("CORR-%d", corrID),
				Signals:     []string{"warning-events", "failed-events"},
				Scope:       "namespace",
				ScopeName:   ns,
				RiskLevel:   risk,
				Confidence:  conf,
				Description: fmt.Sprintf("Namespace %s has %d warnings and %d failed events in last 30min — potential cascade", ns, signals["warning-events"], signals["failed-events"]),
				Evidence: []string{
					fmt.Sprintf("warnings (30m): %d", signals["warning-events"]),
					fmt.Sprintf("failed events: %d", signals["failed-events"]),
				},
				ETA: "1-8h",
			})
			corrID++
		}
	}

	// Build hotspots
	for ns, signals := range nsSignals {
		total := 0
		var types []string
		for sig, count := range signals {
			total += count
			types = append(types, sig)
		}
		if total >= 3 {
			heatScore := minInt(total*10, 100)
			severity := "low"
			if heatScore > 60 {
				severity = "high"
			} else if heatScore > 30 {
				severity = "medium"
			}
			sort.Strings(types)
			result.Hotspots = append(result.Hotspots, SignalHotspot{
				Scope:       "namespace",
				Name:        ns,
				SignalCount: total,
				SignalTypes: types,
				HeatScore:   heatScore,
				Severity:    severity,
			})
		}
	}
	sort.Slice(result.Hotspots, func(i, j int) bool {
		return result.Hotspots[i].HeatScore > result.Hotspots[j].HeatScore
	})

	// Node pressure hotspots
	for nodeName, signals := range nodeSignals {
		total := 0
		for _, count := range signals {
			total += count
		}
		if total > 0 {
			var types []string
			for sig := range signals {
				types = append(types, sig)
			}
			sort.Strings(types)
			result.Hotspots = append(result.Hotspots, SignalHotspot{
				Scope:       "node",
				Name:        nodeName,
				SignalCount: total,
				SignalTypes: types,
				HeatScore:   minInt(total*20, 100),
				Severity:    "critical",
			})
		}
	}

	// Emerging risks
	result.EmergingRisks = detectEmergingRisks(nsSignals, nodeSignals, nsPods)

	// Signal matrix
	result.SignalMatrix = buildSignalMatrix(nsSignals, nodeSignals)

	// Summary
	result.Summary.TotalSignalsAnalyzed = len(nsSignals) + len(nodeSignals)
	result.Summary.CorrelationsFound = len(result.Correlations)
	result.Summary.HighRiskCorrelations = countHighRisk(result.Correlations)
	result.Summary.HotspotsIdentified = len(result.Hotspots)
	result.Summary.EmergingRisks = len(result.EmergingRisks)
	result.Summary.SignalSources = 6 // pod-restarts, pod-phase, crashloop, oom, node-pressure, events

	// Health score
	result.HealthScore = computeCorrelationScore(result.Summary, len(pods.Items))
	result.Grade = scoreToGrade(result.HealthScore)

	// Recommendations
	result.Recommendations = generateCorrelationRecs(result)

	writeJSON(w, result)
}

// detectEmergingRisks identifies patterns that may become incidents.
func detectEmergingRisks(nsSignals map[string]map[string]int, nodeSignals map[string]map[string]int, nsPods map[string][]corev1.Pod) []EmergingRisk {
	var risks []EmergingRisk

	// Risk: namespace with high signal density
	for ns, sigs := range nsSignals {
		total := 0
		for _, c := range sigs {
			total += c
		}
		if total > 10 {
			risks = append(risks, EmergingRisk{
				Type:        "signal-saturation",
				Scope:       ns,
				Description: fmt.Sprintf("Namespace %s has %d anomalous signals — saturation imminent", ns, total),
				Probability: minInt(total*5, 90),
				Impact:      "high",
				Mitigation:  "Investigate signal sources and reduce load or fix failing pods",
			})
		}
	}

	// Risk: node pressure
	for node, sigs := range nodeSignals {
		if sigs["disk-pressure"] > 0 {
			risks = append(risks, EmergingRisk{
				Type:        "disk-exhaustion",
				Scope:       node,
				Description: fmt.Sprintf("Node %s has disk pressure — eviction threshold approaching", node),
				Probability: 70,
				Impact:      "critical",
				Mitigation:  "Clean up images, logs, and unused volumes on this node",
			})
		}
		if sigs["memory-pressure"] > 0 {
			risks = append(risks, EmergingRisk{
				Type:        "memory-exhaustion",
				Scope:       node,
				Description: fmt.Sprintf("Node %s has memory pressure — pods may be evicted", node),
				Probability: 65,
				Impact:      "high",
				Mitigation:  "Add memory limits, right-size workloads, or add nodes",
			})
		}
	}

	return risks
}

// buildSignalMatrix creates a summary matrix of signal sources.
func buildSignalMatrix(nsSignals map[string]map[string]int, nodeSignals map[string]map[string]int) []SignalMatrixEntry {
	var matrix []SignalMatrixEntry

	// Aggregate signal types across all namespaces
	aggSignals := map[string]int{}
	for _, sigs := range nsSignals {
		for sig, count := range sigs {
			aggSignals[sig] += count
		}
	}

	signalOrder := []string{"crashloop", "restart-spike", "restart-activity", "pending-pods", "failed-pods", "oom-kills", "missing-requests", "missing-limits", "warning-events", "failed-events", "unhealthy-events"}
	for _, sig := range signalOrder {
		count := aggSignals[sig]
		status := "healthy"
		if count > 0 {
			status = "warning"
			if count > 5 {
				status = "critical"
			}
		}
		matrix = append(matrix, SignalMatrixEntry{
			Source:       sig,
			Status:       status,
			AnomalyCount: count,
			Detail:       fmt.Sprintf("%d occurrences across cluster", count),
		})
	}

	// Node signals
	nodeAgg := map[string]int{}
	for _, sigs := range nodeSignals {
		for sig, count := range sigs {
			nodeAgg[sig] += count
		}
	}
	for _, sig := range []string{"memory-pressure", "disk-pressure", "pid-pressure", "network-unavailable"} {
		count := nodeAgg[sig]
		status := "healthy"
		if count > 0 {
			status = "critical"
		}
		matrix = append(matrix, SignalMatrixEntry{
			Source:       "node:" + sig,
			Status:       status,
			AnomalyCount: count,
			Detail:       fmt.Sprintf("%d nodes affected", count),
		})
	}

	return matrix
}

// countHighRisk counts critical and high risk correlations.
func countHighRisk(correlations []CorrelatedSignal) int {
	count := 0
	for _, c := range correlations {
		if c.RiskLevel == "critical" || c.RiskLevel == "high" {
			count++
		}
	}
	return count
}

// computeCorrelationScore computes a health score from correlation data.
func computeCorrelationScore(s CorrelationSummary, totalPods int) int {
	score := 100
	// Each high-risk correlation deducts heavily
	score -= s.HighRiskCorrelations * 10
	// Each emerging risk deducts moderately
	score -= s.EmergingRisks * 5
	// Hotspots deduct based on severity
	for _, h := range []int{s.HotspotsIdentified} {
		_ = h
	}
	if s.HotspotsIdentified > 0 {
		score -= minInt(s.HotspotsIdentified*3, 15)
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateCorrelationRecs produces recommendations.
func generateCorrelationRecs(r SignalCorrelationResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Signal correlation: %d correlations found (%d high-risk), %d hotspots, %d emerging risks — score %d/100",
		r.Summary.CorrelationsFound, r.Summary.HighRiskCorrelations, r.Summary.HotspotsIdentified, r.Summary.EmergingRisks, r.HealthScore))

	for _, c := range r.Correlations {
		if c.RiskLevel == "critical" || c.RiskLevel == "high" {
			recs = append(recs, fmt.Sprintf("[%s] %s: %s (ETA: %s, confidence: %d%%)",
				strings.ToUpper(c.RiskLevel), c.ScopeName, c.Description, c.ETA, c.Confidence))
		}
	}

	if len(r.Hotspots) > 0 {
		top := r.Hotspots[0]
		recs = append(recs, fmt.Sprintf("Top hotspot: %s/%s (heat %d/100, %d signals) — %s",
			top.Scope, top.Name, top.HeatScore, top.SignalCount, strings.Join(top.SignalTypes, ", ")))
	}

	for _, risk := range r.EmergingRisks {
		if risk.Probability > 50 {
			recs = append(recs, fmt.Sprintf("Emerging risk (%d%%): %s — %s", risk.Probability, risk.Scope, risk.Description))
		}
	}

	if r.Summary.CorrelationsFound == 0 && r.Summary.EmergingRisks == 0 {
		recs = append(recs, "No significant signal correlations detected — cluster is stable")
	}

	return recs
}
