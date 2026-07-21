package dashboard

import "testing"

func TestClusterRunbookResult1903(t *testing.T) {
	r := ClusterRunbookResult{
		Summary:     RunbookSummary1903{TotalWorkloads: 60, CriticalSOPs: 4, Nodes: 1},
		HealthScore: 80,
	}
	if r.Summary.CriticalSOPs != 4 {
		t.Errorf("expected 4, got %d", r.Summary.CriticalSOPs)
	}
}

func TestAPIDriftResult1903(t *testing.T) {
	r := APIDriftResult{
		Summary:     APIDriftSummary{TotalAPIs: 150, CurrentAPIs: 140, DeprecatedAPIs: 5, RemovedAPIs: 2},
		HealthScore: 93,
	}
	if r.Summary.RemovedAPIs != 2 {
		t.Errorf("expected 2, got %d", r.Summary.RemovedAPIs)
	}
}

func TestTopologyDocResult1903(t *testing.T) {
	r := TopologyDocResult{
		Summary:     TopologySummary1903{TotalNamespaces: 4, TotalWorkloads: 60, TotalServices: 30, TotalIngress: 5},
		HealthScore: 100,
	}
	if r.Summary.TotalServices != 30 {
		t.Errorf("expected 30, got %d", r.Summary.TotalServices)
	}
}

func TestGenerateCriticalSOPs1903(t *testing.T) {
	sops := generateCriticalSOPs1903()
	if len(sops) < 3 {
		t.Errorf("expected >= 3 SOPs, got %d", len(sops))
	}
}

func TestBuildRunbookRecs1903(t *testing.T) {
	r := &ClusterRunbookResult{Summary: RunbookSummary1903{RunbookSections: 5, CriticalSOPs: 4, TotalWorkloads: 60, Namespaces: 4, Nodes: 1}}
	recs := buildRunbookRecs1903(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildAPIDriftRecs1903(t *testing.T) {
	r := &APIDriftResult{Summary: APIDriftSummary{TotalAPIs: 150, CurrentAPIs: 140, DeprecatedAPIs: 5, RemovedAPIs: 2, PreviewAPIs: 3}}
	recs := buildAPIDriftRecs1903(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildTopologyRecs1903(t *testing.T) {
	r := &TopologyDocResult{Summary: TopologySummary1903{TotalNamespaces: 4, TotalWorkloads: 60, TotalServices: 30, EdgeNodes: 2}}
	recs := buildTopologyRecs1903(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
