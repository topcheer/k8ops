package dashboard

import "testing"

func TestRestartPolResult1995(t *testing.T) {
	r := RestartPolResult1995{Summary: RestartPolSummary1995{TotalPods: 80, AlwaysPolicy: 75, NeverPolicy: 2, Misconfigured: 3}}
	if r.Summary.Misconfigured != 3 {
		t.Errorf("expected 3")
	}
}
func TestRestartPolEntry1995(t *testing.T) {
	e := RestartPolEntry1995{Pod: "app", Namespace: "prod", Policy: "Never", Issue: "won't recover"}
	if e.Policy != "Never" {
		t.Errorf("expected Never")
	}
}
func TestRevHistResult1995(t *testing.T) {
	r := RevHistResult1995{Summary: RevHistSummary1995{TotalDeployments: 50, WithCustomLimit: 10, TooLow: 2}}
	if r.Summary.TooLow != 2 {
		t.Errorf("expected 2")
	}
}
func TestRevHistEntry1995(t *testing.T) {
	limit := int32(5)
	e := RevHistEntry1995{Name: "api", Namespace: "prod", HistoryLimit: &limit}
	if *e.HistoryLimit != 5 {
		t.Errorf("expected 5")
	}
}
func TestEnvHealthResult1995(t *testing.T) {
	r := EnvHealthResult1995{Summary: EnvHealthSummary1995{TotalContainers: 100, WithEnv: 60, Hardcoded: 5}}
	if r.Summary.Hardcoded != 5 {
		t.Errorf("expected 5")
	}
}
func TestEnvHealthEntry1995(t *testing.T) {
	e := EnvHealthEntry1995{Pod: "app", Namespace: "prod", Container: "web", EnvName: "PASSWORD", Issue: "hardcoded"}
	if e.EnvName != "PASSWORD" {
		t.Errorf("expected PASSWORD")
	}
}
func TestSensitiveEnvNames1995(t *testing.T) {
	if !sensitiveEnvNames1995["PASSWORD"] {
		t.Errorf("expected PASSWORD to be sensitive")
	}
	if sensitiveEnvNames1995["LOG_LEVEL"] {
		t.Errorf("LOG_LEVEL should not be sensitive")
	}
}
func TestRestartPolSummary1995(t *testing.T) {
	s := RestartPolSummary1995{OnFailure: 3}
	if s.OnFailure != 3 {
		t.Errorf("expected 3")
	}
}
