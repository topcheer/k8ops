package dashboard

import (
	"testing"
)

func TestAPScore(t *testing.T) {
	// No audit = 0
	if score := apScore(APSummary{}); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Fully compliant
	s := APSummary{
		AuditEnabled:          true,   // +40
		HasPolicy:             true,   // +25
		SensitiveVerbsAudited: true,   // +15
		MaxAgeDays:            90,     // +10
		MaxBackupFiles:        10,     // +5
		LogBackend:            "both", // +5
	}
	if score := apScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Partial
	s = APSummary{
		AuditEnabled: true, // +40
		HasPolicy:    true, // +25
		MaxAgeDays:   30,   // +5
	}
	// 40 + 25 + 5 = 70
	if score := apScore(s); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}
}

func TestAPGenRecs(t *testing.T) {
	s := APSummary{
		AuditEnabled:    false,
		HasPolicy:       false,
		MaxAgeDays:      0,
		ComplianceScore: 0,
	}

	recs := apGenRecs(s, nil)

	if len(recs) < 2 {
		t.Errorf("Expected at least 2 recommendations, got %d", len(recs))
	}

	foundEnable := false
	for _, r := range recs {
		if strContains(r, "Enable") {
			foundEnable = true
		}
	}
	if !foundEnable {
		t.Error("Expected recommendation to enable audit logging")
	}
}

func TestAPGenRecsPartial(t *testing.T) {
	s := APSummary{
		AuditEnabled:    true,
		HasPolicy:       false,
		MaxAgeDays:      7,
		MaxBackupFiles:  3,
		LogBackend:      "file",
		ComplianceScore: 45,
	}

	recs := apGenRecs(s, nil)

	foundPolicy := false
	foundRetention := false
	foundWebhook := false
	for _, r := range recs {
		if strContains(r, "audit policy") {
			foundPolicy = true
		}
		if strContains(r, "retention") {
			foundRetention = true
		}
		if strContains(r, "webhook") {
			foundWebhook = true
		}
	}
	if !foundPolicy {
		t.Error("Expected recommendation about audit policy")
	}
	if !foundRetention {
		t.Error("Expected recommendation about retention")
	}
	if !foundWebhook {
		t.Error("Expected recommendation about webhook backend")
	}
}

func TestAPGenRecsCompliant(t *testing.T) {
	s := APSummary{
		AuditEnabled:    true,
		HasPolicy:       true,
		MaxAgeDays:      90,
		ComplianceScore: 95,
	}

	recs := apGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestAPStatusRank(t *testing.T) {
	if apStatusRank("fail") != 0 {
		t.Error("Expected 0")
	}
	if apStatusRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if apStatusRank("pass") != 3 {
		t.Error("Expected 3")
	}
}

func TestAPIssueRank(t *testing.T) {
	if apIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if apIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
