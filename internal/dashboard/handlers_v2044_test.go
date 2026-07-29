package dashboard

import "testing"

func TestRSStaleResult2044(t *testing.T) {
	r := RSStaleResult2044{Summary: RSStaleSummary2044{TotalRS: 100, ActiveRS: 50, StaleRS: 50}}
	if r.Summary.StaleRS != 50 {
		t.Errorf("expected 50")
	}
}
func TestRSStaleEntry2044(t *testing.T) {
	e := RSStaleEntry2044{Name: "api-old", Namespace: "prod", OwnerDeploy: "api"}
	if e.OwnerDeploy != "api" {
		t.Errorf("expected api")
	}
}
func TestPullPolicyResult2044(t *testing.T) {
	r := PullPolicyResult2044{Summary: PullPolicySummary2044{TotalContainers: 200, AlwaysPolicy: 50, IfNotPresent: 140, NeverPolicy: 10}}
	if r.Summary.NeverPolicy != 10 {
		t.Errorf("expected 10")
	}
}
func TestPullPolicyEntry2044(t *testing.T) {
	e := PullPolicyEntry2044{Pod: "app", Namespace: "prod", Container: "web", Image: "nginx:1.25", Policy: "Never"}
	if e.Policy != "Never" {
		t.Errorf("expected Never")
	}
}
func TestMaxSurgeResult2044(t *testing.T) {
	r := MaxSurgeResult2044{Summary: MaxSurgeSummary2044{TotalDeploys: 50, WithSurge: 30, HighSurge: 5, DefaultSurge: 20}}
	if r.Summary.HighSurge != 5 {
		t.Errorf("expected 5")
	}
}
func TestMaxSurgeEntry2044(t *testing.T) {
	e := MaxSurgeEntry2044{Name: "api", Namespace: "prod", MaxSurge: "5", MaxUnavail: "1"}
	if e.MaxSurge != "5" {
		t.Errorf("expected 5")
	}
}
