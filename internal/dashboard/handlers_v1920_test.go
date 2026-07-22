package dashboard

import "testing"

func TestPolicyCatalogResult1920(t *testing.T) {
	r := PolicyCatalogResult1920{
		Summary:     PolicyCatalogSummary{TotalNetworkPolicies: 5, NamespacesWithPSA: 3, TotalRBACBindings: 12},
		HealthScore: 85,
	}
	if r.Summary.TotalNetworkPolicies != 5 {
		t.Errorf("expected 5, got %d", r.Summary.TotalNetworkPolicies)
	}
	if r.HealthScore != 85 {
		t.Errorf("expected 85, got %d", r.HealthScore)
	}
}

func TestPolicyCatalogSummary1920(t *testing.T) {
	s := PolicyCatalogSummary{
		NamespacesWithoutNetPol: 10,
		NamespacesWithoutPSA:    7,
		ClusterAdminBindings:    3,
	}
	if s.NamespacesWithoutNetPol != 10 {
		t.Errorf("expected 10, got %d", s.NamespacesWithoutNetPol)
	}
	if s.ClusterAdminBindings != 3 {
		t.Errorf("expected 3, got %d", s.ClusterAdminBindings)
	}
}

func TestPolicyGapEntry1920(t *testing.T) {
	g := PolicyGapEntry1920{
		Namespace: "production",
		GapType:   "NetworkPolicy",
		Severity:  "high",
	}
	if g.Severity != "high" {
		t.Errorf("expected high, got %s", g.Severity)
	}
}

func TestServiceDepGraphResult1920(t *testing.T) {
	r := ServiceDepGraphResult1920{
		Summary: ServiceDepSummary1920{TotalServices: 25, TotalDeps: 40, CrossNSDeps: 5, OrphanCount: 3},
	}
	if r.Summary.TotalServices != 25 {
		t.Errorf("expected 25, got %d", r.Summary.TotalServices)
	}
	if r.Summary.CrossNSDeps != 5 {
		t.Errorf("expected 5, got %d", r.Summary.CrossNSDeps)
	}
}

func TestServiceDepHub1920(t *testing.T) {
	h := ServiceDepHub1920{
		Service:     "postgres",
		Namespace:   "data",
		Connections: 8,
	}
	if h.Connections != 8 {
		t.Errorf("expected 8, got %d", h.Connections)
	}
}

func TestPerfBaselineResult1920(t *testing.T) {
	r := PerfBaselineResult1920{
		Summary: PerfBaselineSummary1920{TotalWorkloads: 15, AvgCPURequest: 1.5, TotalAnomalies: 3},
	}
	if r.Summary.TotalWorkloads != 15 {
		t.Errorf("expected 15, got %d", r.Summary.TotalWorkloads)
	}
	if r.Summary.AvgCPURequest != 1.5 {
		t.Errorf("expected 1.5, got %f", r.Summary.AvgCPURequest)
	}
}

func TestPerfThreshold1920(t *testing.T) {
	t1 := PerfThreshold1920{Metric: "cpu-request", Threshold: "0.5-2 cores", ActionType: "alert-if-exceeded"}
	if t1.Metric != "cpu-request" {
		t.Errorf("expected cpu-request, got %s", t1.Metric)
	}
}

func TestGetPartHelper1920(t *testing.T) {
	parts := []string{"ns", "name"}
	if getPart(parts, 0) != "ns" {
		t.Errorf("expected ns")
	}
	if getPart(parts, 1) != "name" {
		t.Errorf("expected name")
	}
	if getPart(parts, 5) != "" {
		t.Errorf("expected empty for out of bounds")
	}
}
