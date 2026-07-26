package dashboard

import "testing"

func TestKubeletPodLimitResult1988(t *testing.T) {
	r := KubeletPodLimitResult1988{Summary: KubeletPodLimitSummary1988{TotalNodes: 5, MaxPodsPerNode: 110, AvgPodsPerNode: 45.0}}
	if r.Summary.MaxPodsPerNode != 110 {
		t.Errorf("expected 110")
	}
}
func TestKubeletPodLimitEntry1988(t *testing.T) {
	e := KubeletPodLimitEntry1988{Name: "node-1", PodCount: 80, Limit: 110, Utilization: 72.7}
	if e.Utilization != 72.7 {
		t.Errorf("expected 72.7")
	}
}
func TestDNSPressureResult1988(t *testing.T) {
	r := DNSPressureResult1988{Summary: DNSPressureSummary1988{TotalPods: 100, EstDNSQPS: 50.0, PressureLevel: "low"}}
	if r.Summary.EstDNSQPS != 50.0 {
		t.Errorf("expected 50")
	}
}
func TestDNSPressureEntry1988(t *testing.T) {
	e := DNSPressureEntry1988{Namespace: "prod", PodCount: 30, EstQPS: 15.0}
	if e.EstQPS != 15.0 {
		t.Errorf("expected 15")
	}
}
func TestCNIIPAMResult1988(t *testing.T) {
	r := CNIIPAMResult1988{Summary: CNIIPAMSummary1988{TotalNodes: 5, TotalIPs: 550, UsedIPs: 200, UtilizationPct: 36.4}}
	if r.Summary.UtilizationPct != 36.4 {
		t.Errorf("expected 36.4")
	}
}
func TestCNIIPAMEntry1988(t *testing.T) {
	e := CNIIPAMEntry1988{Node: "node-1", CIDR: "10.42.1.0/24", UsedIPs: 45, AvailableIPs: 254}
	if e.AvailableIPs != 254 {
		t.Errorf("expected 254")
	}
}
func TestKubeletPodLimitSummary1988(t *testing.T) {
	s := KubeletPodLimitSummary1988{NodesNearLimit: 2, HighPodNode: "node-3"}
	if s.NodesNearLimit != 2 {
		t.Errorf("expected 2")
	}
}
func TestCNIIPAMSummary1988(t *testing.T) {
	s := CNIIPAMSummary1988{ExhaustionRisk: "low"}
	if s.ExhaustionRisk != "low" {
		t.Errorf("expected low")
	}
}
