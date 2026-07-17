package dashboard

import (
	"testing"
)

func TestBuildConfigAuditRecs(t *testing.T) {
	r := &ConfigAuditTrailResult{
		Summary: ConfigAuditTrailSummary{TotalWorkloads: 50, TotalChanges: 100, RecentChanges: 5, StaleWorkloads: 10},
	}
	recs := buildConfigAuditRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildNodeUtilDeepRecs(t *testing.T) {
	r := &NodeUtilizationDeepResult{
		Summary: NodeUtilDeepSummary{AvgCPUUtil: 45, AvgMemUtil: 55, AvgPodDensity: 60, OverloadedNodes: 2, IdleNodes: 0, MaxCPUUtil: 85, MaxMemUtil: 70},
	}
	recs := buildNodeUtilDeepRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildSecretRotRecs(t *testing.T) {
	r := &SecretRotationPlanResult{
		Summary: SecretRotSummary{NeedsRotation: 10, TotalSecrets: 50, Critical: 2, High: 5},
	}
	recs := buildSecretRotRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
