package dashboard

import (
	"testing"
)

func TestPodRestartForensicsResultStruct1882(t *testing.T) {
	r := PodRestartForensicsDeepResult{
		Summary:     RestartForensicDeepSummary{TotalPods: 83, TotalRestarts: 150, CrashLoopPods: 3},
		HealthScore: 45,
	}
	if r.Summary.CrashLoopPods != 3 {
		t.Errorf("expected 3, got %d", r.Summary.CrashLoopPods)
	}
}

func TestDeploymentHealthTrendResultStruct1882(t *testing.T) {
	r := DeploymentHealthTrendResult{
		Summary:     DeployHealthTrendSummary{TotalDeployments: 50, HealthyDeployments: 48},
		HealthScore: 96,
	}
	if r.Summary.HealthyDeployments != 48 {
		t.Errorf("expected 48, got %d", r.Summary.HealthyDeployments)
	}
}

func TestEventCorrelationMatrixResultStruct1882(t *testing.T) {
	r := EventCorrelationMatrixResult1882{
		Summary:     EventCorrMatrixSummary{TotalEvents: 200, WarningEvents: 15, UniqueReasons: 8},
		HealthScore: 92,
	}
	if r.Summary.WarningEvents != 15 {
		t.Errorf("expected 15, got %d", r.Summary.WarningEvents)
	}
}

func TestTruncateStrSafe(t *testing.T) {
	if truncateStrSafe1882("hello", 10) != "hello" {
		t.Error("expected no truncation")
	}
	if truncateStrSafe1882("hello world", 5) != "hello..." {
		t.Error("expected truncation")
	}
}
