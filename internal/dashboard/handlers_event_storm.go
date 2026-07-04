package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventStormSeverity classifies the intensity of an event storm.
type EventStormSeverity string

const (
	StormCritical EventStormSeverity = "critical" // >50 events in 15min
	StormHigh     EventStormSeverity = "high"     // >20 events in 15min
	StormMedium   EventStormSeverity = "medium"   // >10 events in 15min
	StormLow      EventStormSeverity = "low"      // >5 events in 15min
)

// EventStormResult is the full event storm analysis output.
type EventStormResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         EventStormSummary  `json:"summary"`
	Namespaces      []EventNsSummary   `json:"namespaces"`
	TopReasons      []EventReasonAgg   `json:"topReasons"`
	FlappingRes     []FlappingResource `json:"flappingResources,omitempty"`
	RecentEvents    []EventEntry       `json:"recentEvents,omitempty"`
	StormDetected   bool               `json:"stormDetected"`
	Recommendations []string           `json:"recommendations"`
}

// EventStormSummary aggregates cluster-wide event metrics.
type EventStormSummary struct {
	TotalWarningEvents int                `json:"totalWarningEvents"`
	Events15Min        int                `json:"events15Min"`
	Events1Hour        int                `json:"events1Hour"`
	Events24Hour       int                `json:"events24Hour"`
	UniqueReasons      int                `json:"uniqueReasons"`
	AffectedResources  int                `json:"affectedResources"`
	StormSeverity      EventStormSeverity `json:"stormSeverity"`
	TopNamespace       string             `json:"topNamespace"`
}

// EventNsSummary aggregates events per namespace.
type EventNsSummary struct {
	Namespace     string `json:"namespace"`
	WarningCount  int    `json:"warningCount"`
	RecentCount   int    `json:"recentCount"` // last 15 min
	UniqueReasons int    `json:"uniqueReasons"`
	TopReason     string `json:"topReason"`
}

// EventReasonAgg aggregates events by reason.
type EventReasonAgg struct {
	Reason   string `json:"reason"`
	Count    int    `json:"count"`
	LastSeen string `json:"lastSeen"`
	Severity string `json:"severity"` // Warning / Normal
	Message  string `json:"message"`
}

// FlappingResource describes a resource with rapidly repeating events.
type FlappingResource struct {
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Reason    string  `json:"reason"`
	Count     int     `json:"count"`
	FlapRate  float64 `json:"flapRate"` // events per minute
	FirstSeen string  `json:"firstSeen"`
	LastSeen  string  `json:"lastSeen"`
}

// EventEntry is a single recent warning event.
type EventEntry struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Kind       string  `json:"kind"`
	Reason     string  `json:"reason"`
	Message    string  `json:"message"`
	Count      int32   `json:"count"`
	AgeMinutes float64 `json:"ageMinutes"`
}

