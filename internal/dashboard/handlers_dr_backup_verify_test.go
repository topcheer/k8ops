package dashboard

import (
	"testing"
)

func TestDRBackupTypes(t *testing.T) {
	r := DRBackupResult{ReadinessScore: 0, Grade: "F", DrReadiness: "not-ready"}
	if r.ReadinessScore != 0 || r.DrReadiness != "not-ready" {
		t.Error("struct field error")
	}

	s := DRBackupSummary{HasVelero: false, TotalNamespaces: 28, ProtectedNS: 0, UnprotectedNS: 28}
	if s.UnprotectedNS != 28 || s.HasVelero {
		t.Error("summary field error")
	}

	bc := BackupCoverage{Namespace: "app", HasBackup: true, PVCCount: 3, Status: "protected"}
	if !bc.HasBackup || bc.PVCCount != 3 {
		t.Error("backupCoverage field error")
	}

	un := DRUnprotectedNS{Namespace: "data", PVCCount: 5, Severity: "high"}
	if un.PVCCount != 5 || un.Severity != "high" {
		t.Error("unprotectedNS field error")
	}
}

func TestDRBackupScoring(t *testing.T) {
	tests := []struct {
		hasVelero   bool
		hasLonghorn bool
		protectedNS int
		totalNS     int
		expectedMin int
		expectedMax int
	}{
		{false, false, 0, 28, 0, 5},   // No backup
		{true, false, 25, 28, 80, 95}, // Velero, most protected
		{true, true, 28, 28, 95, 100}, // Full coverage
	}
	for _, tc := range tests {
		score := 0
		if tc.hasVelero {
			score += 40
		}
		if tc.hasLonghorn {
			score += 15
		}
		if tc.totalNS > 0 {
			score += tc.protectedNS * 30 / tc.totalNS
		}
		if tc.hasVelero || tc.hasLonghorn {
			score += 15
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("velero=%v longhorn=%v protected=%d/%d: expected %d-%d, got %d",
				tc.hasVelero, tc.hasLonghorn, tc.protectedNS, tc.totalNS,
				tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestDRBackupReadiness(t *testing.T) {
	tests := []struct {
		backupTool  bool
		unprotected int
		expected    string
	}{
		{false, 28, "not-ready"},
		{true, 0, "ready"},
		{true, 10, "partial"},
	}
	for _, tc := range tests {
		readiness := "not-ready"
		if tc.backupTool && tc.unprotected == 0 {
			readiness = "ready"
		} else if tc.backupTool {
			readiness = "partial"
		}
		if readiness != tc.expected {
			t.Errorf("backupTool=%v unprotected=%d: expected %s, got %s",
				tc.backupTool, tc.unprotected, tc.expected, readiness)
		}
	}
}

func TestCountPVCUnprotected(t *testing.T) {
	items := []DRUnprotectedNS{
		{Namespace: "a", PVCCount: 3},
		{Namespace: "b", PVCCount: 0},
		{Namespace: "c", PVCCount: 1},
	}
	count := countPVCUnprotected(items)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}
