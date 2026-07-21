package dashboard

import "testing"

func TestCapAuditResult1908(t *testing.T) {
	r := CapAuditResult{
		Summary:     CapAuditSummary1908{TotalContainers: 72, Privileged: 2, HighRiskCaps: 8, WithAllDropped: 5},
		HealthScore: 86,
	}
	if r.Summary.Privileged != 2 {
		t.Errorf("expected 2, got %d", r.Summary.Privileged)
	}
}

func TestHostNSAuditResult1908(t *testing.T) {
	r := HostNSAuditResult{
		Summary:     HostNSAuditSummary{TotalWorkloads: 67, HostPID: 1, HostNetwork: 3, HostPathMounts: 5, HostPathUnrestricted: 3},
		HealthScore: 90,
	}
	if r.Summary.HostNetwork != 3 {
		t.Errorf("expected 3, got %d", r.Summary.HostNetwork)
	}
}

func TestPSSComplianceResult1908(t *testing.T) {
	r := PSSComplianceResult{
		Summary:     PSSComplianceSummary{TotalWorkloads: 67, PassBaseline: 40, FailBaseline: 27, PassRestricted: 10, FailRestricted: 57},
		HealthScore: 37,
	}
	if r.Summary.FailBaseline != 27 {
		t.Errorf("expected 27, got %d", r.Summary.FailBaseline)
	}
}

func TestBuildCapAuditRecs1908(t *testing.T) {
	r := &CapAuditResult{Summary: CapAuditSummary1908{TotalContainers: 72, Privileged: 2, WithCapAdd: 5, HighRiskCaps: 8, WithAllDropped: 5}}
	recs := buildCapAuditRecs1908(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildHostNSRecs1908(t *testing.T) {
	r := &HostNSAuditResult{Summary: HostNSAuditSummary{TotalWorkloads: 67, HostPID: 1, HostNetwork: 3, HostIPC: 0, HostPathMounts: 5, HostPathUnrestricted: 3}}
	recs := buildHostNSRecs1908(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildPSSRecs1908(t *testing.T) {
	r := &PSSComplianceResult{Summary: PSSComplianceSummary{TotalWorkloads: 67, PassBaseline: 40, FailBaseline: 27, PassRestricted: 10, FailRestricted: 57, RunAsRoot: 45}}
	recs := buildPSSRecs1908(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
