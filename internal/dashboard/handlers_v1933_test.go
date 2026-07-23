package dashboard

import "testing"

func TestMeshReadyResult1933(t *testing.T) {
	r := MeshReadyResult1933{Summary: MeshReadySummary1933{TotalPods: 80, WithSidecar: 30, WithoutSidecar: 50}}
	if r.Summary.WithoutSidecar != 50 {
		t.Errorf("expected 50")
	}
}
func TestMeshGapEntry1933(t *testing.T) {
	e := MeshGapEntry1933{Name: "web", Reason: "no sidecar", Severity: "low"}
	if e.Severity != "low" {
		t.Errorf("expected low")
	}
}
func TestVolAccessResult1933(t *testing.T) {
	r := VolAccessResult1933{Summary: VolAccessSummary1933{TotalPVCs: 15, RWOCount: 12, RWXCount: 2}}
	if r.Summary.RWOCount != 12 {
		t.Errorf("expected 12")
	}
}
func TestVolAccessIssue1933(t *testing.T) {
	i := VolAccessIssue1933{IssueType: "rwo-shared", Severity: "high"}
	if i.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestPDBGapResult1933(t *testing.T) {
	r := PDBGapResult1933{Summary: PDBGapSummary1933{TotalWorkloads: 30, WithPDB: 5, WithoutPDB: 25, CriticalUnprot: 3}}
	if r.Summary.CriticalUnprot != 3 {
		t.Errorf("expected 3")
	}
}
func TestPDBGapEntry1933(t *testing.T) {
	e := PDBGapEntry1933{Name: "api", Replicas: 1, Severity: "critical"}
	if e.Severity != "critical" {
		t.Errorf("expected critical")
	}
}
func TestMatchLabelsHelper(t *testing.T) {
	if !matchLabels1933(map[string]string{"app": "web"}, map[string]string{"app": "web", "env": "prod"}) {
		t.Errorf("expected match")
	}
	if matchLabels1933(map[string]string{"app": "web"}, map[string]string{"app": "db"}) {
		t.Errorf("expected no match")
	}
}
func TestContainsHelper(t *testing.T) {
	if !containsStr1933("sidecar.istio.io/inject", "istio.io") {
		t.Errorf("expected true")
	}
	if containsStr1933("hello", "world") {
		t.Errorf("expected false")
	}
}
