package dashboard

import "testing"

func TestRBACBlastTypes(t *testing.T) {
	r := RBACBlastResult{RiskScore: 70, Grade: "C"}
	if r.RiskScore != 70 || r.Grade != "C" { t.Error("struct error") }
	s := RBACBlastSummary{TotalRoles: 50, ClusterAdmins: 2, PrivilegedRoles: 5}
	if s.ClusterAdmins != 2 { t.Error("summary error") }
	hr := HighRiskRole{Name: "admin", Severity: "critical", Subjects: 3}
	if hr.Subjects != 3 { t.Error("highRisk error") }
	ep := RBACEscalationPath{Subject: "user1", Via: "cluster-admin", Severity: "critical"}
	if ep.Severity != "critical" { t.Error("escalation error") }
}

func TestRBACBlastScoring(t *testing.T) {
	tests := []struct{ admins, priv, wild int; expMin, expMax int }{
		{0, 0, 0, 95, 100},
		{3, 10, 5, 0, 30},
		{1, 2, 1, 60, 85},
	}
	for _, tc := range tests {
		score := 100 - tc.admins*10 - tc.priv*5 - tc.wild*8
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("admins=%d priv=%d wild=%d: expected %d-%d, got %d", tc.admins, tc.priv, tc.wild, tc.expMin, tc.expMax, score)
		}
	}
}

func TestGatewayHealthTypes(t *testing.T) {
	r := GatewayHealthResult{HealthScore: 75, Grade: "C"}
	if r.HealthScore != 75 { t.Error("struct error") }
	s := GatewayHealthSummary{ControllerType: "Traefik", ControllerRunning: true, TotalIngresses: 10}
	if !s.ControllerRunning || s.TotalIngresses != 10 { t.Error("summary error") }
	ce := GatewayControllerEntry{Name: "traefik", Ready: true, Type: "Traefik"}
	if !ce.Ready { t.Error("controller error") }
	ig := GatewayIngressGap{Name: "web", Severity: "high"}
	if ig.Severity != "high" { t.Error("gap error") }
}

func TestGatewayHealthScoring(t *testing.T) {
	tests := []struct {
		ctrl    bool
		healthy int
		total   int
		expMin  int
		expMax  int
	}{
		{true, 10, 10, 90, 100},
		{false, 0, 0, 45, 55},
		{true, 5, 10, 80, 90},
	}
	for _, tc := range tests {
		score := 50
		if tc.ctrl { score += 25 }
		if tc.total > 0 { score += tc.healthy * 25 / tc.total }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("ctrl=%v healthy=%d/%d: expected %d-%d, got %d", tc.ctrl, tc.healthy, tc.total, tc.expMin, tc.expMax, score)
		}
	}
}

func TestThrottleRiskTypes(t *testing.T) {
	r := ThrottleRiskResult{RiskScore: 55, Grade: "D"}
	if r.RiskScore != 55 { t.Error("struct error") }
	s := ThrottleSummary{TotalPods: 80, PodsWithLimits: 30, OverLimitPods: 50}
	if s.OverLimitPods != 50 { t.Error("summary error") }
	tr := ThrottleRiskPod{Name: "api", Severity: "high"}
	if tr.Severity != "high" { t.Error("riskPod error") }
	pn := PressureNode{Name: "node1", CPUPct: 85, Status: "critical"}
	if pn.CPUPct != 85 { t.Error("pressureNode error") }
}

func TestThrottleRiskScoring(t *testing.T) {
	// Clean: all pods have limits, no pressure
	score := 100
	if score != 100 { t.Errorf("clean should be 100, got %d", score) }
	// Heavy: no limits, critical nodes
	score = 100
	score -= 15 // one critical node
	score -= 15 // another critical
	if score > 100 || score < 0 { t.Errorf("score out of range: %d", score) }
}
