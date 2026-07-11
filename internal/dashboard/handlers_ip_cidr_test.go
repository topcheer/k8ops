package dashboard

import "testing"

func TestCidrCapacity(t *testing.T) {
	tests := []struct {
		cidr string
		want int64
	}{
		{"10.244.0.0/24", 256},
		{"10.244.0.0/16", 65536},
		{"10.244.0.0/30", 4},
		{"192.168.1.0/28", 16},
		{"fd00::/108", int64(1) << 20},
	}
	for _, tt := range tests {
		got, err := cidrCapacity(tt.cidr)
		if err != nil {
			t.Errorf("cidrCapacity(%q) error: %v", tt.cidr, err)
			continue
		}
		if got != tt.want {
			t.Errorf("cidrCapacity(%q) = %d, want %d", tt.cidr, got, tt.want)
		}
	}
}

func TestIPCidrScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  IPCIDRSummary
		minScore int
		maxScore int
	}{
		{"low utilization", IPCIDRSummary{TotalPodCIDRCap: 1000, OverallUtilPct: 30}, 90, 100},
		{"high utilization", IPCIDRSummary{TotalPodCIDRCap: 1000, OverallUtilPct: 85, NodesNearFull: 3}, 50, 75},
		{"full nodes", IPCIDRSummary{TotalPodCIDRCap: 1000, OverallUtilPct: 95, NodesFull: 2, NodesNearFull: 5}, 0, 40},
		{"no capacity", IPCIDRSummary{TotalPodCIDRCap: 0}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ipCIDRScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestIPCidrRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &IPCIDRResult{Summary: IPCIDRSummary{TotalPodCIDRCap: 1000, OverallUtilPct: 20}}
		recs := ipCIDRRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &IPCIDRResult{Summary: IPCIDRSummary{
			TotalPodCIDRCap: 1000, OverallUtilPct: 85,
			NodesFull: 1, NodesNearFull: 3,
			NodesWithPodCIDR: 8, TotalNodes: 10,
		}}
		recs := ipCIDRRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
