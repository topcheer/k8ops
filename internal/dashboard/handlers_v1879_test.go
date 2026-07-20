package dashboard

import (
	"testing"
)

func TestSecretMountAuditResultStruct1879(t *testing.T) {
	r := SecretMountAuditResult{
		Summary:     SecretMountSummary{TotalMounts: 20, EnvVarMounts: 15, VolumeMounts: 5},
		HealthScore: 25,
	}
	if r.Summary.EnvVarMounts != 15 {
		t.Errorf("expected 15, got %d", r.Summary.EnvVarMounts)
	}
}

func TestLabelPropagationResultStruct1879(t *testing.T) {
	r := LabelPropagationResult{
		Summary:     LabelPropSummary{TotalWorkloads: 50, WithLabels: 45, OrphanedSelectors: 3},
		HealthScore: 90,
	}
	if r.Summary.OrphanedSelectors != 3 {
		t.Errorf("expected 3, got %d", r.Summary.OrphanedSelectors)
	}
}

func TestCronJobOrphanResultStruct1879(t *testing.T) {
	r := CronJobOrphanResult{
		Summary:     CronJobOrphanSummary{TotalCronJobs: 5, NoResourceLimit: 3},
		HealthScore: 40,
	}
	if r.Summary.NoResourceLimit != 3 {
		t.Errorf("expected 3, got %d", r.Summary.NoResourceLimit)
	}
}
