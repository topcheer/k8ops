package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IncidentTimelineResult reconstructs incident timelines from Kubernetes
// events, pod state transitions, and crash/restart patterns.
type IncidentTimelineResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	LookbackHours   int                     `json:"lookbackHours"`
	Summary         IncidentTimelineSummary `json:"summary"`
	Timeline        []IncidentTimelineEvent `json:"timeline"`
	Incidents       []IncidentGroup         `json:"incidents"`
	TopIncident     IncidentGroup           `json:"topIncident"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type IncidentTimelineSummary struct {
	TotalEvents       int     `json:"totalEvents"`
	WarningEvents     int     `json:"warningEvents"`
	NormalEvents      int     `json:"normalEvents"`
	CrashEvents       int     `json:"crashEvents"`
	ActiveIncidents   int     `json:"activeIncidents"`
	AffectedWorkloads int     `json:"affectedWorkloads"`
	EventRate         float64 `json:"eventsPerHour"`
}

type IncidentTimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Kind        string    `json:"kind"`
	Reason      string    `json:"reason"`
	Message     string    `json:"message"`
	Namespace   string    `json:"namespace"`
	Workload    string    `json:"workload"`
	Severity    string    `json:"severity"`
	ActionTaken string    `json:"actionTaken"`
}

type IncidentGroup struct {
	ID             string                  `json:"id"`
	StartTime      time.Time               `json:"startTime"`
	EndTime        *time.Time              `json:"endTime"`
	Duration       string                  `json:"duration"`
	Severity       string                  `json:"severity"`
	Title          string                  `json:"title"`
	Workload       string                  `json:"workload"`
	Namespace      string                  `json:"namespace"`
	EventCount     int                     `json:"eventCount"`
	RootCause      string                  `json:"rootCause"`
	TimelineEvents []IncidentTimelineEvent `json:"timelineEvents"`
	Status         string                  `json:"status"`
}

// handleIncidentTimeline handles GET /api/operations/incident-timeline
func (s *Server) handleIncidentTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	lookback := 24 // hours
	result := IncidentTimelineResult{
		ScannedAt:     time.Now(),
		LookbackHours: lookback,
	}

	since := time.Now().Add(-time.Duration(lookback) * time.Hour)
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("lastTimestamp>%s", since.Format(time.RFC3339)),
	})

	// Also get recent pods for crash/restart info
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	wlSet := make(map[string]bool)

	// Build timeline from events
	for _, ev := range events.Items {
		if isSystemNamespace(ev.InvolvedObject.Namespace) {
			continue
		}

		severity := "info"
		if ev.Type == corev1.EventTypeWarning {
			severity = "warning"
			result.Summary.WarningEvents++
		} else {
			result.Summary.NormalEvents++
		}

		// Detect crash-related events
		isCrash := false
		crashReasons := []string{"BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "Evicted", "OOMKilled"}
		for _, cr := range crashReasons {
			if ev.Reason == cr {
				isCrash = true
				severity = "critical"
				result.Summary.CrashEvents++
				break
			}
		}

		wlName := ev.InvolvedObject.Name
		if ev.InvolvedObject.Kind == "Pod" {
			// Try to extract workload name from pod name pattern
			for _, pod := range pods.Items {
				if pod.Name == ev.InvolvedObject.Name {
					for _, ref := range pod.OwnerReferences {
						if ref.Controller != nil && *ref.Controller {
							wlName = ref.Name
							break
						}
					}
					break
				}
			}
		}

		if wlName != "" {
			wlSet[ev.InvolvedObject.Namespace+"/"+wlName] = true
		}

		ts := ev.LastTimestamp.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}

		result.Timeline = append(result.Timeline, IncidentTimelineEvent{
			Timestamp:   ts,
			Kind:        ev.InvolvedObject.Kind,
			Reason:      ev.Reason,
			Message:     truncateIncidentStr(ev.Message, 200),
			Namespace:   ev.InvolvedObject.Namespace,
			Workload:    wlName,
			Severity:    severity,
			ActionTaken: ev.Action,
		})

		if isCrash {
			_ = isCrash // keep for potential future use
		}
	}

	// Add pod restart/crash events
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
				ts := cs.LastTerminationState.Terminated.FinishedAt.Time
				if ts.After(since) {
					reason := cs.LastTerminationState.Terminated.Reason
					if reason == "" {
						reason = "Unknown"
					}
					result.Timeline = append(result.Timeline, IncidentTimelineEvent{
						Timestamp: ts,
						Kind:      "Pod",
						Reason:    reason,
						Message:   fmt.Sprintf("Container %s terminated (exit=%d)", cs.Name, cs.LastTerminationState.Terminated.ExitCode),
						Namespace: pod.Namespace,
						Workload:  wlName,
						Severity:  "critical",
					})
					result.Summary.CrashEvents++
					wlSet[pod.Namespace+"/"+wlName] = true
				}
			}
		}
	}

	result.Summary.TotalEvents = len(result.Timeline)
	result.Summary.AffectedWorkloads = len(wlSet)
	result.Summary.EventRate = float64(len(result.Timeline)) / float64(lookback)

	// Sort timeline by timestamp descending
	sort.Slice(result.Timeline, func(i, j int) bool {
		return result.Timeline[i].Timestamp.After(result.Timeline[j].Timestamp)
	})

	// Group events into incidents (events within 5 minutes of each other for same workload)
	result.Incidents = groupTimelineEvents(result.Timeline)
	result.Summary.ActiveIncidents = countActiveIncidents(result.Incidents)

	// Top incident
	if len(result.Incidents) > 0 {
		topIdx := 0
		for i, inc := range result.Incidents {
			if inc.EventCount > result.Incidents[topIdx].EventCount {
				topIdx = i
			}
		}
		result.TopIncident = result.Incidents[topIdx]
	}

	// Health score
	result.HealthScore = 100
	if result.Summary.CrashEvents > 0 {
		result.HealthScore -= result.Summary.CrashEvents * 2
	}
	if result.Summary.ActiveIncidents > 0 {
		result.HealthScore -= result.Summary.ActiveIncidents * 5
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	case result.HealthScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildIncidentTimelineRecs(&result)
	writeJSON(w, result)
}

func groupTimelineEvents(events []IncidentTimelineEvent) []IncidentGroup {
	var incidents []IncidentGroup
	if len(events) == 0 {
		return incidents
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	currentGroup := []IncidentTimelineEvent{events[0]}
	groupStart := events[0].Timestamp
	lastTime := events[0].Timestamp
	wlName := events[0].Workload
	ns := events[0].Namespace

	for i := 1; i < len(events); i++ {
		ev := events[i]
		gap := ev.Timestamp.Sub(lastTime)
		sameContext := ev.Workload == wlName && ev.Namespace == ns

		if gap < 5*time.Minute && sameContext {
			currentGroup = append(currentGroup, ev)
			lastTime = ev.Timestamp
		} else {
			// Close current group
			if len(currentGroup) >= 2 {
				incidents = append(incidents, makeIncident(currentGroup, groupStart, lastTime))
			}
			currentGroup = []IncidentTimelineEvent{ev}
			groupStart = ev.Timestamp
			lastTime = ev.Timestamp
			wlName = ev.Workload
			ns = ev.Namespace
		}
	}
	if len(currentGroup) >= 2 {
		incidents = append(incidents, makeIncident(currentGroup, groupStart, lastTime))
	}

	return incidents
}

func makeIncident(events []IncidentTimelineEvent, start, end time.Time) IncidentGroup {
	maxSeverity := "info"
	rootCause := "Unknown"
	for _, ev := range events {
		if ev.Severity == "critical" {
			maxSeverity = "critical"
			rootCause = ev.Reason
			break
		} else if ev.Severity == "warning" && maxSeverity != "critical" {
			maxSeverity = "warning"
			rootCause = ev.Reason
		}
	}

	status := "resolved"
	if time.Since(end) < 10*time.Minute {
		status = "active"
	}

	return IncidentGroup{
		ID:             fmt.Sprintf("INC-%s-%d", events[0].Workload, start.Unix()),
		StartTime:      start,
		EndTime:        &end,
		Duration:       end.Sub(start).Round(time.Second).String(),
		Severity:       maxSeverity,
		Title:          fmt.Sprintf("%s in %s", rootCause, events[0].Namespace),
		Workload:       events[0].Workload,
		Namespace:      events[0].Namespace,
		EventCount:     len(events),
		RootCause:      rootCause,
		TimelineEvents: events,
		Status:         status,
	}
}

func countActiveIncidents(incidents []IncidentGroup) int {
	count := 0
	for _, inc := range incidents {
		if inc.Status == "active" {
			count++
		}
	}
	return count
}

func truncateIncidentStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func buildIncidentTimelineRecs(r *IncidentTimelineResult) []string {
	recs := []string{
		fmt.Sprintf("事件时间线: %d 事件 (%.1f/h), %d 个事故, %d 活跃", r.Summary.TotalEvents, r.Summary.EventRate, len(r.Incidents), r.Summary.ActiveIncidents),
	}
	if r.Summary.CrashEvents > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个崩溃事件在过去 %d 小时内", r.Summary.CrashEvents, r.LookbackHours))
	}
	if r.Summary.ActiveIncidents > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个活跃事故需要关注", r.Summary.ActiveIncidents))
	}
	if r.TopIncident.EventCount > 0 {
		recs = append(recs, fmt.Sprintf("最大事故: %s/%s (%s, %d 事件, %s)", r.TopIncident.Namespace, r.TopIncident.Workload, r.TopIncident.Severity, r.TopIncident.EventCount, r.TopIncident.Duration))
	}
	if r.HealthScore < 60 {
		recs = append(recs, "建议: 检查最近事件模式, 设置告警规则减少 MTTR")
	}
	return recs
}
