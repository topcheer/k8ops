package dashboard

import (
	"testing"
)

func TestFragBinPackingScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  FragSummary
		minScore int
		maxScore int
	}{
		{
			name: "high efficiency no fragmentation",
			summary: FragSummary{
				SchedulableNodes: 10,
				AvgCPUEfficiency: 80,
				AvgMemEfficiency: 75,
				FragmentedNodes:  0,
			},
			minScore: 70,
			maxScore: 100,
		},
		{
			name: "low efficiency high fragmentation",
			summary: FragSummary{
				SchedulableNodes: 10,
				AvgCPUEfficiency: 20,
				AvgMemEfficiency: 25,
				FragmentedNodes:  5,
			},
			minScore: 0,
			maxScore: 30,
		},
		{
			name: "no nodes",
			summary: FragSummary{
				SchedulableNodes: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := fragBinPackingScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestFragFragmentationScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  FragSummary
		minScore int
		maxScore int
	}{
		{
			name: "no fragmentation",
			summary: FragSummary{
				SchedulableNodes: 10,
				FragmentedNodes:  0,
				StrandedCPUMilli: 0,
			},
			minScore: 90,
			maxScore: 100,
		},
		{
			name: "high fragmentation",
			summary: FragSummary{
				SchedulableNodes: 10,
				FragmentedNodes:  8,
				StrandedCPUMilli: 5000,
				StrandedMemMi:    10000,
				AvgCPUEfficiency: 50,
			},
			minScore: 0,
			maxScore: 30,
		},
		{
			name: "no schedulable nodes",
			summary: FragSummary{
				SchedulableNodes: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := fragFragmentationScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestComputeFragScore(t *testing.T) {
	tests := []struct {
		name           string
		availCPU       int64
		availMem       int64
		availPods      int64
		allocCPU       int64
		allocMem       int64
		maxPods        int64
		wantRange      [2]int // [min, max]
		wantFragmented bool
	}{
		{
			name:     "balanced available",
			availCPU: 4000, availMem: 8000, availPods: 50,
			allocCPU: 8000, allocMem: 32000, maxPods: 110,
			wantRange: [2]int{0, 20},
		},
		{
			name:     "no pods but resources available",
			availCPU: 4000, availMem: 8000, availPods: 0,
			allocCPU: 8000, allocMem: 32000, maxPods: 110,
			wantRange: [2]int{40, 60},
		},
		{
			name:     "CPU available but no memory",
			availCPU: 4000, availMem: 100, availPods: 50,
			allocCPU: 8000, allocMem: 32000, maxPods: 110,
			wantRange: [2]int{10, 40},
		},
		{
			name:     "memory available but no CPU",
			availCPU: 50, availMem: 16000, availPods: 50,
			allocCPU: 8000, allocMem: 32000, maxPods: 110,
			wantRange: [2]int{10, 40},
		},
		{
			name:     "zero allocatable",
			availCPU: 0, availMem: 0, availPods: 0,
			allocCPU: 0, allocMem: 0, maxPods: 0,
			wantRange: [2]int{0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := computeFragScore(tt.availCPU, tt.availMem, tt.availPods, tt.allocCPU, tt.allocMem, tt.maxPods)
			if score < tt.wantRange[0] || score > tt.wantRange[1] {
				t.Errorf("fragScore = %d, want [%d, %d]", score, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

func TestFragRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &FragResult{
			Summary: FragSummary{
				SchedulableNodes:   10,
				FragmentedNodes:    0,
				StrandedCPUMilli:   0,
				BinPackingScore:    75,
				AvgCPUEfficiency:   70,
				AvgMemEfficiency:   65,
				FragmentationScore: 85,
			},
		}
		recs := fragRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with issues", func(t *testing.T) {
		r := &FragResult{
			Summary: FragSummary{
				SchedulableNodes:   10,
				FragmentedNodes:    5,
				StrandedCPUMilli:   3000,
				StrandedMemMi:      6000,
				BinPackingScore:    35,
				FragmentationScore: 30,
				AvgCPUEfficiency:   40,
				AvgMemEfficiency:   35,
				AvgPodSlotUsage:    85,
			},
		}
		recs := fragRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
