package dashboard

import "testing"

func TestQuotaWasteResult1960(t *testing.T) {
	r := QuotaWasteResult1960{Summary: QuotaWasteSummary1960{TotalNamespaces: 15, WithQuota: 8, WastefulNamespaces: 3, TotalWastedCPU: 12.5}}
	if r.Summary.WastefulNamespaces != 3 {
		t.Errorf("expected 3")
	}
	if r.Summary.TotalWastedCPU != 12.5 {
		t.Errorf("expected 12.5")
	}
}
func TestQuotaWasteEntry1960(t *testing.T) {
	e := QuotaWasteEntry1960{Namespace: "prod", QuotaCPU: 10, UsedCPU: 3, WastedCPU: 7, UtilizationPct: 30.0}
	if e.WastedCPU != 7 {
		t.Errorf("expected 7")
	}
	if e.UtilizationPct != 30.0 {
		t.Errorf("expected 30.0")
	}
}
func TestAdmissionHealthResult1960(t *testing.T) {
	r := AdmissionHealthResult1960{Summary: AdmissionHealthSummary1960{TotalMutatingWebhooks: 5, TotalValidatingWebhooks: 3, HealthyWebhooks: 7, MisconfiguredWebhooks: 1}}
	if r.Summary.TotalMutatingWebhooks != 5 {
		t.Errorf("expected 5")
	}
	if r.Summary.MisconfiguredWebhooks != 1 {
		t.Errorf("expected 1")
	}
}
func TestAdmissionWebhookEntry1960(t *testing.T) {
	e := AdmissionWebhookEntry1960{Name: "pod-_mutator", Kind: "MutatingWebhookConfiguration", FailurePolicy: "Fail", TimeoutSeconds: 10, HasCABundle: true}
	if e.FailurePolicy != "Fail" {
		t.Errorf("expected Fail")
	}
	if !e.HasCABundle {
		t.Errorf("expected true")
	}
}
func TestAdmissionIssue1960(t *testing.T) {
	e := AdmissionIssue1960{Name: "bad-webhook", IssueType: "missing-ca-bundle", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestClockSyncResult1960(t *testing.T) {
	r := ClockSyncResult1960{Summary: ClockSyncSummary1960{TotalNodes: 5, SyncedNodes: 4, SkewedNodes: 1, MaxSkewSeconds: 120.5}}
	if r.Summary.SkewedNodes != 1 {
		t.Errorf("expected 1")
	}
	if r.Summary.MaxSkewSeconds != 120.5 {
		t.Errorf("expected 120.5")
	}
}
func TestClockSyncNodeEntry1960(t *testing.T) {
	e := ClockSyncNodeEntry1960{Name: "node-1", SkewSeconds: 3.2, Status: "synced", HasNTPLbl: true}
	if e.Status != "synced" {
		t.Errorf("expected synced")
	}
	if !e.HasNTPLbl {
		t.Errorf("expected true")
	}
}
func TestClockSyncRisk1960(t *testing.T) {
	e := ClockSyncRisk1960{Node: "node-2", RiskType: "clock-skew", Severity: "high"}
	if e.RiskType != "clock-skew" {
		t.Errorf("expected clock-skew")
	}
}
