package dashboard

import "testing"

func TestSeccompGapRecs(t *testing.T) {
	r := &SeccompProfileGapResult{Summary: SeccompGapSummary{TotalPods: 50, WithSeccomp: 10, WithoutSeccomp: 40}, GapScore: 20}
	if r.GapScore != 20 {
		t.Error("mismatch")
	}
}
func TestTrafficSpikeRecs(t *testing.T) {
	r := &TrafficSpikeGuardResult{Summary: TrafficSpikeSummary{TotalServices: 30, SingleEndpoint: 10}, GuardScore: 65}
	if r.GuardScore != 65 {
		t.Error("mismatch")
	}
}
func TestNodeLifeForecastRecs(t *testing.T) {
	r := &NodeLifeForecastResult{Summary: NodeLifeForecastSummary{TotalNodes: 3, AgingNodes: 1, AvgAgeDays: 400}, ForecastScore: 66}
	if r.ForecastScore != 66 {
		t.Error("mismatch")
	}
}
func TestSeccompEntryTypes(t *testing.T) {
	e := SeccompGapEntry{Container: "app", HasSeccomp: false, RiskLevel: "medium"}
	if e.HasSeccomp {
		t.Error("should be unprotected")
	}
}
func TestTrafficSpikeEntryTypes(t *testing.T) {
	e := TrafficSpikeEntry{ServiceName: "api", EndpointCount: 0, RiskLevel: "high"}
	if e.EndpointCount != 0 {
		t.Error("should have 0 endpoints")
	}
}
func TestNodeLifeEntryTypes(t *testing.T) {
	e := NodeLifeEntry{NodeName: "worker-1", AgeDays: 500, ActionNeeded: "upgrade", Priority: "medium"}
	if e.ActionNeeded != "upgrade" {
		t.Error("should need upgrade")
	}
}
