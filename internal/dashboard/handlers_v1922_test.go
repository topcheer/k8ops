package dashboard

import "testing"

func TestRollbackWindowResult1922(t *testing.T) {
	r := RollbackWindowResult1922{
		Summary:     RollbackWindowSummary1922{TotalDeployments: 20, RollbackReady: 18, NotReady: 2},
		HealthScore: 90,
	}
	if r.Summary.RollbackReady != 18 {
		t.Errorf("expected 18, got %d", r.Summary.RollbackReady)
	}
}

func TestRollbackRiskEntry1922(t *testing.T) {
	e := RollbackRiskEntry1922{Name: "api", Namespace: "prod", RiskType: "single-replica"}
	if e.RiskType != "single-replica" {
		t.Errorf("expected single-replica, got %s", e.RiskType)
	}
}

func TestDNSScalabilityResult1922(t *testing.T) {
	r := DNSScalabilityResult1922{
		Summary: DNSScalabilitySummary1922{CoreDNSReplicas: 2, TotalPods: 100, PodsPerCoreDNS: 50},
	}
	if r.Summary.CoreDNSReplicas != 2 {
		t.Errorf("expected 2, got %d", r.Summary.CoreDNSReplicas)
	}
	if r.Summary.PodsPerCoreDNS != 50 {
		t.Errorf("expected 50, got %f", r.Summary.PodsPerCoreDNS)
	}
}

func TestDNSBottleneck1922(t *testing.T) {
	b := DNSBottleneck1922{Type: "no-autoscaler", Severity: "medium"}
	if b.Severity != "medium" {
		t.Errorf("expected medium, got %s", b.Severity)
	}
}

func TestConnPoolResult1922(t *testing.T) {
	r := ConnPoolResult1922{
		Summary: ConnPoolSummary1922{TotalEndpoints: 30, HighFanOutEPs: 3, SinglePodEPs: 5},
	}
	if r.Summary.HighFanOutEPs != 3 {
		t.Errorf("expected 3, got %d", r.Summary.HighFanOutEPs)
	}
}

func TestConnPoolEntry1922(t *testing.T) {
	e := ConnPoolEntry1922{Name: "db", Namespace: "data", ReadyAddresses: 3, EstConnections: 150, IsAtRisk: true}
	if !e.IsAtRisk {
		t.Errorf("expected at risk")
	}
	if e.EstConnections != 150 {
		t.Errorf("expected 150, got %d", e.EstConnections)
	}
}

func TestConnPoolRiskEntry1922(t *testing.T) {
	r := ConnPoolRiskEntry1922{RiskType: "no-ready-endpoints", Detail: "0 ready"}
	if r.RiskType != "no-ready-endpoints" {
		t.Errorf("expected no-ready-endpoints, got %s", r.RiskType)
	}
}
