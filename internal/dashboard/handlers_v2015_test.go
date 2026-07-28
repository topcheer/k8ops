package dashboard

import "testing"

func TestValWebhookResult2015(t *testing.T) {
	r := ValWebhookResult2015{Summary: ValWebhookSummary2015{TotalWebhooks: 5, CatchAll: 1}}
	if r.Summary.CatchAll != 1 {
		t.Errorf("expected 1")
	}
}
func TestValWebhookEntry2015(t *testing.T) {
	e := ValWebhookEntry2015{Name: "wh/validate", FailurePolicy: "Fail", IsCatchAll: true, TimeoutSec: 10}
	if e.TimeoutSec != 10 {
		t.Errorf("expected 10")
	}
}
func TestTLSCertResult2015(t *testing.T) {
	r := TLSCertResult2015{Summary: TLSCertSummary2015{TotalIngresses: 30, WithTLS: 25, WithoutTLS: 5}}
	if r.Summary.WithoutTLS != 5 {
		t.Errorf("expected 5")
	}
}
func TestTLSCertEntry2015(t *testing.T) {
	e := TLSCertEntry2015{Name: "api-ing", Namespace: "prod", TLSSecret: "tls-cert"}
	if e.TLSSecret != "tls-cert" {
		t.Errorf("expected tls-cert")
	}
}
func TestSATokenVolResult2015(t *testing.T) {
	r := SATokenVolResult2015{Summary: SATokenVolSummary2015{TotalPods: 90, AutoMountTrue: 85, AutoMountFalse: 5}}
	if r.Summary.AutoMountFalse != 5 {
		t.Errorf("expected 5")
	}
}
func TestSATokenVolEntry2015(t *testing.T) {
	e := SATokenVolEntry2015{Pod: "app", Namespace: "prod", SAName: "app-sa"}
	if e.SAName != "app-sa" {
		t.Errorf("expected app-sa")
	}
}
func TestValWebhookSummary2015(t *testing.T) {
	s := ValWebhookSummary2015{WithFailPolicy: 4}
	if s.WithFailPolicy != 4 {
		t.Errorf("expected 4")
	}
}
func TestTLSCertSummary2015(t *testing.T) {
	s := TLSCertSummary2015{SecretBased: 20}
	if s.SecretBased != 20 {
		t.Errorf("expected 20")
	}
}
