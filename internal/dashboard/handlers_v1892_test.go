package dashboard

import (
	"testing"
)

func TestImageRegistryResultStruct1892(t *testing.T) {
	r := ImageRegistryResult{
		Summary: ImageRegistrySummary{
			TotalImages:       80,
			UniqueRegistries:  5,
			TrustedRegistries: 3,
			UntrustedImages:   10,
			LatestTagCount:    15,
			WithoutDigest:     70,
		},
		HealthScore: 35,
	}
	if r.Summary.UntrustedImages != 10 {
		t.Errorf("expected 10 untrusted, got %d", r.Summary.UntrustedImages)
	}
}

func TestSAMountExposureResultStruct1892(t *testing.T) {
	r := SAMountExposureResult{
		Summary: SAMountExposureSummary{
			TotalWorkloads:   60,
			AutoMountEnabled: 55,
			OverMounted:      40,
			HighPrivSAs:      3,
		},
		HealthScore: 10,
	}
	if r.Summary.OverMounted != 40 {
		t.Errorf("expected 40 over-mounted, got %d", r.Summary.OverMounted)
	}
}

func TestTLSVersionResultStruct1892(t *testing.T) {
	r := TLSVersionResult{
		Summary: TLSVersionSummary{
			TotalIngresses: 20,
			WithTLS:        15,
			WithoutTLS:     5,
			TotalCerts:     18,
			ExpiredCerts:   2,
			ExpiringSoon:   3,
		},
		HealthScore: 55,
	}
	if r.Summary.WithoutTLS != 5 {
		t.Errorf("expected 5 without TLS, got %d", r.Summary.WithoutTLS)
	}
}

func TestExtractRegistry1892(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"nginx", "docker.io"},
		{"nginx:1.21", "docker.io"},
		{"docker.io/nginx:1.21", "docker.io"},
		{"registry.iot2.win/k8ops:v1890", "registry.iot2.win"},
		{"ghcr.io/topcheer/k8ops:v1891", "ghcr.io"},
		{"localhost:5000/myapp:latest", "localhost:5000"},
		{"quay.io/jetstack/cert-manager", "quay.io"},
	}
	for _, tt := range tests {
		got := extractRegistry1892(tt.image)
		if got != tt.expected {
			t.Errorf("extractRegistry1892(%q) = %q, want %q", tt.image, got, tt.expected)
		}
	}
}

func TestBuildImageRegistryRecs1892(t *testing.T) {
	result := &ImageRegistryResult{
		Summary: ImageRegistrySummary{
			TotalImages:       50,
			UniqueRegistries:  4,
			TrustedRegistries: 3,
			UntrustedImages:   5,
			LatestTagCount:    10,
			WithoutDigest:     40,
		},
	}
	recs := buildImageRegistryRecs1892(result)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestBuildSAMountRecs1892(t *testing.T) {
	result := &SAMountExposureResult{
		Summary: SAMountExposureSummary{
			TotalWorkloads:   40,
			AutoMountEnabled: 35,
			OverMounted:      30,
			HighPrivSAs:      2,
			UnusedSAs:        5,
		},
	}
	recs := buildSAMountRecs1892(result)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestBuildTLSVersionRecs1892(t *testing.T) {
	result := &TLSVersionResult{
		Summary: TLSVersionSummary{
			TotalIngresses: 15,
			WithTLS:        10,
			WithoutTLS:     5,
			TotalCerts:     12,
			ExpiredCerts:   1,
			ExpiringSoon:   3,
		},
	}
	recs := buildTLSVersionRecs1892(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(recs))
	}
}
