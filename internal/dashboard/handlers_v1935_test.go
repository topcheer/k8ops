package dashboard

import "testing"

func TestRevisionTimelineResult1935(t *testing.T) {
	r := RevisionTimelineResult1935{Summary: RevisionTimelineSummary1935{TotalDeployments: 30, TotalRevisions: 120, StaleRevisions: 5}}
	if r.Summary.TotalRevisions != 120 {
		t.Errorf("expected 120")
	}
}
func TestRevisionEntry1935(t *testing.T) {
	e := RevisionEntry1935{Name: "api", CurrentRev: "5", OldRevCount: 4}
	if e.OldRevCount != 4 {
		t.Errorf("expected 4")
	}
}
func TestQoSDistResult1935(t *testing.T) {
	r := QoSDistResult1935{Summary: QoSDistSummary1935{TotalPods: 80, Guaranteed: 20, Burstable: 50, BestEffort: 10}}
	if r.Summary.BestEffort != 10 {
		t.Errorf("expected 10")
	}
}
func TestQoSNSStat1935(t *testing.T) {
	s := QoSNSStat1935{Namespace: "prod", Guaranteed: 5, Burstable: 10, BestEffort: 2, Total: 17}
	if s.Total != 17 {
		t.Errorf("expected 17")
	}
}
func TestDSHealthResult1935(t *testing.T) {
	r := DSHealthResult1935{Summary: DSHealthSummary1935{TotalDS: 5, HealthyDS: 4, DesiredScheduled: 10, NumberMisscheduled: 1}}
	if r.Summary.HealthyDS != 4 {
		t.Errorf("expected 4")
	}
}
func TestDSEntry1935(t *testing.T) {
	e := DSEntry1935{Name: "fluentd", DesiredScheduled: 3, CurrentScheduled: 3, NumberReady: 2}
	if e.NumberReady != 2 {
		t.Errorf("expected 2")
	}
}
func TestDSIssue1935(t *testing.T) {
	i := DSIssue1935{IssueType: "not-all-ready", Severity: "high"}
	if i.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestOldRevisionEntry1935(t *testing.T) {
	e := OldRevisionEntry1935{Name: "web", Revision: "3", Age: "15d"}
	if e.Age != "15d" {
		t.Errorf("expected 15d")
	}
}
