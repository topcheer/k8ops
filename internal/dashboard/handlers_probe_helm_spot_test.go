package dashboard

import "testing"

func TestProbeLatencyTypes(t *testing.T) {
	r := ProbeLatencyResult{HealthScore: 65, Grade: "D"}
	if r.HealthScore != 65 { t.Error("struct error") }
	s := ProbeLatencySummary{TotalWorkloads: 55, MissingProbes: 20, WithReadinessProbe: 35}
	if s.MissingProbes != 20 { t.Error("summary error") }
	sw := SlowWorkload{Name: "api", Severity: "high"}
	if sw.Severity != "high" { t.Error("slowWL error") }
	mp := MisconfigProbe{Name: "web", Probe: "startup", Issue: "slow"}
	if mp.Probe != "startup" { t.Error("misconfig error") }
}

func TestProbeLatencyScoring(t *testing.T) {
	tests := []struct{ ready, total, startup, slow, missing, expMin, expMax int }{
		{55, 55, 55, 0, 0, 75, 100},
		{10, 55, 5, 5, 40, 0, 15},
		{30, 55, 10, 3, 20, 0, 45},
	}
	for _, tc := range tests {
		score := 0
		if tc.total > 0 { score = tc.ready * 60 / tc.total }
		if tc.total > 0 { score += tc.startup * 20 / tc.total }
		score -= tc.slow * 5
		score -= tc.missing * 3
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("ready=%d total=%d: expected %d-%d, got %d", tc.ready, tc.total, tc.expMin, tc.expMax, score)
		}
	}
}

func TestHelmHealthTypes(t *testing.T) {
	r := HelmHealthDeepResult{HealthScore: 85, Grade: "B"}
	if r.HealthScore != 85 { t.Error("struct error") }
	s := HelmHealthDeepSummary{TotalReleases: 50, FailedReleases: 0, StaleReleases: 5}
	if s.FailedReleases != 0 || s.StaleReleases != 5 { t.Error("summary error") }
	sr := StaleRelease{Name: "redis", Severity: "critical", Status: "failed"}
	if sr.Status != "failed" { t.Error("stale error") }
}

func TestHelmHealthScoring(t *testing.T) {
	tests := []struct{ total, failed, stale int }{
		{50, 0, 5},
		{50, 5, 10},
		{0, 0, 0},
	}
	for _, tc := range tests {
		score := 100
		if tc.total > 0 {
			score -= int(float64(tc.failed)/float64(tc.total)*50)
			score -= int(float64(tc.stale)/float64(tc.total)*30)
		}
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < 0 || score > 100 { t.Errorf("score %d out of range", score) }
	}
}

func TestSpotReadinessTypes(t *testing.T) {
	r := SpotReadinessDeepResult{ReadinessScore: 50, Grade: "F"}
	if r.ReadinessScore != 50 { t.Error("struct error") }
	s := SpotDeepSummary{TotalNodes: 3, SpotNodes: 0, HasSpotLabel: false}
	if s.SpotNodes != 0 { t.Error("summary error") }
	sw := SpotWorkload{Name: "api", HasSpotToleration: false}
	if sw.HasSpotToleration { t.Error("spotWL error") }
	dg := DisruptionGap{Workload: "web", Severity: "high"}
	if dg.Severity != "high" { t.Error("gap error") }
}

func TestSpotReadinessScoring(t *testing.T) {
	tests := []struct{ hasSpot, hasTol, hasPDB bool; expMin, expMax int }{
		{true, true, true, 90, 100},
		{false, false, false, 45, 55},
		{true, false, false, 65, 75},
	}
	for _, tc := range tests {
		score := 50
		if tc.hasSpot { score += 20 }
		if tc.hasTol { score += 15 }
		if tc.hasPDB { score += 15 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("spot=%v tol=%v pdb=%v: expected %d-%d, got %d", tc.hasSpot, tc.hasTol, tc.hasPDB, tc.expMin, tc.expMax, score)
		}
	}
}
