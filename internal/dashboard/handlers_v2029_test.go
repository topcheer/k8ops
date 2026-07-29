package dashboard

import "testing"

func TestCostAnomalyResult2029(t *testing.T) {
	r := CostAnomalyResult2029{Summary: CostAnomalySummary2029{TotalWorkloads: 100, HighCost: 5, AvgCPUCost: 50.0, AvgMemCost: 30.0}}
	if r.Summary.HighCost != 5 {
		t.Errorf("expected 5")
	}
}
func TestCostAnomalyEntry2029(t *testing.T) {
	e := CostAnomalyEntry2029{Name: "prod", Namespace: "prod", MonthlyCost: 800.0, Reason: "10 CPU cores"}
	if e.MonthlyCost != 800.0 {
		t.Errorf("expected 800")
	}
}
func TestRightSizeResult2029(t *testing.T) {
	r := RightSizeResult2029{Summary: RightSizeSummary2029{TotalContainers: 200, OverProvisioned: 15, NoLimits: 30, AvgCPURequest: 0.5, AvgMemRequest: 512}}
	if r.Summary.OverProvisioned != 15 {
		t.Errorf("expected 15")
	}
}
func TestRightSizeEntry2029(t *testing.T) {
	e := RightSizeEntry2029{Pod: "app", Namespace: "prod", Container: "web", CPURequest: 4.0, MemRequest: 8192}
	if e.CPURequest != 4.0 {
		t.Errorf("expected 4.0")
	}
}
func TestImageDedupResult2029(t *testing.T) {
	r := ImageDedupResult2029{Summary: ImageDedupSummary2029{TotalPods: 50, UniqueImages: 10, SharedImages: 3, DuplicationRatio: 0.3}}
	if r.Summary.DuplicationRatio != 0.3 {
		t.Errorf("expected 0.3")
	}
}
func TestImageDedupEntry2029(t *testing.T) {
	e := ImageDedupEntry2029{Image: "nginx:1.25", PodCount: 20, Namespaces: []string{"prod", "dev"}}
	if e.PodCount != 20 {
		t.Errorf("expected 20")
	}
}
