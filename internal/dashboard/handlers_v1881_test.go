package dashboard

import (
	"testing"
)

func TestHPACooldownResultStruct1881(t *testing.T) {
	r := HPACooldownResult{
		Summary:     HPACooldownSummary{TotalHPAs: 0, WithBehavior: 0},
		HealthScore: 100,
	}
	if r.Summary.TotalHPAs != 0 {
		t.Errorf("expected 0, got %d", r.Summary.TotalHPAs)
	}
}

func TestResourceRequestSaturationResultStruct1881(t *testing.T) {
	r := ResourceRequestSaturationResult{
		Summary: RequestSatSummary{
			TotalCPUReq: 3.5, TotalCPUAlloc: 16.0,
			CPUSaturationPct: 21.875,
		},
		HealthScore: 78,
	}
	if r.Summary.CPUSaturationPct != 21.875 {
		t.Errorf("expected 21.875, got %.3f", r.Summary.CPUSaturationPct)
	}
}

func TestClusterPodLimitResultStruct1881(t *testing.T) {
	r := ClusterPodLimitResult{
		Summary: PodLimitSummary{
			TotalPodCapacity: 110, CurrentPods: 83, HeadroomPods: 27,
		},
		HealthScore: 25,
	}
	if r.Summary.HeadroomPods != 27 {
		t.Errorf("expected 27, got %d", r.Summary.HeadroomPods)
	}
}
