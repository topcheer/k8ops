package dashboard

import (
	"testing"
)

func TestSATokenLifecycleResultStruct1867(t *testing.T) {
	r := SATokenLifecycleResult{
		Summary: SATokenLifecycleSummary{
			TotalSAs: 5, WithTokens: 2, LongLivedTokens: 1, UnusedSAs: 3,
		},
		RiskScore: 60,
	}
	if r.Summary.TotalSAs != 5 {
		t.Errorf("expected 5, got %d", r.Summary.TotalSAs)
	}
	if len(r.RiskyTokens) != 0 {
		t.Errorf("expected 0 risky, got %d", len(r.RiskyTokens))
	}
}

func TestEndpointHealthDeepResultStruct1867(t *testing.T) {
	r := EndpointHealthDeepResult{
		Summary: EndpointHealthSummary{
			TotalServices: 10, HealthySvcs: 8, DegradedSvcs: 2,
		},
		HealthScore: 80,
	}
	if r.Summary.HealthySvcs != 8 {
		t.Errorf("expected 8, got %d", r.Summary.HealthySvcs)
	}
	if r.HealthScore != 80 {
		t.Errorf("expected 80, got %d", r.HealthScore)
	}
}

func TestOvercommitRiskResultStruct1867(t *testing.T) {
	r := OvercommitRiskResult{
		Summary: OvercommitRiskSummary{
			TotalCPUAlloc: 4.0, CPURequested: 2.0, CPULimited: 10.0,
			CPUOvercommitPct: 250,
		},
		RiskScore: 50,
	}
	if r.Summary.CPUOvercommitPct != 250 {
		t.Errorf("expected 250, got %.0f", r.Summary.CPUOvercommitPct)
	}
	if r.RiskScore != 50 {
		t.Errorf("expected 50, got %d", r.RiskScore)
	}
}
