package dashboard

import (
	"testing"
)

func TestImgHygParse(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		registry    string
		repo        string
		tag         string
		isLatest    bool
		hasDigest   bool
		isVersioned bool
	}{
		{
			name:        "versioned tag",
			image:       "registry.iot2.win/k8ops:v15.12",
			registry:    "registry.iot2.win",
			repo:        "k8ops",
			tag:         "v15.12",
			isVersioned: true,
		},
		{
			name:     "latest tag",
			image:    "nginx:latest",
			registry: "docker.io",
			repo:     "nginx",
			tag:      "latest",
			isLatest: true,
		},
		{
			name:     "no tag defaults latest",
			image:    "nginx",
			registry: "docker.io",
			repo:     "nginx",
			tag:      "latest",
			isLatest: true,
		},
		{
			name:      "digest pinned",
			image:     "registry.iot2.win/k8ops@sha256:abc123",
			registry:  "registry.iot2.win",
			repo:      "k8ops",
			tag:       "latest",
			hasDigest: true,
			isLatest:  true,
		},
		{
			name:        "docker hub with org",
			image:       "bitnami/redis:7.2.4",
			registry:    "docker.io",
			repo:        "bitnami/redis",
			tag:         "7.2.4",
			isVersioned: true,
		},
		{
			name:        "localhost registry",
			image:       "localhost:5000/myapp:v1.0",
			registry:    "localhost:5000",
			repo:        "myapp",
			tag:         "v1.0",
			isVersioned: true,
		},
		{
			name:     "gcr.io with path",
			image:    "gcr.io/distroless/static-debian12:nonroot",
			registry: "gcr.io",
			repo:     "distroless/static-debian12",
			tag:      "nonroot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := imgHygParse(tt.image)
			if entry.Registry != tt.registry {
				t.Errorf("Registry = %q, want %q", entry.Registry, tt.registry)
			}
			if entry.Repository != tt.repo {
				t.Errorf("Repository = %q, want %q", entry.Repository, tt.repo)
			}
			if entry.Tag != tt.tag {
				t.Errorf("Tag = %q, want %q", entry.Tag, tt.tag)
			}
			if entry.IsLatest != tt.isLatest {
				t.Errorf("IsLatest = %v, want %v", entry.IsLatest, tt.isLatest)
			}
			if entry.HasDigest != tt.hasDigest {
				t.Errorf("HasDigest = %v, want %v", entry.HasDigest, tt.hasDigest)
			}
			if entry.IsVersioned != tt.isVersioned {
				t.Errorf("IsVersioned = %v, want %v", entry.IsVersioned, tt.isVersioned)
			}
		})
	}
}

func TestImgHygIsVersion(t *testing.T) {
	if !imgHygIsVersion("v15.12") {
		t.Error("Expected true for v15.12")
	}
	if !imgHygIsVersion("1.2.3") {
		t.Error("Expected true for 1.2.3")
	}
	if imgHygIsVersion("latest") {
		t.Error("Expected false for latest")
	}
	if imgHygIsVersion("") {
		t.Error("Expected false for empty")
	}
}

func TestImgHygIsTrustedReg(t *testing.T) {
	trusted := []string{"registry.k8s.io", "gcr.io", "quay.io", "registry.iot2.win", "docker.io", "ghcr.io"}
	for _, reg := range trusted {
		if !imgHygIsTrustedReg(reg) {
			t.Errorf("Expected %s to be trusted", reg)
		}
	}
	if imgHygIsTrustedReg("random-registry.com") {
		t.Error("Expected random-registry.com to be untrusted")
	}
}

func TestImgHygAssessRisk(t *testing.T) {
	// latest + no digest + docker.io + no version
	entry := ImgHygEntry{IsLatest: true, HasDigest: false, Registry: "docker.io", IsVersioned: false}
	// 15 + 10 + 5 + 5 = 35 → high
	if level := imgHygAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for latest+no-digest+docker.io, got %s", level)
	}

	// versioned + trusted registry + digest
	entry = ImgHygEntry{IsLatest: false, HasDigest: true, Registry: "registry.iot2.win", IsVersioned: true}
	if level := imgHygAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for clean image, got %s", level)
	}
}

func TestImgHygCalcScore(t *testing.T) {
	// Clean
	s := ImgHygieneSummary{TotalContainers: 10, VersionTagged: 10}
	if score := imgHygCalcScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With issues
	s = ImgHygieneSummary{
		TotalContainers: 10,
		LatestTagCount:  3, // -24
		UntaggedOrEmpty: 1, // -5
		DuplicateImages: 2, // -6
		UntrustedReg:    1, // -2
	}
	// 100 - 24 - 5 - 6 - 2 = 63
	if score := imgHygCalcScore(s); score != 63 {
		t.Errorf("Expected 63, got %d", score)
	}

	// Empty
	if score := imgHygCalcScore(ImgHygieneSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestImgHygGenRecs(t *testing.T) {
	s := ImgHygieneSummary{
		LatestTagCount:  3,
		DigestPinned:    2,
		DuplicateImages: 1,
		UntrustedReg:    1,
		HygieneScore:    50,
	}
	dups := []ImgHygDuplicate{
		{BaseImage: "docker.io/nginx", Variants: []string{"docker.io/nginx:1.21", "docker.io/nginx:1.25"}},
	}

	recs := imgHygGenRecs(s, dups)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundLatest := false
	foundDigest := false
	foundDup := false
	for _, r := range recs {
		if containsSubstr(r, ":latest") {
			foundLatest = true
		}
		if containsSubstr(r, "digest") {
			foundDigest = true
		}
		if containsSubstr(r, "multiple variants") {
			foundDup = true
		}
	}
	if !foundLatest {
		t.Error("Expected recommendation about :latest tags")
	}
	if !foundDigest {
		t.Error("Expected recommendation about digest pinning")
	}
	if !foundDup {
		t.Error("Expected recommendation about duplicates")
	}
}

func TestImgHygContains(t *testing.T) {
	if !imgHygContains([]string{"a", "b"}, "a") {
		t.Error("Expected true for 'a'")
	}
	if imgHygContains([]string{"a", "b"}, "c") {
		t.Error("Expected false for 'c'")
	}
	if imgHygContains(nil, "a") {
		t.Error("Expected false for nil")
	}
}

func TestImgHygRiskRank(t *testing.T) {
	if imgHygRiskRank("high") != 0 {
		t.Error("Expected 0 for high")
	}
	if imgHygRiskRank("medium") != 1 {
		t.Error("Expected 1 for medium")
	}
	if imgHygRiskRank("low") != 2 {
		t.Error("Expected 2 for low")
	}
}
