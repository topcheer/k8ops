package dashboard

import (
	"testing"
)

func TestBackupComplianceResultStruct1890(t *testing.T) {
	r := BackupComplianceResult{
		Summary: BackupComplianceSummary{
			TotalNamespaces:   10,
			WithBackupPolicy:  3,
			WithoutPolicy:     7,
			TotalPVCs:         15,
			PVCsBackedUp:      5,
			PVCsAtRisk:        10,
			TotalSecrets:      30,
			SecretsWithBackup: 10,
		},
		HealthScore: 40,
	}
	if r.Summary.WithoutPolicy != 7 {
		t.Errorf("expected 7 namespaces without policy, got %d", r.Summary.WithoutPolicy)
	}
	if r.Summary.PVCsAtRisk != 10 {
		t.Errorf("expected 10 PVCs at risk, got %d", r.Summary.PVCsAtRisk)
	}
}

func TestLabelTaxonomyResultStruct1890(t *testing.T) {
	r := LabelTaxonomyResult{
		Summary: LabelTaxonomySummary{
			TotalResources:      100,
			ResourcesWithLabels: 60,
			UniqueLabels:        25,
			StandardLabels:      5,
			CustomLabels:        20,
			InconsistentCount:   8,
			CoveragePercent:     60,
		},
		HealthScore: 45,
	}
	if r.Summary.CoveragePercent != 60 {
		t.Errorf("expected 60%% coverage, got %d%%", r.Summary.CoveragePercent)
	}
	if r.Summary.InconsistentCount != 8 {
		t.Errorf("expected 8 inconsistencies, got %d", r.Summary.InconsistentCount)
	}
}

func TestChangeImpactResultStruct1890(t *testing.T) {
	r := ChangeImpactResult{
		Summary: ChangeImpactSummary{
			TotalRecentChanges: 120,
			CriticalChanges:    5,
			HighRiskChanges:    15,
			WithRollbackPlan:   40,
			WithoutRollback:    10,
		},
		HealthScore: 80,
	}
	if r.Summary.CriticalChanges != 5 {
		t.Errorf("expected 5 critical changes, got %d", r.Summary.CriticalChanges)
	}
	if r.Summary.WithoutRollback != 10 {
		t.Errorf("expected 10 without rollback, got %d", r.Summary.WithoutRollback)
	}
}

func TestClassifyBackupTool1890(t *testing.T) {
	tests := []struct {
		annotation string
		expected   string
	}{
		{"backup.velero.io/backup-volumes", "Velero"},
		{"k8up.io/backup", "K8up"},
		{"stork.libopenstorage.org/snapshot-name", "Stork"},
		{"unknown.io/backup", "unknown"},
	}
	for _, tt := range tests {
		got := classifyBackupTool(tt.annotation)
		if got != tt.expected {
			t.Errorf("classifyBackupTool(%q) = %q, want %q", tt.annotation, got, tt.expected)
		}
	}
}

func TestDetectLabelInconsistencies1890(t *testing.T) {
	// "app_name" and "app.kubernetes.io/name" both normalize to "name"
	labelCounts := map[string]int{
		"app":                    10,
		"app.kubernetes.io/name": 5,
		"app_name":               3,
		"team":                   8,
	}
	inconsistencies := detectLabelInconsistencies(labelCounts)
	if len(inconsistencies) == 0 {
		t.Fatal("expected inconsistencies, got 0")
	}
	// Verify that "app.kubernetes.io/name" and "app_name" are detected as variants
	foundVariant := false
	for _, inc := range inconsistencies {
		hasK8s := false
		hasUnderscore := false
		for _, v := range inc.Variants {
			if v == "app.kubernetes.io/name" {
				hasK8s = true
			}
			if v == "app_name" {
				hasUnderscore = true
			}
		}
		if hasK8s && hasUnderscore {
			foundVariant = true
		}
	}
	if !foundVariant {
		t.Error("expected 'app.kubernetes.io/name' and 'app_name' to appear as variants of same concept")
	}
}

func TestIsChangeEvent1890(t *testing.T) {
	tests := []struct {
		reason string
		msg    string
		expect bool
	}{
		{"Scaled", "deployment scaled up", true},
		{"Created", "pod created", true},
		{"ImageUpdated", "container image updated", true},
		{"Random", "nothing happened", false},
	}
	for _, tt := range tests {
		got := isChangeEvent(tt.reason, tt.msg)
		if got != tt.expect {
			t.Errorf("isChangeEvent(%q, %q) = %v, want %v", tt.reason, tt.msg, got, tt.expect)
		}
	}
}

func TestClassifyChangeRisk1890(t *testing.T) {
	tests := []struct {
		reason string
		msg    string
		expect string
	}{
		{"BackOff", "crashloop backoff", "critical"},
		{"ImageUpdated", "container image updated to v2", "high"},
		{"Scaled", "scaled down to 0", "medium"},
		{"Random", "nothing special", "low"},
	}
	for _, tt := range tests {
		got := classifyChangeRisk(tt.reason, tt.msg)
		if got != tt.expect {
			t.Errorf("classifyChangeRisk(%q, %q) = %q, want %q", tt.reason, tt.msg, got, tt.expect)
		}
	}
}
