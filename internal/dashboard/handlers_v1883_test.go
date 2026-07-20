package dashboard

import (
	"testing"
)

func TestImagePullLatencyResultStruct1883(t *testing.T) {
	r := ImagePullLatencyResult{
		Summary:     ImagePullLatencySummary{TotalImages: 107, UniqueRegistries: 5, PullErrors: 2},
		HealthScore: 90,
	}
	if r.Summary.PullErrors != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PullErrors)
	}
}

func TestProbeTimeoutResultStruct1883(t *testing.T) {
	r := ProbeTimeoutResult{
		Summary:     ProbeTimeoutSummary{TotalContainers: 90, WithLiveness: 40, NoProbe: 15},
		HealthScore: 50,
	}
	if r.Summary.NoProbe != 15 {
		t.Errorf("expected 15, got %d", r.Summary.NoProbe)
	}
}

func TestInitContainerHealthAuditResultStruct1883(t *testing.T) {
	r := InitContainerHealthAuditResult{
		Summary:     InitContainerHealthSummary{TotalWorkloads: 50, WithInitContainer: 5, NoResourceLimit: 3},
		HealthScore: 40,
	}
	if r.Summary.NoResourceLimit != 3 {
		t.Errorf("expected 3, got %d", r.Summary.NoResourceLimit)
	}
}
