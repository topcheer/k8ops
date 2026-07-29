package dashboard

import "testing"

func TestRolloutWindowResult2038(t *testing.T) {
	r := RolloutWindowResult2038{Summary: RolloutWindowSummary2038{TotalDeploys: 50, WithConditions: 40, Progressing: 3, Available: 45}}
	if r.Summary.Progressing != 3 {
		t.Errorf("expected 3")
	}
}
func TestRolloutWindowEntry2038(t *testing.T) {
	e := RolloutWindowEntry2038{Name: "api", Namespace: "prod", Replicas: 3, Updated: 2, Ready: 2}
	if e.Updated != 2 {
		t.Errorf("expected 2")
	}
}
func TestInitContainerResult2038(t *testing.T) {
	r := InitContainerResult2038{Summary: InitContainerSummary2038{TotalPods: 100, PodsWithInit: 20, TotalInitCtnrs: 30, HeavyInitCtnrs: 5}}
	if r.Summary.TotalInitCtnrs != 30 {
		t.Errorf("expected 30")
	}
}
func TestInitContainerEntry2038(t *testing.T) {
	e := InitContainerEntry2038{Pod: "app", Namespace: "prod", Container: "init-db"}
	if e.Container != "init-db" {
		t.Errorf("expected init-db")
	}
}
func TestProbeConfigResult2038(t *testing.T) {
	r := ProbeConfigResult2038{Summary: ProbeConfigSummary2038{TotalContainers: 200, WithLiveness: 150, WithReadiness: 120, NoProbes: 30}}
	if r.Summary.NoProbes != 30 {
		t.Errorf("expected 30")
	}
}
func TestProbeConfigEntry2038(t *testing.T) {
	e := ProbeConfigEntry2038{Pod: "api", Namespace: "prod", Container: "web", Missing: "readiness"}
	if e.Missing != "readiness" {
		t.Errorf("expected readiness")
	}
}
