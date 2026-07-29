package dashboard

import "testing"

func TestTopoPinResult2079(t *testing.T) {
	r := TopoPinResult2079{Summary: TopoPinSummary2079{TotalPods: 100, WithNodeSel: 20, WithNodeName: 5}}
	if r.Summary.WithNodeName != 5 {
		t.Errorf("expected 5")
	}
}
func TestIngRuleResult2079(t *testing.T) {
	r := IngRuleResult2079{Summary: IngRuleSummary2079{TotalIngresses: 20, TotalRules: 50, AvgRules: 2}}
	if r.Summary.AvgRules != 2 {
		t.Errorf("expected 2")
	}
}
func TestVCClaimResult2080(t *testing.T) {
	r := VCClaimResult2080{Summary: VCClaimSummary2080{TotalSTS: 10, WithVCTempl: 7, MissingVCT: 3}}
	if r.Summary.MissingVCT != 3 {
		t.Errorf("expected 3")
	}
}
func TestPhaseDistResult2081(t *testing.T) {
	r := PhaseDistResult2081{Summary: PhaseDistSummary2081{Total: 100, Running: 95, Pending: 3, Failed: 2}}
	if r.Summary.Failed != 2 {
		t.Errorf("expected 2")
	}
}
func TestEgressAuditResult2082(t *testing.T) {
	r := EgressAuditResult2082{Summary: EgressAuditSummary2082{TotalNS: 20, WithEgress: 5, OpenEgress: 15}}
	if r.Summary.OpenEgress != 15 {
		t.Errorf("expected 15")
	}
}
func TestNodeArchResult2083(t *testing.T) {
	r := NodeArchResult2083{Summary: NodeArchSummary2083{TotalNodes: 3, UniqueArch: 1}}
	if r.Summary.UniqueArch != 1 {
		t.Errorf("expected 1")
	}
}
func TestPodForecastResult2084(t *testing.T) {
	r := PodForecastResult2084{Summary: PodForecastSummary2084{CurrentPods: 90, MaxCapacity: 110, GrowthHeadroom: 20}}
	if r.Summary.GrowthHeadroom != 20 {
		t.Errorf("expected 20")
	}
}
func TestHACoverResult2084(t *testing.T) {
	r := HACoverResult2084{Summary: HACoverSummary2084{TotalMultiReplica: 30, HACovered: 10, NotHA: 20}}
	if r.Summary.NotHA != 20 {
		t.Errorf("expected 20")
	}
}
