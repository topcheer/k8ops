package dashboard

import (
	"strings"
	"testing"
)

func TestHNAssessRisk(t *testing.T) {
	if hnAssessRisk([]string{"privileged:c1", "hostNetwork"}) != "critical" {
		t.Error("Expected critical for privileged + hostNS")
	}
	if hnAssessRisk([]string{"privileged:c1"}) != "high" {
		t.Error("Expected high for privileged only")
	}
	if hnAssessRisk([]string{"hostPID"}) != "high" {
		t.Error("Expected high for hostPID")
	}
	if hnAssessRisk([]string{"runAsRoot:c1", "hostPath:vol", "capAdd:c1:SYS_ADMIN"}) != "high" {
		t.Error("Expected high for >=3 exposures")
	}
	if hnAssessRisk([]string{"runAsRoot:c1"}) != "medium" {
		t.Error("Expected medium for single exposure")
	}
	if hnAssessRisk(nil) != "low" {
		t.Error("Expected low for no exposures")
	}
}

func TestHNScore(t *testing.T) {
	if score := hnScore(HNSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := HNSummary{
		TotalPods:        50,
		PrivilegedCtrs:   2, // -20
		HostNetworkPods:  3, // -15
		HostPIDPods:      1, // -5
		HostPathMounts:   5, // -15
		CapAddContainers: 4, // -12
		RunAsRootCtrs:    6, // -12
	}
	// 100 - 20 - 15 - 5 - 15 - 12 - 12 = 21
	if score := hnScore(s); score != 21 {
		t.Errorf("Expected 21, got %d", score)
	}
}

func TestHNGenRecs(t *testing.T) {
	s := HNSummary{
		PrivilegedCtrs:   2,
		HostNetworkPods:  3,
		HostPIDPods:      1,
		HostPathMounts:   5,
		CapAddContainers: 4,
		RunAsRootCtrs:    6,
		ExposureScore:    20,
	}
	recs := hnGenRecs(s)
	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundPriv := false
	foundHostNet := false
	for _, r := range recs {
		if strings.Contains(r, "privileged") {
			foundPriv = true
		}
		if strings.Contains(r, "hostNetwork") {
			foundHostNet = true
		}
	}
	if !foundPriv {
		t.Error("Expected recommendation about privileged containers")
	}
	if !foundHostNet {
		t.Error("Expected recommendation about hostNetwork")
	}
}

func TestHNGenRecsClean(t *testing.T) {
	s := HNSummary{ExposureScore: 100}
	recs := hnGenRecs(s)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestHNRiskRank(t *testing.T) {
	if hnRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if hnRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestHNIssueRank(t *testing.T) {
	if hnIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if hnIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
