package dashboard

import (
	"testing"
)

func TestIPFIsFailure(t *testing.T) {
	failures := []string{"ImagePullBackOff", "ErrImagePull", "ErrImageNeverPull", "CreateContainerError", "CrashLoopBackOff"}
	for _, r := range failures {
		if !ipfIsFailure(r) {
			t.Errorf("Expected '%s' to be a failure", r)
		}
	}

	nonFailures := []string{"ContainerCreating", "Running", "PodInitializing", ""}
	for _, r := range nonFailures {
		if ipfIsFailure(r) {
			t.Errorf("Expected '%s' to NOT be a failure", r)
		}
	}
}

func TestIPFAssessRisk(t *testing.T) {
	if level := ipfAssessRisk("ImagePullBackOff", 0); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}
	if level := ipfAssessRisk("CreateContainerError", 0); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}
	if level := ipfAssessRisk("CrashLoopBackOff", 5); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := ipfAssessRisk("CrashLoopBackOff", 25); level != "critical" {
		t.Errorf("Expected critical for high restarts, got %s", level)
	}
	if level := ipfAssessRisk("InvalidImageName", 0); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := ipfAssessRisk("Unknown", 2); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}
}

func TestIPFTruncateMessage(t *testing.T) {
	short := "pull access denied"
	if msg := ipfTruncateMessage(short); msg != short {
		t.Errorf("Expected unchanged, got %s", msg)
	}

	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	msg := ipfTruncateMessage(long)
	if len(msg) > 200 {
		t.Errorf("Expected max 200 chars, got %d", len(msg))
	}
	if msg[len(msg)-3:] != "..." {
		t.Error("Expected truncation suffix")
	}
}

func TestIPFExtractRegistry(t *testing.T) {
	if reg := ipfExtractRegistry("nginx"); reg != "docker.io" {
		t.Errorf("Expected docker.io, got %s", reg)
	}
	if reg := ipfExtractRegistry("library/nginx"); reg != "docker.io" {
		t.Errorf("Expected docker.io, got %s", reg)
	}
	if reg := ipfExtractRegistry("gcr.io/project/image"); reg != "gcr.io" {
		t.Errorf("Expected gcr.io, got %s", reg)
	}
	if reg := ipfExtractRegistry("registry.iot2.win/k8ops:v15"); reg != "registry.iot2.win" {
		t.Errorf("Expected registry.iot2.win, got %s", reg)
	}
	if reg := ipfExtractRegistry("quay.io/coreos/etcd"); reg != "quay.io" {
		t.Errorf("Expected quay.io, got %s", reg)
	}
}

func TestIPFImageRisk(t *testing.T) {
	stat := &IPFImageStat{FailureCount: 8}
	if level := ipfImageRisk(stat); level != "critical" {
		t.Errorf("Expected critical for >5, got %s", level)
	}

	stat = &IPFImageStat{FailureCount: 3}
	if level := ipfImageRisk(stat); level != "high" {
		t.Errorf("Expected high for >2, got %s", level)
	}

	stat = &IPFImageStat{FailureCount: 1}
	if level := ipfImageRisk(stat); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}
}

func TestIPFScore(t *testing.T) {
	if score := ipfScore(IPFSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := IPFSummary{TotalPods: 50, FailedPods: 5}
	// failurePct = 10%, score = 100 - 20 = 80
	if score := ipfScore(s); score != 80 {
		t.Errorf("Expected 80, got %d", score)
	}

	s = IPFSummary{TotalPods: 10, FailedPods: 8}
	// failurePct = 80%, score = 100 - 160 = -60 → 0
	if score := ipfScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestIPFNSScore(t *testing.T) {
	if score := ipfNSScore(IPFNSEntry{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	ns := IPFNSEntry{TotalPods: 20, FailedPods: 4}
	// failurePct = 20%, score = 100 - 20 = 80
	if score := ipfNSScore(ns); score != 80 {
		t.Errorf("Expected 80, got %d", score)
	}
}

func TestIPFGenRecs(t *testing.T) {
	s := IPFSummary{
		TotalPods:            50,
		FailedPods:           5,
		ImagePullBackOff:     3,
		ErrImagePull:         1,
		RegistryAuthFailure:  2,
		RateLimited:          1,
		CreateContainerError: 1,
		UniqueFailedImages:   3,
		RetriesTotal:         15,
		HealthScore:          65,
	}
	byImage := []IPFImageStat{
		{Image: "nginx:latest", FailureCount: 3},
	}

	recs := ipfGenRecs(s, byImage)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundPullFail := false
	foundAuth := false
	foundRateLimit := false
	for _, r := range recs {
		if strContains(r, "image pull failure") {
			foundPullFail = true
		}
		if strContains(r, "imagePullSecrets") {
			foundAuth = true
		}
		if strContains(r, "rate-limited") || strContains(r, "Docker Hub mirror") {
			foundRateLimit = true
		}
	}
	if !foundPullFail {
		t.Error("Expected recommendation about image pull failures")
	}
	if !foundAuth {
		t.Error("Expected recommendation about registry auth")
	}
	if !foundRateLimit {
		t.Error("Expected recommendation about rate limiting")
	}
}

func TestIPFGenRecsClean(t *testing.T) {
	s := IPFSummary{TotalPods: 50, FailedPods: 0}
	recs := ipfGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestIPFRiskRank(t *testing.T) {
	if ipfRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if ipfRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if ipfRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
}

func TestIPFIssueRank(t *testing.T) {
	if ipfIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if ipfIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
