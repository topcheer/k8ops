package dashboard

import "testing"

func TestAutoscalerReadyResult1958(t *testing.T) {
	r := AutoscalerReadyResult1958{Summary: AutoscalerReadySummary1958{TotalNodes: 5, ReadyNodes: 4, PendingPods: 2}}
	if r.Summary.TotalNodes != 5 {
		t.Errorf("expected 5")
	}
	if r.Summary.PendingPods != 2 {
		t.Errorf("expected 2")
	}
}
func TestAutoscalerPoolEntry1958(t *testing.T) {
	e := AutoscalerPoolEntry1958{PoolName: "default-pool", NodeCount: 3, HasTaints: true}
	if e.NodeCount != 3 {
		t.Errorf("expected 3")
	}
	if !e.HasTaints {
		t.Errorf("expected true")
	}
}
func TestAutoscalerPendingPod1958(t *testing.T) {
	e := AutoscalerPendingPod1958{Name: "app-1", Namespace: "default", Reason: "Unschedulable"}
	if e.Reason != "Unschedulable" {
		t.Errorf("expected Unschedulable")
	}
}
func TestRequestHeadroomResult1958(t *testing.T) {
	r := RequestHeadroomResult1958{Summary: RequestHeadroomSummary1958{TotalCPUReq: 10.5, TotalCPUCapacity: 32.0, CPUHeadroomPct: 67.2}}
	if r.Summary.CPUHeadroomPct != 67.2 {
		t.Errorf("expected 67.2")
	}
}
func TestRequestHeadroomForecast1958(t *testing.T) {
	f := RequestHeadroomForecast1958{CPUExhaustDays: 120, MemExhaustDays: 90, EarliestBottleneck: "Memory"}
	if f.EarliestBottleneck != "Memory" {
		t.Errorf("expected Memory")
	}
	if f.CPUExhaustDays <= f.MemExhaustDays {
		t.Errorf("expected CPU > Mem")
	}
}
func TestRequestHeadroomNS1958(t *testing.T) {
	e := RequestHeadroomNS1958{Namespace: "kube-system", CPUReq: 2.0, MemReq: 4.5, Pods: 10}
	if e.Pods != 10 {
		t.Errorf("expected 10")
	}
}
func TestFailoverReadyResult1958(t *testing.T) {
	r := FailoverReadyResult1958{Summary: FailoverReadySummary1958{TotalZones: 3, FailoverScore: 89.5, WorkloadsWithoutPDB: 5}}
	if r.Summary.TotalZones != 3 {
		t.Errorf("expected 3")
	}
	if r.Summary.FailoverScore != 89.5 {
		t.Errorf("expected 89.5")
	}
}
func TestFailoverZoneEntry1958(t *testing.T) {
	e := FailoverZoneEntry1958{Zone: "us-east-1a", NodeCount: 4, PodCount: 50, Distribution: 45.5}
	if e.Distribution != 45.5 {
		t.Errorf("expected 45.5")
	}
}
func TestFailoverWorkloadEntry1958(t *testing.T) {
	e := FailoverWorkloadEntry1958{Name: "api-server", Kind: "Deployment", Replicas: 3, HasPDB: false, Zones: 1, Risk: "high"}
	if e.Risk != "high" {
		t.Errorf("expected high")
	}
	if e.HasPDB {
		t.Errorf("expected false")
	}
}
func TestContainsStr1958v(t *testing.T) {
	if !containsStr1958("my-app-pod", "app") {
		t.Errorf("expected true")
	}
	if containsStr1958("hello", "xyz") {
		t.Errorf("expected false")
	}
}
func TestForecastDays1958(t *testing.T) {
	d := forecastDays1958(0.5, 5.0)
	if d <= 0 || d >= 999 {
		t.Errorf("expected finite positive days, got %d", d)
	}
	d2 := forecastDays1958(1.5, 5.0)
	if d2 != 0 {
		t.Errorf("expected 0 for over capacity")
	}
}
