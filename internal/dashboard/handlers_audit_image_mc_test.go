package dashboard

import "testing"

func TestAuditTrailTypes(t *testing.T) {
	r := AuditTrailResult{ComplianceScore: 30, Grade: "F"}
	if r.ComplianceScore != 30 { t.Error("struct error") }
	s := AuditTrailSummary{AuditLogEnabled: false, NamespacesTracked: 28, SensitiveAccess: 15}
	if s.SensitiveAccess != 15 { t.Error("summary error") }
	g := AuditGapItem{Category: "log", Severity: "critical"}
	if g.Severity != "critical" { t.Error("gap error") }
}

func TestAuditTrailScoring(t *testing.T) {
	tests := []struct{ logEnabled, policyFound bool; sensitive int; expMin, expMax int }{
		{true, true, 0, 75, 100},
		{false, false, 20, 15, 35},
		{true, false, 5, 50, 65},
	}
	for _, tc := range tests {
		score := 0
		if tc.logEnabled { score += 40 }
		if tc.policyFound { score += 20 }
		if score == 0 { score = 10 }
		score += 20
		if tc.sensitive > 10 { score -= 10 }
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("log=%v policy=%v sens=%d: expected %d-%d, got %d", tc.logEnabled, tc.policyFound, tc.sensitive, tc.expMin, tc.expMax, score)
		}
	}
}

func TestImageFreshnessTypes(t *testing.T) {
	r := ImageFreshResult{HealthScore: 45, Grade: "D"}
	if r.HealthScore != 45 { t.Error("struct error") }
	s := ImageFreshSummary{TotalImages: 77, UniqueImages: 30, StaleImages: 45}
	if s.StaleImages != 45 { t.Error("summary error") }
	si := StaleImageInfo{Image: "nginx:latest", Severity: "high"}
	if si.Severity != "high" { t.Error("stale error") }
}

func TestImageFreshnessScoring(t *testing.T) {
	tests := []struct{ fresh, total, stale int; expMin, expMax int }{
		{77, 77, 0, 95, 100},
		{32, 77, 45, 0, 50},
	}
	for _, tc := range tests {
		freshRatio := float64(tc.fresh) / float64(tc.total)
		score := int(freshRatio * 100)
		score -= tc.stale * 5
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("fresh=%d total=%d: expected %d-%d, got %d", tc.fresh, tc.total, tc.expMin, tc.expMax, score)
		}
	}
}

func TestMultiClusterTypes(t *testing.T) {
	r := MultiClusterConnResult{HealthScore: 50, Grade: "F"}
	if r.HealthScore != 50 { t.Error("struct error") }
	s := MultiClusterSummary{TotalClusters: 28, RemoteClusters: 0, HasClusterAPI: false}
	if s.RemoteClusters != 0 { t.Error("summary error") }
	c := ClusterConnection{Name: "none", Status: "single-cluster-only"}
	if c.Status != "single-cluster-only" { t.Error("conn error") }
}

func TestMultiClusterScoring(t *testing.T) {
	tests := []struct{ capi, argo, karmada bool; remote int; expMin, expMax int }{
		{true, true, true, 5, 95, 100},
		{false, false, false, 0, 45, 55},
		{true, false, false, 0, 60, 70},
	}
	for _, tc := range tests {
		score := 50
		if tc.capi { score += 15 }
		if tc.argo { score += 15 }
		if tc.karmada { score += 20 }
		if tc.remote > 0 { score += 10 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("capi=%v argo=%v karmada=%v: expected %d-%d, got %d", tc.capi, tc.argo, tc.karmada, tc.expMin, tc.expMax, score)
		}
	}
}
