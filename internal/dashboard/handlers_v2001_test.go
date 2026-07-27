package dashboard

import "testing"

func TestShareProcResult2001(t *testing.T) {
	r := ShareProcResult2001{Summary: ShareProcSummary2001{TotalPods: 80, WithSharePID: 3}}
	if r.Summary.WithSharePID != 3 {
		t.Errorf("expected 3")
	}
}
func TestShareProcEntry2001(t *testing.T) {
	e := ShareProcEntry2001{Pod: "app", Namespace: "prod"}
	if e.Pod != "app" {
		t.Errorf("expected app")
	}
}
func TestPodPrioResult2001(t *testing.T) {
	r := PodPrioResult2001{Summary: PodPrioSummary2001{TotalPods: 90, WithPC: 10, WithoutPC: 80}}
	if r.Summary.WithoutPC != 80 {
		t.Errorf("expected 80")
	}
}
func TestPodPrioEntry2001(t *testing.T) {
	e := PodPrioEntry2001{Pod: "app", Namespace: "prod"}
	if e.Namespace != "prod" {
		t.Errorf("expected prod")
	}
}
func TestSubPathResult2001(t *testing.T) {
	r := SubPathResult2001{Summary: SubPathSummary2001{TotalContainers: 100, WithSubPath: 20}}
	if r.Summary.WithSubPath != 20 {
		t.Errorf("expected 20")
	}
}
func TestSubPathEntry2001(t *testing.T) {
	e := SubPathEntry2001{Pod: "app", Namespace: "prod", Volume: "config", SubPath: "app.conf"}
	if e.SubPath != "app.conf" {
		t.Errorf("expected app.conf")
	}
}
func TestShareProcSummary2001(t *testing.T) {
	s := ShareProcSummary2001{Without: 77}
	if s.Without != 77 {
		t.Errorf("expected 77")
	}
}
func TestSubPathSummary2001(t *testing.T) {
	s := SubPathSummary2001{WithoutSubPath: 80}
	if s.WithoutSubPath != 80 {
		t.Errorf("expected 80")
	}
}
