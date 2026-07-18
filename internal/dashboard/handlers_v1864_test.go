package dashboard

import "testing"

func TestRestartStormRecs(t *testing.T) {
	r := &PodStormResult{Summary: PodRestartStormSummary{TotalPods: 50, RestartingPods: 20, WorkloadsAffected: 8}, StormScore: 60}
	if r.StormScore != 60 {
		t.Error("score mismatch")
	}
}

func TestPipelineAuditRecs(t *testing.T) {
	r := &DeployPipelineAuditResult{Summary: PipelineAuditSummary{TotalWorkloads: 30, FreshImages: 10, StaleImages: 20}, PipelineScore: 35}
	if r.PipelineScore != 35 {
		t.Error("score mismatch")
	}
}

func TestScorecardDeepRecs(t *testing.T) {
	r := &PlatformScorecardDeepResult{OverallScore: 55, Grade: "C", TrendDirection: "needs-work"}
	if r.OverallScore != 55 {
		t.Error("score mismatch")
	}
}

func TestStormEntryTypes(t *testing.T) {
	e := StormEntry{Workload: "api", Restarts: 50, Severity: "critical"}
	if e.Severity != "critical" {
		t.Error("should be critical")
	}
}

func TestPipelineEntryTypes(t *testing.T) {
	e := PipelineEntry{Workload: "web", PipelineReady: false, Gaps: []string{"missing-probe"}}
	if e.PipelineReady {
		t.Error("should not be ready")
	}
}

func TestScorecardStatus(t *testing.T) {
	if scorecardStatus(85) != "strong" {
		t.Error("85 should be strong")
	}
	if scorecardStatus(65) != "adequate" {
		t.Error("65 should be adequate")
	}
	if scorecardStatus(45) != "weak" {
		t.Error("45 should be weak")
	}
	if scorecardStatus(20) != "critical" {
		t.Error("20 should be critical")
	}
}
