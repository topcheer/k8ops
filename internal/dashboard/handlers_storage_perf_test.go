package dashboard

import (
	"testing"
)

func TestClassifyStorageTier(t *testing.T) {
	tests := []struct {
		provisioner string
		scName      string
		expected    string
	}{
		{"ebs.csi.aws.com", "gp3", "fast"},
		{"kubernetes.io/aws-ebs", "io1", "fast"},
		{"kubernetes.io/gce-pd", "pd-ssd", "fast"},
		{"kubernetes.io/aws-ebs", "gp2", "standard"},
		{"rancher.io/local-path", "local-path", "standard"},
		{"kubernetes.io/aws-ebs", "st1", "slow"},
		{"unknown.provisioner", "mystery", "unknown"},
		{"kubernetes.io/azure-disk", "managed-premium", "fast"},
	}

	for _, tt := range tests {
		got := classifyStorageTier(tt.provisioner, tt.scName)
		if got != tt.expected {
			t.Errorf("classifyStorageTier(%q, %q) = %q, want %q", tt.provisioner, tt.scName, got, tt.expected)
		}
	}
}

func TestInferWorkloadType(t *testing.T) {
	tests := []struct {
		name     string
		ns       string
		labels   map[string]string
		expected string
	}{
		{"postgres-main", "db", nil, "database"},
		{"redis-cache", "prod", nil, "database"},
		{"my-app", "default", map[string]string{"app": "mysql"}, "database"},
		{"api-server", "prod", nil, "general"},
		{"kafka-broker", "messaging", nil, "message-queue"},
		{"fluentd-logger", "logging", nil, "logging"},
	}

	for _, tt := range tests {
		got := inferWorkloadType(tt.name, tt.ns, tt.labels)
		if got != tt.expected {
			t.Errorf("inferWorkloadType(%q, %q, %v) = %q, want %q", tt.name, tt.ns, tt.labels, got, tt.expected)
		}
	}
}

func TestAssessStorageMismatch(t *testing.T) {
	tests := []struct {
		current  string
		expected string
		want     string
	}{
		{"slow", "fast", "critical"},
		{"standard", "fast", "warning"},
		{"fast", "fast", ""},
		{"standard", "standard", ""},
		{"unknown", "fast", ""},
		{"slow", "standard", "warning"},
	}

	for _, tt := range tests {
		got := assessStorageMismatch(tt.current, tt.expected)
		if got != tt.want {
			t.Errorf("assessStorageMismatch(%q, %q) = %q, want %q", tt.current, tt.expected, got, tt.want)
		}
	}
}

func TestComputeStoragePerfScore(t *testing.T) {
	// No PVCs → perfect score
	s0 := StoragePerfSummary{TotalPVCs: 0}
	if score := computeStoragePerfScore(s0); score != 100 {
		t.Errorf("expected 100 for no PVCs, got %d", score)
	}

	// All good → high score
	s1 := StoragePerfSummary{TotalPVCs: 10, FastTierPVCs: 8, StandardTierPVCs: 2}
	if score := computeStoragePerfScore(s1); score < 90 {
		t.Errorf("expected score >= 90 for good config, got %d", score)
	}

	// Many mismatches → low score
	s2 := StoragePerfSummary{TotalPVCs: 10, MismatchedPVCs: 8, StandardTierPVCs: 2}
	if score := computeStoragePerfScore(s2); score > 70 {
		t.Errorf("expected score <= 70 for many mismatches, got %d", score)
	}

	// Many unknown tiers
	s3 := StoragePerfSummary{TotalPVCs: 10, UnknownTierPVCs: 8}
	if score := computeStoragePerfScore(s3); score > 85 {
		t.Errorf("expected score <= 85 for unknown tiers, got %d", score)
	}
}

func TestExpectedTierForWorkloadType(t *testing.T) {
	if tier := expectedTierForWorkloadType("database"); tier != "fast" {
		t.Errorf("expected 'fast' for database, got %q", tier)
	}
	if tier := expectedTierForWorkloadType("general"); tier != "standard" {
		t.Errorf("expected 'standard' for general, got %q", tier)
	}
}
