package dashboard

import (
	"encoding/json"
	"testing"
)

func TestParseSLOTarget(t *testing.T) {
	tests := []struct {
		input    string
		expected SLOTarget
	}{
		{"99.9", SLO99_9},
		{"99.9%", SLO99_9},
		{"99.5", SLO99_5},
		{"99", SLO99_0},
		{"99.0", SLO99_0},
		{"95", SLO95_0},
		{"unknown", SLO99_9}, // default
		{"", SLO99_9},        // empty -> default
	}

	for _, tt := range tests {
		got := parseSLOTarget(tt.input)
		if got != tt.expected {
			t.Errorf("parseSLOTarget(%q) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestSLOTargetToFloat(t *testing.T) {
	tests := []struct {
		target   SLOTarget
		expected float64
	}{
		{SLO99_9, 99.9},
		{SLO99_5, 99.5},
		{SLO99_0, 99.0},
		{SLO95_0, 95.0},
	}

	for _, tt := range tests {
		got := sloTargetToFloat(tt.target)
		if got != tt.expected {
			t.Errorf("sloTargetToFloat(%s) = %.1f, want %.1f", tt.target, got, tt.expected)
		}
	}
}

func TestRoundTo(t *testing.T) {
	if got := roundTo(3.14159, 2); got != 3.14 {
		t.Errorf("roundTo(3.14159, 2) = %f, want 3.14", got)
	}
	if got := roundTo(3.146, 2); got != 3.15 {
		t.Errorf("roundTo(3.146, 2) = %f, want 3.15", got)
	}
	if got := roundTo(100.0, 1); got != 100.0 {
		t.Errorf("roundTo(100.0, 1) = %f, want 100.0", got)
	}
}

func TestComputeBurnRate_Healthy(t *testing.T) {
	// 99.9% target, 99.99% availability (well under budget)
	// error rate 0.01% / budget 0.1% = 10% consumed
	burn := computeBurnRate(99.99, SLO99_9, 10000, 1)

	if burn.ConsumedPercent >= 50 {
		t.Errorf("expected low consumption for healthy SLO, got %.1f%%", burn.ConsumedPercent)
	}
	if burn.BudgetMinutes <= 0 {
		t.Errorf("expected positive budget minutes, got %.1f", burn.BudgetMinutes)
	}
}

func TestComputeBurnRate_Violated(t *testing.T) {
	// 99.9% target, 99.0% availability (over budget)
	burn := computeBurnRate(99.0, SLO99_9, 10000, 100)

	if burn.ConsumedPercent < 100 {
		t.Errorf("expected >100%% consumption for violated SLO, got %.1f%%", burn.ConsumedPercent)
	}
}

func TestComputeBurnRate_BudgetMinutes(t *testing.T) {
	// 99.9% = 0.1% error budget = 43200 * 0.001 = 43.2 min/month
	burn := computeBurnRate(100.0, SLO99_9, 0, 0)

	if burn.BudgetMinutes < 43.0 || burn.BudgetMinutes > 43.5 {
		t.Errorf("expected ~43.2 min budget for 99.9%%, got %.1f", burn.BudgetMinutes)
	}

	// 99.0% = 1.0% error budget = 43200 * 0.01 = 432 min/month
	burn95 := computeBurnRate(100.0, SLO99_0, 0, 0)
	if burn95.BudgetMinutes < 431.0 || burn95.BudgetMinutes > 433.0 {
		t.Errorf("expected ~432 min budget for 99.0%%, got %.1f", burn95.BudgetMinutes)
	}
}

func TestComputeSLOWindows(t *testing.T) {
	windows := computeSLOWindows(10000, 50, SLO99_9)

	if len(windows) != 4 {
		t.Fatalf("expected 4 windows, got %d", len(windows))
	}

	// Check window names
	names := []string{"5m", "1h", "6h", "24h"}
	for i, w := range windows {
		if w.Window != names[i] {
			t.Errorf("window[%d] = %s, want %s", i, w.Window, names[i])
		}
	}

	// 24h window should have all requests
	if windows[3].Requests != 10000 {
		t.Errorf("24h window should have 10000 requests, got %d", windows[3].Requests)
	}
}

func TestComputeSLOWindows_StatusExhausted(t *testing.T) {
	// 50% error rate with 99.9% target -> definitely exhausted
	windows := computeSLOWindows(1000, 500, SLO99_9)

	found := false
	for _, w := range windows {
		if w.Status == "exhausted" || w.Status == "critical" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected exhausted or critical status for 50% error rate")
	}
}

func TestComputeSLOWindows_StatusHealthy(t *testing.T) {
	// 0% error rate -> all healthy
	windows := computeSLOWindows(10000, 0, SLO99_9)

	for _, w := range windows {
		if w.Status != "healthy" {
			t.Errorf("expected healthy for 0 errors, window %s = %s", w.Window, w.Status)
		}
		if w.Availability != 100.0 {
			t.Errorf("expected 100%% availability, got %.1f", w.Availability)
		}
	}
}

func TestSLOResult_JSON(t *testing.T) {
	result := SLOResult{
		Target:        SLO99_9,
		Availability:  99.95,
		TotalRequests: 10000,
		ErrorRequests: 5,
		ErrorRate:     0.05,
		Windows: []SLOWindow{
			{Window: "24h", Requests: 10000, Errors: 5, Availability: 99.95, BudgetLeft: 50.0, Status: "warning"},
		},
		ByEndpoint: []SLOEndpoint{
			{Endpoint: "GET /api/health", Requests: 500, Errors: 0, P99Ms: 12.5},
		},
		LatencySLO: LatencySLO{
			Target:      "p99 < 500ms",
			P99Ms:       45.3,
			BreachCount: 0,
		},
		BurnRate: BurnRateInfo{
			BudgetMinutes:   43.2,
			ConsumedPercent: 50.0,
			BurnRate1h:      0.5,
		},
		Verdict: "warning",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded SLOResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Target != SLO99_9 {
		t.Errorf("expected 99.9%% target, got %s", decoded.Target)
	}
	if decoded.Availability != 99.95 {
		t.Errorf("expected 99.95 availability, got %f", decoded.Availability)
	}
	if len(decoded.Windows) != 1 {
		t.Errorf("expected 1 window, got %d", len(decoded.Windows))
	}
	if decoded.Verdict != "warning" {
		t.Errorf("expected warning verdict, got %s", decoded.Verdict)
	}
}

func TestSLOWindowStatusRank(t *testing.T) {
	if sloWindowStatusRank("exhausted") >= sloWindowStatusRank("critical") {
		t.Error("exhausted should rank before critical")
	}
	if sloWindowStatusRank("critical") >= sloWindowStatusRank("warning") {
		t.Error("critical should rank before warning")
	}
	if sloWindowStatusRank("warning") >= sloWindowStatusRank("healthy") {
		t.Error("warning should rank before healthy")
	}
}

func TestBurnRateInfo_AlertThreshold(t *testing.T) {
	burn := computeBurnRate(99.9, SLO99_9, 1000, 1)
	// Alert threshold should be the SRE recommended 14.4x
	if burn.AlertThreshold != 14.4 {
		t.Errorf("expected alert threshold 14.4, got %.1f", burn.AlertThreshold)
	}
}

func TestComputeBurnRate_DifferentTargets(t *testing.T) {
	// 99.9% target gets less budget than 99.0%
	burn999 := computeBurnRate(100.0, SLO99_9, 0, 0)
	burn990 := computeBurnRate(100.0, SLO99_0, 0, 0)

	if burn999.BudgetMinutes >= burn990.BudgetMinutes {
		t.Errorf("99.9%% should have less budget (%.1f) than 99.0%% (%.1f)", burn999.BudgetMinutes, burn990.BudgetMinutes)
	}
}
