package dashboard

import "testing"

func TestDNSExfilResult1913(t *testing.T) {
	r := DNSExfilResult{
		Summary:     DNSExfilSummary{TotalWorkloads: 64, WithoutNetPolicy: 60, SuspiciousEnvs: 5, HighRiskWorkloads: 55},
		HealthScore: 14,
	}
	if r.Summary.HighRiskWorkloads != 55 {
		t.Errorf("expected 55, got %d", r.Summary.HighRiskWorkloads)
	}
}

func TestPortForwardResult1913(t *testing.T) {
	r := PortForwardResult{
		Summary:     PortForwardSummary{TotalContainers: 72, HostPortCount: 3, HighRiskPorts: 2, NodePortSvcs: 5},
		HealthScore: 90,
	}
	if r.Summary.HighRiskPorts != 2 {
		t.Errorf("expected 2, got %d", r.Summary.HighRiskPorts)
	}
}

func TestImageProvenanceResult1913(t *testing.T) {
	r := ImageProvenanceResult1913{
		Summary:     ImageProvSummary1913{TotalContainers: 72, TrustedRegistries: 3, UntrustedRegistry: 5, PinnedImages: 57, ByDigestCount: 10, HasImagePolicy: 0},
		HealthScore: 80,
	}
	if r.Summary.UntrustedRegistry != 5 {
		t.Errorf("expected 5, got %d", r.Summary.UntrustedRegistry)
	}
}

func TestBuildDNSExfilRecs1913(t *testing.T) {
	r := &DNSExfilResult{Summary: DNSExfilSummary{TotalWorkloads: 64, HighRiskWorkloads: 55, WithoutNetPolicy: 60, SuspiciousEnvs: 5}}
	recs := buildDNSExfilRecs1913(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildPortForwardRecs1913(t *testing.T) {
	r := &PortForwardResult{Summary: PortForwardSummary{HostPortCount: 3, NodePortSvcs: 5, LoadBalancerSvcs: 2, HighRiskPorts: 2}}
	recs := buildPortForwardRecs1913(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildImageProvRecs1913(t *testing.T) {
	r := &ImageProvenanceResult1913{Summary: ImageProvSummary1913{TotalContainers: 72, TrustedRegistries: 3, UntrustedRegistry: 5, PinnedImages: 57, ByDigestCount: 10, HasImagePolicy: 0}}
	recs := buildImageProvRecs1913(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
