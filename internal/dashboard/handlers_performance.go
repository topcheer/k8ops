package dashboard

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// APISample represents a single API response time measurement.
type APISample struct {
	Method   string
	Path     string
	Duration time.Duration
	Status   int
}

// APIEndpointStats holds computed statistics for a single endpoint.
type APIEndpointStats struct {
	Path   string  `json:"path"`
	Method string  `json:"method"`
	Count  int     `json:"count"`
	P50    float64 `json:"p50Ms"`
	P95    float64 `json:"p95Ms"`
	P99    float64 `json:"p99Ms"`
	Max    float64 `json:"maxMs"`
	Avg    float64 `json:"avgMs"`
	Errors int     `json:"errors"`
}

// apiPerformanceTracker tracks API response times in a ring buffer for percentile computation.
type apiPerformanceTracker struct {
	mu      sync.Mutex
	samples []APISample
	maxSize int
}

func newAPIPerformanceTracker(maxSize int) *apiPerformanceTracker {
	return &apiPerformanceTracker{
		maxSize: maxSize,
		samples: make([]APISample, 0, maxSize),
	}
}

// Record adds a new API response time sample.
func (t *apiPerformanceTracker) Record(s APISample) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.samples) >= t.maxSize {
		// Overwrite oldest (ring buffer)
		t.samples = t.samples[1:]
	}
	t.samples = append(t.samples, s)
}

// Stats returns per-endpoint statistics grouped by method+path.
func (t *apiPerformanceTracker) Stats() []APIEndpointStats {
	t.mu.Lock()
	samples := make([]APISample, len(t.samples))
	copy(samples, t.samples)
	t.mu.Unlock()

	// Group by method+path
	groups := map[string][]APISample{}
	for _, s := range samples {
		key := s.Method + " " + s.Path
		groups[key] = append(groups[key], s)
	}

	stats := make([]APIEndpointStats, 0, len(groups))
	for key, groupSamples := range groups {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) < 2 {
			continue
		}

		durations := make([]float64, len(groupSamples))
		totalMs := 0.0
		errors := 0
		maxMs := 0.0

		for i, s := range groupSamples {
			ms := float64(s.Duration.Microseconds()) / 1000.0
			durations[i] = ms
			totalMs += ms
			if ms > maxMs {
				maxMs = ms
			}
			if s.Status >= 400 {
				errors++
			}
		}

		sort.Float64s(durations)
		count := len(durations)

		stat := APIEndpointStats{
			Method: parts[0],
			Path:   parts[1],
			Count:  count,
			P50:    percentile(durations, 50),
			P95:    percentile(durations, 95),
			P99:    percentile(durations, 99),
			Max:    maxMs,
			Avg:    totalMs / float64(count),
			Errors: errors,
		}
		stats = append(stats, stat)
	}

	// Sort by p95 descending (slowest endpoints first)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].P95 > stats[j].P95
	})

	return stats
}

// percentile computes the p-th percentile from a sorted slice.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// handleAPIPerformance returns API latency statistics.
// GET /api/system/performance
func (s *Server) handleAPIPerformance(w http.ResponseWriter, r *http.Request) {
	if s.perfTracker == nil {
		writeJSON(w, map[string]any{
			"endpoints": []APIEndpointStats{},
			"summary":   map[string]any{"note": "performance tracking not initialized"},
		})
		return
	}

	stats := s.perfTracker.Stats()

	// Build summary
	totalReqs, totalErrors := 0, 0
	var allP95 []float64
	for _, st := range stats {
		totalReqs += st.Count
		totalErrors += st.Errors
		allP95 = append(allP95, st.P95)
	}
	sort.Float64s(allP95)

	summary := map[string]any{
		"totalRequests":    totalReqs,
		"totalErrors":      totalErrors,
		"errorRate":        0.0,
		"endpointsTracked": len(stats),
	}
	if totalReqs > 0 {
		summary["errorRate"] = float64(totalErrors) / float64(totalReqs) * 100
	}
	if len(allP95) > 0 {
		summary["clusterP95Ms"] = percentile(allP95, 50)
	}

	writeJSON(w, map[string]any{
		"summary":   summary,
		"endpoints": stats,
	})
}
