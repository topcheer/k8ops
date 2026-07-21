package dashboard

import "testing"

func TestComplianceReportResult1909(t *testing.T) {
	r := ComplianceReportResult{
		Summary:     ComplianceSummary{TotalChecks: 9, PassedChecks: 5, FailedChecks: 3, CISScore: 66},
		HealthScore: 55,
	}
	if r.Summary.FailedChecks != 3 {
		t.Errorf("expected 3, got %d", r.Summary.FailedChecks)
	}
}

func TestSLOHandbookResult1909(t *testing.T) {
	r := SLOHandbookResult{
		Summary:     SLOHandbookSummary{TotalServices: 100, AvgAvailability: 99.5, TotalErrorBudget: 7},
		HealthScore: 99,
	}
	if r.Summary.AvgAvailability != 99.5 {
		t.Errorf("expected 99.5, got %f", r.Summary.AvgAvailability)
	}
}

func TestClusterFAQResult1909(t *testing.T) {
	r := ClusterFAQResult{
		Summary:     FAQSummary{TotalFAQs: 10, ClusterVersion: "v1.28", NodeCount: 1, NamespaceCount: 4},
		HealthScore: 100,
	}
	if r.Summary.NodeCount != 1 {
		t.Errorf("expected 1, got %d", r.Summary.NodeCount)
	}
}

func TestBuildComplianceRecs1909(t *testing.T) {
	r := &ComplianceReportResult{
		Summary:      ComplianceSummary{TotalChecks: 9, PassedChecks: 6, FailedChecks: 3, CISScore: 66, PCIScore: 66, SOC2Score: 66},
		FailedChecks: []ComplianceCheck1909{{Severity: "high"}, {Severity: "medium"}},
	}
	recs := buildComplianceRecs1909(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildSLOHandbookRecs1909(t *testing.T) {
	r := &SLOHandbookResult{
		Summary: SLOHandbookSummary{TotalServices: 100, AvgAvailability: 99.5, TotalErrorBudget: 7},
		SLOs:    []SLOEntry{{RiskLevel: "critical"}, {RiskLevel: "low"}},
	}
	recs := buildSLOHandbookRecs1909(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildFAQRecs1909(t *testing.T) {
	r := &ClusterFAQResult{Summary: FAQSummary{TotalFAQs: 10, CommonIssues: 4}}
	recs := buildFAQRecs1909(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestFrameworkStatus1909(t *testing.T) {
	if frameworkStatus(95) != "compliant" {
		t.Error("expected compliant")
	}
	if frameworkStatus(75) != "partial" {
		t.Error("expected partial")
	}
	if frameworkStatus(50) != "non-compliant" {
		t.Error("expected non-compliant")
	}
}
