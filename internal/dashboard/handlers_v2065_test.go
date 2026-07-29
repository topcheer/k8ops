package dashboard

import "testing"

func TestPVReclaimResult2065(t *testing.T) {
	r := PVReclaimResult2065{Summary: PVReclaimSummary2065{TotalPVs: 20, RetainPVs: 15, DeletePVs: 5}}
	if r.Summary.RetainPVs != 15 {
		t.Errorf("expected 15")
	}
}
func TestSAInvResult2065(t *testing.T) {
	r := SAInvResult2065{Summary: SAInvSummary2065{TotalSAs: 50, UnusedSAs: 10, DefaultSAs: 20}}
	if r.Summary.UnusedSAs != 10 {
		t.Errorf("expected 10")
	}
}
func TestNodeCondTLResult2065(t *testing.T) {
	r := NodeCondTLResult2065{Summary: NodeCondTLSummary2065{TotalNodes: 5, NodesWithConds: 1}}
	if r.Summary.NodesWithConds != 1 {
		t.Errorf("expected 1")
	}
}
