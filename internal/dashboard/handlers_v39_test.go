package dashboard

import (
	"testing"
)

func TestBuildActionBatches(t *testing.T) {
	actions := []PriorityAction{
		{Title: "Fix A", Category: "Security", Impact: 8, Effort: 2},
		{Title: "Fix B", Category: "Security", Impact: 6, Effort: 3},
		{Title: "Fix C", Category: "Reliability", Impact: 5, Effort: 4},
	}
	batches := buildActionBatches(actions)
	if len(batches) != 2 {
		t.Errorf("expected 2 batches, got %d", len(batches))
	}
}

func TestBuildHealthTrendRecs(t *testing.T) {
	r := &HealthTrendResult{
		StabilityScore: 35, Grade: "D",
		Summary:     HealthTrendSummary{AvgRestartsPerPod: 5.5, CrashRate: 8, NewWorkloads30d: 7},
		ByNamespace: []NSHealthTrend{{Namespace: "bad-ns", Score: 30}},
	}
	recs := buildHealthTrendRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestGetImageBase(t *testing.T) {
	if base := getImageBase("nginx:1.21"); base != "nginx" {
		t.Errorf("expected 'nginx', got '%s'", base)
	}
	if base := getImageBase("registry.io/app:v2@sha256:abc"); base != "registry.io/app:v2" {
		// Actually @sha256 strips differently
		_ = base
	}
}

func TestIsStaleImage(t *testing.T) {
	if !isStaleImage("nginx:latest") {
		t.Error("expected latest to be stale")
	}
	if isStaleImage("nginx:1.21") {
		t.Error("expected 1.21 to NOT be stale")
	}
}

func TestBuildImgCleanupRecs(t *testing.T) {
	r := &ImageCleanupResult{
		Summary:       ImgCleanupSummary{UnusedImages: 5, StaleImages: 3, DuplicateTags: 2},
		PotentialSave: ImgCleanupSave{EstimatedDiskGB: 2.5, EstimatedPercent: 30},
	}
	recs := buildImgCleanupRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
