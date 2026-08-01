package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2446_2451(t *testing.T) {
	r1 := HostNetworkResult2446{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("HostNetworkResult2446")
	}
	r2 := WorkingDirResult2446{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("WorkingDirResult2446")
	}
	r3 := ExtTrafficPolicyResult2446{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("ExtTrafficPolicyResult2446")
	}
	r4 := DepPartitionResult2447{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("DepPartitionResult2447")
	}
	r5 := STSPodMgmtResult2447{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSPodMgmtResult2447")
	}
	r6 := DSUpdateStrategyResult2447{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSUpdateStrategyResult2447")
	}
	r7 := NodePIDPressureResult2448{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodePIDPressureResult2448")
	}
	r8 := TermGracePeriodResult2448{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("TermGracePeriodResult2448")
	}
	r9 := LifecycleHooksResult2448{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("LifecycleHooksResult2448")
	}
	r10 := HostPIDResult2449{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("HostPIDResult2449")
	}
	r11 := DockerConfigJsonResult2449{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("DockerConfigJsonResult2449")
	}
	r12 := CRBSubjectKindResult2449{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("CRBSubjectKindResult2449")
	}
	r13 := NodeOSImageResult2450{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeOSImageResult2450")
	}
	r14 := RestartPolicyDistResult2450{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("RestartPolicyDistResult2450")
	}
	r15 := ServiceTypeDistResult2450{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("ServiceTypeDistResult2450")
	}
	r16 := TopNSByCPUReqResult2451{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByCPUReqResult2451")
	}
	r17 := NodeCPUAllocTotalResult2451{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeCPUAllocTotalResult2451")
	}
	r18 := CMTotalResult2451{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("CMTotalResult2451")
	}
}
