package dashboard

import (
	"testing"
)

func TestTechDebtRadarResultStruct1878(t *testing.T) {
	r := TechDebtRadarResult{
		Summary:        TechDebtSummary{TotalItems: 6, CriticalDebt: 1, HighDebt: 3},
		TotalDebtScore: 35,
		HealthScore:    65,
	}
	if r.Summary.CriticalDebt != 1 {
		t.Errorf("expected 1, got %d", r.Summary.CriticalDebt)
	}
}

func TestSREScorecardResultStruct1878(t *testing.T) {
	r := SREScorecardResult{
		Summary:     SREScorecardSummary{TotalServices: 85, ErrorBudgetUsed: 15.5},
		HealthScore: 75,
	}
	if r.Summary.ErrorBudgetUsed != 15.5 {
		t.Errorf("expected 15.5, got %.1f", r.Summary.ErrorBudgetUsed)
	}
}

func TestComplianceCrosswalkResultStruct1878(t *testing.T) {
	r := ComplianceCrosswalkResult{
		Summary:     CrosswalkSummary{TotalChecks: 6, PassedChecks: 2, FailedChecks: 4},
		HealthScore: 33,
	}
	if r.Summary.FailedChecks != 4 {
		t.Errorf("expected 4, got %d", r.Summary.FailedChecks)
	}
}

func TestStatusFromScore(t *testing.T) {
	if statusFromScore(95) != "healthy" {
		t.Error("expected healthy")
	}
	if statusFromScore(75) != "warning" {
		t.Error("expected warning")
	}
	if statusFromScore(50) != "critical" {
		t.Error("expected critical")
	}
}

func TestSevFromCount(t *testing.T) {
	if sevFromCount(0, 10) != "low" {
		t.Error("expected low")
	}
	if sevFromCount(5, 10) != "medium" {
		t.Error("expected medium")
	}
	if sevFromCount(15, 10) != "high" {
		t.Error("expected high")
	}
	if sevFromCount(25, 10) != "critical" {
		t.Error("expected critical")
	}
}
