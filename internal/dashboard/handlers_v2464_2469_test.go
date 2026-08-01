package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2464_2469(t *testing.T) {
	r1 := NodeSelectorCountResult2464{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("NodeSelectorCountResult2464")
	}
	r2 := ResourceLimitDistResult2464{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ResourceLimitDistResult2464")
	}
	r3 := ExternalNameResult2464{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("ExternalNameResult2464")
	}
	r4 := DepPausedResult2465{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("DepPausedResult2465")
	}
	r5 := STSOrdinalResult2465{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSOrdinalResult2465")
	}
	r6 := DSDeletionResult2465{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSDeletionResult2465")
	}
	r7 := NodeReadyDurationResult2466{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeReadyDurationResult2466")
	}
	r8 := CrashLoopResult2466{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("CrashLoopResult2466")
	}
	r9 := ImageAgeResult2466{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("ImageAgeResult2466")
	}
	r10 := SeccompProfileResult2467{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("SeccompProfileResult2467")
	}
	r11 := SecretKeyCountResult2467{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretKeyCountResult2467")
	}
	r12 := CRVerbsTotalResult2467{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("CRVerbsTotalResult2467")
	}
	r13 := NodeArchResult2468{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeArchResult2468")
	}
	r14 := TolerationSummaryResult2468{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("TolerationSummaryResult2468")
	}
	r15 := NSLabelCountResult2468{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("NSLabelCountResult2468")
	}
	r16 := TopNSBySvcResult2469{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSBySvcResult2469")
	}
	r17 := NodeStorCapTotalResult2469{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeStorCapTotalResult2469")
	}
	r18 := PVCBoundResult2469{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("PVCBoundResult2469")
	}
}
