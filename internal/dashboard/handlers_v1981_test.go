package dashboard

import "testing"

func TestPodDensityResult1981(t *testing.T) {
	r := PodDensityResult1981{Summary: PodDensitySummary1981{TotalNodes: 5, TotalPods: 200, AvgPodsPerNode: 40, DensityPct: 36.4}}
	if r.Summary.DensityPct != 36.4 {
		t.Errorf("expected 36.4")
	}
}
func TestPodDensityNodeEntry1981(t *testing.T) {
	e := PodDensityNodeEntry1981{Name: "node-1", PodCount: 50, CPUAlloc: 16.0, Density: 45.5}
	if e.PodCount != 50 {
		t.Errorf("expected 50")
	}
}
func TestImageCacheResult1981(t *testing.T) {
	r := ImageCacheResult1981{Summary: ImageCacheSummary1981{TotalImages: 30, TotalImageRefs: 100, ReuseRatio: 3.33}}
	if r.Summary.ReuseRatio != 3.33 {
		t.Errorf("expected 3.33")
	}
}
func TestImageCacheEntry1981(t *testing.T) {
	e := ImageCacheEntry1981{Image: "nginx:1.25", UseCount: 15}
	if e.UseCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestBinPackResult1981(t *testing.T) {
	r := BinPackResult1981{Summary: BinPackSummary1981{TotalNodes: 5, AvgBinPackPct: 65.0, BestNode: "node-1", BestPct: 85.0}}
	if r.Summary.BestPct != 85.0 {
		t.Errorf("expected 85")
	}
}
func TestBinPackNodeEntry1981(t *testing.T) {
	e := BinPackNodeEntry1981{Name: "node-1", CPUPackPct: 70.0, MemPackPct: 60.0, OverallPct: 65.0}
	if e.OverallPct != 65.0 {
		t.Errorf("expected 65")
	}
}
func TestPodDensitySummary1981(t *testing.T) {
	s := PodDensitySummary1981{MaxPodsPerNode: 80}
	if s.MaxPodsPerNode != 80 {
		t.Errorf("expected 80")
	}
}
func TestImageCacheSummary1981(t *testing.T) {
	s := ImageCacheSummary1981{CacheHitEst: 85.0}
	if s.CacheHitEst != 85.0 {
		t.Errorf("expected 85")
	}
}
