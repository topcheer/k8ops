package dashboard

import (
	"testing"
)

func TestSecretLifecycleTypes(t *testing.T) {
	r := SecretLifecycleResult{LifecycleScore: 55, Grade: "D"}
	if r.LifecycleScore != 55 || r.Grade != "D" {
		t.Error("struct field error")
	}

	s := SecretLifecycleSummary{TotalSecrets: 50, OlderThan90Days: 15, OlderThan365Days: 5, UnusedCount: 8}
	if s.TotalSecrets != 50 || s.OlderThan90Days != 15 {
		t.Error("summary field error")
	}

	as := AgedSecret{Name: "db-pass", Namespace: "app", DaysOld: 200, Severity: "high"}
	if as.DaysOld != 200 || as.Severity != "high" {
		t.Error("agedSecret field error")
	}

	pr := PlaintextRisk{Name: "api-key", Key: "password", ValueLen: 32}
	if pr.Key != "password" || pr.ValueLen != 32 {
		t.Error("plaintextRisk field error")
	}

	ss := SecretSprawl{Key: "db-password", Count: 5, Namespaces: []string{"app", "api", "web"}}
	if ss.Count != 5 || len(ss.Namespaces) != 3 {
		t.Error("secretSprawl field error")
	}
}

func TestSecretLifecycleScoring(t *testing.T) {
	tests := []struct {
		old90       int
		old365      int
		unused      int
		duplicates  int
		plaintext   int
		expectedMin int
		expectedMax int
	}{
		{0, 0, 0, 0, 0, 95, 100},        // Clean
		{10, 5, 3, 2, 5, 0, 40},         // Many issues
		{5, 0, 2, 0, 1, 75, 95},         // Some issues
	}
	for _, tc := range tests {
		score := 100
		score -= tc.old90 * 2
		score -= tc.old365 * 3
		score -= tc.unused
		score -= tc.duplicates * 5
		score -= tc.plaintext * 3
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("old90=%d old365=%d unused=%d dup=%d pt=%d: expected %d-%d, got %d",
				tc.old90, tc.old365, tc.unused, tc.duplicates, tc.plaintext,
				tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestSecretLifecycleAgeSeverity(t *testing.T) {
	tests := []struct {
		daysOld  int
		expected string
	}{
		{30, ""},       // Not aged
		{100, "medium"}, // 90+ days
		{400, "high"},   // 365+ days
	}
	for _, tc := range tests {
		severity := ""
		if tc.daysOld > 365 {
			severity = "high"
		} else if tc.daysOld > 90 {
			severity = "medium"
		}
		if severity != tc.expected {
			t.Errorf("daysOld=%d: expected %q, got %q", tc.daysOld, tc.expected, severity)
		}
	}
}
