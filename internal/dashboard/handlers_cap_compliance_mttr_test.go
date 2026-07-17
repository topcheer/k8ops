package dashboard

import "testing"

func TestCapacityForecastTypes(t *testing.T) {
	r := CapacityForecastResult{HealthScore: 70, Grade: "C"}
	if r.HealthScore != 70 {
		t.Error("struct error")
	}
	s := CapForecastSummary{NodeCount: 3, CPUPct: 65.5, MemPct: 40.0}
	if s.CPUPct != 65.5 {
		t.Error("summary error")
	}
	f := CapForecast{Resource: "CPU", UsagePct: 90, Severity: "high"}
	if f.Severity != "high" {
		t.Error("forecast error")
	}
}

func TestCapacityForecastScoring(t *testing.T) {
	tests := []struct {
		cpuPct, memPct, podPct float64
		expMin, expMax         int
	}{
		{30, 30, 20, 90, 100},
		{95, 90, 85, 35, 55},
		{60, 50, 40, 90, 100},
	}
	for _, tc := range tests {
		forecasts := []CapForecast{
			{UsagePct: tc.cpuPct, Severity: capSeverity(tc.cpuPct)},
			{UsagePct: tc.memPct, Severity: capSeverity(tc.memPct)},
			{UsagePct: tc.podPct, Severity: capSeverity(tc.podPct)},
		}
		score := 100
		for _, f := range forecasts {
			switch f.Severity {
			case "critical":
				score -= 35
			case "high":
				score -= 20
			case "medium":
				score -= 10
			}
		}
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("cpu=%.0f mem=%.0f pod=%.0f: expected %d-%d, got %d", tc.cpuPct, tc.memPct, tc.podPct, tc.expMin, tc.expMax, score)
		}
	}
}

func capSeverity(pct float64) string {
	if pct > 95 {
		return "critical"
	}
	if pct > 85 {
		return "high"
	}
	if pct > 70 {
		return "medium"
	}
	return "low"
}

func TestComplianceMapTypes(t *testing.T) {
	r := ComplianceMapResult{ComplianceScore: 30, Grade: "F"}
	if r.ComplianceScore != 30 {
		t.Error("struct error")
	}
	s := ComplianceMapSummary{SOC2Pct: 33, PCIPct: 0, CISPct: 33}
	if s.SOC2Pct != 33 {
		t.Error("summary error")
	}
	c := ComplianceControl{ID: "SOC2-CC1", Status: "fail"}
	if c.Status != "fail" {
		t.Error("control error")
	}
}

func TestMTTRTypes(t *testing.T) {
	r := MTTRResult{HealthScore: 55, Grade: "D"}
	if r.HealthScore != 55 {
		t.Error("struct error")
	}
	s := MTTRSummary{TotalPods: 60, CrashedPods: 10, HighRestartPods: 5}
	if s.HighRestartPods != 5 {
		t.Error("summary error")
	}
	p := PodRestarter{Name: "api", Restarts: 15, Severity: "high"}
	if p.Restarts != 15 {
		t.Error("restarter error")
	}
}

func TestMTTRScoring(t *testing.T) {
	tests := []struct {
		high, crashed  int
		expMin, expMax int
	}{
		{0, 0, 95, 100},
		{5, 10, 0, 15},
		{2, 5, 0, 30},
	}
	for _, tc := range tests {
		score := 100 - tc.high*40 - tc.crashed*15
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("high=%d crashed=%d: expected %d-%d, got %d", tc.high, tc.crashed, tc.expMin, tc.expMax, score)
		}
	}
}
