package dashboard

import (
	"testing"
)

func TestPriorityClassAuditResultStruct1891(t *testing.T) {
	r := PriorityClassAuditResult{
		Summary: PriorityClassAuditSummary{
			TotalWorkloads:    60,
			WithPriorityClass: 5,
			WithoutPriority:   55,
			DefinedClasses:    3,
			BestEffort:        55,
		},
		HealthScore: 8,
	}
	if r.Summary.WithoutPriority != 55 {
		t.Errorf("expected 55 without priority, got %d", r.Summary.WithoutPriority)
	}
}

func TestServiceExposureResultStruct1891(t *testing.T) {
	r := ServiceExposureResult{
		Summary: ServiceExposureSummary{
			TotalServices:   100,
			ClusterIP:       80,
			NodePort:        5,
			LoadBalancer:    10,
			PubliclyExposed: 15,
			OverExposed:     3,
		},
		HealthScore: 65,
	}
	if r.Summary.OverExposed != 3 {
		t.Errorf("expected 3 over-exposed, got %d", r.Summary.OverExposed)
	}
}

func TestAntiaffinityHAResultStruct1891(t *testing.T) {
	r := AntiaffinityHAResult{
		Summary: AntiaffinityHASummary{
			TotalWorkloads:   60,
			WithAntiAffinity: 10,
			SingleReplica:    25,
			MultiReplicaNoHA: 20,
			HAReady:          15,
			TotalNodes:       1,
		},
		HealthScore: 12, // capped by single node
	}
	if r.Summary.SingleReplica != 25 {
		t.Errorf("expected 25 single-replica, got %d", r.Summary.SingleReplica)
	}
}

func TestSafePercent1891(t *testing.T) {
	if safePercent1891(50, 100) != 50 {
		t.Errorf("expected 50, got %d", safePercent1891(50, 100))
	}
	if safePercent1891(10, 0) != 0 {
		t.Errorf("expected 0 for zero denom, got %d", safePercent1891(10, 0))
	}
}

func TestAnalyzeWorkloadPriority1891(t *testing.T) {
	result := &PriorityClassAuditResult{}
	pcMap := map[string]int32{
		"high":     1000000,
		"standard": 1000,
	}

	// Test without priority class
	analyzeWorkloadPriority1891(result, "test-app", "default", "Deployment", "", pcMap)
	if result.Summary.WithoutPriority != 1 {
		t.Errorf("expected 1 without priority, got %d", result.Summary.WithoutPriority)
	}

	// Test with high priority
	analyzeWorkloadPriority1891(result, "critical-app", "default", "Deployment", "high", pcMap)
	if result.Summary.SystemCriticals != 1 {
		t.Errorf("expected 1 system critical, got %d", result.Summary.SystemCriticals)
	}

	// Test with non-existent priority class
	analyzeWorkloadPriority1891(result, "broken-app", "default", "Deployment", "nonexistent", pcMap)
	if len(result.WorkloadClasses) != 3 {
		t.Errorf("expected 3 workloads, got %d", len(result.WorkloadClasses))
	}
}

func TestDetectPreemptionPairs1891(t *testing.T) {
	result := &PriorityClassAuditResult{
		WorkloadClasses: []WorkloadPriorityEntry{
			{Name: "low", Namespace: "ns1", PriorityValue: 0},
			{Name: "high", Namespace: "ns1", PriorityValue: 1000000},
			{Name: "mid", Namespace: "ns2", PriorityValue: 1000},
		},
	}
	pairs := detectPreemptionPairs1891(result)
	if len(pairs) == 0 {
		t.Error("expected preemption pairs, got 0")
	}
	// "high" in ns1 should preempt "low" in ns1
	found := false
	for _, p := range pairs {
		if p.Preemptor == "high" && p.Victim == "low" {
			found = true
		}
	}
	if !found {
		t.Error("expected high to preempt low in same namespace")
	}
}

func TestBuildServiceExposureRecs1891(t *testing.T) {
	result := &ServiceExposureResult{
		Summary: ServiceExposureSummary{
			TotalServices:   50,
			InternalSafe:    40,
			PubliclyExposed: 10,
			LoadBalancer:    5,
			NodePort:        5,
			OverExposed:     2,
		},
	}
	recs := buildServiceExposureRecs1891(result)
	if len(recs) == 0 {
		t.Error("expected recommendations, got 0")
	}
	// Should mention over-exposed
	foundOverExp := false
	for _, r := range recs {
		if r != "" {
			foundOverExp = true
		}
	}
	if !foundOverExp {
		t.Error("expected non-empty recommendations")
	}
}
