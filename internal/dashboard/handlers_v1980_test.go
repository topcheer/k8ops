package dashboard

import "testing"

func TestSecretInvResult1980(t *testing.T) {
	r := SecretInvResult1980{Summary: SecretInvSummary1980{TotalSecrets: 40, OpaqueSecrets: 30, TLSCerts: 5, OldSecrets: 8}}
	if r.Summary.OldSecrets != 8 {
		t.Errorf("expected 8")
	}
}
func TestSecretInvEntry1980(t *testing.T) {
	e := SecretInvEntry1980{Name: "db-pass", Namespace: "prod", Type: "Opaque", KeyCount: 2, AgeDays: 45.5}
	if e.AgeDays != 45.5 {
		t.Errorf("expected 45.5")
	}
}
func TestSAInvResult1980(t *testing.T) {
	r := SAInvResult1980{Summary: SAInvSummary1980{TotalSAs: 50, WithBindings: 30, Unbound: 20}}
	if r.Summary.Unbound != 20 {
		t.Errorf("expected 20")
	}
}
func TestSAInvEntry1980(t *testing.T) {
	e := SAInvEntry1980{Name: "app-sa", Namespace: "prod", HasSecret: true, AutoMount: false, BindingCount: 2}
	if e.BindingCount != 2 {
		t.Errorf("expected 2")
	}
}
func TestEventTypeResult1980(t *testing.T) {
	r := EventTypeResult1980{Summary: EventTypeSummary1980{TotalEvents: 500, UniqueReasons: 15, WarningCount: 100}}
	if r.Summary.UniqueReasons != 15 {
		t.Errorf("expected 15")
	}
}
func TestEventTypeEntry1980(t *testing.T) {
	e := EventTypeEntry1980{Name: "Pulled", Count: 200}
	if e.Count != 200 {
		t.Errorf("expected 200")
	}
}
func TestSecretInvSummary1980(t *testing.T) {
	s := SecretInvSummary1980{DockerConfig: 3, AvgAgeDays: 60.0}
	if s.AvgAgeDays != 60.0 {
		t.Errorf("expected 60")
	}
}
func TestSAInvSummary1980(t *testing.T) {
	s := SAInvSummary1980{DefaultSAs: 10, WithImagePull: 5}
	if s.DefaultSAs != 10 {
		t.Errorf("expected 10")
	}
}
