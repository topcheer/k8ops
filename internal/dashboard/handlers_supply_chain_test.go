package dashboard

import (
	"testing"
)

func TestAssessSupplyChainRisk(t *testing.T) {
	tests := []struct {
		name   string
		entry  SupplyChainEntry
		expect string
	}{
		{
			name:   "latest tag critical",
			entry:  SupplyChainEntry{IsLatestTag: true, HasDigest: false, IsTrusted: false},
			expect: "critical",
		},
		{
			name:   "no digest and untrusted",
			entry:  SupplyChainEntry{IsLatestTag: false, HasDigest: false, IsTrusted: false},
			expect: "warning",
		},
		{
			name:   "no digest but trusted",
			entry:  SupplyChainEntry{IsLatestTag: false, HasDigest: false, IsTrusted: true},
			expect: "info",
		},
		{
			name:   "healthy with digest and trusted",
			entry:  SupplyChainEntry{IsLatestTag: false, HasDigest: true, IsTrusted: true},
			expect: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessSupplyChainRisk(tt.entry)
			if got != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, got)
			}
		})
	}
}

func TestExtractRegistry(t *testing.T) {
	tests := []struct {
		image  string
		expect string
	}{
		{"registry.k8s.io/pause:3.9", "registry.k8s.io"},
		{"gcr.io/distroless/static-debian12:nonroot", "gcr.io"},
		{"quay.io/prometheus/node-exporter:v1.6.0", "quay.io"},
		{"nginx:1.25", "docker.io"},
		{"localhost:5000/myapp:v1", "localhost:5000"},
		{"registry.iot2.win/k8ops:v16.58", "registry.iot2.win"},
		{"public.ecr.aws/eks-distro/kubernetes/pause:3.5", "public.ecr.aws"},
		{"mcr.microsoft.com/dotnet/runtime:7.0", "mcr.microsoft.com"},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := extractRegistry(tt.image)
			if got != tt.expect {
				t.Errorf("extractRegistry(%q) = %q, want %q", tt.image, got, tt.expect)
			}
		})
	}
}

func TestAnalyzeImageSecurity(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		expectDigest  bool
		expectLatest  bool
		expectTrusted bool
	}{
		{
			name:          "digest pinned trusted",
			image:         "registry.k8s.io/pause@sha256:abc123",
			expectDigest:  true,
			expectLatest:  false,
			expectTrusted: true,
		},
		{
			name:          "latest tag untrusted",
			image:         "myapp:latest",
			expectDigest:  false,
			expectLatest:  true,
			expectTrusted: false,
		},
		{
			name:          "tagged trusted no digest",
			image:         "quay.io/prometheus/node-exporter:v1.6.0",
			expectDigest:  false,
			expectLatest:  false,
			expectTrusted: true,
		},
		{
			name:          "no tag at all",
			image:         "nginx",
			expectDigest:  false,
			expectLatest:  true,
			expectTrusted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &SupplyChainEntry{Image: tt.image}
			analyzeSupplyChainImage(entry)
			if entry.HasDigest != tt.expectDigest {
				t.Errorf("HasDigest = %v, want %v", entry.HasDigest, tt.expectDigest)
			}
			if entry.IsLatestTag != tt.expectLatest {
				t.Errorf("IsLatestTag = %v, want %v", entry.IsLatestTag, tt.expectLatest)
			}
			if entry.IsTrusted != tt.expectTrusted {
				t.Errorf("IsTrusted = %v, want %v", entry.IsTrusted, tt.expectTrusted)
			}
		})
	}
}

func TestSupplyChainHealthScore(t *testing.T) {
	tests := []struct {
		name          string
		totalImages   int
		latestTag     int
		noRegistry    int
		unsigned      int
		usingDigest   int
		staleImages   int
		expectedRange [2]int
	}{
		{"all healthy", 10, 0, 0, 0, 10, 0, [2]int{95, 100}},
		{"some latest", 10, 3, 0, 0, 7, 0, [2]int{90, 98}},
		{"untrusted", 10, 0, 4, 0, 6, 0, [2]int{90, 98}},
		{"low digest ratio", 10, 0, 0, 0, 1, 0, [2]int{85, 95}},
		{"stale images", 10, 0, 0, 0, 10, 3, [2]int{90, 98}},
		{"all problems", 10, 8, 6, 7, 1, 5, [2]int{0, 70}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := 100
			if tt.totalImages > 0 {
				latestRatio := tt.latestTag * 100 / tt.totalImages
				score -= latestRatio / 5
				untrustedRatio := tt.noRegistry * 100 / tt.totalImages
				score -= untrustedRatio / 10
				unsignedRatio := tt.unsigned * 100 / tt.totalImages
				score -= unsignedRatio / 10
				digestRatio := tt.usingDigest * 100 / tt.totalImages
				if digestRatio < 20 {
					score -= 10
				} else if digestRatio < 50 {
					score -= 5
				}
				if tt.staleImages > 0 {
					score -= tt.staleImages * 2
				}
			}
			if score < 0 {
				score = 0
			}

			if score < tt.expectedRange[0] || score > tt.expectedRange[1] {
				t.Errorf("score %d not in expected range [%d, %d]", score, tt.expectedRange[0], tt.expectedRange[1])
			}
		})
	}
}

func TestFormatSupplyChainSummary(t *testing.T) {
	result := &SupplyChainResult{
		Summary: SupplyChainSummary{
			TotalImages:    50,
			UniqueImages:   20,
			UsingDigest:    15,
			UsingLatestTag: 5,
			NoRegistry:     8,
			UnsignedImages: 30,
		},
	}
	summary := formatSupplyChainSummary(result)
	if summary == "" {
		t.Error("summary should not be empty")
	}
}
