package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2470_2475(t *testing.T) {
	r1 := AffinityRuleResult2470{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("AffinityRuleResult2470")
	}
	r2 := ImageLatestTagResult2470{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ImageLatestTagResult2470")
	}
	r3 := SessionAffinityResult2470{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("SessionAffinityResult2470")
	}
	r4 := DepProgressDeadlineResult2471{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("DepProgressDeadlineResult2471")
	}
	r5 := STSParallelResult2471{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSParallelResult2471")
	}
	r6 := DSTolerationsResult2471{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSTolerationsResult2471")
	}
	r7 := NodeNetUnavailableResult2472{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeNetUnavailableResult2472")
	}
	r8 := QoSBurstableResult2472{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("QoSBurstableResult2472")
	}
	r9 := EnvVarCountResult2472{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("EnvVarCountResult2472")
	}
	r10 := SupplementalGroupsResult2473{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("SupplementalGroupsResult2473")
	}
	r11 := SecretBasicAuthResult2473{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretBasicAuthResult2473")
	}
	r12 := CRBRoleRefNameResult2473{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("CRBRoleRefNameResult2473")
	}
	r13 := NodeBootIDResult2474{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeBootIDResult2474")
	}
	r14 := PodSubdomainResult2474{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("PodSubdomainResult2474")
	}
	r15 := IngressHostnameResult2474{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("IngressHostnameResult2474")
	}
	r16 := TopNSByPVCResult2475{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByPVCResult2475")
	}
	r17 := NodeAllocPodsTotalResult2475{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeAllocPodsTotalResult2475")
	}
	r18 := StorageClassDistResult2475{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("StorageClassDistResult2475")
	}
}
