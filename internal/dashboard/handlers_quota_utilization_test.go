package dashboard

import (
	"testing"
)

func TestQUAssessQuotaRisk(t *testing.T) {
	if level := quAssessQuotaRisk(95); level != "critical" {
		t.Errorf("Expected critical for 95%%, got %s", level)
	}
	if level := quAssessQuotaRisk(85); level != "high" {
		t.Errorf("Expected high for 85%%, got %s", level)
	}
	if level := quAssessQuotaRisk(70); level != "medium" {
		t.Errorf("Expected medium for 70%%, got %s", level)
	}
	if level := quAssessQuotaRisk(30); level != "low" {
		t.Errorf("Expected low for 30%%, got %s", level)
	}
}

func TestQUNSRisk(t *testing.T) {
	ns := QUNSEntry{HasQuota: false}
	if level := quNSRisk(ns); level != "high" {
		t.Errorf("Expected high for no quota, got %s", level)
	}

	ns = QUNSEntry{HasQuota: true, MaxUtilization: 95}
	if level := quNSRisk(ns); level != "critical" {
		t.Errorf("Expected critical for 95%%, got %s", level)
	}

	ns = QUNSEntry{HasQuota: true, MaxUtilization: 50}
	if level := quNSRisk(ns); level != "low" {
		t.Errorf("Expected low for 50%%, got %s", level)
	}
}

func TestQUScore(t *testing.T) {
	if score := quScore(QUSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := QUSummary{
		TotalNamespaces: 10,
		NSWithoutQuota:  3,  // -15
		CriticalQuotas:  2,  // -16
		UnboundedRatio:  40, // -12
	}
	// 100 - 15 - 16 - 12 = 57
	if score := quScore(s); score != 57 {
		t.Errorf("Expected 57, got %d", score)
	}
}

func TestQUGenRecs(t *testing.T) {
	s := QUSummary{
		TotalNamespaces:  10,
		NSWithoutQuota:   3,
		NSWithLimitRange: 5,
		CriticalQuotas:   2,
		NoRequests:       5,
		NoLimits:         8,
		UnboundedRatio:   45,
		ComplianceScore:  40,
	}
	critical := []QUEntry{
		{Namespace: "app1", Name: "quota1", MaxUtil: 95},
	}

	recs := quGenRecs(s, critical, nil)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundNoQuota := false
	foundCritical := false
	foundNoLimits := false
	foundLimitRange := false
	for _, r := range recs {
		if strContains(r, "NO ResourceQuota") {
			foundNoQuota = true
		}
		if strContains(r, ">80%") {
			foundCritical = true
		}
		if strContains(r, "resource limits") {
			foundNoLimits = true
		}
		if strContains(r, "LimitRange") {
			foundLimitRange = true
		}
	}
	if !foundNoQuota {
		t.Error("Expected recommendation about missing quotas")
	}
	if !foundCritical {
		t.Error("Expected recommendation about critical quotas")
	}
	if !foundNoLimits {
		t.Error("Expected recommendation about missing limits")
	}
	if !foundLimitRange {
		t.Error("Expected recommendation about LimitRange")
	}
}

func TestQUGenRecsClean(t *testing.T) {
	s := QUSummary{
		TotalNamespaces:  10,
		NSWithoutQuota:   0,
		CriticalQuotas:   0,
		UnboundedRatio:   5,
		NSWithLimitRange: 10,
	}
	recs := quGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestQUGetOrCreateNS(t *testing.T) {
	m := make(map[string]*QUNSEntry)
	e1 := quGetOrCreateNS(m, "default")
	e1.HasQuota = true

	e2 := quGetOrCreateNS(m, "default")
	if !e2.HasQuota {
		t.Error("Expected same entry")
	}

	e3 := quGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}

func TestQUIssueRank(t *testing.T) {
	if quIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if quIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if quIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
