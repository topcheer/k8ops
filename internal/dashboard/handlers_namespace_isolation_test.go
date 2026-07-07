package dashboard

import (
	"testing"
)

func TestNIIsSystem(t *testing.T) {
	prefixes := []string{"kube-", "cattle-", "traefik"}
	if !niIsSystem("kube-system", prefixes) {
		t.Error("Expected kube-system to be system")
	}
	if !niIsSystem("cattle-monitoring", prefixes) {
		t.Error("Expected cattle- to be system")
	}
	if !niIsSystem("default", prefixes) {
		t.Error("Expected default to be system")
	}
	if niIsSystem("my-app", prefixes) {
		t.Error("Expected my-app to NOT be system")
	}
}

func TestNIAssessRisk(t *testing.T) {
	entry := NIEntry{IsSystem: true}
	if niAssessRisk(entry) != "low" {
		t.Error("Expected low for system ns")
	}

	entry = NIEntry{HasNetworkPolicy: false, HasResourceQuota: false, HasLimitRange: false}
	if niAssessRisk(entry) != "high" {
		t.Error("Expected high for no controls")
	}

	entry = NIEntry{HasNetworkPolicy: true, HasResourceQuota: false}
	if niAssessRisk(entry) != "medium" {
		t.Error("Expected medium for partial")
	}

	entry = NIEntry{HasNetworkPolicy: true, HasResourceQuota: true, HasLimitRange: true}
	if niAssessRisk(entry) != "low" {
		t.Error("Expected low for fully isolated")
	}
}

func TestNIScore(t *testing.T) {
	if score := niScore(NISummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := NISummary{
		UserNamespaces:    10,
		WithNetworkPolicy: 5, // 5 unisolated * 5 = -25
		WithResourceQuota: 7, // 3 no quota * 3 = -9
		WithLimitRange:    8, // 2 no lr * 2 = -4
	}
	// 100 - 25 - 9 - 4 = 62
	if score := niScore(s); score != 62 {
		t.Errorf("Expected 62, got %d", score)
	}
}

func TestNIGenRecs(t *testing.T) {
	s := NISummary{
		UserNamespaces:    10,
		WithNetworkPolicy: 5,
		WithResourceQuota: 7,
		WithLimitRange:    8,
		FullyIsolated:     4,
		IsolationScore:    50,
	}
	unisolated := []NIEntry{{Name: "app1"}}
	noQuota := []NIEntry{{Name: "app2"}}

	recs := niGenRecs(s, unisolated, noQuota)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundNp := false
	foundQuota := false
	for _, r := range recs {
		if strContains(r, "NetworkPolicy") {
			foundNp = true
		}
		if strContains(r, "ResourceQuota") {
			foundQuota = true
		}
	}
	if !foundNp {
		t.Error("Expected recommendation about NetworkPolicy")
	}
	if !foundQuota {
		t.Error("Expected recommendation about ResourceQuota")
	}
}

func TestNIGenRecsClean(t *testing.T) {
	s := NISummary{UserNamespaces: 5, WithNetworkPolicy: 5, WithResourceQuota: 5, FullyIsolated: 5}
	recs := niGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestNIRiskRank(t *testing.T) {
	if niRiskRank("high") != 0 {
		t.Error("Expected 0")
	}
	if niRiskRank("medium") != 1 {
		t.Error("Expected 1")
	}
	if niRiskRank("low") != 2 {
		t.Error("Expected 2")
	}
}
