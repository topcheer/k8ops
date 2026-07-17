package dashboard

import (
	"testing"
)

func TestBuildMaturityRecs(t *testing.T) {
	r := &ClusterMaturityResult{
		CurrentLevel: 2, LevelName: "Managed", ScorePct: 50,
		TargetLevel: 3, NextLevelName: "Defined",
		Gaps: []MaturityGap{
			{Capability: "Resource Quotas", Level: 3, Action: "Create ResourceQuota"},
			{Capability: "Network Policies", Level: 3, Action: "Add NetworkPolicy"},
		},
	}
	recs := buildMaturityRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestGapAction(t *testing.T) {
	if a := gapAction("Resource Limits"); a == "Resource Limits" {
		t.Error("expected action text, got input back")
	}
	if a := gapAction("Nonexistent"); a != "Refer to Kubernetes best practices" {
		t.Errorf("expected fallback, got %s", a)
	}
}

func TestFormatCPUStr(t *testing.T) {
	if v := formatCPUStr(0.5); v != "500m" {
		t.Errorf("expected 500m, got %s", v)
	}
	if v := formatCPUStr(2.0); v != "2.0" {
		t.Errorf("expected 2.0, got %s", v)
	}
}

func TestBuildDeployRiskRecs(t *testing.T) {
	r := &DeployRiskResult{
		OverallRisk: 65, Verdict: "risky",
		RiskFactors: []DeployRiskFactor{
			{Name: "Single Node", Risk: 90, Detail: "1 node", Mitigation: "Add nodes"},
			{Name: "Crash Rate", Risk: 20, Detail: "2 crashes", Mitigation: "Fix crashes"},
		},
	}
	recs := buildDeployRiskRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}
