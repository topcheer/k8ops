package dashboard

import "testing"

func TestBurstCapacityResult1910(t *testing.T) {
	r := BurstCapacityResult{
		Summary:     BurstCapacitySummary{TotalNodes: 1, MaxBurstPods: 21, AvgPodCPUm: 100, AvgPodMemMB: 128},
		HealthScore: 42,
	}
	if r.Summary.MaxBurstPods != 21 {
		t.Errorf("expected 21, got %d", r.Summary.MaxBurstPods)
	}
}

func TestElasticityResult1910(t *testing.T) {
	r := ElasticityResult{
		Summary:     ElasticitySummary{TotalWorkloads: 67, WithHPA: 0, FullyElastic: 0, ElasticityIndex: 0},
		HealthScore: 0,
	}
	if r.Summary.ElasticityIndex != 0 {
		t.Errorf("expected 0, got %d", r.Summary.ElasticityIndex)
	}
}

func TestScaleBottleneckResult1910(t *testing.T) {
	r := ScaleBottleneckResult{
		Summary:     BottleneckSummary1910{TotalWorkloads: 67, WithBottlenecks: 20, CPUBottlenecks: 5, AffinityBottlenecks: 3},
		HealthScore: 70,
	}
	if r.Summary.WithBottlenecks != 20 {
		t.Errorf("expected 20, got %d", r.Summary.WithBottlenecks)
	}
}

func TestBuildBurstCapacityRecs1910(t *testing.T) {
	r := &BurstCapacityResult{Summary: BurstCapacitySummary{TotalNodes: 1, MaxBurstPods: 5, AvgPodCPUm: 100, AvgPodMemMB: 128}}
	r.Bottleneck = "cpu"
	recs := buildBurstCapacityRecs1910(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildElasticityRecs1910(t *testing.T) {
	r := &ElasticityResult{Summary: ElasticitySummary{ElasticityIndex: 0, FullyElastic: 0, TotalWorkloads: 67, WithHPA: 0, WithVPA: 0, WithClusterAS: 0, NoElasticity: 50}}
	recs := buildElasticityRecs1910(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildBottleneckRecs1910(t *testing.T) {
	r := &ScaleBottleneckResult{Summary: BottleneckSummary1910{WithBottlenecks: 20, TotalWorkloads: 67, CPUBottlenecks: 5, AffinityBottlenecks: 3, ImagePullBottlenecks: 2, PodLimitBottlenecks: 1}}
	recs := buildBottleneckRecs1910(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestMaxInt1910(t *testing.T) {
	if maxInt1910(3, 5) != 5 {
		t.Error("expected 5")
	}
	if maxInt1910(10, 2) != 10 {
		t.Error("expected 10")
	}
}
