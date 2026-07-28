package dashboard

import "testing"

func TestTolScopeResult2025(t *testing.T) {
	r := TolScopeResult2025{Summary: TolScopeSummary2025{TotalPods: 90, WithTol: 50, CatchAllTol: 2}}
	if r.Summary.CatchAllTol != 2 {
		t.Errorf("expected 2")
	}
}
func TestTolScopeEntry2025(t *testing.T) {
	e := TolScopeEntry2025{Key: "node-role", Effect: "NoSchedule", Count: 30}
	if e.Count != 30 {
		t.Errorf("expected 30")
	}
}
func TestHostPortResult2025(t *testing.T) {
	r := HostPortResult2025{Summary: HostPortSummary2025{TotalContainers: 100, WithHostPort: 5, PrivilegedPorts: 2}}
	if r.Summary.PrivilegedPorts != 2 {
		t.Errorf("expected 2")
	}
}
func TestHostPortEntry2025(t *testing.T) {
	e := HostPortEntry2025{Pod: "app", Namespace: "prod", ContainerPort: 8080, HostPort: 80}
	if e.HostPort != 80 {
		t.Errorf("expected 80")
	}
}
func TestProgDeadResult2025(t *testing.T) {
	r := ProgDeadResult2025{Summary: ProgDeadSummary2025{TotalDeployments: 30, WithDeadline: 10, UsingDefault: 20, WithTimeout: 1}}
	if r.Summary.WithTimeout != 1 {
		t.Errorf("expected 1")
	}
}
func TestProgDeadEntry2025(t *testing.T) {
	e := ProgDeadEntry2025{Name: "api", Namespace: "prod", ProgressDeadline: 300}
	if e.ProgressDeadline != 300 {
		t.Errorf("expected 300")
	}
}
func TestTolScopeSummary2025(t *testing.T) {
	s := TolScopeSummary2025{NoExecute: 5}
	if s.NoExecute != 5 {
		t.Errorf("expected 5")
	}
}
func TestHostPortSummary2025(t *testing.T) {
	s := HostPortSummary2025{TotalHostPorts: 8}
	if s.TotalHostPorts != 8 {
		t.Errorf("expected 8")
	}
}
