package dashboard

import "testing"

func TestOOMRiskResult2057(t *testing.T) {
	r := OOMRiskResult2057{Summary: OOMRiskSummary2057{TotalPods: 100, WithMemLimit: 80, NoMemLimit: 20, AtRisk: 10}}
	if r.Summary.AtRisk != 10 {
		t.Errorf("expected 10")
	}
}
func TestAPIServerQPSResult2057(t *testing.T) {
	r := APIServerQPSResult2057{Summary: APIServerQPSSummary2057{WatchCount: 50, ListCount: 30, TotalObjects: 5000}}
	if r.Summary.TotalObjects != 5000 {
		t.Errorf("expected 5000")
	}
}
func TestNodePressureResult2057(t *testing.T) {
	r := NodePressureResult2057{Summary: NodePressureSummary2057{TotalNodes: 5, HealthyNodes: 4, PressuredNodes: 1}}
	if r.Summary.PressuredNodes != 1 {
		t.Errorf("expected 1")
	}
}
