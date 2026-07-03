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

// EventSummary groups events by reason for a compact dashboard view.
type EventSummary struct {
	Reason     string `json:"reason"`
	Count      int    `json:"count"`
	LastSeen   string `json:"lastSeen"`
	Severity   string `json:"severity"`
	Sample     string `json:"sampleMessage"`
	AffectedNS int    `json:"affectedNamespaces"`
	TopObject  string `json:"topObject"`
}

// handleEventSummary returns aggregated warning events grouped by reason.
// GET /api/events/summary
func (s *Server) handleEventSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	// Fetch warning events
	var events *corev1.EventList
	var err error
	opts := metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         500,
	}

	if ns != "" {
		events, err = rc.clientset.CoreV1().Events(ns).List(ctx, opts)
	} else {
		events, err = rc.clientset.CoreV1().Events("").List(ctx, opts)
	}
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Group by reason
	type groupKey struct {
		reason string
	}
	groups := map[string][]*corev1.Event{}
	for i := range events.Items {
		ev := &events.Items[i]
		reason := ev.Reason
		if reason == "" {
			reason = "Unknown"
		}
		groups[reason] = append(groups[reason], ev)
	}

	// Build summaries
	summaries := make([]EventSummary, 0, len(groups))
	for reason, evs := range groups {
		totalCount := 0
		nsSet := map[string]bool{}
		var latest corev1.Event
		var worst corev1.Event

		for _, ev := range evs {
			totalCount += int(ev.Count)
			if ev.Count == 0 {
				totalCount++
			}
			nsSet[ev.InvolvedObject.Namespace] = true
			if ev.LastTimestamp.After(latest.LastTimestamp.Time) {
				latest = *ev
			}
			// Track the one with highest count as worst
			if ev.Count > worst.Count {
				worst = *ev
			}
		}

		severity := "warning"
		if strings.Contains(strings.ToLower(reason), "fail") ||
			strings.Contains(strings.ToLower(reason), "crash") ||
			strings.Contains(strings.ToLower(reason), "error") {
			severity = "critical"
		}

		summaries = append(summaries, EventSummary{
			Reason:     reason,
			Count:      totalCount,
			LastSeen:   latest.LastTimestamp.Format(time.RFC3339),
			Severity:   severity,
			Sample:     truncate(worst.Message, 200),
			AffectedNS: len(nsSet),
			TopObject:  fmt.Sprintf("%s/%s", worst.InvolvedObject.Kind, worst.InvolvedObject.Name),
		})
	}

	// Sort by count descending
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Count > summaries[j].Count
	})

	// Build cluster-level stats
	totalWarnings := 0
	criticalCount := 0
	nsAffected := map[string]bool{}
	for _, s := range summaries {
		totalWarnings += s.Count
		if s.Severity == "critical" {
			criticalCount++
		}
	}

	// Collect affected namespaces from raw events
	for _, ev := range events.Items {
		if ev.InvolvedObject.Namespace != "" {
			nsAffected[ev.InvolvedObject.Namespace] = true
		}
	}

	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"totalReasons":  len(summaries),
			"totalWarnings": totalWarnings,
			"criticalCount": criticalCount,
			"affectedNS":    len(nsAffected),
		},
		"items": summaries,
	})
}
