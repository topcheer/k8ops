package dashboard

import "testing"

func TestPodUptimeResult1975(t *testing.T) {
	r := PodUptimeResult1975{Summary: PodUptimeSummary1975{TotalPods: 80, AvgUptimeHours: 240.5}}
	if r.Summary.AvgUptimeHours != 240.5 {
		t.Errorf("expected 240.5")
	}
}
func TestPodUptimeNSEntry1975(t *testing.T) {
	e := PodUptimeNSEntry1975{Namespace: "prod", PodCount: 15, AvgHours: 500.0}
	if e.AvgHours != 500.0 {
		t.Errorf("expected 500")
	}
}
func TestNSCostResult1975(t *testing.T) {
	r := NSCostResult1975{Summary: NSCostSummary1975{TotalNamespaces: 10, EstMonthlyCost: 125.50}}
	if r.Summary.EstMonthlyCost != 125.50 {
		t.Errorf("expected 125.50")
	}
}
func TestNSCostEntry1975(t *testing.T) {
	e := NSCostEntry1975{Namespace: "prod", CPUReq: 5.0, MemReq: 10.0, PodCount: 20, MonthlyCost: 50.0}
	if e.MonthlyCost != 50.0 {
		t.Errorf("expected 50")
	}
}
func TestReplicaHealthResult1975(t *testing.T) {
	r := ReplicaHealthResult1975{Summary: ReplicaHealthSummary1975{TotalDeployments: 30, FullyReady: 25, PartiallyReady: 3, NotReady: 2}}
	if r.Summary.PartiallyReady != 3 {
		t.Errorf("expected 3")
	}
}
func TestReplicaHealthEntry1975(t *testing.T) {
	e := ReplicaHealthEntry1975{Name: "api", Namespace: "prod", Desired: 3, Ready: 2, Status: "partial"}
	if e.Status != "partial" {
		t.Errorf("expected partial")
	}
}
func TestNSCostSummary1975(t *testing.T) {
	s := NSCostSummary1975{TotalCPUReq: 12.5, TotalMemReq: 45.0}
	if s.TotalCPUReq != 12.5 {
		t.Errorf("expected 12.5")
	}
}
func TestReplicaHealthSummary1975(t *testing.T) {
	s := ReplicaHealthSummary1975{ZeroReplicas: 5}
	if s.ZeroReplicas != 5 {
		t.Errorf("expected 5")
	}
}
