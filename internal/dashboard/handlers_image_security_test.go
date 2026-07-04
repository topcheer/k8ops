package dashboard

import (
	"testing"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		image    string
		registry string
		repo     string
		tag      string
	}{
		{"nginx", "", "nginx", ""},
		{"nginx:1.21", "", "nginx", "1.21"},
		{"nginx:latest", "", "nginx", "latest"},
		{"registry.io/app:v1", "registry.io", "app", "v1"},
		{"gcr.io/project/app:v2.0", "gcr.io", "project/app", "v2.0"},
		{"localhost:5000/myapp:dev", "localhost:5000", "myapp", "dev"},
		{"registry.iot2.win/k8ops:v14.91", "registry.iot2.win", "k8ops", "v14.91"},
		{"quay.io/brancz/kube-rbac-proxy:v0.18.2", "quay.io", "brancz/kube-rbac-proxy", "v0.18.2"},
	}

	for _, tt := range tests {
		reg, repo, tag := parseImageReference(tt.image)
		if reg != tt.registry || repo != tt.repo || tag != tt.tag {
			t.Errorf("parseImageReference(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.image, reg, repo, tag, tt.registry, tt.repo, tt.tag)
		}
	}
}

func TestParseImageReferenceWithDigest(t *testing.T) {
	// When called from analyzeImageSecurity, digest is stripped first.
	// Test parseImageReference with already-stripped image.
	reg, repo, tag := parseImageReference("registry.io/app:v1")
	if reg != "registry.io" || repo != "app" || tag != "v1" {
		t.Errorf("Expected (registry.io, app, v1), got (%s, %s, %s)", reg, repo, tag)
	}
}

func TestIsOldVersionTag(t *testing.T) {
	tests := []struct {
		tag    string
		expect bool
	}{
		{"v1", true},
		{"1.0", true},
		{"2", true},
		{"v2", true},
		{"latest", false},
		{"", false},
		{"v14.91", false},
		{"v1.2.3", false},
		{"alpine", false},
	}

	for _, tt := range tests {
		got := isOldVersionTag(tt.tag)
		if got != tt.expect {
			t.Errorf("isOldVersionTag(%q) = %v, want %v", tt.tag, got, tt.expect)
		}
	}
}

func TestAnalyzeImageSecurityLatest(t *testing.T) {
	entry := &ImageSecEntry{Image: "nginx:latest"}
	analyzeImageSecurity(entry)

	if !entry.IsLatest {
		t.Error("Expected IsLatest = true")
	}
	if entry.DigestPinned {
		t.Error("Expected DigestPinned = false")
	}
	if entry.HasNoTag {
		t.Error("Expected HasNoTag = false")
	}
	if len(entry.Issues) < 2 {
		t.Errorf("Expected at least 2 issues, got %d", len(entry.Issues))
	}
	if entry.RiskLevel != "high" && entry.RiskLevel != "critical" {
		t.Errorf("Expected high or critical risk, got %s", entry.RiskLevel)
	}
}

func TestAnalyzeImageSecurityNoTag(t *testing.T) {
	entry := &ImageSecEntry{Image: "nginx"}
	analyzeImageSecurity(entry)

	if !entry.HasNoTag {
		t.Error("Expected HasNoTag = true")
	}
	if entry.Tag != "" {
		t.Errorf("Expected empty tag, got %s", entry.Tag)
	}
	// No tag + no digest + unknown registry should be at least high
	if entry.RiskLevel != "critical" {
		t.Errorf("Expected critical risk for no-tag image, got %s", entry.RiskLevel)
	}
}

func TestAnalyzeImageSecurityDigestPinned(t *testing.T) {
	entry := &ImageSecEntry{Image: "registry.io/app@sha256:abc123def456"}
	analyzeImageSecurity(entry)

	if !entry.DigestPinned {
		t.Error("Expected DigestPinned = true")
	}
	if !entry.IsPublic && entry.RiskLevel != "low" {
		t.Errorf("Expected low risk for digest-pinned private image, got %s", entry.RiskLevel)
	}
}

func TestAnalyzeImageSecurityPublicRegistry(t *testing.T) {
	entry := &ImageSecEntry{Image: "gcr.io/project/app:v1.0.0"}
	analyzeImageSecurity(entry)

	if !entry.IsPublic {
		t.Error("Expected IsPublic = true for gcr.io")
	}
	// v1.0.0 is NOT old version (> 4 chars)
	if entry.OldVersion {
		t.Error("Expected OldVersion = false for v1.0.0")
	}
}

func TestAnalyzeImageSecurityUnknownRegistry(t *testing.T) {
	entry := &ImageSecEntry{Image: "myapp:v2.1.0"}
	analyzeImageSecurity(entry)

	if !entry.IsUnknown {
		t.Error("Expected IsUnknown = true")
	}
}

