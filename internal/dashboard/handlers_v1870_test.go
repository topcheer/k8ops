package dashboard

import (
	"testing"
)

func TestImmutableConfigResultStruct1870(t *testing.T) {
	r := ImmutableConfigResult{
		Summary: ImmutableConfigSummary{
			TotalCMs:         10,
			ImmutableCMs:     3,
			TotalSecrets:     15,
			ImmutableSecrets: 2,
		},
		HealthScore: 20,
	}
	if r.Summary.TotalCMs != 10 {
		t.Errorf("expected 10, got %d", r.Summary.TotalCMs)
	}
	if r.HealthScore != 20 {
		t.Errorf("expected 20, got %d", r.HealthScore)
	}
}

func TestSidecarInjectionResultStruct1870(t *testing.T) {
	r := SidecarInjectionResult{
		Summary: SidecarInjectSummary{
			TotalPods:    50,
			WithSidecars: 10,
			MissingMesh:  40,
		},
		HealthScore: 30,
	}
	if r.Summary.MissingMesh != 40 {
		t.Errorf("expected 40, got %d", r.Summary.MissingMesh)
	}
}

func TestResourceQuotaDriftResultStruct1870(t *testing.T) {
	r := ResourceQuotaDriftResult{
		Summary: QuotaDriftSummary{
			TotalNamespaces: 20,
			WithQuotas:      5,
			WithoutQuotas:   15,
			SaturatedQuotas: 2,
		},
		HealthScore: 20,
	}
	if r.Summary.WithoutQuotas != 15 {
		t.Errorf("expected 15, got %d", r.Summary.WithoutQuotas)
	}
}

func TestFormatQuotaResourceName(t *testing.T) {
	got := formatQuotaResName("requests.cpu")
	if got != "cpu" {
		t.Errorf("expected 'cpu', got '%s'", got)
	}
}
