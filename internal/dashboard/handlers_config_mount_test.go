package dashboard

import "testing"

func TestAssessConfigMountRisk(t *testing.T) {
	// Healthy: optional, not large, no subpath
	entry := ConfigMountEntry{IsOptional: true, IsLarge: false, HasSubPath: false}
	if got := assessConfigMountRisk(entry); got != "healthy" {
		t.Errorf("healthy risk = %s, want healthy", got)
	}

	// Critical: not optional + large + subpath
	entry = ConfigMountEntry{IsOptional: false, IsLarge: true, HasSubPath: true}
	if got := assessConfigMountRisk(entry); got != "critical" {
		t.Errorf("critical risk = %s, want critical", got)
	}

	// Warning: large only
	entry = ConfigMountEntry{IsOptional: true, IsLarge: true, HasSubPath: false}
	if got := assessConfigMountRisk(entry); got != "warning" {
		t.Errorf("large risk = %s, want warning", got)
	}
}

func TestComputeConfigMountScore(t *testing.T) {
	score := computeConfigMountScore(ConfigMountSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	score = computeConfigMountScore(ConfigMountSummary{
		TotalPods:       20,
		MissingCMRefs:   2,
		LargeConfigMaps: 3,
		NoOptionalFlag:  5,
		SubPathMounts:   2,
	}, 8)
	if score > 45 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-45", score)
	}

	score = computeConfigMountScore(ConfigMountSummary{
		TotalPods:       20,
		ConfigMapMounts: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("healthy score = %d, want 100", score)
	}
}
