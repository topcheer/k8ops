package dashboard

import "testing"

func TestControlPlaneResult1912(t *testing.T) {
	r := ControlPlaneResult{
		Summary:     ControlPlaneSummary{TotalComponents: 5, Healthy: 5, Degraded: 0, Unhealthy: 0, TotalReplicas: 10, ReadyReplicas: 10},
		HealthScore: 100,
	}
	if r.Summary.Healthy != 5 {
		t.Errorf("expected 5, got %d", r.Summary.Healthy)
	}
}

func TestCSIDriverResult1912(t *testing.T) {
	r := CSIDriverResult{
		Summary:     CSIDriverSummary{TotalDrivers: 3, HealthyDrivers: 2, NodePluginsFound: 3, PluginPodsReady: 8, PluginPodsTotal: 10},
		HealthScore: 80,
	}
	if r.Summary.PluginPodsReady != 8 {
		t.Errorf("expected 8, got %d", r.Summary.PluginPodsReady)
	}
}

func TestCertRenewalResult1912(t *testing.T) {
	r := CertRenewalResult{
		Summary:     CertRenewalSummary{TotalSecrets: 20, TLSSecrets: 10, Expiring30d: 5, Expiring7d: 2, Expired: 1},
		HealthScore: 85,
	}
	if r.Summary.Expired != 1 {
		t.Errorf("expected 1, got %d", r.Summary.Expired)
	}
}

func TestBuildControlPlaneRecs1912(t *testing.T) {
	r := &ControlPlaneResult{Summary: ControlPlaneSummary{TotalComponents: 5, Healthy: 3, Degraded: 1, Unhealthy: 1, ReadyReplicas: 8, TotalReplicas: 10}}
	recs := buildControlPlaneRecs1912(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildCSIDriverRecs1912(t *testing.T) {
	r := &CSIDriverResult{Summary: CSIDriverSummary{TotalDrivers: 3, HealthyDrivers: 2, NodePluginsFound: 3, PluginPodsReady: 8, PluginPodsTotal: 10}, Issues: []CSIDriverEntry1912{{}}}
	recs := buildCSIDriverRecs1912(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildCertRenewalRecs1912(t *testing.T) {
	r := &CertRenewalResult{Summary: CertRenewalSummary{TotalSecrets: 20, Expiring30d: 5, Expiring7d: 2, Expired: 1}}
	recs := buildCertRenewalRecs1912(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestMinInt1912(t *testing.T) {
	if minInt1912(3, 5) != 3 {
		t.Error("expected 3")
	}
	if minInt1912(7, 2) != 2 {
		t.Error("expected 2")
	}
}
