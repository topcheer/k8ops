package dashboard

import "testing"

func TestCrashBudgetRecs(t *testing.T) {
	r := &CrashBudgetTrackerResult{Summary: CrashBudgetSummary{TotalWorkloads: 30, CrashFreeWls: 20, TotalCrashes: 50, MonthlyBudget: 10}, BudgetScore: 66}
	if r.BudgetScore != 66 {
		t.Error("mismatch")
	}
}
func TestHelmDriftRecs(t *testing.T) {
	r := &HelmDriftMonitorResult{Summary: HelmDriftSummary{TotalReleases: 10, HealthyReleases: 8, DriftedReleases: 2}, MonitorScore: 80}
	if r.MonitorScore != 80 {
		t.Error("mismatch")
	}
}
func TestAPICoverageGapRecs(t *testing.T) {
	r := &APICoverageGapResult{Summary: APICoverageGapSummary{TotalResourceTypes: 14, ObservedTypes: 8, CriticalGaps: 3}, CoverageScore: 57}
	if r.CoverageScore != 57 {
		t.Error("mismatch")
	}
}
func TestCrashBudgetTypes(t *testing.T) {
	e := CrashBudgetEntry{Workload: "api", Restarts: 5, BudgetUsed: 50, Status: "within-budget"}
	if e.Status != "within-budget" {
		t.Error("should be within budget")
	}
}
func TestHelmDriftTypes(t *testing.T) {
	e := HelmDriftEntry{ReleaseName: "myapp", Status: "deployed", HasDrift: false, RiskLevel: "low"}
	if e.HasDrift {
		t.Error("should not have drift")
	}
}
func TestAPICoverageTypes(t *testing.T) {
	e := APICoverageEntry{ResourceType: "Secret", Count: 50, HasCoverage: false, GapLevel: "critical"}
	if e.HasCoverage {
		t.Error("Secret should not have coverage")
	}
}
