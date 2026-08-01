package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2488_2493(t *testing.T) {
	r1 := PodOverheadResult2488{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("PodOverheadResult2488")
	}
	r2 := ImageWithoutTagResult2488{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ImageWithoutTagResult2488")
	}
	r3 := PublishNotReadyResult2488{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("PublishNotReadyResult2488")
	}
	r4 := RSStatusReplicasResult2489{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSStatusReplicasResult2489")
	}
	r5 := STSUpdateStrategyResult2489{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSUpdateStrategyResult2489")
	}
	r6 := DSDesiredCountResult2489{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSDesiredCountResult2489")
	}
	r7 := NodeUnschedulableResult2490{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeUnschedulableResult2490")
	}
	r8 := ImagePullBackOffResult2490{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("ImagePullBackOffResult2490")
	}
	r9 := VolumeMountResult2490{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("VolumeMountResult2490")
	}
	r10 := RunAsGroupResult2491{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("RunAsGroupResult2491")
	}
	r11 := SecretAuthTokenResult2491{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretAuthTokenResult2491")
	}
	r12 := RBAPIGroupsResult2491{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("RBAPIGroupsResult2491")
	}
	r13 := NodeOSArchResult2492{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeOSArchResult2492")
	}
	r14 := PodPriorityValueResult2492{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("PodPriorityValueResult2492")
	}
	r15 := NSFinalizerResult2492{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("NSFinalizerResult2492")
	}
	r16 := TopNSByEventResult2493{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByEventResult2493")
	}
	r17 := NodeAllocStorTotalResult2493{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeAllocStorTotalResult2493")
	}
	r18 := PriorityClassCountResult2493{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("PriorityClassCountResult2493")
	}
}
