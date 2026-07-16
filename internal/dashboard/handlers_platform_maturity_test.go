package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPlatformMaturityBasic(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	if result.OverallScore < 0 || result.OverallScore > 100 {
		t.Errorf("OverallScore should be 0-100, got %d", result.OverallScore)
	}
	if len(result.Dimensions) != 6 {
		t.Errorf("expected 6 dimensions, got %d", len(result.Dimensions))
	}
}

func TestPlatformMaturityLevels(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	validLevels := map[string]bool{
		"initial": true, "managed": true, "defined": true,
		"measured": true, "optimizing": true,
	}
	if !validLevels[result.OverallLevel] {
		t.Errorf("invalid overall level: %s", result.OverallLevel)
	}

	for _, dim := range result.Dimensions {
		if !validLevels[dim.Level] {
			t.Errorf("dimension %s has invalid level: %s", dim.Name, dim.Level)
		}
	}
}

func TestPlatformMaturityBlindSpots(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.BlindSpotStatus) != 6 {
		t.Errorf("expected 6 blind spots, got %d", len(result.BlindSpotStatus))
	}

	validStatus := map[string]bool{"addressed": true, "gap": true, "critical-gap": true}
	for _, bs := range result.BlindSpotStatus {
		if !validStatus[bs.Status] {
			t.Errorf("blind spot %s has invalid status: %s", bs.BlindSpot, bs.Status)
		}
		if bs.BlindSpot == "" || bs.Dimension == "" {
			t.Error("blind spot should have name and dimension")
		}
	}
}

func TestPlatformMaturityDimensionsSorted(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should be sorted by score ascending (weakest first)
	for i := 1; i < len(result.Dimensions); i++ {
		if result.Dimensions[i].Score < result.Dimensions[i-1].Score {
			t.Errorf("dimensions not sorted: %s(%d) before %s(%d)",
				result.Dimensions[i-1].Name, result.Dimensions[i-1].Score,
				result.Dimensions[i].Name, result.Dimensions[i].Score)
		}
	}
}

func TestPlatformMaturityRoadmap(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Roadmap items should be prioritized (ascending)
	for i := 1; i < len(result.Roadmap); i++ {
		if result.Roadmap[i].Priority < result.Roadmap[i-1].Priority {
			t.Error("roadmap items should be sorted by priority")
		}
	}

	// Each item should have required fields
	for _, item := range result.Roadmap {
		if item.Action == "" || item.Dimension == "" || item.Effort == "" {
			t.Error("roadmap item should have action, dimension, and effort")
		}
	}
}

func TestPlatformMaturityRecommendations(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/docs/platform-maturity", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handlePlatformMaturity(w, req)

	var result MaturityResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("should generate recommendations")
	}

	// First recommendation should mention overall maturity
	if len(result.Recommendations) > 0 {
		found := false
		for _, rec := range result.Recommendations {
			if rec != "" && (containsStr(rec, "maturity") || containsStr(rec, "Overall") || containsStr(rec, "platform")) {
				found = true
				break
			}
		}
		if !found {
			t.Error("should have a recommendation mentioning overall maturity")
		}
	}
}

func TestScoreToMaturityLevel(t *testing.T) {
	tests := []struct {
		score int
		level string
	}{
		{0, "initial"}, {24, "initial"},
		{25, "managed"}, {49, "managed"},
		{50, "defined"}, {74, "defined"},
		{75, "measured"}, {89, "measured"},
		{90, "optimizing"}, {100, "optimizing"},
	}
	for _, tc := range tests {
		got := scoreToMaturityLevel(tc.score)
		if got != tc.level {
			t.Errorf("scoreToMaturityLevel(%d) = %s, want %s", tc.score, got, tc.level)
		}
	}
}
