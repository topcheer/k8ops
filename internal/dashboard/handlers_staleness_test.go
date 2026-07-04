package dashboard

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractImageTag(t *testing.T) {
	tests := []struct {
		image  string
		expect string
	}{
		{"nginx:v1.2.3", "v1.2.3"},
		{"nginx", ""},
		{"nginx:latest", "latest"},
		{"registry.io/app:v2", "v2"},
		{"registry.io/app@sha256:abc123", "sha256"},
		{"registry.io/path/app:v1.0", "v1.0"},
	}

	for _, tt := range tests {
		got := extractImageTag(tt.image)
		if got != tt.expect {
			t.Errorf("extractImageTag(%q) = %q, want %q", tt.image, got, tt.expect)
		}
	}
}

func TestGetLatestUpdateTime(t *testing.T) {
	// With restartedAt annotation
	annotTime := time.Now().Add(-5 * 24 * time.Hour).Truncate(time.Second)
	annotations := map[string]string{
		"kubectl.kubernetes.io/restartedAt": annotTime.Format(time.RFC3339),
	}
	creation := metav1.Time{Time: time.Now().Add(-100 * 24 * time.Hour)}

	got := getLatestUpdateTime(annotations, creation)
	// Compare within 1 second tolerance (RFC3339 truncates to seconds)
	if got.Time.Sub(annotTime).Abs() > time.Second {
		t.Errorf("Expected annotation time ~%v, got %v", annotTime, got.Time)
	}
}

func TestGetLatestUpdateTimeNoAnnotation(t *testing.T) {
	// Without annotation, should return creation time
	creation := metav1.Time{Time: time.Now().Add(-50 * 24 * time.Hour)}
	got := getLatestUpdateTime(nil, creation)
	if !got.Time.Equal(creation.Time) {
		t.Errorf("Expected creation time, got different time")
	}
}

func TestGetLatestUpdateTimeInvalidAnnotation(t *testing.T) {
	// Invalid annotation should fall back to creation time
	creation := metav1.Time{Time: time.Now().Add(-50 * 24 * time.Hour)}
	annotations := map[string]string{
		"kubectl.kubernetes.io/restartedAt": "not-a-date",
	}
	got := getLatestUpdateTime(annotations, creation)
	if !got.Time.Equal(creation.Time) {
		t.Errorf("Expected creation time for invalid annotation")
	}
}

func TestAssessStalenessRisk(t *testing.T) {
	tests := []struct {
		name   string
		wl     StaleWorkload
		expect string
	}{
		{
			"fresh-no-latest",
			StaleWorkload{Status: "fresh", UsesLatest: false},
			"low",
		},
		{
			"stale-with-latest",
			StaleWorkload{Status: "stale", UsesLatest: true},
			"high",
		},
		{
			"ancient-no-tag",
			StaleWorkload{Status: "ancient", UsesNoTag: true},
			"critical",
		},
		{
			"very-stale",
			StaleWorkload{Status: "very-stale"},
			"high",
		},
		{
			"recent",
			StaleWorkload{Status: "recent"},
			"low",
		},
	}

	for _, tt := range tests {
		got := assessStalenessRisk(tt.wl)
		if got != tt.expect {
			t.Errorf("assessStalenessRisk(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestCalculateFreshnessScore(t *testing.T) {
	// Perfect score
	perfect := StalenessSummary{TotalWorkloads: 10, FreshWorkloads: 10}
	if score := calculateFreshnessScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := StalenessSummary{
		TotalWorkloads: 10,
		AncientWL:      1, // -10
		VeryStaleWL:    3, // includes 1 ancient, so 2 more at -5 = -10
		StaleWorkloads: 5, // includes 3 very stale, so 2 more at -2 = -4
		UsingLatest:    2, // -6
	}
	// 100 - 10 - 10 - 4 - 6 = 70
	score := calculateFreshnessScore(withIssues)
	if score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	// Floor at 0 (10 ancient * 10 = 100 penalty)
	terrible := StalenessSummary{
		TotalWorkloads: 10,
		StaleWorkloads: 10,
		VeryStaleWL:    10,
		AncientWL:      10,
	}
	if score := calculateFreshnessScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty
	empty := StalenessSummary{}
	if score := calculateFreshnessScore(empty); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}
}

func TestBuildAgeBuckets(t *testing.T) {
	workloads := []StaleWorkload{
		{UpdatedDaysAgo: 3},
		{UpdatedDaysAgo: 5},
		{UpdatedDaysAgo: 15},
		{UpdatedDaysAgo: 45},
		{UpdatedDaysAgo: 100},
		{UpdatedDaysAgo: 200},
	}

	buckets := buildAgeBuckets(workloads)

	if buckets[0].Count != 2 { // <7d
		t.Errorf("Expected 2 in <7d, got %d", buckets[0].Count)
	}
	if buckets[1].Count != 1 { // 7-30d
		t.Errorf("Expected 1 in 7-30d, got %d", buckets[1].Count)
	}
	if buckets[2].Count != 1 { // 30-90d
		t.Errorf("Expected 1 in 30-90d, got %d", buckets[2].Count)
	}
	if buckets[3].Count != 1 { // 90-180d
		t.Errorf("Expected 1 in 90-180d, got %d", buckets[3].Count)
	}
	if buckets[4].Count != 1 { // >180d
		t.Errorf("Expected 1 in >180d, got %d", buckets[4].Count)
	}
}

func TestGenerateStalenessRecommendations(t *testing.T) {
	result := StalenessResult{
		Summary: StalenessSummary{
			AncientWL:       2,
			VeryStaleWL:     3,
			UsingLatest:     1,
			NoRecentDeploys: 5,
			FreshnessScore:  40,
		},
		ByNamespace: []StaleNsStat{
			{Namespace: "default", Stale: 3, Total: 10},
		},
	}

	recs := generateStalenessRecommendations(result)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundAncient := false
	foundLatest := false
	for _, r := range recs {
		if containsSubstr(r, "180 days") {
			foundAncient = true
		}
		if containsSubstr(r, "latest") {
			foundLatest = true
		}
	}
	if !foundAncient {
		t.Error("Expected recommendation about ancient workloads")
	}
	if !foundLatest {
		t.Error("Expected recommendation about :latest tags")
	}
}

func TestGenerateStalenessRecommendationsClean(t *testing.T) {
	result := StalenessResult{
		Summary: StalenessSummary{
			TotalWorkloads: 10,
			FreshWorkloads: 10,
			FreshnessScore: 100,
		},
	}

	recs := generateStalenessRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for fresh cluster, got %d", len(recs))
	}
}

func TestGetOrCreateStaleNs(t *testing.T) {
	m := make(map[string]*StaleNsStat)

	e1 := getOrCreateStaleNs(m, "default")
	e1.Total = 5

	e2 := getOrCreateStaleNs(m, "default")
	if e2.Total != 5 {
		t.Errorf("Expected same entry with Total=5, got %d", e2.Total)
	}

	e3 := getOrCreateStaleNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected kube-system, got %s", e3.Namespace)
	}
}
