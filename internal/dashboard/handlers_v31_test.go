package dashboard

import (
	"testing"
)

func TestBuildSvcCatalogRecs(t *testing.T) {
	r := &ServiceCatalogResult{
		Summary: SvcCatalogSummary{NoEndpoints: 3, WithSelector: 5, LoadBalancer: 8},
	}
	recs := buildSvcCatalogRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestJoinStrs(t *testing.T) {
	if v := joinStrs([]string{}, ", "); v != "" {
		t.Errorf("expected empty, got %s", v)
	}
	if v := joinStrs([]string{"a", "b"}, ", "); v != "a, b" {
		t.Errorf("expected 'a, b', got %s", v)
	}
}

func TestMatchesSelector(t *testing.T) {
	wl := map[string]string{"app": "web", "env": "prod"}
	if !matchesSelector(wl, map[string]string{"app": "web"}) {
		t.Error("expected match for subset selector")
	}
	if matchesSelector(wl, map[string]string{"app": "db"}) {
		t.Error("expected no match for different value")
	}
}

func TestBuildTopologyRecs(t *testing.T) {
	r := &ResTopologyResult{
		Summary: ResTopoSummary{OrphanedCount: 5, SharedCount: 2},
	}
	recs := buildTopologyRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestSplitTopologyID(t *testing.T) {
	parts := splitTopologyID("cm/default/my-config")
	if len(parts) != 3 || parts[0] != "cm" {
		t.Errorf("unexpected parts: %v", parts)
	}
}
