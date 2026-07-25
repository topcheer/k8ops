package dashboard

import "testing"

func TestPodEffResult1969(t *testing.T) {
	r := PodEffResult1969{Summary: PodEffSummary1969{TotalPods: 50, EfficientPods: 40, WastefulPods: 10, AvgEfficiency: 78.5}}
	if r.Summary.WastefulPods != 10 {
		t.Errorf("expected 10")
	}
}
func TestPodEffEntry1969(t *testing.T) {
	e := PodEffEntry1969{Name: "app", Namespace: "prod", CPUReq: 2.0, MemReqGB: 4.0, Efficiency: 85.0, Status: "efficient"}
	if e.Efficiency != 85.0 {
		t.Errorf("expected 85")
	}
}
func TestSvcHealthResult1969(t *testing.T) {
	r := SvcHealthResult1969{Summary: SvcHealthSummary1969{TotalServices: 30, Healthy: 25, Unhealthy: 5}}
	if r.Summary.Unhealthy != 5 {
		t.Errorf("expected 5")
	}
}
func TestSvcHealthEntry1969(t *testing.T) {
	e := SvcHealthEntry1969{Name: "api", Namespace: "prod", Type: "ClusterIP", Healthy: true, Endpoints: 3}
	if !e.Healthy {
		t.Errorf("expected true")
	}
}
func TestClusterUtilResult1969(t *testing.T) {
	r := ClusterUtilResult1969{Summary: ClusterUtilSummary1969{TotalNodes: 5, AllocatableCPU: 80, RequestedCPU: 40, CPUUtilization: 50.0}}
	if r.Summary.CPUUtilization != 50.0 {
		t.Errorf("expected 50")
	}
}
func TestClusterUtilNSEntry1969(t *testing.T) {
	e := ClusterUtilNSEntry1969{Namespace: "prod", CPUReq: 10.5, MemReq: 20.0, Pods: 15}
	if e.CPUReq != 10.5 {
		t.Errorf("expected 10.5")
	}
}
func TestPodEffSummary1969(t *testing.T) {
	s := PodEffSummary1969{OverProvisioned: 8, UnderProvisioned: 3}
	if s.OverProvisioned != 8 {
		t.Errorf("expected 8")
	}
}
func TestSvcHealthSummary1969(t *testing.T) {
	s := SvcHealthSummary1969{NoEndpoints: 5}
	if s.NoEndpoints != 5 {
		t.Errorf("expected 5")
	}
}
