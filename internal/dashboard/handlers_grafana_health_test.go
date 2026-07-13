package dashboard

import (
	"testing"
)

func TestExtractJSONValue(t *testing.T) {
	json := `{"title":"My Dashboard","refresh":"30s","datasource":"Prometheus"}`
	if got := extractJSONValue(json, "title"); got != "My Dashboard" {
		t.Errorf("title = %q, want My Dashboard", got)
	}
	if got := extractJSONValue(json, "refresh"); got != "30s" {
		t.Errorf("refresh = %q, want 30s", got)
	}
	if got := extractJSONValue(json, "datasource"); got != "Prometheus" {
		t.Errorf("datasource = %q, want Prometheus", got)
	}
	if got := extractJSONValue(json, "nonexistent"); got != "" {
		t.Errorf("nonexistent = %q, want empty", got)
	}
}

func TestCountPanels(t *testing.T) {
	json := `{"panels":[{"type":"graph"},{"type":"table"},{"type":"stat"}]}`
	if got := countPanels(json); got != 3 {
		t.Errorf("countPanels = %d, want 3", got)
	}
	if got := countPanels(""); got != 0 {
		t.Errorf("empty countPanels = %d, want 0", got)
	}
}

func TestExtractGrafanaVersion(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"grafana/grafana:10.2.0", "10.2.0"},
		{"grafana/grafana:9.5.1-ubuntu", "9.5.1"},
		{"grafana/grafana", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractGrafanaVersion(tt.image); got != tt.expected {
			t.Errorf("extractGrafanaVersion(%q) = %q, want %q", tt.image, got, tt.expected)
		}
	}
}

func TestCountUniqueDatasources(t *testing.T) {
	dashboards := []GrafanaDashboard{
		{DatasourceRef: "Prometheus"},
		{DatasourceRef: "Prometheus"},
		{DatasourceRef: "Loki"},
		{DatasourceRef: ""},
	}
	if got := countUniqueDatasources(dashboards); got != 2 {
		t.Errorf("countUniqueDatasources = %d, want 2", got)
	}
}

func TestAssessGrafanaDashRisk(t *testing.T) {
	// Healthy dashboard
	dash := GrafanaDashboard{
		RefreshRate:   "30s",
		DatasourceRef: "Prometheus",
		HasTimeRange:  true,
		PanelCount:    5,
	}
	if got := assessGrafanaDashRisk(dash); got != "healthy" {
		t.Errorf("healthy dash risk = %s, want healthy", got)
	}

	// Critical: stale + no datasource + no time range
	dash = GrafanaDashboard{
		IsStale:       true,
		DatasourceRef: "",
		HasTimeRange:  false,
		PanelCount:    5,
	}
	if got := assessGrafanaDashRisk(dash); got != "critical" {
		t.Errorf("critical dash risk = %s, want critical", got)
	}
}

func TestComputeGrafanaHealthScore(t *testing.T) {
	// No Grafana → 50
	score := computeGrafanaHealthScore(GrafanaSummary{GrafanaDetected: false}, 0)
	if score != 50 {
		t.Fatalf("no-grafana score = %d, want 50", score)
	}

	// All healthy
	score = computeGrafanaHealthScore(GrafanaSummary{
		GrafanaDetected:    true,
		GrafanaPodCount:    2,
		ReadyPods:          2,
		TotalDashboards:    10,
		DashboardsWithData: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Not ready pods + broken dashboards
	score = computeGrafanaHealthScore(GrafanaSummary{
		GrafanaDetected:  true,
		GrafanaPodCount:  3,
		ReadyPods:        1,
		BrokenDashboards: 3,
		StaleDashboards:  2,
	}, 5)
	if score > 50 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-50", score)
	}
}
