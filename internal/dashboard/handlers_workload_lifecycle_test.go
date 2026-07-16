package dashboard

import (
	"testing"
)

func TestClassifyLifecycleStage(t *testing.T) {
	tests := []struct {
		name       string
		ns         string
		labels     map[string]string
		ageDays    int
		replicas   int
		wantStage  string
	}{
		{"my-app", "production", map[string]string{"env": "prod"}, 30, 3, "production"},
		{"api-server", "prod-backend", nil, 60, 5, "production"},
		{"staging-app", "staging", nil, 15, 2, "staging"},
		{"dev-tool", "dev", nil, 5, 1, "development"},
		{"old-service", "legacy-apps", nil, 400, 1, "legacy"},
		{"v1-old-api", "default", map[string]string{"deprecated": "true"}, 200, 1, "deprecated"},
	}

	for _, tt := range tests {
		stage, conf, signals := classifyLifecycleStage(tt.name, tt.ns, tt.labels, nil, tt.ageDays, tt.replicas)
		if stage != tt.wantStage {
			t.Errorf("classifyLifecycleStage(%q, %q) = %q (conf %d, signals %v), want %q",
				tt.name, tt.ns, stage, conf, signals, tt.wantStage)
		}
		if conf <= 0 {
			t.Errorf("confidence should be > 0, got %d", conf)
		}
		if len(signals) == 0 {
			t.Errorf("should have at least 1 signal for %q", tt.name)
		}
	}
}

func TestStageToPriority(t *testing.T) {
	tests := []struct {
		stage string
		want  string
	}{
		{"production", "P0"},
		{"staging", "P1"},
		{"development", "P2"},
		{"deprecated", "P3"},
		{"legacy", "P3"},
		{"unknown", "P2"},
	}
	for _, tt := range tests {
		got := stageToPriority(tt.stage)
		if got != tt.want {
			t.Errorf("stageToPriority(%q) = %q, want %q", tt.stage, got, tt.want)
		}
	}
}

func TestStageToRisk(t *testing.T) {
	if risk := stageToRisk("production", 10); risk != "critical" {
		t.Errorf("expected 'critical' for production, got %q", risk)
	}
	if risk := stageToRisk("legacy", 400); risk != "high" {
		t.Errorf("expected 'high' for legacy 400d, got %q", risk)
	}
	if risk := stageToRisk("deprecated", 100); risk != "medium" {
		t.Errorf("expected 'medium' for deprecated 100d, got %q", risk)
	}
}

func TestComputeLifecycleScore(t *testing.T) {
	// No workloads → perfect
	s0 := WLLifecycleSummary{TotalWorkloads: 0}
	if score := computeLifecycleScore(s0); score != 100 {
		t.Errorf("expected 100 for no workloads, got %d", score)
	}

	// Good mix
	s1 := WLLifecycleSummary{TotalWorkloads: 10, Production: 5, Staging: 3, Development: 2}
	if score := computeLifecycleScore(s1); score < 80 {
		t.Errorf("expected score >= 80 for good mix, got %d", score)
	}

	// Lots of deprecated/legacy
	s2 := WLLifecycleSummary{TotalWorkloads: 10, Production: 2, Deprecated: 4, Legacy: 3, StaleWorkloads: 5}
	if score := computeLifecycleScore(s2); score > 70 {
		t.Errorf("expected score <= 70 for many deprecated, got %d", score)
	}
}

func TestGenerateWLLifecycleRecs(t *testing.T) {
	// No workloads
	s0 := WLLifecycleSummary{TotalWorkloads: 0}
	recs := generateWLLifecycleRecs(s0, nil, nil)
	if len(recs) != 1 {
		t.Errorf("expected 1 rec for no workloads, got %d", len(recs))
	}

	// With cleanup candidates
	s1 := WLLifecycleSummary{
		TotalWorkloads:    10,
		Production:        4,
		Staging:           2,
		Development:       2,
		Deprecated:        1,
		Legacy:            1,
		CleanupCandidates: 2,
		StaleWorkloads:    1,
	}
	targets := []CleanupTarget{
		{Name: "old-svc", Namespace: "default", Stage: "deprecated", Action: "archive"},
	}
	stages := []StageStat{
		{Stage: "production", Count: 4, Pct: 40},
		{Stage: "deprecated", Count: 1, Pct: 10},
	}
	recs = generateWLLifecycleRecs(s1, targets, stages)
	if len(recs) < 3 {
		t.Errorf("expected multiple recs, got %d", len(recs))
	}
}

func TestInferWorkloadTypeByLabel(t *testing.T) {
	// Label with database indicator
	wt := inferWorkloadType("app", "default", map[string]string{"app": "postgres", "tier": "database"})
	if wt != "database" {
		t.Errorf("expected 'database', got %q", wt)
	}
}
