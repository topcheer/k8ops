package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	tests := []struct {
		sorted   []float64
		p        int
		expected float64
	}{
		{[]float64{1, 2, 3, 4, 5}, 50, 3},
		{[]float64{1, 2, 3, 4, 5}, 95, 5},
		{[]float64{1, 2, 3, 4, 5}, 99, 5},
		{[]float64{10}, 50, 10},
		{[]float64{}, 50, 0},
	}

	for _, tt := range tests {
		got := percentile(tt.sorted, tt.p)
		if got != tt.expected {
			t.Errorf("percentile(%v, %d) = %v, want %v", tt.sorted, tt.p, got, tt.expected)
		}
	}
}

func TestAPIPerformanceTracker(t *testing.T) {
	tracker := newAPIPerformanceTracker(100)

	// Record some samples
	tracker.Record(APISample{Method: "GET", Path: "/api/health", Duration: 5 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "GET", Path: "/api/health", Duration: 10 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "GET", Path: "/api/health", Duration: 15 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "POST", Path: "/api/scale", Duration: 100 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "POST", Path: "/api/scale", Duration: 200 * time.Millisecond, Status: 500})

	stats := tracker.Stats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 endpoint groups, got %d", len(stats))
	}

	// Find the scale endpoint (should be first due to higher p95)
	var scaleStat APIEndpointStats
	for _, s := range stats {
		if s.Path == "/api/scale" {
			scaleStat = s
		}
	}

	if scaleStat.Count != 2 {
		t.Errorf("expected 2 requests for /api/scale, got %d", scaleStat.Count)
	}
	if scaleStat.Errors != 1 {
		t.Errorf("expected 1 error for /api/scale, got %d", scaleStat.Errors)
	}
}

func TestAPIPerformanceTracker_RingBuffer(t *testing.T) {
	tracker := newAPIPerformanceTracker(3)

	tracker.Record(APISample{Method: "GET", Path: "/a", Duration: 1 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "GET", Path: "/b", Duration: 1 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "GET", Path: "/c", Duration: 1 * time.Millisecond, Status: 200})
	tracker.Record(APISample{Method: "GET", Path: "/d", Duration: 1 * time.Millisecond, Status: 200})

	stats := tracker.Stats()
	// Should have 3 samples (ring buffer), so at most 3 unique paths
	paths := map[string]bool{}
	for _, s := range stats {
		paths[s.Path] = true
	}
	if len(paths) > 3 {
		t.Errorf("ring buffer should cap at 3, got %d paths", len(paths))
	}
}

func TestHandleAPIPerformance_NilTracker(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/performance", nil)
	rr := httptest.NewRecorder()

	s.handleAPIPerformance(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
