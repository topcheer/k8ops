package dashboard

import (
	"testing"
)

func TestBuildRestartAnalysisRecs(t *testing.T) {
	r := &RestartAnalyzerResult{
		StabilityScore: 40, Grade: "C",
		Summary: RestartAnalysisSummary{AvgRestarts: 4.5, OOMKills: 2, CrashLoops: 1, ProbeFailures: 3, RestartingPods: 6},
	}
	recs := buildRestartAnalysisRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}

func TestBuildEnvLeakRecs(t *testing.T) {
	r := &EnvLeakScannerResult{
		Summary:     EnvLeakSummary{PlaintextLeaks: 10, HighRisk: 3},
		TopPatterns: []EnvLeakPatternStat{{Pattern: "password", Count: 5, Example: "DB_PASSWORD"}},
	}
	recs := buildEnvLeakRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestClassifyEnvLeak(t *testing.T) {
	if p := classifyEnvLeak("DB_PASSWORD", "secret123"); p != "password" {
		t.Errorf("expected 'password', got '%s'", p)
	}
	if p := classifyEnvLeak("API_KEY", "abc"); p != "api-key" {
		t.Errorf("expected 'api-key', got '%s'", p)
	}
	if p := classifyEnvLeak("APP_NAME", "myapp"); p != "" {
		t.Errorf("expected empty for non-sensitive, got '%s'", p)
	}
}

func TestMaskValue(t *testing.T) {
	if v := maskValue("abc123456"); v != "abc***" {
		t.Errorf("expected 'abc***', got '%s'", v)
	}
	if v := maskValue("ab"); v != "***" {
		t.Errorf("expected '***', got '%s'", v)
	}
}

func TestBuildStrategyAuditRecs(t *testing.T) {
	r := &UpdateStrategyAuditorResult{
		Summary: StrategyAuditSummary{RiskyStrategy: 2, NoMaxSurge: 5, OldRevisionLimit: 10},
	}
	recs := buildStrategyAuditRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
