package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func makeNodesWithProviderID(pid string) []corev1.Node {
	return []corev1.Node{{Spec: corev1.NodeSpec{ProviderID: pid}}}
}

func TestDetectCloudVendor(t *testing.T) {
	tests := []struct {
		name     string
		providerID string
		expected string
	}{
		{"aws-node", "aws:///us-east-1a/i-abc123", "aws"},
		{"gcp-node", "gce:///project/zone/instance", "gcp"},
		{"azure-node", "azure:///subscriptions/abc/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1", "azure"},
		{"unknown", "", "unknown"},
	}

	for _, tt := range tests {
		nodes := makeNodesWithProviderID(tt.providerID)
		got := detectCloudVendor(nodes)
		if got != tt.expected {
			t.Errorf("detectCloudVendor(%q) = %q, want %q", tt.providerID, got, tt.expected)
		}
	}
}

func TestComputePortabilityScore(t *testing.T) {
	// All portable → high score
	s1 := PortabilitySummary{TotalWorkloads: 10, PortableWorkloads: 10}
	if score := computePortabilityScore(s1, 0); score < 90 {
		t.Errorf("expected score >= 90 for all portable, got %d", score)
	}

	// Half locked → lower score
	s2 := PortabilitySummary{TotalWorkloads: 10, PortableWorkloads: 5, LockedWorkloads: 5, CloudSpecificSC: 2, CloudAnnotations: 3}
	if score := computePortabilityScore(s2, 10); score > 70 {
		t.Errorf("expected score <= 70 for half locked, got %d", score)
	}
}

func TestGenerateMigrationPlan(t *testing.T) {
	// No issues → simple plan
	s1 := PortabilitySummary{}
	steps := generateMigrationPlan(s1, nil)
	if len(steps) != 1 {
		t.Errorf("expected 1 step for clean state, got %d", len(steps))
	}

	// Multiple issues → multiple steps
	s2 := PortabilitySummary{
		CloudSpecificSC:    2,
		CloudVolumes:       3,
		CloudAnnotations:   5,
		CloudNodeSelectors: 1,
		LockedWorkloads:    4,
	}
	steps = generateMigrationPlan(s2, nil)
	if len(steps) < 4 {
		t.Errorf("expected at least 4 steps, got %d", len(steps))
	}
	// Check priority ordering
	for i := 1; i < len(steps); i++ {
		if steps[i].Priority < steps[i-1].Priority {
			t.Error("migration steps not in priority order")
		}
	}
}

func TestGeneratePortabilityRecs(t *testing.T) {
	// No workloads
	s0 := PortabilitySummary{TotalWorkloads: 0}
	recs := generatePortabilityRecs(s0, nil, "aws")
	if len(recs) != 1 {
		t.Errorf("expected 1 rec for no workloads, got %d", len(recs))
	}

	// High lockin risk
	s1 := PortabilitySummary{
		TotalWorkloads:    10,
		PortableWorkloads: 3,
		LockedWorkloads:   7,
		PortabilityPct:    30,
		LockinRiskLevel:   "critical",
		CloudSpecificSC:   2,
		CloudVolumes:      3,
		CloudAnnotations:  5,
	}
	recs = generatePortabilityRecs(s1, nil, "aws")
	if len(recs) < 3 {
		t.Errorf("expected multiple recs for high lockin, got %d", len(recs))
	}
}