// handleEventStorm analyzes Warning events for storm patterns, flapping,
// and cascade failure indicators.
// GET /api/operations/event-storm?namespace=xxx&hours=1
func (s *Server) handleEventStorm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	events, err := rc.clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         1000,
	})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	now := time.Now()
	result := EventStormResult{ScannedAt: now}

	// Aggregation maps
	nsMap := make(map[string]*EventNsSummary)
	reasonMap := make(map[string]*EventReasonAgg)
	flapMap := make(map[string]*FlappingResource)
	affectedResources := make(map[string]bool)
	recent15 := 0
	recent1h := 0
	recent24h := 0
	totalWarnings := 0

	for i := range events.Items {
		ev := &events.Items[i]
		evTime := ev.LastTimestamp.Time
		if evTime.IsZero() {
			evTime = ev.CreationTimestamp.Time
		}

		ageMin := now.Sub(evTime).Minutes()
		if ageMin < 0 {
			ageMin = 0
		}

		// Only count events from last 24h for stats
		if ageMin > 24*60 {
			continue
		}

		totalWarnings++
		recent24h++

		if ageMin <= 60 {
			recent1h++
		}
		if ageMin <= 15 {
			recent15++
		}

		// Namespace aggregation
		nsEntry := getOrCreateEventNs(nsMap, ev.InvolvedObject.Namespace)
		nsEntry.WarningCount += int(ev.Count)
		if ageMin <= 15 {
			nsEntry.RecentCount += int(ev.Count)
		}
		nsEntry.UniqueReasons++ // approximate; each event is a unique reason entry

		// Track affected resources
		resKey := fmt.Sprintf("%s/%s/%s", ev.InvolvedObject.Namespace, ev.InvolvedObject.Kind, ev.InvolvedObject.Name)
		affectedResources[resKey] = true

		// Reason aggregation
		reasonKey := ev.Reason
		if re, ok := reasonMap[reasonKey]; ok {
			re.Count += int(ev.Count)
			if evTime.Sub(now) < 0 { // newer
				re.LastSeen = evTime.Format(time.RFC3339)
			}
		} else {
			msg := ev.Message
			if len(msg) > 120 {
				msg = msg[:120] + "..."
			}
			reasonMap[reasonKey] = &EventReasonAgg{
				Reason:   ev.Reason,
				Count:    int(ev.Count),
				LastSeen: evTime.Format(time.RFC3339),
				Severity: string(ev.Type),
				Message:  msg,
			}
		}

		// Flapping detection: same resource + reason repeated >3 times
		flapKey := fmt.Sprintf("%s/%s/%s/%s", ev.InvolvedObject.Namespace, ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Reason)
		if fl, ok := flapMap[flapKey]; ok {
			fl.Count += int(ev.Count)
		} else {
			flapMap[flapKey] = &FlappingResource{
				Kind:      ev.InvolvedObject.Kind,
				Name:      ev.InvolvedObject.Name,
				Namespace: ev.InvolvedObject.Namespace,
				Reason:    ev.Reason,
				Count:     int(ev.Count),
				FirstSeen: ev.FirstTimestamp.Time.Format(time.RFC3339),
				LastSeen:  evTime.Format(time.RFC3339),
			}
		}

		// Collect recent events (last 15 min, max 50)
		if ageMin <= 15 && len(result.RecentEvents) < 50 {
			result.RecentEvents = append(result.RecentEvents, EventEntry{
				Name:       ev.InvolvedObject.Name,
				Namespace:  ev.InvolvedObject.Namespace,
				Kind:       ev.InvolvedObject.Kind,
				Reason:     ev.Reason,
				Message:    truncateMsg(ev.Message, 100),
				Count:      ev.Count,
				AgeMinutes: ageMin,
			})
		}
	}

	// Build summary
	result.Summary = EventStormSummary{
		TotalWarningEvents: totalWarnings,
		Events15Min:        recent15,
		Events1Hour:        recent1h,
		Events24Hour:       recent24h,
		UniqueReasons:      len(reasonMap),
		AffectedResources:  len(affectedResources),
		StormSeverity:      classifyStorm(recent15),
	}

	// Determine top namespace
	topNs := ""
	topNsCount := 0
	for _, ns := range nsMap {
		if ns.WarningCount > topNsCount {
			topNsCount = ns.WarningCount
			topNs = ns.Namespace
		}
	}
	result.Summary.TopNamespace = topNs
	result.StormDetected = recent15 > 20

	// Build namespace summaries
	for _, ns := range nsMap {
		result.Namespaces = append(result.Namespaces, *ns)
	}
	sort.Slice(result.Namespaces, func(i, j int) bool {
		return result.Namespaces[i].WarningCount > result.Namespaces[j].WarningCount
	})

	// Build top reasons
	for _, r := range reasonMap {
		result.TopReasons = append(result.TopReasons, *r)
	}
	sort.Slice(result.TopReasons, func(i, j int) bool {
		return result.TopReasons[i].Count > result.TopReasons[j].Count
	})

	// Build flapping resources (count > 3 and within 1 hour)
	for _, fl := range flapMap {
		if fl.Count >= 3 {
			// Calculate flap rate
			lastT, _ := time.Parse(time.RFC3339, fl.LastSeen)
			firstT, _ := time.Parse(time.RFC3339, fl.FirstSeen)
			duration := lastT.Sub(firstT).Minutes()
			if duration > 0 {
				fl.FlapRate = float64(fl.Count) / duration
			}
			result.FlappingRes = append(result.FlappingRes, *fl)
		}
	}
	sort.Slice(result.FlappingRes, func(i, j int) bool {
		return result.FlappingRes[i].Count > result.FlappingRes[j].Count
	})
	if len(result.FlappingRes) > 20 {
		result.FlappingRes = result.FlappingRes[:20]
	}

	// Sort recent events by age
	sort.Slice(result.RecentEvents, func(i, j int) bool {
		return result.RecentEvents[i].AgeMinutes < result.RecentEvents[j].AgeMinutes
	})

	// Generate recommendations
	result.Recommendations = generateStormRecommendations(result)

	writeJSON(w, result)
}

// classifyStorm determines storm severity from event count in last 15 minutes.
func classifyStorm(recent15 int) EventStormSeverity {
	switch {
	case recent15 > 50:
		return StormCritical
	case recent15 > 20:
		return StormHigh
	case recent15 > 10:
		return StormMedium
	case recent15 > 5:
		return StormLow
	default:
		return ""
	}
}

// generateStormRecommendations produces actionable recommendations.
func generateStormRecommendations(result EventStormResult) []string {
	var recs []string

	if result.StormDetected {
		recs = append(recs, fmt.Sprintf("Event storm detected: %d warning events in last 15 minutes — investigate immediately", result.Summary.Events15Min))
	}

	if result.Summary.TopNamespace != "" {
		recs = append(recs, fmt.Sprintf("Namespace %q has the most warning events — check resources in this namespace", result.Summary.TopNamespace))
	}

	if len(result.TopReasons) > 0 {
		top := result.TopReasons[0]
		recs = append(recs, fmt.Sprintf("Most frequent event reason: %q (%d occurrences) — %s", top.Reason, top.Count, truncateMsg(top.Message, 80)))
	}

	flapCount := len(result.FlappingRes)
	if flapCount > 0 {
		recs = append(recs, fmt.Sprintf("%d flapping resource(s) detected with repeated events — check for oscillating controllers or failing health checks", flapCount))
	}

	if result.Summary.AffectedResources > 10 {
		recs = append(recs, fmt.Sprintf("%d resources affected by warnings — this may indicate a cluster-wide issue rather than isolated failures", result.Summary.AffectedResources))
	}

	if result.Summary.Events1Hour > 100 {
		recs = append(recs, fmt.Sprintf("%d warning events in the last hour — consider setting up event-based alerts in Alertmanager", result.Summary.Events1Hour))
	}

	return recs
}

// getOrCreateEventNs returns or creates a namespace summary entry.
func getOrCreateEventNs(m map[string]*EventNsSummary, ns string) *EventNsSummary {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &EventNsSummary{Namespace: ns}
	m[ns] = e
	return e
}

// truncateMsg shortens a message string.
func truncateMsg(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
