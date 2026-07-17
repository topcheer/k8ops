package dashboard

import (
	"strings"
	"testing"
)

func TestBuildQuotaGenRecs(t *testing.T) {
	r := &QuotaGeneratorResult{Summary: QuotaGenSummary{MissingQuota: 5, TotalNamespaces: 10}}
	recs := buildQuotaGenRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildProbeGenRecs(t *testing.T) {
	r := &ProbeGeneratorResult{Summary: ProbeGenSummary{MissingBoth: 3, MissingLiveness: 1}}
	recs := buildProbeGenRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestGenerateProbePatchJSON(t *testing.T) {
	p := generateProbePatchJSON("app", "both", 8080)
	if p == "" {
		t.Error("expected non-empty patch")
	}
	if !strings.Contains(p, "livenessProbe") || !strings.Contains(p, "readinessProbe") {
		t.Error("expected both probes in patch")
	}
}

func TestInsightStatus(t *testing.T) {
	if insightStatus(90) != "healthy" {
		t.Error("expected healthy")
	}
	if insightStatus(65) != "warning" {
		t.Error("expected warning")
	}
	if insightStatus(45) != "at-risk" {
		t.Error("expected at-risk")
	}
	if insightStatus(20) != "critical" {
		t.Error("expected critical")
	}
}

func TestBuildInsightsRecs(t *testing.T) {
	r := &PlatformInsightsResult{
		OverallScore: 35, Grade: "D",
		Categories: []InsightCategory{
			{Name: "Availability", Score: 20, Detail: "1 node"},
		},
		CriticalAlerts: []InsightAlert{{Category: "Availability", Severity: "critical"}},
	}
	recs := buildInsightsRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
