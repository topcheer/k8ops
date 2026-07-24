package dashboard

import "testing"

func TestAntiAffGapResult1959(t *testing.T) {
	r := AntiAffGapResult1959{Summary: AntiAffGapSummary1959{TotalMultiReplica: 10, WithAntiAffinity: 7, WithoutAntiAff: 3}}
	if r.Summary.TotalMultiReplica != 10 {
		t.Errorf("expected 10")
	}
	if r.Summary.WithoutAntiAff != 3 {
		t.Errorf("expected 3")
	}
}
func TestAntiAffGapEntry1959(t *testing.T) {
	e := AntiAffGapEntry1959{Name: "api-server", Namespace: "prod", Kind: "Deployment", Replicas: 3, HasPodAnti: false, RiskLevel: "high"}
	if e.RiskLevel != "high" {
		t.Errorf("expected high")
	}
	if e.HasPodAnti {
		t.Errorf("expected false")
	}
}
func TestCmdAuditResult1959(t *testing.T) {
	r := CmdAuditResult1959{Summary: CmdAuditSummary1959{TotalContainers: 50, ShellEntrypoint: 5, PrivilegedCmd: 2}}
	if r.Summary.ShellEntrypoint != 5 {
		t.Errorf("expected 5")
	}
	if r.Summary.PrivilegedCmd != 2 {
		t.Errorf("expected 2")
	}
}
func TestCmdAuditIssue1959(t *testing.T) {
	e := CmdAuditIssue1959{Container: "app", Pod: "app-xyz", IssueType: "privileged", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
	if e.IssueType != "privileged" {
		t.Errorf("expected privileged")
	}
}
func TestCmdAuditNSEntry1959(t *testing.T) {
	e := CmdAuditNSEntry1959{Namespace: "default", Containers: 15, Issues: 3}
	if e.Issues != 3 {
		t.Errorf("expected 3")
	}
}
func TestAnnotSignalResult1959(t *testing.T) {
	r := AnnotSignalResult1959{Summary: AnnotSignalSummary1959{TotalWorkloads: 20, WithOwner: 15, MissingCritical: 5}}
	if r.Summary.WithOwner != 15 {
		t.Errorf("expected 15")
	}
	if r.Summary.MissingCritical != 5 {
		t.Errorf("expected 5")
	}
}
func TestAnnotSignalCoverage1959(t *testing.T) {
	c := AnnotSignalCoverage1959{OwnerPct: 75.0, ManagedByPct: 60.0, ReloaderPct: 30.0}
	if c.OwnerPct != 75.0 {
		t.Errorf("expected 75.0")
	}
	if c.ReloaderPct >= c.ManagedByPct {
		t.Errorf("expected reloader < managed-by")
	}
}
func TestAnnotSignalEntry1959(t *testing.T) {
	e := AnnotSignalEntry1959{Name: "web", Namespace: "default", Kind: "Deployment", HasOwner: true, HasManagedBy: true, Score: 40}
	if e.Score != 40 {
		t.Errorf("expected 40")
	}
	if !e.HasOwner {
		t.Errorf("expected true")
	}
}
