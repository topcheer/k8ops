package dashboard

import "testing"

func TestDAVScore(t *testing.T) {
	// Clean cluster
	if score := davScore(DAVSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}
	// Has removed APIs
	s := DAVSummary{RemovedCount: 2, DeprecatedCount: 1}
	// 100 - 60 - 15 = 25
	if score := davScore(s); score != 25 {
		t.Errorf("Expected 25, got %d", score)
	}
	// Only deprecated
	s = DAVSummary{DeprecatedCount: 3}
	// 100 - 45 = 55
	if score := davScore(s); score != 55 {
		t.Errorf("Expected 55, got %d", score)
	}
}

func TestDAVGenRecs(t *testing.T) {
	s := DAVSummary{
		RemovedCount:    2,
		DeprecatedCount: 1,
		ReadinessScore:  25,
	}
	removed := []DAVEntry{
		{Resource: "CronJob", OldVersion: "batch/v1beta1", NewVersion: "batch/v1"},
		{Resource: "PSP", OldVersion: "policy/v1beta1", NewVersion: "(removed)"},
	}
	deprecated := []DAVEntry{
		{Resource: "Ingress", OldVersion: "networking.k8s.io/v1beta1", NewVersion: "networking.k8s.io/v1"},
	}
	recs := davGenRecs(s, deprecated, removed)
	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}
	foundRemoved := false
	foundMigrate := false
	for _, r := range recs {
		if strContains(r, "REMOVED") {
			foundRemoved = true
		}
		if strContains(r, "Migrate") {
			foundMigrate = true
		}
	}
	if !foundRemoved {
		t.Error("Expected recommendation about removed APIs")
	}
	if !foundMigrate {
		t.Error("Expected migration recommendation")
	}
}

func TestDAVGenRecsClean(t *testing.T) {
	s := DAVSummary{ReadyForUpgrade: true}
	recs := davGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestDAVGenRecsNoReplacement(t *testing.T) {
	s := DAVSummary{RemovedCount: 1, ReadinessScore: 70}
	removed := []DAVEntry{
		{Resource: "PodSecurityPolicy", OldVersion: "policy/v1beta1", NewVersion: "(removed)"},
	}
	recs := davGenRecs(s, nil, removed)
	foundNoReplace := false
	for _, r := range recs {
		if strContains(r, "no replacement") {
			foundNoReplace = true
		}
	}
	if !foundNoReplace {
		t.Error("Expected recommendation about no replacement")
	}
}

func TestDAVIssueRank(t *testing.T) {
	if davIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if davIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
