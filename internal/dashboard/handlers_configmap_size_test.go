package dashboard

import (
	"testing"
)

func TestCMSAnalyzeEntry(t *testing.T) {
	entry := cmsAnalyzeEntry("test-cm", "default", "ConfigMap",
		map[string]string{"key1": "hello", "key2": "world"},
		nil,
	)
	// "hello" + "world" = 10 bytes = ~0.01KB
	if entry.KeyCount != 2 {
		t.Errorf("Expected 2 keys, got %d", entry.KeyCount)
	}
	if entry.SizeKB < 0.009 || entry.SizeKB > 0.011 {
		t.Errorf("Expected ~0.01KB, got %.4f", entry.SizeKB)
	}
}

func TestCMSAssessRisk(t *testing.T) {
	if cmsAssessRisk(CMEntry{SizeKB: 2048}) != "high" {
		t.Error("Expected high for >1MB")
	}
	if cmsAssessRisk(CMEntry{SizeKB: 700}) != "medium" {
		t.Error("Expected medium for >512KB")
	}
	if cmsAssessRisk(CMEntry{SizeKB: 100}) != "low" {
		t.Error("Expected low")
	}
}

func TestCMSScore(t *testing.T) {
	if score := cmsScore(CMSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := CMSummary{
		OversizedCMs:      3,   // -15
		OversizedSecrets:  2,   // -10
		TotalCMSizeMB:     150, // -5
		TotalSecretSizeMB: 60,  // -5
	}
	// 100 - 15 - 10 - 5 - 5 = 65
	if score := cmsScore(s); score != 65 {
		t.Errorf("Expected 65, got %d", score)
	}
}

func TestCMSGenRecs(t *testing.T) {
	s := CMSummary{
		OversizedCMs:      3,
		OversizedSecrets:  2,
		TotalCMSizeMB:     80,
		TotalSecretSizeMB: 30,
		HealthScore:       50,
	}
	oversizedCMs := []CMEntry{{Namespace: "app", Name: "big-config", SizeKB: 2048}}

	recs := cmsGenRecs(s, oversizedCMs, nil)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundOversized := false
	foundSecret := false
	for _, r := range recs {
		if strContains(r, "exceed 1MB") {
			foundOversized = true
		}
		if strContains(r, "external secret") {
			foundSecret = true
		}
	}
	if !foundOversized {
		t.Error("Expected recommendation about oversized ConfigMaps")
	}
	if !foundSecret {
		t.Error("Expected recommendation about external secret management")
	}
}

func TestCMSGenRecsClean(t *testing.T) {
	s := CMSummary{TotalCMSizeMB: 5, TotalSecretSizeMB: 2}
	recs := cmsGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestCMSGetOrCreateNS(t *testing.T) {
	m := make(map[string]*CMNSEntry)
	e1 := cmsGetOrCreateNS(m, "default")
	e1.CMCount = 5

	e2 := cmsGetOrCreateNS(m, "default")
	if e2.CMCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.CMCount)
	}

	e3 := cmsGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}
