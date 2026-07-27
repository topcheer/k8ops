package dashboard

import "testing"

func TestEphStoreResult2007(t *testing.T) {
	r := EphStoreResult2007{Summary: EphStoreSummary2007{TotalContainers: 80, WithEphLimit: 20, WithoutLimit: 60}}
	if r.Summary.WithoutLimit != 60 {
		t.Errorf("expected 60")
	}
}
func TestEphStoreEntry2007(t *testing.T) {
	e := EphStoreEntry2007{Pod: "app", Namespace: "prod", Container: "web", Issue: "no eph limit"}
	if e.Issue != "no eph limit" {
		t.Errorf("expected issue")
	}
}
func TestCondHistResult2007(t *testing.T) {
	r := CondHistResult2007{Summary: CondHistSummary2007{TotalDeployments: 30, WithAvailable: 28, WithReplicaFail: 1}}
	if r.Summary.WithReplicaFail != 1 {
		t.Errorf("expected 1")
	}
}
func TestCondHistEntry2007(t *testing.T) {
	e := CondHistEntry2007{Name: "api", Namespace: "prod", Available: true, Progressing: false, ReplicaFailure: false}
	if !e.Available {
		t.Errorf("expected true")
	}
}
func TestResGapResult2007(t *testing.T) {
	r := ResGapResult2007{Summary: ResGapSummary2007{TotalContainers: 100, WithRequest: 80, WithoutRequest: 20}}
	if r.Summary.WithoutRequest != 20 {
		t.Errorf("expected 20")
	}
}
func TestResGapEntry2007(t *testing.T) {
	e := ResGapEntry2007{Pod: "app", Namespace: "prod", Container: "web", HasReq: true, HasLimit: false}
	if e.HasLimit {
		t.Errorf("expected false")
	}
}
func TestEphStoreSummary2007(t *testing.T) {
	s := EphStoreSummary2007{WithEphRequest: 15}
	if s.WithEphRequest != 15 {
		t.Errorf("expected 15")
	}
}
func TestResGapSummary2007(t *testing.T) {
	s := ResGapSummary2007{WithoutLimit: 40}
	if s.WithoutLimit != 40 {
		t.Errorf("expected 40")
	}
}
