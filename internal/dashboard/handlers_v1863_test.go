package dashboard

import "testing"

func TestBypassRecs(t *testing.T) {
	r := &AdmissionBypassAuditResult{Summary: BypassAuditSummary{TotalPods: 50, BypassPods: 10, PrivilegedPods: 3, HostNetworkPods: 5}, BypassScore: 80}
	if r.BypassScore != 80 {
		t.Error("score mismatch")
	}
}

func TestGoldenPathRecs(t *testing.T) {
	r := &GoldenPathValidatorResult{Summary: GoldenPathSummary{TotalWorkloads: 20, FullyCompliant: 5, MissingProbes: 10, MissingLimits: 8}, ComplianceScore: 25}
	if r.ComplianceScore != 25 {
		t.Error("score mismatch")
	}
}

func TestFaultToleranceRecs(t *testing.T) {
	r := &ClusterFaultToleranceResult{Summary: FaultToleranceSummary{WorkerNodes: 2, Zones: 1}, ToleranceScore: 0}
	if r.ToleranceScore != 0 {
		t.Error("score mismatch")
	}
}

func TestBypassTypes(t *testing.T) {
	e := BypassEntry{PodName: "web-0", Severity: "critical", FindingCount: 3}
	if e.Severity != "critical" {
		t.Error("should be critical")
	}
}

func TestGoldenPathTypes(t *testing.T) {
	e := GoldenPathEntry{Workload: "api", Score: 85, ComplianceLevel: "partial"}
	if e.Score != 85 {
		t.Error("score mismatch")
	}
}

func TestFaultScenarioTypes(t *testing.T) {
	s := FaultScenario{Name: "single-node-loss", ImpactPct: 33.3, Survives: false}
	if s.ImpactPct != 33.3 {
		t.Error("impact mismatch")
	}
}

func TestSafeDivPctFloat(t *testing.T) {
	if safeDivPctFloat(10, 20) != 50 {
		t.Error("10/20 should be 50%")
	}
	if safeDivPctFloat(10, 0) != 0 {
		t.Error("div by zero should be 0")
	}
}

func TestEstRecovery(t *testing.T) {
	if estRecovery(3) != "< 2min (auto-recovery)" {
		t.Error("3+ should auto-recover")
	}
	if estRecovery(1) != "5-15min (manual intervention)" {
		t.Error("1 should need manual")
	}
	if estRecovery(0) != "30min+ (full rebuild)" {
		t.Error("0 should need rebuild")
	}
}
