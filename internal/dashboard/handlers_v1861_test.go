package dashboard

import "testing"

func TestRuntimeDriftRecs(t *testing.T) {
	r := &RuntimeDriftDetectResult{Summary: RuntimeDriftSummary{TotalPods: 50, DriftedPods: 5, ImageDrifts: 3, EnvDrifts: 2}, DriftScore: 90}
	if r.DriftScore != 90 {
		t.Error("score mismatch")
	}
}

func TestSvcMeshReadinessRecs(t *testing.T) {
	r := &SvcMeshReadinessResult{Summary: SvcMeshReadinessSummary{TotalServices: 30, MeshReady: 15, BlockingIssues: 15}, ReadinessScore: 50}
	if r.ReadinessScore != 50 {
		t.Error("score mismatch")
	}
}

func TestNodePoolRightsizeRecs(t *testing.T) {
	r := &NodePoolRightsizeResult{Summary: NodePoolRightsizeSummary{TotalNodes: 5, OverProvisioned: 2, RightSized: 3, PotentialSavings: 150.0}, RightsizeScore: 60}
	if r.RightsizeScore != 60 {
		t.Error("score mismatch")
	}
}

func TestRuntimeDriftTypes(t *testing.T) {
	e := RuntimeDriftEntry{PodName: "web-0", DriftType: "image", Severity: "high", Detail: "nginx:1.21 vs nginx:1.22"}
	if e.Severity != "high" {
		t.Error("should be high")
	}
}

func TestSvcMeshTypes(t *testing.T) {
	e := SvcMeshEntry{ServiceName: "api", MeshReady: true, ReadinessPct: 100}
	if !e.MeshReady {
		t.Error("should be ready")
	}
}

func TestNodeRightsizeTypes(t *testing.T) {
	e := NodeRightsizeEntry{NodeName: "worker-1", Recommendation: "downsize", CPUUtilPct: 15}
	if e.Recommendation != "downsize" {
		t.Error("should be downsize")
	}
}

func TestGradeFromScore(t *testing.T) {
	var g string
	gradeFromScore(&g, 85)
	if g != "A" {
		t.Errorf("expected A, got %s", g)
	}
	gradeFromScore(&g, 50)
	if g != "C" {
		t.Errorf("expected C, got %s", g)
	}
	gradeFromScore(&g, 10)
	if g != "F" {
		t.Errorf("expected F, got %s", g)
	}
}
