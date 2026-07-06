package dashboard

import (
	"testing"
)

func TestOrphScore(t *testing.T) {
	// Empty
	if score := orphScore(OrphSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := OrphSummary{TotalServices: 10, TotalConfigMaps: 20, TotalSecrets: 15}
	if score := orphScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With orphans
	s = OrphSummary{
		TotalServices:   10,
		TotalConfigMaps: 20,
		TotalSecrets:    15,
		TotalPVCs:       5,
		TotalIngresses:  5,
		TotalOrphaned:   11, // 11/55 = 20%, score = 100 - 40 = 60
	}
	if score := orphScore(s); score != 60 {
		t.Errorf("Expected 60, got %d", score)
	}

	// Heavily orphaned
	s = OrphSummary{
		TotalServices:   5,
		TotalConfigMaps: 5,
		TotalSecrets:    5,
		TotalOrphaned:   12, // 12/15 = 80%, score = 100 - 160 < 0 → 0
	}
	if score := orphScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestOrphGenRecs(t *testing.T) {
	s := OrphSummary{
		OrphSecrets:   3,
		OrphIngress:   2,
		OrphServices:  4,
		OrphPVCs:      1,
		OrphConfigs:   5,
		TotalOrphaned: 15,
		HygieneScore:  40,
	}

	recs := orphGenRecs(s)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundSecrets := false
	foundIngress := false
	foundServices := false
	foundPVCs := false
	foundCleanup := false
	for _, r := range recs {
		if strContains(r, "orphaned Secret") {
			foundSecrets = true
		}
		if strContains(r, "orphaned Ingress") {
			foundIngress = true
		}
		if strContains(r, "orphaned Service") {
			foundServices = true
		}
		if strContains(r, "orphaned PVC") {
			foundPVCs = true
		}
		if strContains(r, "cleanup") {
			foundCleanup = true
		}
	}
	if !foundSecrets {
		t.Error("Expected recommendation about orphaned secrets")
	}
	if !foundIngress {
		t.Error("Expected recommendation about orphaned ingresses")
	}
	if !foundServices {
		t.Error("Expected recommendation about orphaned services")
	}
	if !foundPVCs {
		t.Error("Expected recommendation about orphaned PVCs")
	}
	if !foundCleanup {
		t.Error("Expected recommendation about CI/CD cleanup")
	}
}

func TestOrphGenRecsClean(t *testing.T) {
	s := OrphSummary{TotalOrphaned: 0}
	recs := orphGenRecs(s)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestOrphAddNS(t *testing.T) {
	m := make(map[string]*OrphNSEntry)
	orphAddNS(m, "default", "Service")
	orphAddNS(m, "default", "Service")
	orphAddNS(m, "default", "Secret")
	orphAddNS(m, "app1", "ConfigMap")

	if m["default"].OrphanCount != 3 {
		t.Errorf("Expected 3 orphans in default, got %d", m["default"].OrphanCount)
	}
	if m["default"].Details["Service"] != 2 {
		t.Errorf("Expected 2 services, got %d", m["default"].Details["Service"])
	}
	if m["default"].Details["Secret"] != 1 {
		t.Errorf("Expected 1 secret, got %d", m["default"].Details["Secret"])
	}
	if m["app1"].OrphanCount != 1 {
		t.Errorf("Expected 1 orphan in app1, got %d", m["app1"].OrphanCount)
	}
}

func TestOrphIssueRank(t *testing.T) {
	if orphIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if orphIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if orphIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
