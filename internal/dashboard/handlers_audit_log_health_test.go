package dashboard

import (
	"testing"
)

func TestComputeAuditLogScore(t *testing.T) {
	// No exporters → significant penalty
	score := computeAuditLogScore(AuditLogSummary{}, 0)
	if score != 80 {
		t.Fatalf("no-exporter score = %d, want 80", score)
	}

	// All healthy with exporters
	score = computeAuditLogScore(AuditLogSummary{
		FluentBitDetected: true,
		ExporterPodCount:  3,
		ReadyExporters:    3,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Not ready exporters + high event rate
	score = computeAuditLogScore(AuditLogSummary{
		FluentBitDetected: true,
		ExporterPodCount:  4,
		ReadyExporters:    1,
		HighEventRate:     2,
	}, 5)
	if score > 60 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-60", score)
	}

	// Score never below 0
	score = computeAuditLogScore(AuditLogSummary{
		ExporterPodCount: 10,
		ReadyExporters:   0,
		HighEventRate:    5,
	}, 20)
	if score < 0 {
		t.Fatalf("score should not be negative, got %d", score)
	}
}
