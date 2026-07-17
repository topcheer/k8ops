package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIAccessResult analyzes API server access patterns from audit logs.
// It identifies hot resources, high-frequency callers, unusual access patterns,
// and potential security concerns from API server interactions.
type APIAccessResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         AccessSummary    `json:"summary"`
	TopCallers      []AccessCaller   `json:"topCallers"`
	TopResources    []AccessResource `json:"topResources"`
	ByVerb          []VerbStat       `json:"byVerb"`
	ByNamespace     []AccessNSStat   `json:"byNamespace"`
	Anomalies       []AccessAnomaly  `json:"anomalies"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

// AccessSummary aggregates API access statistics.
type AccessSummary struct {
	TotalEvents     int     `json:"totalEvents"`
	UniqueUsers     int     `json:"uniqueUsers"`
	UniqueResources int     `json:"uniqueResources"`
	GetCount        int     `json:"getCount"`
	ListCount       int     `json:"listCount"`
	CreateCount     int     `json:"createCount"`
	UpdateCount     int     `json:"updateCount"`
	DeleteCount     int     `json:"deleteCount"`
	WatchCount      int     `json:"watchCount"`
	FailedCount     int     `json:"failedCount"`
	AvgPerMinute    float64 `json:"avgPerMinute"`
}

// AccessCaller describes a top API caller.
type AccessCaller struct {
	Username  string `json:"username"`
	UserAgent string `json:"userAgent"`
	Count     int    `json:"count"`
	Verb      string `json:"topVerb"`
	Resource  string `json:"topResource"`
}

// AccessResource describes a hot resource.
type AccessResource struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Count      int    `json:"count"`
	AccessType string `json:"accessType"` // read, write, mixed
}

// VerbStat per-verb statistics.
type VerbStat struct {
	Verb  string  `json:"verb"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

// AccessNSStat per-namespace access stats.
type AccessNSStat struct {
	Namespace  string `json:"namespace"`
	ReadCount  int    `json:"readCount"`
	WriteCount int    `json:"writeCount"`
	Total      int    `json:"total"`
}

