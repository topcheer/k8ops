package dashboard

import (
	"testing"
)

func TestSBMakeEntry(t *testing.T) {
	e := sbMakeEntry("pods_per_node", 100, 110, "desc")
	if e.RatioPercent < 90 || e.RatioPercent > 91 {
		t.Errorf("Expected ~90.9%%, got %.1f", e.RatioPercent)
	}
	if e.Status != "bottleneck" {
		t.Errorf("Expected bottleneck for >90%%, got %s", e.Status)
	}

	e = sbMakeEntry("total_nodes", 10, 5000, "desc")
	if e.Status != "healthy" {
		t.Errorf("Expected healthy for <50%%, got %s", e.Status)
	}

	e = sbMakeEntry("total_pods", 136000, 150000, "desc")
	if e.Status != "bottleneck" {
		t.Errorf("Expected bottleneck for >90%%, got %s", e.Status)
	}

	e = sbMakeEntry("total_services", 3000, 5000, "desc")
	if e.Status != "warning" {
		t.Errorf("Expected warning for >50%%, got %s", e.Status)
	}

	// Add critical test: 80/110 = 72.7%
	e = sbMakeEntry("max_pods", 80, 110, "desc")
	if e.Status != "critical" {
		t.Errorf("Expected critical for >70%%, got %s", e.Status)
	}
}

func TestSBScore(t *testing.T) {
	entries := []SBEntry{
		{Ratio: 0.1},
		{Ratio: 0.3},
	}
	// min safe = (1-0.3)*100 = 70
	if score := sbScore(entries); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	entries = []SBEntry{
		{Ratio: 0.95},
	}
	// min safe = (1-0.95)*100 = 5
	if score := sbScore(entries); score != 5 {
		t.Errorf("Expected 5, got %d", score)
	}

	if score := sbScore(nil); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestSBGenRecs(t *testing.T) {
	summary := SBSummary{
		MaxPodsPerNode:  95,
		AvgPodsPerNode:  82,
		TotalServices:   3500,
		TotalNamespaces: 100,
		RiskScore:       20,
	}
	bottlenecks := []SBEntry{
		{Resource: "max_pods_per_node", RatioPercent: 86.4, Current: 95, Limit: 110},
	}
	byNode := []SBNodeEntry{
		{NodeName: "node-1", PodCount: 95, PodRatio: 0.86},
		{NodeName: "node-2", PodCount: 90, PodRatio: 0.82},
	}

	recs := sbGenRecs(summary, bottlenecks, byNode)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundBottleneck := false
	foundMaxPods := false
	foundNearLimit := false
	for _, r := range recs {
		if strContains(r, "Primary bottleneck") {
			foundBottleneck = true
		}
		if strContains(r, "Max pods") {
			foundMaxPods = true
		}
		if strContains(r, "80%") {
			foundNearLimit = true
		}
	}
	if !foundBottleneck {
		t.Error("Expected recommendation about bottleneck")
	}
	if !foundMaxPods {
		t.Error("Expected recommendation about max pods")
	}
	if !foundNearLimit {
		t.Error("Expected recommendation about near-limit nodes")
	}
}

func TestSBGenRecsClean(t *testing.T) {
	summary := SBSummary{RiskScore: 90}
	recs := sbGenRecs(summary, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}
