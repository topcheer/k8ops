package dashboard

import "testing"

func TestComputeEnvAuditScore(t *testing.T) {
	if s := computeEnvAuditScore(EnvAuditSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	if s := computeEnvAuditScore(EnvAuditSummary{TotalWorkloads: 10, PlaintextSecrets: 5}); s > 75 {
		t.Errorf("expected <= 75, got %d", s)
	}
}

func TestComputeScalingSimScore(t *testing.T) {
	if s := computeScalingSimScore(ScalingSimSummary{HeadroomCPU: 50, HeadroomMem: 50}); s < 90 {
		t.Errorf("expected >= 90, got %d", s)
	}
	if s := computeScalingSimScore(ScalingSimSummary{HeadroomCPU: 10, HeadroomMem: 10}); s > 50 {
		t.Errorf("expected <= 50, got %d", s)
	}
}

func TestComputePlacementScore(t *testing.T) {
	if s := computePlacementScore(PlacementSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	if s := computePlacementScore(PlacementSummary{TotalWorkloads: 10, SPofWorkloads: 3, AvgNodeDiversity: 0.3}); s > 60 {
		t.Errorf("expected <= 60, got %d", s)
	}
}