// AccessAnomaly describes an unusual access pattern.
type AccessAnomaly struct {
	Type     string `json:"type"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
	User     string `json:"user,omitempty"`
	Resource string `json:"resource,omitempty"`
}

// handleAPIAccess handles GET /api/operations/api-access-pattern
func (s *Server) handleAPIAccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APIAccessResult{ScannedAt: time.Now()}

	// Fetch recent events as a proxy for API access patterns
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	now := time.Now()

	// Analyze event patterns to infer API access
	userCounts := map[string]*AccessCaller{}
	resourceCounts := map[string]*AccessResource{}
	nsAccess := map[string]*AccessNSStat{}
	totalEvents := 0
	failedEvents := 0
	readCount := 0
	writeCount := 0

	for _, evt := range events.Items {
		age := now.Sub(evt.CreationTimestamp.Time)
		if age > time.Hour {
			continue // last 1 hour only
		}

		totalEvents++

		// Classify by event source/user
		user := evt.ReportingController
		if user == "" {
			user = evt.Source.Component
		}
		if user == "" {
			user = "unknown"
		}

		if userCounts[user] == nil {
			userCounts[user] = &AccessCaller{Username: user}
		}
		userCounts[user].Count++

		// Classify by resource
		resKey := evt.InvolvedObject.Kind + "/" + evt.InvolvedObject.Namespace
		if resourceCounts[resKey] == nil {
			resourceCounts[resKey] = &AccessResource{
				Resource:  evt.InvolvedObject.Kind,
				Namespace: evt.InvolvedObject.Namespace,
			}
		}
		resourceCounts[resKey].Count++

		// Read vs write
		if evt.Type == "Normal" {
			readCount++
			resourceCounts[resKey].AccessType = "read"
		} else {
			writeCount++
			if resourceCounts[resKey].AccessType == "read" {
				resourceCounts[resKey].AccessType = "mixed"
			} else {
				resourceCounts[resKey].AccessType = "write"
			}
		}

		// Failed events
		if evt.Type == "Warning" {
			failedEvents++
		}

		// Namespace stats
		ns := evt.InvolvedObject.Namespace
		if ns == "" {
			ns = "cluster"
		}
		if nsAccess[ns] == nil {
			nsAccess[ns] = &AccessNSStat{Namespace: ns}
		}
		nsAccess[ns].Total++
		if evt.Type == "Normal" {
			nsAccess[ns].ReadCount++
		} else {
			nsAccess[ns].WriteCount++
		}
	}

	// Build top callers
	for _, uc := range userCounts {
		result.TopCallers = append(result.TopCallers, *uc)
	}
	sort.Slice(result.TopCallers, func(i, j int) bool { return result.TopCallers[i].Count > result.TopCallers[j].Count })
	if len(result.TopCallers) > 15 {
		result.TopCallers = result.TopCallers[:15]
	}

	// Build top resources
	for _, rc := range resourceCounts {
		result.TopResources = append(result.TopResources, *rc)
	}
	sort.Slice(result.TopResources, func(i, j int) bool { return result.TopResources[i].Count > result.TopResources[j].Count })
	if len(result.TopResources) > 15 {
		result.TopResources = result.TopResources[:15]
	}

	// Verb stats from event types
	result.ByVerb = []VerbStat{
		{Verb: "read (normal)", Count: readCount},
		{Verb: "write (warning)", Count: writeCount},
	}
	if totalEvents > 0 {
		for i := range result.ByVerb {
			result.ByVerb[i].Pct = float64(result.ByVerb[i].Count) / float64(totalEvents) * 100
		}
	}

	// Namespace stats
	for _, ns := range nsAccess {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool { return result.ByNamespace[i].Total > result.ByNamespace[j].Total })

	// Summary
	result.Summary = AccessSummary{
		TotalEvents:     totalEvents,
		UniqueUsers:     len(userCounts),
		UniqueResources: len(resourceCounts),
		GetCount:        readCount,
		ListCount:       0, // not available from events
		FailedCount:     failedEvents,
		AvgPerMinute:    float64(totalEvents) / 60.0,
	}

	// Detect anomalies
	result.Anomalies = detectAccessAnomalies(result.Summary, userCounts, resourceCounts)

	// Score
	result.HealthScore = computeAccessScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateAccessRecs(result)

	writeJSON(w, result)
}

// detectAccessAnomalies finds unusual patterns.
func detectAccessAnomalies(s AccessSummary, users map[string]*AccessCaller, resources map[string]*AccessResource) []AccessAnomaly {
	var anomalies []AccessAnomaly

	// High failure rate
	if s.TotalEvents > 0 {
		failRate := float64(s.FailedCount) / float64(s.TotalEvents) * 100
		if failRate > 30 {
			anomalies = append(anomalies, AccessAnomaly{
				Type:     "high-failure-rate",
				Detail:   fmt.Sprintf("%.1f%% of API events are warnings/failures (%d/%d)", failRate, s.FailedCount, s.TotalEvents),
				Severity: "high",
			})
		}
	}

	// Single user dominating
	for name, uc := range users {
		if s.TotalEvents > 0 && uc.Count > s.TotalEvents/2 {
			anomalies = append(anomalies, AccessAnomaly{
				Type:     "dominant-caller",
				Detail:   fmt.Sprintf("User %s generates %d/%d events (>50%%)", name, uc.Count, s.TotalEvents),
				Severity: "medium",
				User:     name,
			})
		}
	}

	// Resource hot spot
	for key, rc := range resources {
		if rc.Count > 100 {
			anomalies = append(anomalies, AccessAnomaly{
				Type:     "resource-hotspot",
				Detail:   fmt.Sprintf("%s has %d events in 1h — high access pattern", key, rc.Count),
				Severity: "low",
				Resource: rc.Resource,
			})
		}
	}

	return anomalies
}

// computeAccessScore computes API health score.
func computeAccessScore(s AccessSummary) int {
	score := 100
	if s.TotalEvents == 0 {
		return score
	}
	// Penalize high failure rate
	failRate := float64(s.FailedCount) / float64(s.TotalEvents) * 100
	if failRate > 50 {
		score -= 30
	} else if failRate > 30 {
		score -= 20
	} else if failRate > 15 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateAccessRecs produces recommendations.
func generateAccessRecs(r APIAccessResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("API access: %d events in 1h (%.1f/min), %d users, %d resources — score %d/100",
		r.Summary.TotalEvents, r.Summary.AvgPerMinute, r.Summary.UniqueUsers, r.Summary.UniqueResources, r.HealthScore))

	if r.Summary.FailedCount > 0 {
		failPct := 0.0
		if r.Summary.TotalEvents > 0 {
			failPct = float64(r.Summary.FailedCount) / float64(r.Summary.TotalEvents) * 100
		}
		recs = append(recs, fmt.Sprintf("%.1f%% failed/warning events (%d) — investigate controller errors", failPct, r.Summary.FailedCount))
	}

	for _, a := range r.Anomalies {
		if a.Severity == "high" {
			recs = append(recs, fmt.Sprintf("ANOMALY [%s]: %s", a.Type, a.Detail))
		}
	}

	if len(r.TopCallers) > 0 {
		top := r.TopCallers[0]
		recs = append(recs, fmt.Sprintf("Top caller: %s (%d events)", top.Username, top.Count))
	}

	return recs
}
