package dashboard

import (
	"testing"
)

func TestBuildPodHealthRecs(t *testing.T) {
	r := &PodHealthIndexResult{
		Summary: PodHealthSummary{CrashLoop: 2, HighRestart: 5, PendingPods: 1, NotReady: 3},
	}
	recs := buildPodHealthRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}

func TestBuildNSQuotaRecs(t *testing.T) {
	r := &NamespaceQuotaMapResult{
		Summary: NSQuotaMapSummary{WithoutQuota: 5, AtRiskNS: 2, WithLimitRange: 3, TotalNamespaces: 10},
	}
	recs := buildNSQuotaRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildSecretExpRecs(t *testing.T) {
	r := &SecretExposureScanResult{
		Summary: SecretExposureSummary{OrphanedSecrets: 10, PlaintextInEnv: 5, DockerConfig: 2, TLS: 3},
	}
	recs := buildSecretExpRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}

func TestAppendUniqueVal(t *testing.T) {
	s := []string{"a", "b"}
	s = appendUniqueVal(s, "a") // should not add duplicate
	if len(s) != 2 {
		t.Errorf("expected 2, got %d", len(s))
	}
	s = appendUniqueVal(s, "c") // should add new
	if len(s) != 3 {
		t.Errorf("expected 3, got %d", len(s))
	}
}
