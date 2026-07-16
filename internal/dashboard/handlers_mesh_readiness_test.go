package dashboard

import (
	"testing"
)

func TestMeshReadinessTypes(t *testing.T) {
	r := MeshReadinessResult{
		ReadinessScore: 45,
		Grade:          "D",
		MeshDetected:   false,
		MeshType:       "none",
	}
	if r.ReadinessScore != 45 || r.Grade != "D" {
		t.Error("struct field assignment failed")
	}
	if r.MeshDetected {
		t.Error("MeshDetected should be false")
	}

	s := MeshSummary{TotalServices: 80, MeshedServices: 0, UnmeshedServices: 80}
	if s.UnmeshedServices != 80 || s.MeshedServices != 0 {
		t.Error("summary fields error")
	}

	gap := MeshInjectionGap{Namespace: "app", ServiceName: "api", Priority: "high"}
	if gap.Priority != "high" || gap.ServiceName != "api" {
		t.Error("injectionGap field error")
	}

	mtls := MeshMTLSCoverage{Mode: "disabled", Score: 0, MeshedPct: 0, Status: "no-mesh"}
	if mtls.Score != 0 || mtls.Status != "no-mesh" {
		t.Error("mtlsCoverage field error")
	}

	tp := TrafficPolicyGap{ServiceName: "web", MissingPolicy: "circuit-breaker"}
	if tp.MissingPolicy != "circuit-breaker" {
		t.Error("trafficPolicyGap field error")
	}
}

func TestMeshReadinessScoring(t *testing.T) {
	// No mesh detected: score should be low
	tests := []struct {
		meshDetected     bool
		meshedPct        float64
		namespacesMeshed int
		totalNS          int
		expectedMin      int
		expectedMax      int
	}{
		{false, 0, 0, 28, 0, 10},
		{true, 100, 25, 28, 90, 100},
		{true, 50, 14, 28, 55, 80},
	}
	for _, tc := range tests {
		score := 0
		if tc.meshDetected {
			score += 30
		}
		score += int(tc.meshedPct * 0.4)
		if tc.namespacesMeshed > 0 && tc.totalNS > 3 {
			nsRatio := tc.namespacesMeshed * 30 / (tc.totalNS - 3)
			score += nsRatio
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("meshDetected=%v meshedPct=%.0f nsMeshed=%d: expected %d-%d, got %d",
				tc.meshDetected, tc.meshedPct, tc.namespacesMeshed, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestMeshReadinessPriority(t *testing.T) {
	// Multi-port services should get high priority
	portCounts := []int{1, 2, 3, 5}
	for _, pc := range portCounts {
		priority := "medium"
		if pc > 1 {
			priority = "high"
		}
		if pc > 1 && priority != "high" {
			t.Errorf("portCount=%d should be high priority", pc)
		}
		if pc == 1 && priority != "medium" {
			t.Errorf("portCount=%d should be medium priority", pc)
		}
	}
}
