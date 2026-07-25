package dashboard

import "testing"

func TestInitDepResult1977(t *testing.T) {
	r := InitDepResult1977{Summary: InitDepSummary1977{TotalPods: 80, PodsWithInit: 20, TotalInitCtrs: 35}}
	if r.Summary.PodsWithInit != 20 {
		t.Errorf("expected 20")
	}
}
func TestInitDepEntry1977(t *testing.T) {
	e := InitDepEntry1977{Pod: "app", Namespace: "prod", InitCount: 3, Names: []string{"init-db", "init-config"}, HasResource: true}
	if e.InitCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestStrategyCompResult1977(t *testing.T) {
	r := StrategyCompResult1977{Summary: StrategyCompSummary1977{TotalDeployments: 50, RollingUpdate: 45, Recreate: 5}}
	if r.Summary.Recreate != 5 {
		t.Errorf("expected 5")
	}
}
func TestStrategyCompEntry1977(t *testing.T) {
	e := StrategyCompEntry1977{Name: "api", Namespace: "prod", Strategy: "RollingUpdate", MaxSurge: "25%", MaxUnavailable: "1"}
	if e.Strategy != "RollingUpdate" {
		t.Errorf("expected RollingUpdate")
	}
}
func TestStrategyCompIssue1977(t *testing.T) {
	e := StrategyCompIssue1977{Name: "api", Issue: "Recreate strategy", Severity: "medium"}
	if e.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
func TestPullSecretResult1977(t *testing.T) {
	r := PullSecretResult1977{Summary: PullSecretSummary1977{TotalNamespaces: 20, WithPullSecret: 15, WithoutSecret: 5}}
	if r.Summary.WithoutSecret != 5 {
		t.Errorf("expected 5")
	}
}
func TestPullSecretNSEntry1977(t *testing.T) {
	e := PullSecretNSEntry1977{Namespace: "prod", HasSecret: false, PrivateRegImages: 3}
	if e.PrivateRegImages != 3 {
		t.Errorf("expected 3")
	}
}
func TestInitDepSummary1977(t *testing.T) {
	s := InitDepSummary1977{MaxInitCount: 5}
	if s.MaxInitCount != 5 {
		t.Errorf("expected 5")
	}
}
