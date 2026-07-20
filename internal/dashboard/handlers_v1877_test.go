package dashboard

import (
	"testing"
)

func TestRevisionHistoryResultStruct1877(t *testing.T) {
	r := RevisionHistoryResult{
		Summary:     RevisionHistorySummary{TotalDeployments: 50, HighHistoryDeploy: 2},
		HealthScore: 90,
	}
	if r.Summary.HighHistoryDeploy != 2 {
		t.Errorf("expected 2, got %d", r.Summary.HighHistoryDeploy)
	}
}

func TestResourceLimitCoverageResultStruct1877(t *testing.T) {
	r := ResourceLimitCoverageResult{
		Summary:     RLCSummary{TotalContainers: 100, WithCPULimit: 45, WithMemLimit: 50},
		HealthScore: 47,
	}
	if r.Summary.WithCPULimit != 45 {
		t.Errorf("expected 45, got %d", r.Summary.WithCPULimit)
	}
}

func TestEphemeralStorageQuotaResultStruct1877(t *testing.T) {
	r := EphemeralStorageQuotaResult{
		Summary:     EphemeralSummary{TotalPods: 83, UnboundedPods: 83},
		HealthScore: 0,
	}
	if r.Summary.UnboundedPods != 83 {
		t.Errorf("expected 83, got %d", r.Summary.UnboundedPods)
	}
}
