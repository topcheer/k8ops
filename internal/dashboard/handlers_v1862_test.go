package dashboard

import "testing"

func TestRestartForensicsRecs(t *testing.T) {
	r := &PodRestartForensicsResult{Summary: RestartForensicsSummary{TotalPods: 50, RestartedPods: 5, TotalRestarts: 12, OOMKills: 3}, ForensicsScore: 90}
	if r.ForensicsScore != 90 {
		t.Error("score mismatch")
	}
}

func TestDeployWindowRecs(t *testing.T) {
	r := &DeployWindowOptimizerResult{Summary: DeployWindowSummary{TotalDeploys: 100, PeakHour: 14, OffHoursDeploys: 10, WeekendDeploys: 5, ChangeFreezeOK: false}, OptimizerScore: 65}
	if r.OptimizerScore != 65 {
		t.Error("score mismatch")
	}
}

func TestPlatformMaturityDeepRecs(t *testing.T) {
	r := &PlatformMaturityDeepResult{OverallScore: 50, CurrentLevel: 2, CurrentStage: "Managed", Gaps: []PlatformMaturityGap{{Dimension: "Security", Priority: "critical"}}}
	if r.CurrentLevel != 2 {
		t.Error("level mismatch")
	}
}

func TestRestartForensicTypes(t *testing.T) {
	e := RestartForensicEntry{Workload: "api", RestartCount: 15, RootCause: "oom", Pattern: "crashloop", Severity: "critical"}
	if e.Pattern != "crashloop" {
		t.Error("pattern mismatch")
	}
}

func TestDeployWindowTypes(t *testing.T) {
	w := DeployWindow{Day: "Tuesday", HourRange: "10:00-12:00", Score: 95}
	if w.Score != 95 {
		t.Error("score mismatch")
	}
}

func TestClassifyExit(t *testing.T) {
	if classifyExit("OOMKilled", 137) != "oom" {
		t.Error("OOMKilled should classify as oom")
	}
	if classifyExit("Error", 1) != "app-error" {
		t.Error("exit 1 should be app-error")
	}
	if classifyExit("", 143) != "signal" {
		t.Error("143 should be signal")
	}
	if classifyExit("", 0) != "completed" {
		t.Error("0 should be completed")
	}
}

func TestPlatformMaturityDimensionTypes(t *testing.T) {
	d := PlatformMaturityDimension{Name: "Automation", Score: 60, MaxScore: 100, Gap: "Add CI/CD"}
	if d.Score > d.MaxScore {
		t.Error("score exceeds max")
	}
}
