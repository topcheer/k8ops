package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2506_2511(t *testing.T) {
	r1 := PreemptionPolicyResult2506{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("PreemptionPolicyResult2506")
	}
	r2 := ImageRegistryDomainResult2506{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ImageRegistryDomainResult2506")
	}
	r3 := ExternalIPsResult2506{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("ExternalIPsResult2506")
	}
	r4 := RSTemplateGenResult2507{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSTemplateGenResult2507")
	}
	r5 := STSReplicasVsReadyResult2507{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSReplicasVsReadyResult2507")
	}
	r6 := DSNumUnavailableResult2507{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSNumUnavailableResult2507")
	}
	r7 := NodeCapCPUResult2508{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeCapCPUResult2508")
	}
	r8 := PodHostNetCountResult2508{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("PodHostNetCountResult2508")
	}
	r9 := VolumeDeviceResult2508{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("VolumeDeviceResult2508")
	}
	r10 := CapAddSummaryResult2509{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("CapAddSummaryResult2509")
	}
	r11 := SecretTypeFullResult2509{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretTypeFullResult2509")
	}
	r12 := CRResourceResult2509{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("CRResourceResult2509")
	}
	r13 := NodeVerCompareResult2510{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeVerCompareResult2510")
	}
	r14 := ActiveDeadlineResult2510{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("ActiveDeadlineResult2510")
	}
	r15 := NSFinalizerListResult2510{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("NSFinalizerListResult2510")
	}
	r16 := TopNSByRSResult2511{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByRSResult2511")
	}
	r17 := NodeMemLimitTotalResult2511{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeMemLimitTotalResult2511")
	}
	r18 := LeaseCountResult2511{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("LeaseCountResult2511")
	}
}
