package dashboard

import (
	"testing"
)

func TestExtractESVersion(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"ghcr.io/external-secrets/external-secrets:v0.9.20", "v0.9.20"},
		{"external-secrets:v0.9.20-ubi", "v0.9.20"},
		{"external-secrets", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractESVersion(tt.image); got != tt.expected {
			t.Errorf("extractESVersion(%q) = %q, want %q", tt.image, got, tt.expected)
		}
	}
}

func TestAssessESRisk(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"Ready", "healthy"},
		{"Synced", "healthy"},
		{"Failed", "critical"},
		{"error", "critical"},
		{"Unknown", "warning"},
		{"", "warning"},
		{"pending", "info"},
		{"Syncing", "info"},
	}
	for _, tt := range tests {
		entry := ExtSecretEntry{Status: tt.status}
		if got := assessESRisk(entry); got != tt.expected {
			t.Errorf("assessESRisk(status=%q) = %s, want %s", tt.status, got, tt.expected)
		}
	}
}

func TestComputeESHealthScore(t *testing.T) {
	// Nothing installed → 50
	score := computeESHealthScore(ExtSecretSummary{}, 0)
	if score != 50 {
		t.Fatalf("not-installed score = %d, want 50", score)
	}

	// All healthy
	score = computeESHealthScore(ExtSecretSummary{
		ESODetected:   true,
		PodCount:      2,
		ReadyPods:     2,
		TotalSecrets:  10,
		SyncedSecrets: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Not ready pods + failures
	score = computeESHealthScore(ExtSecretSummary{
		ESODetected:   true,
		PodCount:      3,
		ReadyPods:     1,
		FailedSecrets: 4,
	}, 3)
	if score > 50 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-50", score)
	}

	// Installed but no secrets
	score = computeESHealthScore(ExtSecretSummary{
		ESODetected:  true,
		PodCount:     1,
		ReadyPods:    1,
		TotalSecrets: 0,
	}, 0)
	if score > 100 {
		t.Fatalf("no-secrets score = %d, should be <= 100", score)
	}
	if score != 95 {
		t.Fatalf("no-secrets score = %d, want 95", score)
	}
}
