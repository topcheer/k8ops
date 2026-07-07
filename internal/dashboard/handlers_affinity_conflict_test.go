package dashboard

import (
	"testing"
)

func TestAffIsAffinityRelated(t *testing.T) {
	related := []string{
		"node(s) didn't match pod anti-affinity/affinity",
		"pod anti-affinity rules",
		"node selector",
		"topology spread",
	}
	for _, r := range related {
		if !affIsAffinityRelated(r) {
			t.Errorf("Expected '%s' to be affinity related", r)
		}
	}

	notRelated := []string{"Insufficient cpu", "node(s) had untolerated taint", ""}
	for _, r := range notRelated {
		if affIsAffinityRelated(r) {
			t.Errorf("Expected '%s' to NOT be affinity related", r)
		}
	}
}

func TestAffAssessRisk(t *testing.T) {
	entry := AffEntry{Phase: "Pending", HasAntiAffinity: true}
	if level := affAssessRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	entry = AffEntry{HasAntiAffinity: true, AntiAffinityType: "required"}
	if level := affAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}

	entry = AffEntry{}
	if level := affAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestAffScore(t *testing.T) {
	if score := affScore(AffSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := AffSummary{
		TotalPods:                 50,
		Conflicts:                 2, // -30
		PendingDueToAffinity:      3, // -24
		WorkloadsWithAntiAffinity: 5, // (5-0)*2 = -10
	}
	// 100 - 30 - 24 - 10 = 36
	if score := affScore(s); score != 36 {
		t.Errorf("Expected 36, got %d", score)
	}
}

func TestAffGenRecs(t *testing.T) {
	s := AffSummary{
		TotalPods:                50,
		Conflicts:                2,
		PendingDueToAffinity:     3,
		RequiredDuringScheduling: 4,
		HealthScore:              30,
	}
	conflicts := []AffEntry{{Namespace: "app", Workload: "api", TopologyKey: "zone"}}

	recs := affGenRecs(s, conflicts, nil)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundConflict := false
	foundRequired := false
	for _, r := range recs {
		if strContains(r, "unsatisfiable") {
			foundConflict = true
		}
		if strContains(r, "required (hard)") {
			foundRequired = true
		}
	}
	if !foundConflict {
		t.Error("Expected recommendation about unsatisfiable rules")
	}
	if !foundRequired {
		t.Error("Expected recommendation about required anti-affinity")
	}
}

func TestAffGenRecsClean(t *testing.T) {
	s := AffSummary{TotalPods: 50}
	recs := affGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestIsTopologyLabel(t *testing.T) {
	topology := []string{
		"kubernetes.io/hostname",
		"topology.kubernetes.io/zone",
		"topology.kubernetes.io/region",
	}
	for _, key := range topology {
		if !isTopologyLabel(key) {
			t.Errorf("Expected '%s' to be topology label", key)
		}
	}

	nonTopology := []string{"app", "version", "tier"}
	for _, key := range nonTopology {
		if isTopologyLabel(key) {
			t.Errorf("Expected '%s' to NOT be topology label", key)
		}
	}
}

func TestAffTruncate(t *testing.T) {
	short := "hello"
	if result := affTruncate(short, 60); result != short {
		t.Errorf("Expected unchanged, got %s", result)
	}

	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	result := affTruncate(long, 20)
	if len(result) > 20 {
		t.Errorf("Expected max 20 chars, got %d", len(result))
	}
}

func TestAffIssueRank(t *testing.T) {
	if affIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if affIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
