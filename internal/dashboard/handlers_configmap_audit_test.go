package dashboard

import (
	"testing"
)

func TestCmAuditRisk(t *testing.T) {
	// Large ConfigMap
	entry := ConfigMapEntry{IsLarge: true}
	if level := cmAuditRisk(entry); level != "high" {
		t.Errorf("Expected high for large CM, got %s", level)
	}

	// Unreferenced + empty
	entry = ConfigMapEntry{IsReferenced: false, DataKeys: 0}
	// 5 + 5 = 10 → medium
	if level := cmAuditRisk(entry); level != "medium" {
		t.Errorf("Expected medium for unreferenced+empty, got %s", level)
	}

	// Clean
	entry = ConfigMapEntry{IsReferenced: true, DataKeys: 3, IsLarge: false}
	if level := cmAuditRisk(entry); level != "low" {
		t.Errorf("Expected low for clean CM, got %s", level)
	}
}

func TestSecretAuditRisk(t *testing.T) {
	// Needs rotation
	entry := SecretEntry{NeedsRotation: true}
	if level := secretAuditRisk(entry); level != "high" {
		t.Errorf("Expected high for stale secret, got %s", level)
	}

	// Unreferenced + mutable
	entry = SecretEntry{IsReferenced: false, IsImmutable: false}
	// 5 + 5 = 10 → medium
	if level := secretAuditRisk(entry); level != "medium" {
		t.Errorf("Expected medium for unreferenced+mutable, got %s", level)
	}

	// Clean
	entry = SecretEntry{NeedsRotation: false, IsReferenced: true, IsImmutable: true}
	if level := secretAuditRisk(entry); level != "low" {
		t.Errorf("Expected low for clean secret, got %s", level)
	}
}

func TestCmAuditScore(t *testing.T) {
	// Clean
	s := ConfigAuditSummary{TotalConfigMaps: 10, TotalSecrets: 5}
	if score := cmAuditScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With issues
	s = ConfigAuditSummary{
		TotalConfigMaps:  10,
		TotalSecrets:     5,
		LargeCMs:         2, // -16
		OldSecrets:       1, // -6
		UnreferencedCMs:  3, // -6
		UnreferencedSecs: 1, // -3
		EmptyConfigs:     2, // -4
	}
	// 100 - 16 - 6 - 6 - 3 - 4 = 65
	if score := cmAuditScore(s); score != 65 {
		t.Errorf("Expected 65, got %d", score)
	}

	// Floor at 0
	s = ConfigAuditSummary{
		TotalConfigMaps: 5,
		LargeCMs:        20, // -160
	}
	if score := cmAuditScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestCmAuditRecs(t *testing.T) {
	s := ConfigAuditSummary{
		LargeCMs:         2,
		OldSecrets:       1,
		UnreferencedCMs:  3,
		UnreferencedSecs: 1,
		EmptyConfigs:     1,
		PlainTextSecrets: 2,
		HealthScore:      45,
	}

	recs := cmAuditRecs(s)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundLarge := false
	foundRotation := false
	foundUnref := false
	for _, r := range recs {
		if containsSubstr(r, "1MB") {
			foundLarge = true
		}
		if containsSubstr(r, "rotation") {
			foundRotation = true
		}
		if containsSubstr(r, "not referenced") {
			foundUnref = true
		}
	}
	if !foundLarge {
		t.Error("Expected recommendation about large ConfigMaps")
	}
	if !foundRotation {
		t.Error("Expected recommendation about secret rotation")
	}
	if !foundUnref {
		t.Error("Expected recommendation about unreferenced ConfigMaps")
	}
}

func TestCmAuditRecsClean(t *testing.T) {
	s := ConfigAuditSummary{
		TotalConfigMaps: 10,
		TotalSecrets:    5,
		HealthScore:     100,
	}
	recs := cmAuditRecs(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestCmAuditRank(t *testing.T) {
	if cmAuditRank("high") != 0 {
		t.Error("Expected 0 for high")
	}
	if cmAuditRank("medium") != 1 {
		t.Error("Expected 1 for medium")
	}
	if cmAuditRank("low") != 2 {
		t.Error("Expected 2 for low")
	}
}

func TestCmIssueRank(t *testing.T) {
	if cmIssueRank("warning") != 0 {
		t.Error("Expected 0 for warning")
	}
	if cmIssueRank("info") != 1 {
		t.Error("Expected 1 for info")
	}
}
