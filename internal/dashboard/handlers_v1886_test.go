package dashboard

import (
	"testing"
)

func TestSeccompProfileResultStruct1886(t *testing.T) {
	r := SeccompProfileResult{
		Summary:     SeccompSummary{TotalPods: 80, WithSeccomp: 5, NoProfile: 75},
		HealthScore: 6,
	}
	if r.Summary.NoProfile != 75 {
		t.Errorf("expected 75, got %d", r.Summary.NoProfile)
	}
}

func TestSATokenAgeResultStruct1886(t *testing.T) {
	r := SATokenAgeResult{
		Summary:     SATokenAgeSummary{TotalSAs: 20, OldSAs: 5, UnusedSAs: 8},
		HealthScore: 60,
	}
	if r.Summary.UnusedSAs != 8 {
		t.Errorf("expected 8, got %d", r.Summary.UnusedSAs)
	}
}

func TestRuntimeClassAuditResultStruct1886(t *testing.T) {
	r := RuntimeClassIsolationResult{
		Summary:     RCIsolationSummary{TotalPods: 80, WithRuntimeClass: 0, NoRuntimeClass: 80},
		HealthScore: 0,
	}
	if r.Summary.NoRuntimeClass != 80 {
		t.Errorf("expected 80, got %d", r.Summary.NoRuntimeClass)
	}
}
