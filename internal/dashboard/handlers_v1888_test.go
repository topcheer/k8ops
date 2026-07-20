package dashboard

import (
	"testing"
)

func TestNodeConditionTrendResultStruct1888(t *testing.T) {
	r := NodeConditionTrendResult{
		Summary:     NodeCondTrendSummary{TotalNodes: 1, ReadyNodes: 1, PressureNodes: 0},
		HealthScore: 100,
	}
	if r.Summary.ReadyNodes != 1 {
		t.Errorf("expected 1, got %d", r.Summary.ReadyNodes)
	}
}

func TestContainerLogSizeResultStruct1888(t *testing.T) {
	r := ContainerLogSizeResult{
		Summary:     ContainerLogSizeSummary{TotalPods: 83, NoLogPolicy: 83, EstTotalLogGB: 2.5},
		HealthScore: 0,
	}
	if r.Summary.NoLogPolicy != 83 {
		t.Errorf("expected 83, got %d", r.Summary.NoLogPolicy)
	}
}

func TestKubeletConfigDriftResultStruct1888(t *testing.T) {
	r := KubeletConfigDriftResult{
		Summary:     KubeletDriftSummary{TotalNodes: 1, Consistent: 1, DriftedCount: 0},
		HealthScore: 100,
	}
	if r.Summary.Consistent != 1 {
		t.Errorf("expected 1, got %d", r.Summary.Consistent)
	}
}

func TestMostCommonKey(t *testing.T) {
	m := map[string]int{"a": 3, "b": 5, "c": 2}
	if mostCommonKey(m) != "b" {
		t.Error("expected b")
	}
}
