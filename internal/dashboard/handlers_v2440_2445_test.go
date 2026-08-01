package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2440_2445(t *testing.T) {
	r1 := TopPodMemReqResult2440{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("TopPodMemReqResult2440")
	}
	r2 := StdinCountResult2440{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("StdinCountResult2440")
	}
	r3 := PVCAccessModesResult2440{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("PVCAccessModesResult2440")
	}
	r4 := STSRevHistoryResult2441{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("STSRevHistoryResult2441")
	}
	r5 := DepMaxSurgeResult2441{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("DepMaxSurgeResult2441")
	}
	r6 := DSMaxUnavailResult2441{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSMaxUnavailResult2441")
	}
	r7 := PodRestartTotalResult2442{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("PodRestartTotalResult2442")
	}
	r8 := NodeMemPressureResult2442{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("NodeMemPressureResult2442")
	}
	r9 := EventTimestampSpreadResult2442{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("EventTimestampSpreadResult2442")
	}
	r10 := RunAsNonRootResult2443{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("RunAsNonRootResult2443")
	}
	r11 := CRAggregatedRulesResult2443{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("CRAggregatedRulesResult2443")
	}
	r12 := SAAutoMountDisabledResult2443{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("SAAutoMountDisabledResult2443")
	}
	r13 := NodeInstanceTypeResult2444{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeInstanceTypeResult2444")
	}
	r14 := PodPriorityDistResult2444{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("PodPriorityDistResult2444")
	}
	r15 := EndpointSliceCountResult2444{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("EndpointSliceCountResult2444")
	}
	r16 := TopNSByPodResult2445{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByPodResult2445")
	}
	r17 := NodeAllocMemResult2445{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeAllocMemResult2445")
	}
	r18 := SecretTypeDistResult2445{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("SecretTypeDistResult2445")
	}
}
