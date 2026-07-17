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

// EventNoiseFilterResult analyzes Kubernetes events to identify noise
// patterns, duplicate events, and actionable signal-to-noise ratio.
type EventNoiseFilterResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	LookbackHours   int                 `json:"lookbackHours"`
	Summary         EventNoiseSummary   `json:"summary"`
	ByReason        []EventNoiseEntry   `json:"byReason"`
	NoisePatterns   []EventNoisePattern `json:"noisePatterns"`
	SignalScore     int                 `json:"signalScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type EventNoiseSummary struct {
	TotalEvents     int     `json:"totalEvents"`
	WarningEvents   int     `json:"warningEvents"`
	NormalEvents    int     `json:"normalEvents"`
	UniqueReasons   int     `json:"uniqueReasons"`
	DuplicateCount  int     `json:"duplicateCount"`
	NoiseRatio      float64 `json:"noiseRatioPct"`
	ActionableRatio float64 `json:"actionableRatioPct"`
	TopNoisyReason  string  `json:"topNoisyReason"`
}

type EventNoiseEntry struct {
	Reason   string `json:"reason"`
	Count    int    `json:"count"`
	Type     string `json:"type"`
	IsNoise  bool   `json:"isNoise"`
	Category string `json:"category"`
}

type EventNoisePattern struct {
	Pattern    string `json:"pattern"`
	Count      int    `json:"count"`
	Impact     string `json:"impact"`
	Mitigation string `json:"mitigation"`
}

// Known noise event reasons that are typically non-actionable
var noiseReasons = map[string]bool{
	"Started":          true,
	"Pulled":           true,
	"Created":          true,
	"Scheduled":        true,
	"SuccessfulCreate": true,
	"NodeReady":        true,
	"NodeSchedulable":  true,
	"AlreadyUnbound":   true,
}

// handleEventNoiseFilter handles GET /api/operations/event-noise-filter
func (s *Server) handleEventNoiseFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	lookback := 24
	result := EventNoiseFilterResult{ScannedAt: time.Now(), LookbackHours: lookback}

	since := time.Now().Add(-time.Duration(lookback) * time.Hour)
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("lastTimestamp>%s", since.Format(time.RFC3339)),
	})

	reasonMap := make(map[string]*EventNoiseEntry)
	dupCount := 0
	noiseCount := 0
	actionCount := 0

	for _, ev := range events.Items {
		result.Summary.TotalEvents++

		key := ev.Reason
		if _, ok := reasonMap[key]; !ok {
			isNoise := noiseReasons[ev.Reason]
			category := "actionable"
			if isNoise {
				category = "noise"
			}
			if strings.Contains(strings.ToLower(ev.Reason), "fail") || strings.Contains(strings.ToLower(ev.Reason), "error") {
				category = "critical"
			}
			reasonMap[key] = &EventNoiseEntry{Reason: ev.Reason, Type: ev.Type, IsNoise: isNoise, Category: category}
		}
		entry := reasonMap[key]
		entry.Count++

		if ev.Count > 5 {
			dupCount++
		}

		if noiseReasons[ev.Reason] {
			noiseCount++
		} else {
			actionCount++
		}

		if ev.Type == corev1.EventTypeWarning {
			result.Summary.WarningEvents++
		} else {
			result.Summary.NormalEvents++
		}
	}
	result.Summary.DuplicateCount = dupCount
	result.Summary.UniqueReasons = len(reasonMap)

	if result.Summary.TotalEvents > 0 {
		result.Summary.NoiseRatio = float64(noiseCount) / float64(result.Summary.TotalEvents) * 100
		result.Summary.ActionableRatio = float64(actionCount) / float64(result.Summary.TotalEvents) * 100
	}

	var entries []EventNoiseEntry
	topNoisy := ""
	topNoisyCount := 0
	for _, e := range reasonMap {
		if e.IsNoise && e.Count > topNoisyCount {
			topNoisyCount = e.Count
			topNoisy = e.Reason
		}
		entries = append(entries, *e)
	}
	result.Summary.TopNoisyReason = topNoisy

	sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	result.ByReason = entries

	// Build noise patterns
	if dupCount > 0 {
		result.NoisePatterns = append(result.NoisePatterns, EventNoisePattern{
			Pattern: "repeated-events", Count: dupCount, Impact: "high",
			Mitigation: "Use event aggregation or increase event TTL cleanup",
		})
	}
	if noiseCount > result.Summary.TotalEvents/2 {
		result.NoisePatterns = append(result.NoisePatterns, EventNoisePattern{
			Pattern: "noise-dominant", Count: noiseCount, Impact: "medium",
			Mitigation: "Filter 'Started/Pulled/Created' events in alerting rules",
		})
	}

	// Signal score: higher actionable ratio = better
	result.SignalScore = int(result.Summary.ActionableRatio)
	switch {
	case result.SignalScore >= 70:
		result.Grade = "A"
	case result.SignalScore >= 50:
		result.Grade = "B"
	case result.SignalScore >= 30:
		result.Grade = "C"
	case result.SignalScore >= 15:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildEventNoiseRecs(&result)
	writeJSON(w, result)
}

func buildEventNoiseRecs(r *EventNoiseFilterResult) []string {
	recs := []string{
		fmt.Sprintf("事件噪声过滤: %d 事件/%dh, 信噪比 %.1f%%, %d 重复", r.Summary.TotalEvents, r.LookbackHours, r.Summary.ActionableRatio, r.Summary.DuplicateCount),
	}
	if r.Summary.NoiseRatio > 60 {
		recs = append(recs, fmt.Sprintf("噪声占比 %.1f%% 过高, 建议过滤 '%s' 类事件", r.Summary.NoiseRatio, r.Summary.TopNoisyReason))
	}
	if r.Summary.DuplicateCount > 100 {
		recs = append(recs, fmt.Sprintf("%d 个重复事件, 建议配置 event aggregation", r.Summary.DuplicateCount))
	}
	return recs
}
