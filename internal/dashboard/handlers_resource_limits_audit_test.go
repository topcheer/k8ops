package dashboard

import (
	"testing"
)

func TestRLEClassify(t *testing.T) {
	summary := &RLESummary{}

	// No limits at all = unbounded critical
	entry := RLEEntry{HasCPUReq: true, HasCPULim: false, HasMemReq: true, HasMemLim: false}
	risk, issue := rleClassify(entry, summary)
	if risk != "critical" || issue != "unbounded" {
		t.Errorf("Expected critical/unbounded, got %s/%s", risk, issue)
	}

	// No mem limit = critical
	entry = RLEEntry{HasCPUReq: true, HasCPULim: true, HasMemReq: true, HasMemLim: false, CPURatio: 2}
	risk, _ = rleClassify(entry, summary)
	if risk != "critical" {
		t.Errorf("Expected critical for no mem limit, got %s", risk)
	}

	// No cpu limit = high
	entry = RLEEntry{HasCPUReq: true, HasCPULim: false, HasMemReq: true, HasMemLim: true, MemRatio: 2}
	risk, _ = rleClassify(entry, summary)
	if risk != "high" {
		t.Errorf("Expected high for no cpu limit, got %s", risk)
	}

	// Over-provisioned (>4x ratio)
	entry = RLEEntry{HasCPUReq: true, HasCPULim: true, HasMemReq: true, HasMemLim: true, CPURatio: 5, MemRatio: 3}
	risk, issue = rleClassify(entry, summary)
	if risk != "medium" || issue != "over-provisioned" {
		t.Errorf("Expected medium/over-provisioned, got %s/%s", risk, issue)
	}

	// Under-provisioned (<1.2x ratio)
	entry = RLEEntry{HasCPUReq: true, HasCPULim: true, HasMemReq: true, HasMemLim: true, CPURatio: 1.1, MemRatio: 2}
	risk, issue = rleClassify(entry, summary)
	if risk != "high" || issue != "under-provisioned" {
		t.Errorf("Expected high/under-provisioned, got %s/%s", risk, issue)
	}

	// Good ratio
	entry = RLEEntry{HasCPUReq: true, HasCPULim: true, HasMemReq: true, HasMemLim: true, CPURatio: 2, MemRatio: 3}
	risk, _ = rleClassify(entry, summary)
	if risk != "low" {
		t.Errorf("Expected low for good ratio, got %s", risk)
	}
}

func TestRLEScore(t *testing.T) {
	// Empty
	if score := rleScore(RLESummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := RLESummary{TotalContainers: 10}
	if score := rleScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = RLESummary{
		TotalContainers:  10,
		NoLimits:         2, // -30
		NoMemLimit:       3, // -24
		NoCPULimit:       2, // -8
		NoRequests:       2, // -10
		UnderProvisioned: 1, // -3
	}
	// 100 - 30 - 24 - 8 - 10 - 3 = 25
	if score := rleScore(s); score != 25 {
		t.Errorf("Expected 25, got %d", score)
	}
}

func TestRLEGenRecs(t *testing.T) {
	s := RLESummary{
		TotalContainers:     10,
		NoLimits:            3,
		NoMemLimit:          4,
		NoCPULimit:          2,
		NoRequests:          1,
		UnderProvisioned:    2,
		OverProvisioned:     1,
		ExcessiveCPURequest: 1,
		ExcessiveMemRequest: 1,
		ComplianceScore:     30,
	}
	unbounded := []RLEEntry{{Namespace: "default", Workload: "app1"}}
	overProv := []RLEEntry{{Namespace: "default", Workload: "app2"}}

	recs := rleGenRecs(s, unbounded, overProv)

	if len(recs) < 8 {
		t.Errorf("Expected at least 8 recommendations, got %d", len(recs))
	}

	foundNoLimits := false
	foundNoMem := false
	foundUnder := false
	for _, r := range recs {
		if strContains(r, "NO resource limits") {
			foundNoLimits = true
		}
		if strContains(r, "memory limits") {
			foundNoMem = true
		}
		if strContains(r, "under-provisioned") {
			foundUnder = true
		}
	}
	if !foundNoLimits {
		t.Error("Expected recommendation about no limits")
	}
	if !foundNoMem {
		t.Error("Expected recommendation about no mem limit")
	}
	if !foundUnder {
		t.Error("Expected recommendation about under-provisioned")
	}
}

func TestRLEGenRecsClean(t *testing.T) {
	s := RLESummary{TotalContainers: 10}
	recs := rleGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRLENSRisk(t *testing.T) {
	ns := RLENSEntry{NoLimitCount: 1}
	if level := rleNSRisk(ns); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	ns = RLENSEntry{OverProvCount: 3}
	if level := rleNSRisk(ns); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}

	ns = RLENSEntry{}
	if level := rleNSRisk(ns); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestRLERiskRank(t *testing.T) {
	if rleRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rleRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if rleRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if rleRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestRLEIssueRank(t *testing.T) {
	if rleIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rleIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}

func TestRLEGetOrCreateNS(t *testing.T) {
	m := make(map[string]*RLENSEntry)
	e1 := rleGetOrCreateNS(m, "default")
	e1.ContainerCount = 5

	e2 := rleGetOrCreateNS(m, "default")
	if e2.ContainerCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.ContainerCount)
	}

	e3 := rleGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}