func TestAnalyzeImageSecurityOldVersion(t *testing.T) {
	entry := &ImageSecEntry{Image: "registry.io/app:v1"}
	analyzeImageSecurity(entry)

	if !entry.OldVersion {
		t.Error("Expected OldVersion = true for v1")
	}
}

func TestCalculateImageSecScore(t *testing.T) {
	// Perfect
	perfect := ImageSecuritySummary{
		UniqueImages:   10,
		PinnedByDigest: 10,
	}
	if score := calculateImageSecScore(perfect); score != 100 {
		t.Errorf("Expected 100 for all pinned, got %d", score)
	}

	// With issues
	withIssues := ImageSecuritySummary{
		UniqueImages:      10,
		UsingLatest:       2, // -10
		NoTag:             1, // -8
		NotPinned:         5, // -10
		OldVersionTags:    3, // -9
		UnknownRegistries: 2, // -6
	}
	// 100 - 10 - 8 - 10 - 9 - 6 = 57
	score := calculateImageSecScore(withIssues)
	if score != 57 {
		t.Errorf("Expected 57, got %d", score)
	}

	// Floor at 0
	terrible := ImageSecuritySummary{
		UniqueImages: 10,
		NoTag:        10, // -80
		UsingLatest:  10, // -50
	}
	if score := calculateImageSecScore(terrible); score != 0 {
		t.Errorf("Expected 0 for terrible, got %d", score)
	}
}

func TestGenerateImageSecRecommendations(t *testing.T) {
	result := ImageSecurityResult{
		Summary: ImageSecuritySummary{
			NoTag:             2,
			UsingLatest:       3,
			NotPinned:         5,
			OldVersionTags:    1,
			UnknownRegistries: 1,
			SecurityScore:     40,
		},
	}

	recs := generateImageSecRecommendations(result)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoTag := false
	foundLatest := false
	foundDigest := false
	for _, r := range recs {
		if containsSubstr(r, "no tag") {
			foundNoTag = true
		}
		if containsSubstr(r, "latest") {
			foundLatest = true
		}
		if containsSubstr(r, "digest") {
			foundDigest = true
		}
	}
	if !foundNoTag {
		t.Error("Expected recommendation about no-tag images")
	}
	if !foundLatest {
		t.Error("Expected recommendation about :latest images")
	}
	if !foundDigest {
		t.Error("Expected recommendation about digest pinning")
	}
}

func TestGenerateImageSecRecommendationsClean(t *testing.T) {
	result := ImageSecurityResult{
		Summary: ImageSecuritySummary{
			UniqueImages:   10,
			PinnedByDigest: 10,
			SecurityScore:  100,
		},
	}

	recs := generateImageSecRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean cluster, got %d", len(recs))
	}
}

func TestGetOrCreateImageEntry(t *testing.T) {
	m := make(map[string]*ImageSecEntry)

	e1 := getOrCreateImageEntry(m, "nginx:v1")
	e1.UsedBy = 3

	e2 := getOrCreateImageEntry(m, "nginx:v1")
	if e2.UsedBy != 3 {
		t.Errorf("Expected same entry with UsedBy=3, got %d", e2.UsedBy)
	}

	e3 := getOrCreateImageEntry(m, "redis:v2")
	if e3.Image != "redis:v2" {
		t.Errorf("Expected redis:v2, got %s", e3.Image)
	}
}

func TestAssessImageRiskLevel(t *testing.T) {
	tests := []struct {
		name   string
		entry  ImageSecEntry
		expect string
	}{
		{
			"clean-pinned-private",
			ImageSecEntry{DigestPinned: true, Registry: "private.io"},
			"low",
		},
		{
			"latest-no-digest",
			ImageSecEntry{IsLatest: true, DigestPinned: false, Registry: "private.io"},
			"high",
		},
		{
			"no-tag-unknown",
			ImageSecEntry{HasNoTag: true, IsUnknown: true, DigestPinned: false},
			"critical",
		},
		{
			"old-version-no-digest",
			ImageSecEntry{OldVersion: true, IsLatest: false, DigestPinned: false, Registry: "private.io"},
			"high",
		},
	}

	for _, tt := range tests {
		got := assessImageRiskLevel(tt.entry)
		if got != tt.expect {
			t.Errorf("assessImageRiskLevel(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestIsSimpleNumber(t *testing.T) {
	tests := []struct {
		s      string
		expect bool
	}{
		{"1", true},
		{"1.0", true},
		{"12", true},
		{"1.2.3", true},
		{"", false},
		{"abc", false},
		{"1a", false},
	}

	for _, tt := range tests {
		got := isSimpleNumber(tt.s)
		if got != tt.expect {
			t.Errorf("isSimpleNumber(%q) = %v, want %v", tt.s, got, tt.expect)
		}
	}
}
