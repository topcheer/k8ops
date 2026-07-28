package dashboard

import "testing"

func TestStartupProbeResult2019(t *testing.T) {
	r := StartupProbeResult2019{Summary: StartupProbeSummary2019{TotalContainers: 100, WithStartupProbe: 10, WithLiveness: 60, WithoutAny: 30}}
	if r.Summary.WithoutAny != 30 {
		t.Errorf("expected 30")
	}
}
func TestStartupProbeEntry2019(t *testing.T) {
	e := StartupProbeEntry2019{Pod: "app", Namespace: "prod", Container: "web"}
	if e.Container != "web" {
		t.Errorf("expected web")
	}
}
func TestCmdHashResult2019(t *testing.T) {
	r := CmdHashResult2019{Summary: CmdHashSummary2019{TotalContainers: 80, WithCommand: 20, UniqueHashes: 50}}
	if r.Summary.UniqueHashes != 50 {
		t.Errorf("expected 50")
	}
}
func TestCmdHashEntry2019(t *testing.T) {
	e := CmdHashEntry2019{Hash: "abc12345", Image: "nginx:1.25", Count: 5}
	if e.Count != 5 {
		t.Errorf("expected 5")
	}
}
func TestStratTypeResult2019(t *testing.T) {
	r := StratTypeResult2019{Summary: StratTypeSummary2019{TotalDeployments: 30, RollingUpdate: 28, Recreate: 2}}
	if r.Summary.Recreate != 2 {
		t.Errorf("expected 2")
	}
}
func TestStratTypeEntry2019(t *testing.T) {
	e := StratTypeEntry2019{Strategy: "RollingUpdate", Count: 28}
	if e.Count != 28 {
		t.Errorf("expected 28")
	}
}
func TestStartupProbeSummary2019(t *testing.T) {
	s := StartupProbeSummary2019{WithStartupProbe: 10}
	if s.WithStartupProbe != 10 {
		t.Errorf("expected 10")
	}
}
func TestCmdHashSummary2019(t *testing.T) {
	s := CmdHashSummary2019{WithArgs: 40}
	if s.WithArgs != 40 {
		t.Errorf("expected 40")
	}
}
