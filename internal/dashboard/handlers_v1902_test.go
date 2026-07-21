package dashboard

import "testing"

func TestVolEncryptionResult1902(t *testing.T) {
	r := VolEncryptionResult{
		Summary:     VolEncSummary{TotalPVCs: 15, EncryptedPVCs: 5, UnencryptedPVCs: 10},
		HealthScore: 33,
	}
	if r.Summary.UnencryptedPVCs != 10 {
		t.Errorf("expected 10, got %d", r.Summary.UnencryptedPVCs)
	}
}

func TestWebhookPostureResult1902(t *testing.T) {
	r := WebhookPostureResult{
		Summary:     WebhookSummary{TotalWebhooks: 6, ValidatingWebhooks: 4, MutatingWebhooks: 2, WithoutTLS: 1},
		HealthScore: 83,
	}
	if r.Summary.MutatingWebhooks != 2 {
		t.Errorf("expected 2, got %d", r.Summary.MutatingWebhooks)
	}
}

func TestKeyRotationResult1902(t *testing.T) {
	r := KeyRotationResult{
		Summary:     KeyRotationSummary{TotalSecrets: 50, FreshSecrets: 10, Overdue90Days: 30, Overdue365Days: 5},
		HealthScore: 10,
	}
	if r.Summary.Overdue365Days != 5 {
		t.Errorf("expected 5, got %d", r.Summary.Overdue365Days)
	}
}

func TestBuildVolEncRecs1902(t *testing.T) {
	r := &VolEncryptionResult{Summary: VolEncSummary{TotalPVCs: 15, EncryptedPVCs: 5, UnencryptedPVCs: 10, EncryptedSCs: 1, TotalStorageClasses: 3}}
	recs := buildVolEncRecs1902(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildWebhookRecs1902(t *testing.T) {
	r := &WebhookPostureResult{Summary: WebhookSummary{TotalWebhooks: 6, WithoutTLS: 1, HighLatency: 2}}
	recs := buildWebhookRecs1902(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildKeyRotationRecs1902(t *testing.T) {
	r := &KeyRotationResult{Summary: KeyRotationSummary{TotalSecrets: 50, FreshSecrets: 10, Overdue90Days: 30, Overdue365Days: 5}}
	recs := buildKeyRotationRecs1902(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
