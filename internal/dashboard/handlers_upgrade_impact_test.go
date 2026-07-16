package dashboard

import (
	"testing"
)

func TestExtractMinorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"v1.28.3", 28},
		{"1.29.0", 29},
		{"v1.30.2+k3s1", 30},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := extractMinorVersion(tt.input)
		if got != tt.want {
			t.Errorf("extractMinorVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestComputeUpgradeScore(t *testing.T) {
	// Clean state → perfect score
	s0 := UpgradeImpactSummary{}
	if score := computeUpgradeScore(s0); score != 100 {
		t.Errorf("expected 100 for clean state, got %d", score)
	}

	// Blocked resources
	s1 := UpgradeImpactSummary{BlockedResources: 3}
	if score := computeUpgradeScore(s1); score >= 100 {
		t.Errorf("expected lower score with blocked resources, got %d", score)
	}

	// Outdated nodes
	s2 := UpgradeImpactSummary{NodeCount: 10, OutdatedNodes: 5}
	if score := computeUpgradeScore(s2); score > 90 {
		t.Errorf("expected lower score with outdated nodes, got %d", score)
	}

	// Incompatible addons
	s3 := UpgradeImpactSummary{AddonCount: 5, CompatibleAddons: 2}
	if score := computeUpgradeScore(s3); score > 90 {
		t.Errorf("expected lower score with incompatible addons, got %d", score)
	}
}

func TestUpgradeVerdict(t *testing.T) {
	if v := upgradeVerdict(80, UpgradeImpactSummary{}); v != "ready" {
		t.Errorf("expected 'ready' for score 80, got %q", v)
	}
	if v := upgradeVerdict(50, UpgradeImpactSummary{}); v != "caution" {
		t.Errorf("expected 'caution' for score 50, got %q", v)
	}
	if v := upgradeVerdict(80, UpgradeImpactSummary{BlockedResources: 1}); v != "blocked" {
		t.Errorf("expected 'blocked' with blocked resources, got %q", v)
	}
	if v := upgradeVerdict(30, UpgradeImpactSummary{}); v != "blocked" {
		t.Errorf("expected 'blocked' for score 30, got %q", v)
	}
}

func TestGenerateUpgradePlan(t *testing.T) {
	// Clean state → minimal plan
	actions := generateUpgradePlan(UpgradeImpactSummary{}, nil, nil, nil)
	if len(actions) < 1 {
		t.Error("expected at least 1 action even for clean state")
	}

	// With blocking changes
	bcs := []UpgradeBreakingChange{
		{Resource: "CronJob", Impact: "block", Mitigation: "Migrate to batch/v1"},
	}
	actions = generateUpgradePlan(UpgradeImpactSummary{BlockedResources: 1}, bcs, nil, nil)
	if len(actions) < 2 {
		t.Error("expected multiple actions with blocking changes")
	}

	// Check phase ordering
	found := false
	for _, a := range actions {
		if a.Phase == "pre-upgrade" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pre-upgrade phase action")
	}
}
