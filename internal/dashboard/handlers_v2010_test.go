package dashboard

import "testing"

func TestPDBCovResult2010(t *testing.T) {
	r := PDBCovResult2010{Summary: PDBCovSummary2010{TotalDeployments: 20, WithPDB: 10, WithoutPDB: 10}}
	if r.Summary.WithoutPDB != 10 {
		t.Errorf("expected 10")
	}
}
func TestPDBCovEntry2010(t *testing.T) {
	e := PDBCovEntry2010{Name: "api", Namespace: "prod", Replicas: 3}
	if e.Replicas != 3 {
		t.Errorf("expected 3")
	}
}
func TestSnapClassResult2010(t *testing.T) {
	r := SnapClassResult2010{Summary: SnapClassSummary2010{TotalClasses: 2, HasDefault: true}}
	if !r.Summary.HasDefault {
		t.Errorf("expected true")
	}
}
func TestSnapClassEntry2010(t *testing.T) {
	e := SnapClassEntry2010{Name: "snap-fast", Driver: "csi-driver", IsDefault: true, DeletionPol: "Delete"}
	if e.DeletionPol != "Delete" {
		t.Errorf("expected Delete")
	}
}
func TestMutWebhookResult2010(t *testing.T) {
	r := MutWebhookResult2010{Summary: MutWebhookSummary2010{TotalWebhooks: 5, CatchAll: 2}}
	if r.Summary.CatchAll != 2 {
		t.Errorf("expected 2")
	}
}
func TestMutWebhookEntry2010(t *testing.T) {
	e := MutWebhookEntry2010{Name: "wh-1", FailurePolicy: "Fail", IsCatchAll: true}
	if !e.IsCatchAll {
		t.Errorf("expected true")
	}
}
func TestPDBCovSummary2010(t *testing.T) {
	s := PDBCovSummary2010{WithPDB: 5}
	if s.WithPDB != 5 {
		t.Errorf("expected 5")
	}
}
func TestMutWebhookSummary2010(t *testing.T) {
	s := MutWebhookSummary2010{WithFailureMode: 4}
	if s.WithFailureMode != 4 {
		t.Errorf("expected 4")
	}
}
