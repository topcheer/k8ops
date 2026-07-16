package dashboard

import (
	"testing"
)

func TestIdleWasteTypes(t *testing.T) {
	r := IdleWasteResult{WasteScore: 85, Grade: "B"}
	if r.WasteScore != 85 || r.Grade != "B" {
		t.Error("struct field error")
	}

	s := IdleWasteSummary{TotalWorkloads: 48, IdleWorkloadCount: 3, UnusedPVs: 2, UnusedLBs: 1}
	if s.IdleWorkloadCount != 3 || s.UnusedPVs != 2 {
		t.Error("summary field error")
	}

	iw := IdleWorkload{Name: "api", Namespace: "app", MonthlyCost: 45.5}
	if iw.MonthlyCost != 45.5 {
		t.Error("idleWorkload field error")
	}

	uv := UnusedVolume{Name: "data", Size: "20Gi", MonthlyCost: 2.0}
	if uv.Size != "20Gi" {
		t.Error("unusedVolume field error")
	}

	us := UnusedService{Name: "lb-svc", Type: "LoadBalancer", MonthlyCost: 18.0}
	if us.Type != "LoadBalancer" {
		t.Error("unusedService field error")
	}

	ce := CostEstimate{IdleWorkloads: 50, UnusedVolumes: 5, UnusedServices: 18, TotalMonthly: 73}
	if ce.TotalMonthly != 73 {
		t.Error("costEstimate field error")
	}
}

func TestParseStorageGBStr(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"10Gi", 10},
		{"50Gi", 50},
		{"100G", 100},
		{"0Gi", 0},
	}
	for _, tc := range tests {
		got := parseStorageGBStr(tc.input)
		if got != tc.expected {
			t.Errorf("parseStorageGBStr(%q) = %.1f, expected %.1f", tc.input, got, tc.expected)
		}
	}
}

func TestEstimateResourceCost(t *testing.T) {
	// 1 core CPU, 1Gi memory
	cost := estimateResourceCost(1000, 1024)
	// 1*25 + 1*4 = 29
	if cost < 28 || cost > 30 {
		t.Errorf("estimateResourceCost(1000m, 1024Mi) = %.2f, expected ~29", cost)
	}

	// 0.5 core CPU, 512Mi memory
	cost = estimateResourceCost(500, 512)
	// 0.5*25 + 0.5*4 = 14.5
	if cost < 13 || cost > 16 {
		t.Errorf("estimateResourceCost(500m, 512Mi) = %.2f, expected ~14.5", cost)
	}
}
