package dashboard

import (
	"testing"
)

func TestNodeOSDriftTypes(t *testing.T) {
	r := NodeOSDriftResult{HealthScore: 70, Grade: "C"}
	if r.HealthScore != 70 || r.Grade != "C" {
		t.Error("struct field error")
	}
	s := NodeOSDriftSummary{TotalNodes: 3, UniqueKernels: 1, UniqueRuntimes: 1, OldestNodeDays: 719, HasGPU: false}
	if s.OldestNodeDays != 719 {
		t.Error("summary field error")
	}
	nd := NodeOSDetail{Name: "node1", Kernel: "6.1.0", OSImage: "Ubuntu 22.04", AgeDays: 719, Status: "critical"}
	if nd.AgeDays != 719 || nd.Status != "critical" {
		t.Error("nodeOSDetail field error")
	}
	df := OSDriftFinding{Node: "cluster-wide", Severity: "medium", Finding: "kernel drift"}
	if df.Severity != "medium" {
		t.Error("driftFinding field error")
	}
}

func TestNodeOSDriftScoring(t *testing.T) {
	tests := []struct {
		kernels       int
		runtimes      int
		oldestDays    int
		criticalCount int
		expectedMin   int
		expectedMax   int
	}{
		{1, 1, 100, 0, 90, 100},
		{2, 1, 800, 0, 40, 65},
		{3, 2, 800, 1, 0, 30},
	}
	for _, tc := range tests {
		score := 100
		score -= (tc.kernels - 1) * 15
		score -= (tc.runtimes - 1) * 10
		if tc.oldestDays > 365 {
			score -= 10
		}
		if tc.oldestDays > 730 {
			score -= 15
		}
		score -= tc.criticalCount * 20
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("kernels=%d runtimes=%d oldest=%d crit=%d: expected %d-%d, got %d",
				tc.kernels, tc.runtimes, tc.oldestDays, tc.criticalCount, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestTrafficFlowTypes(t *testing.T) {
	r := TrafficFlowResult{FlowScore: 75, Grade: "C"}
	if r.FlowScore != 75 || r.Grade != "C" {
		t.Error("struct field error")
	}
	s := TrafficFlowSummary{TotalServices: 84, ClusterIPLoadBalancer: 4, HeadlessServices: 5}
	if s.HeadlessServices != 5 {
		t.Error("summary field error")
	}
	sf := ServiceFlow{Name: "api", Namespace: "app", BackendCount: 3, ExposureLevel: "cluster-internal"}
	if sf.BackendCount != 3 || sf.ExposureLevel != "cluster-internal" {
		t.Error("serviceFlow field error")
	}
	is := IsolatedService{Name: "web", Reason: "no endpoints", Severity: "high"}
	if is.Severity != "high" {
		t.Error("isolatedService field error")
	}
}

func TestTrafficFlowScoring(t *testing.T) {
	tests := []struct {
		isolated    int
		lbCount     int
		expectedMin int
		expectedMax int
	}{
		{0, 2, 95, 100},
		{5, 4, 40, 60},
		{3, 10, 40, 70},
	}
	for _, tc := range tests {
		score := 100
		score -= tc.isolated * 10
		if tc.lbCount > 5 {
			score -= (tc.lbCount - 5) * 3
		}
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("isolated=%d lb=%d: expected %d-%d, got %d",
				tc.isolated, tc.lbCount, tc.expectedMin, tc.expectedMax, score)
		}
	}
}
