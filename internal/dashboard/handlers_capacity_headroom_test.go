package dashboard

import (
	"testing"
)

func TestCHRNodeBottleneck(t *testing.T) {
	// CPU is bottleneck
	entry := CHRNodeEntry{
		CPUHeadroomPct: 5,
		MemHeadroomPct: 50,
		RunningPods:    10,
		MaxPods:        110,
	}
	if b := chrNodeBottleneck(entry); b != "cpu" {
		t.Errorf("Expected cpu bottleneck, got %s", b)
	}

	// Memory is bottleneck
	entry = CHRNodeEntry{
		CPUHeadroomPct: 50,
		MemHeadroomPct: 5,
		RunningPods:    10,
		MaxPods:        110,
	}
	if b := chrNodeBottleneck(entry); b != "memory" {
		t.Errorf("Expected memory bottleneck, got %s", b)
	}

	// Pods is bottleneck
	entry = CHRNodeEntry{
		CPUHeadroomPct: 50,
		MemHeadroomPct: 50,
		RunningPods:    105,
		MaxPods:        110,
	}
	if b := chrNodeBottleneck(entry); b != "pods" {
		t.Errorf("Expected pods bottleneck, got %s", b)
	}
}

func TestCHRNodeRisk(t *testing.T) {
	if level := chrNodeRisk(CHRNodeEntry{CPUHeadroomPct: 5, MemHeadroomPct: 30}); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}
	if level := chrNodeRisk(CHRNodeEntry{CPUHeadroomPct: 20, MemHeadroomPct: 30}); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := chrNodeRisk(CHRNodeEntry{CPUHeadroomPct: 40, MemHeadroomPct: 45}); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}
	if level := chrNodeRisk(CHRNodeEntry{CPUHeadroomPct: 60, MemHeadroomPct: 70}); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestCHRClusterBottleneck(t *testing.T) {
	s := CHRSummary{CPUUtilization: 90, MemUtilization: 50, MaxPodSlots: 110, UsedPodSlots: 20}
	if b := chrClusterBottleneck(s); b != "cpu" {
		t.Errorf("Expected cpu, got %s", b)
	}

	s = CHRSummary{CPUUtilization: 50, MemUtilization: 90, MaxPodSlots: 110, UsedPodSlots: 20}
	if b := chrClusterBottleneck(s); b != "memory" {
		t.Errorf("Expected memory, got %s", b)
	}
}

func TestCHRScore(t *testing.T) {
	// Empty
	if score := chrScore(CHRSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// CPU is bottleneck
	s := CHRSummary{
		CPUUtilization: 80, // 20% free
		MemUtilization: 50, // 50% free
		MaxPodSlots:    100,
		UsedPodSlots:   20, // 80% free
	}
	// min(20, 50, 80) = 20
	if score := chrScore(s); score != 20 {
		t.Errorf("Expected 20, got %d", score)
	}

	// All utilized
	s = CHRSummary{
		CPUUtilization: 95,
		MemUtilization: 90,
		MaxPodSlots:    100,
		UsedPodSlots:   95,
	}
	// min(5, 10, 5) = 5
	if score := chrScore(s); score != 5 {
		t.Errorf("Expected 5, got %d", score)
	}
}

func TestCHRLimitingFactor(t *testing.T) {
	fit := CHRPodFit{MaxFit: 10, FitByCPU: 10, FitByMem: 20, FitByPods: 30}
	if f := chrLimitingFactor(fit); f != "cpu" {
		t.Errorf("Expected cpu, got %s", f)
	}

	fit = CHRPodFit{MaxFit: 10, FitByCPU: 20, FitByMem: 10, FitByPods: 30}
	if f := chrLimitingFactor(fit); f != "memory" {
		t.Errorf("Expected memory, got %s", f)
	}

	fit = CHRPodFit{MaxFit: 10, FitByCPU: 20, FitByMem: 20, FitByPods: 10}
	if f := chrLimitingFactor(fit); f != "pod-slots" {
		t.Errorf("Expected pod-slots, got %s", f)
	}
}

func TestCHRGenRecs(t *testing.T) {
	s := CHRSummary{
		FullNodes:      2,
		CPUUtilization: 85,
		MemUtilization: 70,
		HeadroomScore:  12,
	}
	scaleOut := CHRScaleOut{
		NeedsScaleOut: true,
		UrgencyLevel:  "immediate",
	}
	profiles := []CHRPodFit{
		{Profile: "small", MaxFit: 50, CPUmCPU: 100, MemMB: 128, LimitingFactor: "cpu"},
		{Profile: "medium", MaxFit: 15, CPUmCPU: 500, MemMB: 512, LimitingFactor: "cpu"},
		{Profile: "large", MaxFit: 5, CPUmCPU: 1000, MemMB: 1024, LimitingFactor: "cpu"},
	}
	bottlenecks := []CHRNodeEntry{
		{Name: "node-1", CPUHeadroomPct: 3, MemHeadroomPct: 20},
	}

	recs := chrGenRecs(s, scaleOut, profiles, bottlenecks)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundCapacity := false
	foundFullNodes := false
	foundScaleOut := false
	foundCritical := false
	for _, r := range recs {
		if strContains(r, "medium pods") {
			foundCapacity = true
		}
		if strContains(r, "near capacity") {
			foundFullNodes = true
		}
		if strContains(r, "scale-out") {
			foundScaleOut = true
		}
		if strContains(r, "CRITICAL") {
			foundCritical = true
		}
	}
	if !foundCapacity {
		t.Error("Expected capacity recommendation")
	}
	if !foundFullNodes {
		t.Error("Expected full nodes recommendation")
	}
	if !foundScaleOut {
		t.Error("Expected scale-out recommendation")
	}
	if !foundCritical {
		t.Error("Expected critical headroom recommendation")
	}
}

func TestCHRGenRecsHealthy(t *testing.T) {
	s := CHRSummary{
		HeadroomScore: 65,
		FullNodes:     0,
	}
	recs := chrGenRecs(s, CHRScaleOut{}, []CHRPodFit{}, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestMin3(t *testing.T) {
	if min3(10, 20, 30) != 10 {
		t.Error("Expected 10")
	}
	if min3(20, 10, 30) != 10 {
		t.Error("Expected 10")
	}
	if min3(30, 20, 10) != 10 {
		t.Error("Expected 10")
	}
}
