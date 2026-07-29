package dashboard

import "testing"

func TestSecDataVolResult2052(t *testing.T) {
	r := SecDataVolResult2052{Summary: SecDataVolSummary2052{TotalSecrets: 50, VolumeSecrets: 30, EnvSecrets: 20}}
	if r.Summary.EnvSecrets != 20 {
		t.Errorf("expected 20")
	}
}
func TestDefaultDenyResult2052(t *testing.T) {
	r := DefaultDenyResult2052{Summary: DefaultDenySummary2052{TotalNS: 10, WithDefaultDeny: 3, Unprotected: 7}}
	if r.Summary.Unprotected != 7 {
		t.Errorf("expected 7")
	}
}
func TestWebhookRiskResult2052(t *testing.T) {
	r := WebhookRiskResult2052{Summary: WebhookRiskSummary2052{TotalWebhooks: 15, MutatingCount: 5, ValidatingCount: 10, FailPolicyCount: 3}}
	if r.Summary.FailPolicyCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestWebhookRiskEntry2052(t *testing.T) {
	e := WebhookRiskEntry2052{Name: "webhook.example.com", Type: "mutating", FailurePolicy: "Fail"}
	if e.FailurePolicy != "Fail" {
		t.Errorf("expected Fail")
	}
}
