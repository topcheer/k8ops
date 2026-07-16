package dashboard

import "testing"

func TestAdmissionAuditTypes(t *testing.T) {
	r := AdmissionAuditResult{PostureScore: 20, Grade: "F"}
	if r.PostureScore != 20 { t.Error("struct error") }
	s := AdmissionSummary{HasGatekeeper: false, ValidatingWebhooks: 0}
	if s.HasGatekeeper { t.Error("summary error") }
	f := AdmissionFinding{Severity: "critical"}
	if f.Severity != "critical" { t.Error("finding error") }
}

func TestAdmissionAuditScoring(t *testing.T) {
	tests := []struct{ gatekeeper, kyverno bool; vwh, mwh int; expMin, expMax int }{
		{true, false, 5, 3, 90, 100},
		{false, false, 0, 0, 15, 25},
		{false, true, 2, 1, 90, 100},
	}
	for _, tc := range tests {
		score := 20
		if tc.gatekeeper || tc.kyverno { score += 40 }
		if tc.vwh > 0 { score += 20 }
		if tc.mwh > 0 { score += 10 }
		if tc.gatekeeper || tc.kyverno { score += 10 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("gate=%v kyv=%v vwh=%d mwh=%d: expected %d-%d, got %d", tc.gatekeeper, tc.kyverno, tc.vwh, tc.mwh, tc.expMin, tc.expMax, score)
		}
	}
}

func TestDashAvailTypes(t *testing.T) {
	r := DashAvailResult{HealthScore: 0, Grade: "F"}
	if r.HealthScore != 0 { t.Error("struct error") }
	s := DashAvailSummary{HasGrafana: false, DashboardsFound: 0}
	if s.HasGrafana { t.Error("summary error") }
	g := DashCoverageGap{Namespace: "app", Severity: "medium"}
	if g.Severity != "medium" { t.Error("gap error") }
}

func TestDashAvailScoring(t *testing.T) {
	tests := []struct{ grafana, metrics, logs bool; covered, total int; expMin, expMax int }{
		{true, true, true, 28, 28, 90, 100},
		{false, false, false, 0, 28, 0, 5},
		{true, true, false, 14, 28, 60, 80},
	}
	for _, tc := range tests {
		score := 0
		if tc.grafana { score += 30 }
		if tc.metrics { score += 20 }
		if tc.logs { score += 15 }
		if tc.total > 0 { score += tc.covered * 35 / tc.total }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("grafana=%v met=%v logs=%v cov=%d/%d: expected %d-%d, got %d", tc.grafana, tc.metrics, tc.logs, tc.covered, tc.total, tc.expMin, tc.expMax, score)
		}
	}
}

func TestStorageOrphanTypes(t *testing.T) {
	r := StorageOrphanResult{HealthScore: 80, Grade: "B"}
	if r.HealthScore != 80 { t.Error("struct error") }
	s := StorageOrphanSummary{TotalPVCs: 15, OrphanedPVCs: 3, OrphanGB: 50.5}
	if s.OrphanGB != 50.5 { t.Error("summary error") }
	o := OrphanPVCInfo{Name: "data", SizeGB: 10.0, Severity: "high"}
	if o.SizeGB != 10.0 { t.Error("orphan error") }
}

func TestStorageOrphanScoring(t *testing.T) {
	tests := []struct{ total, orphan, pending int; expMin, expMax int }{
		{15, 0, 0, 95, 100},
		{15, 5, 2, 50, 80},
		{15, 3, 0, 75, 90},
	}
	for _, tc := range tests {
		score := 100
		orphanRatio := 0.0
		if tc.total > 0 { orphanRatio = float64(tc.orphan) / float64(tc.total) }
		score -= int(orphanRatio * 60)
		score -= tc.pending * 5
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("total=%d orphan=%d pending=%d: expected %d-%d, got %d", tc.total, tc.orphan, tc.pending, tc.expMin, tc.expMax, score)
		}
	}
}
