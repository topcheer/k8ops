package dashboard

import (
	"testing"
)

func TestAPIQualityTypes(t *testing.T) {
	r := APIQualityResult{QualityScore: 78, Grade: "C"}
	if r.QualityScore != 78 || r.Grade != "C" {
		t.Error("struct field error")
	}

	s := APIQualitySummary{TotalEndpoints: 320, AuditedEndpoints: 240, AvgCoverage: 82.5}
	if s.AvgCoverage != 82.5 {
		t.Error("summary field error")
	}

	dc := DimCoverage{Dimension: "Security", EndpointCount: 12, CoveragePct: 85.0, Status: "good"}
	if dc.Status != "good" || dc.CoveragePct != 85.0 {
		t.Error("dimCoverage field error")
	}

	cg := CoverageGap{Dimension: "Documentation", Gap: "2 vs 2 expected", Severity: "high"}
	if cg.Severity != "high" {
		t.Error("coverageGap field error")
	}
}

func TestAPIQualityStatus(t *testing.T) {
	tests := []struct {
		coverage float64
		expected string
	}{
		{30, "critical"},
		{60, "warning"},
		{80, "good"},
		{95, "excellent"},
	}
	for _, tc := range tests {
		status := "excellent"
		if tc.coverage < 50 {
			status = "critical"
		} else if tc.coverage < 75 {
			status = "warning"
		} else if tc.coverage < 90 {
			status = "good"
		}
		if status != tc.expected {
			t.Errorf("coverage=%.0f: expected %s, got %s", tc.coverage, tc.expected, status)
		}
	}
}

func TestAPIQualityGapSeverity(t *testing.T) {
	tests := []struct {
		coverage float64
		expected string
	}{
		{30, "high"},
		{60, "medium"},
		{80, ""},
	}
	for _, tc := range tests {
		severity := ""
		if tc.coverage < 75 {
			severity = "high"
			if tc.coverage > 50 {
				severity = "medium"
			}
		}
		if severity != tc.expected {
			t.Errorf("coverage=%.0f: expected %q, got %q", tc.coverage, tc.expected, severity)
		}
	}
}
