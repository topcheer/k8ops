package dashboard

import (
	"strings"
	"testing"
)

func TestNodeTopoScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  NodeTopoSummary
		minScore int
		maxScore int
	}{
		{"multi-zone balanced", NodeTopoSummary{TotalNodes: 10, TotalZones: 3}, 90, 100},
		{"single zone", NodeTopoSummary{TotalNodes: 10, SingleZoneCluster: true}, 55, 65},
		{"nodes without zone", NodeTopoSummary{TotalNodes: 10, NodesWithoutZone: 3}, 80, 90},
		{"high imbalance", NodeTopoSummary{TotalNodes: 10, TotalZones: 2, MaxZoneImbalance: 60}, 45, 85},
		{"no nodes", NodeTopoSummary{}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := nodeTopoScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestNodeTopoRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &NodeTopoResult{Summary: NodeTopoSummary{TotalNodes: 10, TotalZones: 3}, ByZone: []ZoneStat{{Zone: "a", NodeCount: 4}, {Zone: "b", NodeCount: 3}, {Zone: "c", NodeCount: 3}}}
		recs := nodeTopoRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("single zone", func(t *testing.T) {
		r := &NodeTopoResult{Summary: NodeTopoSummary{TotalNodes: 5, SingleZoneCluster: true}}
		recs := nodeTopoRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected recommendation for single zone")
		}
	})
	t.Run("no zone labels", func(t *testing.T) {
		r := &NodeTopoResult{Summary: NodeTopoSummary{TotalNodes: 5, NodesWithoutZone: 3, TotalZones: 2}}
		recs := nodeTopoRecommendations(r)
		found := false
		for _, rec := range recs {
			if strings.Contains(rec, "zone labels") {
				found = true
			}
		}
		if !found {
			t.Error("expected recommendation about missing zone labels")
		}
	})
}
