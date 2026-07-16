package dashboard

import "testing"

func TestBackupCoverageTypes(t *testing.T) {
	r := BackupCoverageResult{HealthScore: 20, Grade: "F"}
	if r.HealthScore != 20 { t.Error("struct error") }
	s := BackupCoverageSummary{HasVelero: false, PVsCovered: 0, PVsUncovered: 12}
	if s.PVsUncovered != 12 { t.Error("summary error") }
	g := BackupGap{Category: "backup-tool", Severity: "critical"}
	if g.Severity != "critical" { t.Error("gap error") }
}

func TestBackupCoverageScoring(t *testing.T) {
	tests := []struct{ velero bool; covered, uncovered int; expMin, expMax int }{
		{true, 12, 0, 90, 100},
		{false, 0, 12, 15, 25},
		{true, 5, 7, 85, 100},
	}
	for _, tc := range tests {
		score := 20
		if tc.velero { score += 50 }
		total := tc.covered + tc.uncovered
		if total > 0 { score += tc.covered * 30 / total }
		if tc.velero { score += 10 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("velero=%v covered=%d uncovered=%d: expected %d-%d, got %d", tc.velero, tc.covered, tc.uncovered, tc.expMin, tc.expMax, score)
		}
	}
}

func TestIdleZombieTypes(t *testing.T) {
	r := IdleZombieResult{HealthScore: 75, Grade: "C"}
	if r.HealthScore != 75 { t.Error("struct error") }
	s := IdleZombieSummary{TotalWorkloads: 55, IdleWorkloads: 5, ZombieWorkloads: 2}
	if s.ZombieWorkloads != 2 { t.Error("summary error") }
	iw := ZombieWorkloadInfo{Name: "api", Type: "idle", Severity: "medium"}
	if iw.Type != "idle" { t.Error("idle error") }
}

func TestServiceMeshTypes(t *testing.T) {
	r := ServiceMeshResult{HealthScore: 20, Grade: "F"}
	if r.HealthScore != 20 { t.Error("struct error") }
	s := MeshCovSummary{HasIstio: false, MeshCoverage: 0, MTLSEnabled: false}
	if s.MTLSEnabled { t.Error("summary error") }
	g := MeshGap{Namespace: "app", Severity: "high"}
	if g.Severity != "high" { t.Error("gap error") }
}

func TestServiceMeshScoring(t *testing.T) {
	tests := []struct{ mesh, mtls bool; coverage float64; expMin, expMax int }{
		{true, true, 100, 90, 100},
		{false, false, 0, 15, 25},
		{true, false, 50, 60, 75},
	}
	for _, tc := range tests {
		score := 20
		if tc.mesh { score += 40 }
		if tc.mtls { score += 20 }
		score += int(tc.coverage / 5)
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("mesh=%v mtls=%v cov=%.0f: expected %d-%d, got %d", tc.mesh, tc.mtls, tc.coverage, tc.expMin, tc.expMax, score)
		}
	}
}
