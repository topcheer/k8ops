package dashboard

import "testing"

func TestNodeHeadroomResult2036(t *testing.T) {
	r := NodeHeadroomResult2036{Summary: NodeHeadroomSummary2036{TotalNodes: 5, LowHeadroom: 1, AvgCPUHeadroom: 40, AvgMemHeadroom: 50}}
	if r.Summary.LowHeadroom != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeHeadroomEntry2036(t *testing.T) {
	e := NodeHeadroomEntry2036{Node: "node-1", CPUHeadroom: 15, MemHeadroom: 30}
	if e.CPUHeadroom != 15 {
		t.Errorf("expected 15")
	}
}
func TestPodDensityResult2036(t *testing.T) {
	r := PodDensityResult2036{Summary: PodDensitySummary2036{TotalNodes: 5, DenseNodes: 1, TotalPods: 100, AvgPodsPerNode: 20}}
	if r.Summary.AvgPodsPerNode != 20 {
		t.Errorf("expected 20")
	}
}
func TestPodDensityEntry2036(t *testing.T) {
	e := PodDensityEntry2036{Node: "node-1", PodCount: 90, MaxPods: 110, Density: 81}
	if e.Density != 81 {
		t.Errorf("expected 81")
	}
}
func TestStorageForecastResult2036(t *testing.T) {
	r := StorageForecastResult2036{Summary: StorageForecastSummary2036{TotalPVCs: 20, BoundPVCs: 18, LargePVCs: 3, TotalCapacity: 2000}}
	if r.Summary.TotalCapacity != 2000 {
		t.Errorf("expected 2000")
	}
}
func TestStorageForecastEntry2036(t *testing.T) {
	e := StorageForecastEntry2036{Name: "data", Namespace: "prod", Size: "500Gi", SizeGB: 500}
	if e.SizeGB != 500 {
		t.Errorf("expected 500")
	}
}
