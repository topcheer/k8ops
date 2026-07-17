package dashboard

import (
	"testing"
)

func TestFixPlanDesc(t *testing.T) {
	if d := fixPlanDesc("PSS"); d == "PSS" {
		t.Error("expected description for PSS")
	}
	if d := fixPlanDesc("Unknown"); d != "Unknown" {
		t.Errorf("expected 'Unknown', got %s", d)
	}
}

func TestFixPlanImpact(t *testing.T) {
	if i := fixPlanImpact("PSS"); i == "" {
		t.Error("expected impact for PSS")
	}
}

func TestGroupFixPlans(t *testing.T) {
	actions := []SecFixAction{
		{Category: "PSS", Severity: "critical", AutoFixable: true},
		{Category: "PSS", Severity: "high", AutoFixable: true},
		{Category: "Network", Severity: "high", AutoFixable: false},
	}
	plans := groupFixPlans(actions)
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	// PSS should be critical priority (has critical severity)
	for _, p := range plans {
		if p.Category == "PSS" && p.Priority != "critical" {
			t.Errorf("expected PSS priority critical, got %s", p.Priority)
		}
	}
}

func TestExtractDimFromPath(t *testing.T) {
	if d := extractDimFromPath("/api/security/fix-plan"); d != "Security" {
		t.Errorf("expected Security, got %s", d)
	}
	if d := extractDimFromPath("/api/operations/chaos"); d != "Operations" {
		t.Errorf("expected Operations, got %s", d)
	}
}

func TestBuildGateRecs(t *testing.T) {
	// Fail verdict with blockers
	r := &ReleaseGateResult{
		OverallVerdict: "fail",
		Blockers: []ReleaseBlocker{
			{Severity: "critical", Action: "test blocker"},
		},
		ByCategory: []GateCategory{
			{Category: "Availability", Passed: 0, Total: 2},
		},
	}
	recs := buildGateRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}

	// Pass verdict
	r2 := &ReleaseGateResult{OverallVerdict: "pass"}
	recs2 := buildGateRecs(r2)
	if len(recs2) == 0 {
		t.Error("expected at least 1 rec for pass")
	}
}
