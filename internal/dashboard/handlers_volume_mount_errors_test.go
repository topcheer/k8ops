package dashboard

import "testing"

func TestVMContainsVolumeError(t *testing.T) {
	if !vmContainsVolumeError("MountVolume.MountDevice failed") {
		t.Error("Expected true for mount error")
	}
	if !vmContainsVolumeError("AttachVolume.Attach failed for volume") {
		t.Error("Expected true for attach error")
	}
	if !vmContainsVolumeError("timeout expired waiting for volumes") {
		t.Error("Expected true for timeout")
	}
	if vmContainsVolumeError("container started successfully") {
		t.Error("Expected false for non-volume message")
	}
	if vmContainsVolumeError("") {
		t.Error("Expected false for empty message")
	}
}

func TestVMClassifyError(t *testing.T) {
	if et, _ := vmClassifyError("timeout expired waiting for volumes"); et != "timeout" {
		t.Errorf("Expected timeout, got %s", et)
	}
	if et, _ := vmClassifyError("failed to provision volume: storageclass not found"); et != "provisioning" {
		t.Errorf("Expected provisioning, got %s", et)
	}
	if et, _ := vmClassifyError("AttachVolume.Attach failed"); et != "attach_fail" {
		t.Errorf("Expected attach_fail, got %s", et)
	}
	if et, _ := vmClassifyError("MountVolume.SetUp failed"); et != "mount_fail" {
		t.Errorf("Expected mount_fail, got %s", et)
	}
}

func TestVMAssessRisk(t *testing.T) {
	if vmAssessRisk(20) != "high" {
		t.Error("Expected high for >15min")
	}
	if vmAssessRisk(7) != "medium" {
		t.Error("Expected medium for >5min")
	}
	if vmAssessRisk(3) != "low" {
		t.Error("Expected low")
	}
}

func TestVMScore(t *testing.T) {
	if score := vmScore(VMSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := VMSummary{
		TotalPods:          50,
		StuckPods:          3, // -30
		MountFailErrors:    2, // -6
		ProvisioningErrors: 1, // -5
	}
	// 100 - 30 - 6 - 5 = 59
	if score := vmScore(s); score != 59 {
		t.Errorf("Expected 59, got %d", score)
	}
}

func TestVMGenRecs(t *testing.T) {
	s := VMSummary{
		StuckPods:          3,
		MountFailErrors:    2,
		AttachFailErrors:   1,
		ProvisioningErrors: 1,
		TimeoutErrors:      1,
		HealthScore:        50,
	}
	recs := vmGenRecs(s)
	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundMount := false
	foundAttach := false
	for _, r := range recs {
		if strContains(r, "mount") {
			foundMount = true
		}
		if strContains(r, "attach") {
			foundAttach = true
		}
	}
	if !foundMount {
		t.Error("Expected recommendation about mount failures")
	}
	if !foundAttach {
		t.Error("Expected recommendation about attach failures")
	}
}

func TestVMGenRecsClean(t *testing.T) {
	recs := vmGenRecs(VMSummary{})
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestVMTruncate(t *testing.T) {
	short := "short message"
	if vmTruncate(short, 50) != short {
		t.Error("Expected unchanged short string")
	}
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	result := vmTruncate(long, 30)
	if len(result) > 30 {
		t.Errorf("Expected max 30 chars, got %d", len(result))
	}
}

func TestVMIssueRank(t *testing.T) {
	if vmIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
