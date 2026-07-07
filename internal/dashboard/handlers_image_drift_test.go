package dashboard

import (
	"testing"
)

func TestIDIsLatestTag(t *testing.T) {
	if !idIsLatestTag("nginx:latest") {
		t.Error("Expected true for :latest")
	}
	if !idIsLatestTag("nginx") {
		t.Error("Expected true for no tag (implicit latest)")
	}
	if idIsLatestTag("nginx:1.21") {
		t.Error("Expected false for :1.21")
	}
	if idIsLatestTag("nginx:1.21@sha256:abc123") {
		t.Error("Expected false for pinned with digest")
	}
	if !idIsLatestTag("nginx@sha256:abc123") {
		t.Error("Expected true for digest-only (no tag)")
	}
}

func TestIDAssessRisk(t *testing.T) {
	entry := IDEntry{HasDrift: true}
	if level := idAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for drift, got %s", level)
	}

	entry = IDEntry{UsesLatest: true}
	if level := idAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for latest, got %s", level)
	}

	entry = IDEntry{HasDigest: true}
	if level := idAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestIDScore(t *testing.T) {
	if score := idScore(IDSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := IDSummary{
		TotalWorkloads:   10,
		DriftedWorkloads: 2, // -30
		UsingLatestTag:   3, // -24
		NoDigest:         5, // -10
	}
	// 100 - 30 - 24 - 10 = 36
	if score := idScore(s); score != 36 {
		t.Errorf("Expected 36, got %d", score)
	}
}

func TestIDGenRecs(t *testing.T) {
	s := IDSummary{
		TotalWorkloads:   10,
		DriftedWorkloads: 2,
		UsingLatestTag:   3,
		NoDigest:         5,
		ConsistencyScore: 36,
	}
	drifted := []IDEntry{{
		Namespace: "app", Name: "api",
		ImageVariants: []IDImageVariant{
			{Image: "nginx:1.21", PodCount: 3},
			{Image: "nginx:1.22", PodCount: 1},
		},
	}}

	recs := idGenRecs(s, drifted)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundDrift := false
	foundLatest := false
	for _, r := range recs {
		if strContains(r, "image drift") {
			foundDrift = true
		}
		if strContains(r, ":latest") {
			foundLatest = true
		}
	}
	if !foundDrift {
		t.Error("Expected recommendation about image drift")
	}
	if !foundLatest {
		t.Error("Expected recommendation about latest tag")
	}
}

func TestIDGenRecsClean(t *testing.T) {
	s := IDSummary{TotalWorkloads: 10}
	recs := idGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}
