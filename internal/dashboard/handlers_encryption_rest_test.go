package dashboard

import "testing"

func TestERScore(t *testing.T) {
	// No encryption = low score
	if score := erScore(ERSummary{}); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Fully configured
	s := ERSummary{
		EncryptionEnabled:   true,  // +60
		HasIdentityProvider: false, // +15
		ProviderCount:       1,     // +15
		ConfigDetected:      true,  // +10
	}
	if score := erScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Partial — encryption but identity provider
	s = ERSummary{
		EncryptionEnabled:   true, // +60
		HasIdentityProvider: true, // no +15
		ProviderCount:       1,    // +15
		ConfigDetected:      true, // +10
	}
	if score := erScore(s); score != 85 {
		t.Errorf("Expected 85, got %d", score)
	}
}

func TestERGenRecs(t *testing.T) {
	s := ERSummary{
		EncryptionEnabled: false,
		ConfigDetected:    true,
		SecurityScore:     0,
	}
	recs := erGenRecs(s, nil)
	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundEnable := false
	for _, r := range recs {
		if strContains(r, "Enable encryption") {
			foundEnable = true
		}
	}
	if !foundEnable {
		t.Error("Expected recommendation to enable encryption")
	}
}

func TestERGenRecsEnabled(t *testing.T) {
	s := ERSummary{
		EncryptionEnabled:   true,
		HasIdentityProvider: false,
		ProviderCount:       1,
		ConfigDetected:      true,
		SecurityScore:       100,
	}
	recs := erGenRecs(s, nil)
	foundPositive := false
	for _, r := range recs {
		if strContains(r, "properly configured") {
			foundPositive = true
		}
	}
	if !foundPositive {
		t.Error("Expected positive recommendation for good configuration")
	}
}

func TestERGenRecsIdentityProvider(t *testing.T) {
	s := ERSummary{
		EncryptionEnabled:   true,
		HasIdentityProvider: true,
		ProviderCount:       2,
		ConfigDetected:      true,
		SecurityScore:       85,
	}
	recs := erGenRecs(s, nil)
	foundIdentity := false
	for _, r := range recs {
		if strContains(r, "identity") {
			foundIdentity = true
		}
	}
	if !foundIdentity {
		t.Error("Expected recommendation about removing identity provider")
	}
}

func TestERStatusRank(t *testing.T) {
	if erStatusRank("fail") != 0 {
		t.Error("Expected 0")
	}
	if erStatusRank("pass") != 3 {
		t.Error("Expected 3")
	}
}

func TestERIssueRank(t *testing.T) {
	if erIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if erIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
