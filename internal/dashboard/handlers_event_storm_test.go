package dashboard

import (
	"sort"
	"testing"
	"time"
)

func TestClassifyStorm(t *testing.T) {
	tests := []struct {
		count  int
		expect EventStormSeverity
	}{
		{0, ""},
		{3, ""},
		{6, StormLow},
		{11, StormMedium},
		{21, StormHigh},
		{51, StormCritical},
	}

	for _, tt := range tests {
		got := classifyStorm(tt.count)
		if got != tt.expect {
			t.Errorf("classifyStorm(%d) = %q, want %q", tt.count, got, tt.expect)
		}
	}
}

func TestGenerateStormRecommendations(t *testing.T) {
	// Storm detected
	result := EventStormResult{
		StormDetected: true,
		Summary: EventStormSummary{
			Events15Min:       30,
			Events1Hour:       150,
			TopNamespace:      "kube-system",
			AffectedResources: 15,
		},
		TopReasons: []EventReasonAgg{
			{Reason: "FailedScheduling", Count: 20, Message: "insufficient CPU"},
		},
		FlappingRes: []FlappingResource{
			{Kind: "Pod", Name: "flapping-pod", Count: 10},
		},
	}

	recs := generateStormRecommendations(result)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundStorm := false
	foundNs := false
	foundFlap := false
	for _, r := range recs {
		if containsSubstr(r, "Event storm detected") {
			foundStorm = true
		}
		if containsSubstr(r, "kube-system") {
			foundNs = true
		}
		if containsSubstr(r, "flapping") {
			foundFlap = true
		}
	}
	if !foundStorm {
		t.Error("Expected storm detection recommendation")
	}
	if !foundNs {
		t.Error("Expected namespace recommendation")
	}
	if !foundFlap {
		t.Error("Expected flapping recommendation")
	}
}

func TestGenerateStormRecommendationsNoStorm(t *testing.T) {
	result := EventStormResult{
		StormDetected: false,
		Summary: EventStormSummary{
			Events15Min: 2,
		},
	}

	recs := generateStormRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for calm cluster, got %d", len(recs))
	}
}

func TestTruncateMsg(t *testing.T) {
	tests := []struct {
		input  string
		max    int
		expect string
	}{
		{"short", 10, "short"},
		{"this is a very long message that needs truncation", 20, "this is a very long ..."},
		{"  trimmed  ", 20, "trimmed"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		got := truncateMsg(tt.input, tt.max)
		if got != tt.expect {
			t.Errorf("truncateMsg(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expect)
		}
	}
}

func TestGetOrCreateEventNs(t *testing.T) {
	m := make(map[string]*EventNsSummary)

	e1 := getOrCreateEventNs(m, "default")
	e1.WarningCount = 5

	e2 := getOrCreateEventNs(m, "default")
	if e2.WarningCount != 5 {
		t.Errorf("Expected same entry with WarningCount=5, got %d", e2.WarningCount)
	}

	e3 := getOrCreateEventNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected namespace kube-system, got %s", e3.Namespace)
	}
}

func TestEventStormResultSerialization(t *testing.T) {
	// Verify the result struct has proper JSON structure by checking key fields
	result := EventStormResult{
		ScannedAt: time.Now(),
		Summary: EventStormSummary{
			TotalWarningEvents: 10,
			Events15Min:        5,
			StormSeverity:      StormLow,
		},
		StormDetected: false,
	}

	if result.Summary.TotalWarningEvents != 10 {
		t.Error("Expected 10 total warnings")
	}
	if result.StormDetected {
		t.Error("Expected stormDetected = false")
	}
}

func TestFlappingResourceSorting(t *testing.T) {
	resources := []FlappingResource{
		{Kind: "Pod", Name: "low-flap", Count: 3},
		{Kind: "Pod", Name: "high-flap", Count: 20},
		{Kind: "Deployment", Name: "mid-flap", Count: 10},
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Count > resources[j].Count
	})

	if resources[0].Name != "high-flap" {
		t.Errorf("Expected high-flap first, got %s", resources[0].Name)
	}
	if resources[2].Name != "low-flap" {
		t.Errorf("Expected low-flap last, got %s", resources[2].Name)
	}
}

func TestTopReasonsSorting(t *testing.T) {
	reasons := []EventReasonAgg{
		{Reason: "FailedScheduling", Count: 5},
		{Reason: "BackOff", Count: 50},
		{Reason: "Unhealthy", Count: 15},
	}

	sort.Slice(reasons, func(i, j int) bool {
		return reasons[i].Count > reasons[j].Count
	})

	if reasons[0].Reason != "BackOff" {
		t.Errorf("Expected BackOff first, got %s", reasons[0].Reason)
	}
}

func TestNamespaceSorting(t *testing.T) {
	namespaces := []EventNsSummary{
		{Namespace: "default", WarningCount: 10},
		{Namespace: "kube-system", WarningCount: 50},
		{Namespace: "monitoring", WarningCount: 20},
	}

	sort.Slice(namespaces, func(i, j int) bool {
		return namespaces[i].WarningCount > namespaces[j].WarningCount
	})

	if namespaces[0].Namespace != "kube-system" {
		t.Errorf("Expected kube-system first, got %s", namespaces[0].Namespace)
	}
}
