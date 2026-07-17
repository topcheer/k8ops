package dashboard

import "testing"

func TestGitOpsSyncTypes(t *testing.T) {
	r := GitOpsSyncResult{HealthScore: 30, Grade: "F"}
	if r.HealthScore != 30 {
		t.Error("struct error")
	}
	s := GitOpsSummary{HasArgoCD: false, TotalApps: 55, OutOfSyncCount: 55}
	if s.OutOfSyncCount != 55 {
		t.Error("summary error")
	}
	o := OutOfSyncApp{Name: "api", Severity: "medium"}
	if o.Severity != "medium" {
		t.Error("app error")
	}
}

func TestGitOpsSyncScoring(t *testing.T) {
	tests := []struct {
		hasTool        bool
		total, synced  int
		expMin, expMax int
	}{
		{true, 10, 10, 95, 100},
		{false, 0, 0, 25, 35},
		{true, 10, 5, 75, 90},
	}
	for _, tc := range tests {
		score := 30
		if tc.hasTool {
			score += 40
		}
		if tc.total > 0 {
			score += tc.synced * 30 / tc.total
		}
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("tool=%v total=%d synced=%d: expected %d-%d, got %d", tc.hasTool, tc.total, tc.synced, tc.expMin, tc.expMax, score)
		}
	}
}

func TestEndpointProbeTypes(t *testing.T) {
	r := EndpointProbeResult{HealthScore: 75, Grade: "C"}
	if r.HealthScore != 75 {
		t.Error("struct error")
	}
	s := EndpointProbeSummary{TotalServices: 84, HealthySvcs: 78, NoBackendSvcs: 3}
	if s.NoBackendSvcs != 3 {
		t.Error("summary error")
	}
	u := UnhealthyEndpoint{Service: "api", Severity: "high"}
	if u.Severity != "high" {
		t.Error("unhealthy error")
	}
}

func TestEndpointProbeScoring(t *testing.T) {
	tests := []struct {
		total, healthy int
		expMin, expMax int
	}{
		{84, 84, 95, 100},
		{84, 40, 40, 55},
	}
	for _, tc := range tests {
		score := int(float64(tc.healthy) / float64(tc.total) * 100)
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("total=%d healthy=%d: expected %d-%d, got %d", tc.total, tc.healthy, tc.expMin, tc.expMax, score)
		}
	}
}

func TestNodeDecommTypes(t *testing.T) {
	r := NodeDecommResult{HealthScore: 40, Grade: "F"}
	if r.HealthScore != 40 {
		t.Error("struct error")
	}
	s := NodeDecommSummary{TotalNodes: 3, OldNodes: 1, RotationUrgency: "medium"}
	if s.OldNodes != 1 || s.RotationUrgency != "medium" {
		t.Error("summary error")
	}
	nr := NodeRotation{Name: "node1", Severity: "high"}
	if nr.Severity != "high" {
		t.Error("rotation error")
	}
}

func TestNodeDecommScoring(t *testing.T) {
	tests := []struct {
		notReady, oldNodes int
		expMin, expMax     int
	}{
		{0, 0, 95, 100},
		{1, 2, 40, 60},
		{0, 1, 80, 95},
	}
	for _, tc := range tests {
		score := 100 - tc.notReady*30 - tc.oldNodes*10
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("notReady=%d old=%d: expected %d-%d, got %d", tc.notReady, tc.oldNodes, tc.expMin, tc.expMax, score)
		}
	}
}
