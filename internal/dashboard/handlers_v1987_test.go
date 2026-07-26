package dashboard

import "testing"

func TestStsHealthResult1987(t *testing.T) {
	r := StsHealthResult1987{Summary: StsHealthSummary1987{TotalSTS: 10, FullyReady: 8, PartiallyReady: 1, NotReady: 1}}
	if r.Summary.PartiallyReady != 1 {
		t.Errorf("expected 1")
	}
}
func TestStsHealthEntry1987(t *testing.T) {
	e := StsHealthEntry1987{Name: "db", Namespace: "prod", Desired: 3, Ready: 2, Status: "partial"}
	if e.Status != "partial" {
		t.Errorf("expected partial")
	}
}
func TestDSCoverageResult1987(t *testing.T) {
	r := DSCoverageResult1987{Summary: DSCoverageSummary1987{TotalDS: 5, FullyScheduled: 4, PartiallyScheduled: 1}}
	if r.Summary.FullyScheduled != 4 {
		t.Errorf("expected 4")
	}
}
func TestDSCoverageEntry1987(t *testing.T) {
	e := DSCoverageEntry1987{Name: "agent", Namespace: "kube-system", DesiredScheduled: 5, CurrentScheduled: 5, Ready: 5}
	if e.Ready != 5 {
		t.Errorf("expected 5")
	}
}
func TestJobCompResult1987(t *testing.T) {
	r := JobCompResult1987{Summary: JobCompSummary1987{TotalJobs: 20, Succeeded: 15, Failed: 3, Running: 2, CompletionRate: 90.0}}
	if r.Summary.CompletionRate != 90.0 {
		t.Errorf("expected 90")
	}
}
func TestJobCompEntry1987(t *testing.T) {
	e := JobCompEntry1987{Name: "batch-1", Namespace: "prod", Status: "succeeded", Succeeded: 1, Failed: 0}
	if e.Status != "succeeded" {
		t.Errorf("expected succeeded")
	}
}
func TestStsHealthSummary1987(t *testing.T) {
	s := StsHealthSummary1987{ZeroReplicas: 2}
	if s.ZeroReplicas != 2 {
		t.Errorf("expected 2")
	}
}
func TestDSCoverageSummary1987(t *testing.T) {
	s := DSCoverageSummary1987{TotalNodes: 5, NotScheduled: 1}
	if s.NotScheduled != 1 {
		t.Errorf("expected 1")
	}
}
