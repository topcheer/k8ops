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

// AuditLogHealthResult is the audit log pipeline & event export health analysis.
type AuditLogHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         AuditLogSummary    `json:"summary"`
	Exporters       []AuditLogExporter `json:"exporters"`
	EventStreams    []EventStreamEntry `json:"eventStreams"`
	Issues          []AuditLogIssue    `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// AuditLogSummary aggregates audit log pipeline statistics.
type AuditLogSummary struct {
	AuditWebhookDetected bool `json:"auditWebhookDetected"`
	AuditLogDetected     bool `json:"auditLogDetected"`
	FluentBitDetected    bool `json:"fluentBitDetected"`
	FluentDDetected      bool `json:"fluentdDetected"`
	VectorDetected       bool `json:"vectorDetected"`
	LokiDetected         bool `json:"lokiDetected"`
	ExporterPodCount     int  `json:"exporterPodCount"`
	ReadyExporters       int  `json:"readyExporters"`
	EventForwarders      int  `json:"eventForwarders"`
	HighEventRate        int  `json:"highEventRateNamespaces"`
	OldEvents            int  `json:"oldEventNamespaces"`
}

// AuditLogExporter describes a log/event exporter pod.
type AuditLogExporter struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // fluent-bit, fluentd, vector, loki, etc.
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Image     string `json:"image"`
	RiskLevel string `json:"riskLevel"`
}

// EventStreamEntry describes event stream health for a namespace.
type EventStreamEntry struct {
	Namespace    string `json:"namespace"`
	EventCount   int    `json:"eventCount"`
	WarningCount int    `json:"warningCount"`
	LastEventAge string `json:"lastEventAge"`
	RiskLevel    string `json:"riskLevel"`
}

// AuditLogIssue is a detected audit log pipeline problem.
type AuditLogIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleAuditLogHealth audits audit log pipeline & event export health.
// GET /api/operations/audit-log-health
func (s *Server) handleAuditLogHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &AuditLogHealthResult{
		ScannedAt: time.Now(),
	}

	// 1. Detect log exporters by scanning pods
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	exporterKinds := map[string]bool{
		"fluent-bit": false,
		"fluentd":    false,
		"vector":     false,
		"loki":       false,
	}

	var exporters []AuditLogExporter
	exporterPodCount := 0
	readyExporters := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		podName := strings.ToLower(pod.Name)

		kind := ""
		for key := range exporterKinds {
			if strings.Contains(podName, key) {
				kind = key
				exporterKinds[key] = true
				break
			}
		}
		if kind == "" {
			for _, c := range pod.Spec.Containers {
				img := strings.ToLower(c.Image)
				for key := range exporterKinds {
					if strings.Contains(img, key) {
						kind = key
						exporterKinds[key] = true
						break
					}
				}
				if kind != "" {
					break
				}
			}
		}
		if kind == "" {
			continue
		}

		exporterPodCount++

		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if ready {
			readyExporters++
		}

		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}

		image := ""
		if len(pod.Spec.Containers) > 0 {
			image = pod.Spec.Containers[0].Image
		}

		entry := AuditLogExporter{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Kind:      kind,
			Ready:     ready,
			Restarts:  restarts,
			Image:     image,
			RiskLevel: "healthy",
		}

		if !ready {
			entry.RiskLevel = "critical"
			result.Issues = append(result.Issues, AuditLogIssue{
				Severity: "critical",
				Type:     "exporter-not-ready",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("%s pod is not ready — log/event export may be impaired", kind),
			})
		}
		if restarts > 3 {
			entry.RiskLevel = "warning"
			result.Issues = append(result.Issues, AuditLogIssue{
				Severity: "warning",
				Type:     "exporter-high-restarts",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("%s pod has %d restarts — may indicate instability", kind, restarts),
			})
		}

		exporters = append(exporters, entry)
	}

	// 2. Check event streams per namespace
	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ns := range nsList.Items {
			if isSystemNamespace(ns.Name) {
				continue
			}

			events, err := rc.clientset.CoreV1().Events(ns.Name).List(ctx, metav1.ListOptions{
				FieldSelector: "type=Warning",
			})
			if err != nil {
				continue
			}

			if len(events.Items) == 0 {
				continue
			}

			warningCount := len(events.Items)
			lastEventTime := time.Time{}
			for _, ev := range events.Items {
				if ev.LastTimestamp.After(lastEventTime) {
					lastEventTime = ev.LastTimestamp.Time
				}
			}

			lastAge := "unknown"
			if !lastEventTime.IsZero() {
				lastAge = time.Since(lastEventTime).Round(time.Hour).String()
			}

			riskLevel := "healthy"
			if warningCount > 50 {
				riskLevel = "warning"
			}
			if warningCount > 100 {
				riskLevel = "critical"
			}

			result.EventStreams = append(result.EventStreams, EventStreamEntry{
				Namespace:    ns.Name,
				EventCount:   warningCount,
				WarningCount: warningCount,
				LastEventAge: lastAge,
				RiskLevel:    riskLevel,
			})

			if warningCount > 50 {
				result.Issues = append(result.Issues, AuditLogIssue{
					Severity: "warning",
					Type:     "high-event-rate",
					Resource: ns.Name,
					Message:  fmt.Sprintf("Namespace %s has %d warning events — investigate recurring issues", ns.Name, warningCount),
				})
			}
		}
	}

	sort.Slice(exporters, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[exporters[i].RiskLevel] < riskOrder[exporters[j].RiskLevel]
	})

	sort.Slice(result.EventStreams, func(i, j int) bool {
		return result.EventStreams[i].WarningCount > result.EventStreams[j].WarningCount
	})
	if len(result.EventStreams) > 20 {
		result.EventStreams = result.EventStreams[:20]
	}

	// Count stats
	highEventRate := 0
	oldEvents := 0
	for _, es := range result.EventStreams {
		if es.WarningCount > 50 {
			highEventRate++
		}
	}

	// Recommendations
	var recommendations []string
	anyExporter := false
	for _, v := range exporterKinds {
		if v {
			anyExporter = true
			break
		}
	}
	if !anyExporter {
		recommendations = append(recommendations, "No log/event exporter detected (fluent-bit, fluentd, vector, loki) — install a log pipeline for observability and compliance")
	}
	if exporterPodCount > 0 && readyExporters < exporterPodCount {
		recommendations = append(recommendations, fmt.Sprintf("%d/%d exporter pod(s) are not ready — check pod logs and configuration", exporterPodCount-readyExporters, exporterPodCount))
	}
	if highEventRate > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d namespace(s) have high warning event rates — investigate and resolve recurring issues", highEventRate))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Audit log pipeline is healthy — log exporters are running and event streams are normal")
	}

	result.Exporters = exporters
	result.Recommendations = recommendations
	result.Summary = AuditLogSummary{
		FluentBitDetected: exporterKinds["fluent-bit"],
		FluentDDetected:   exporterKinds["fluentd"],
		VectorDetected:    exporterKinds["vector"],
		LokiDetected:      exporterKinds["loki"],
		ExporterPodCount:  exporterPodCount,
		ReadyExporters:    readyExporters,
		HighEventRate:     highEventRate,
		OldEvents:         oldEvents,
	}
	result.HealthScore = computeAuditLogScore(result.Summary, len(result.Issues))

	writeJSON(w, result)
}

// computeAuditLogScore computes a 0-100 health score.
func computeAuditLogScore(s AuditLogSummary, issueCount int) int {
	score := 100

	// No exporter is a significant gap
	anyExporter := s.FluentBitDetected || s.FluentDDetected || s.VectorDetected || s.LokiDetected
	if !anyExporter {
		score -= 20
	}

	// Not ready exporters
	score -= (s.ExporterPodCount - s.ReadyExporters) * 10

	// High event rate
	score -= s.HighEventRate * 5

	// General issues
	score -= issueCount * 1

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
