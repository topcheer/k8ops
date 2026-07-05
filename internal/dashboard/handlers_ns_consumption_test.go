package dashboard

import (
	"testing"
)

func TestNSCCalcCost(t *testing.T) {
	config := NSCCostConfig{
		CPUPricePerCorePerMonth:   28.0,
		MemPricePerGBPerMonth:     3.8,
		StoragePricePerGBPerMonth: 0.10,
	}

	// 2 cores, 4GB, 100GB storage
	entry := &NSCEntry{
		CPUReqmCPU: 2000,
		MemReqMB:   4096,
		StorageGB:  100,
	}
	// CPU: 2 * 28 = 56, Mem: 4 * 3.8 = 15.2, Storage: 100 * 0.10 = 10
	// Total = 81.2
	cost := nscCalcCost(entry, config)
	if cost < 81.0 || cost > 81.5 {
		t.Errorf("Expected ~81.2, got %.2f", cost)
	}

	// Empty
	entry = &NSCEntry{}
	cost = nscCalcCost(entry, config)
	if cost != 0 {
		t.Errorf("Expected 0 for empty, got %.2f", cost)
	}
}

func TestNSCAssessRisk(t *testing.T) {
	// Idle namespace
	entry := NSCEntry{IsIdle: true, PodCount: 0}
	if level := nscAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for idle, got %s", level)
	}

	// Extreme over-commit
	entry = NSCEntry{OverCommitRatio: 6}
	if level := nscAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for 6x over-commit, got %s", level)
	}

	// Moderate over-commit
	entry = NSCEntry{OverCommitRatio: 4}
	if level := nscAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 4x over-commit, got %s", level)
	}

	// Normal
	entry = NSCEntry{OverCommitRatio: 2}
	if level := nscAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for 2x, got %s", level)
	}
}

func TestNSCAnalyzeWaste(t *testing.T) {
	entries := []NSCEntry{
		{Namespace: "app1", CPUReqmCPU: 500, CPULimitmCPU: 4000, OverCommitRatio: 8, MemReqMB: 512, MemLimitMB: 4096, EstMonthlyCost: 50},
		{Namespace: "app2", CPUReqmCPU: 2000, CPULimitmCPU: 2000, OverCommitRatio: 1, MemReqMB: 1024, MemLimitMB: 1024, EstMonthlyCost: 100},
		{Namespace: "old-app", IsIdle: true, EstMonthlyCost: 30},
	}
	summary := NSCSourceSummary{
		TotalCPULimitmCPU: 6000,
		TotalCPUReqmCPU:   2500,
		EstMonthlyCost:    180,
	}
	config := NSCCostConfig{CPUPricePerCorePerMonth: 28.0}

	waste := nscAnalyzeWaste(entries, summary, config)

	if waste.OverProvisionedNS == 0 {
		t.Error("Expected over-provisioned namespace detected")
	}
	if waste.IdleCost != 30 {
		t.Errorf("Expected idle cost 30, got %.2f", waste.IdleCost)
	}
	if waste.WastedCPUmCPU <= 0 {
		t.Error("Expected wasted CPU > 0")
	}
	if waste.WasteScore <= 0 {
		t.Error("Expected waste score > 0")
	}
}

func TestNSCGenRecs(t *testing.T) {
	summary := NSCSourceSummary{
		TotalCPUReqmCPU: 5000,
		TotalMemReqMB:   10240,
		TotalStorageGB:  200,
		EstMonthlyCost:  250.0,
		AvgEfficiency:   35,
	}
	waste := NSCWasteAnalysis{
		OverProvisionedNS: 3,
		WastedCPUmCPU:     5000,
		IdleCost:          40,
	}
	topConsumers := []NSCEntry{
		{Namespace: "prod", CostSharePct: 45, EstMonthlyCost: 112.5, PodCount: 15},
	}
	idleNS := []NSCEntry{
		{Namespace: "old", EstMonthlyCost: 40},
	}

	recs := nscGenRecs(summary, waste, topConsumers, idleNS)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundCost := false
	foundTop := false
	foundIdle := false
	foundOver := false
	foundEff := false
	for _, r := range recs {
		if strContains(r, "monthly cost") {
			foundCost = true
		}
		if strContains(r, "Top consumer") {
			foundTop = true
		}
		if strContains(r, "idle") {
			foundIdle = true
		}
		if strContains(r, "over-provisioning") {
			foundOver = true
		}
		if strContains(r, "efficiency") {
			foundEff = true
		}
	}
	if !foundCost {
		t.Error("Expected cost recommendation")
	}
	if !foundTop {
		t.Error("Expected top consumer recommendation")
	}
	if !foundIdle {
		t.Error("Expected idle namespace recommendation")
	}
	if !foundOver {
		t.Error("Expected over-provisioning recommendation")
	}
	if !foundEff {
		t.Error("Expected efficiency recommendation")
	}
}

func TestNSCGetOrCreate(t *testing.T) {
	m := make(map[string]*NSCEntry)

	e1 := nscGetOrCreate(m, "default")
	e1.PodCount = 5

	e2 := nscGetOrCreate(m, "default")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry with PodCount=5, got %d", e2.PodCount)
	}

	e3 := nscGetOrCreate(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
	if e3.PodCount != 0 {
		t.Errorf("Expected 0 pods for new, got %d", e3.PodCount)
	}
}
