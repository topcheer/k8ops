package dashboard

import (
	"testing"
)

func TestAssessEndpointDNSRisk(t *testing.T) {
	// Healthy service with endpoints
	entry := EndpointDNSEntry{
		HasSelector:  true,
		HasEndpoints: true,
	}
	if got := assessEndpointDNSRisk(entry); got != "healthy" {
		t.Errorf("healthy service risk = %s, want healthy", got)
	}

	// Warning: has selector but no endpoints
	entry = EndpointDNSEntry{
		HasSelector:  true,
		HasEndpoints: false,
	}
	if got := assessEndpointDNSRisk(entry); got != "warning" {
		t.Errorf("no-endpoint risk = %s, want warning", got)
	}

	// Info: no selector, not headless
	entry = EndpointDNSEntry{
		HasSelector: false,
		IsHeadless:  false,
		Type:        "ClusterIP",
	}
	if got := assessEndpointDNSRisk(entry); got != "info" {
		t.Errorf("no-selector risk = %s, want info", got)
	}
}

func TestComputeEndpointDNSScore(t *testing.T) {
	// No services → perfect
	score := computeEndpointDNSScore(EndpointDNSSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// No endpoints is most impactful
	score = computeEndpointDNSScore(EndpointDNSSummary{
		TotalServices:   20,
		NoEndpoints:     5,
		NoSelector:      3,
		MultiPortNoName: 2,
	}, 8)
	if score > 65 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-65", score)
	}

	// All healthy
	score = computeEndpointDNSScore(EndpointDNSSummary{
		TotalServices: 20,
		WithEndpoints: 20,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}
}
