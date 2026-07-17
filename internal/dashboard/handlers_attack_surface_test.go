package dashboard

import (
	"testing"
)

func TestComputeExposureScore(t *testing.T) {
	s0 := EPExposureSummary{TotalServices: 0}
	if score := computeExposureScore(s0, 0); score != 100 {
		t.Errorf("expected 100 for no services, got %d", score)
	}

	s1 := EPExposureSummary{TotalServices: 10, WithTLS: 8, WithoutTLS: 2}
	if score := computeExposureScore(s1, 2); score >= 100 {
		t.Errorf("expected lower score with TLS gaps, got %d", score)
	}

	s2 := EPExposureSummary{TotalServices: 10, LoadBalancerSvc: 5}
	if score := computeExposureScore(s2, 0); score > 90 {
		t.Errorf("expected lower score with many LB services, got %d", score)
	}
}

func TestGenerateExposureRecs(t *testing.T) {
	r := EndpointExposureResult{
		Summary:       EPExposureSummary{TotalServices: 20, LoadBalancerSvc: 3, IngressServices: 5, ExposedPorts: 30},
		AttackSurface: AttackSurfaceMap{PublicEndpoints: 8, HighRiskEndpoints: 3, UniqueHosts: 5},
		TLSGaps:       []EPTLSGap{{Resource: "Ingress/test", Severity: "high"}},
		HealthScore:   70,
	}
	recs := generateExposureRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recs, got %d", len(recs))
	}
}
