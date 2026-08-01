package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2476_2481(t *testing.T) {
	r1 := TopologySpreadResult2476{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("TopologySpreadResult2476")
	}
	r2 := ImageRegistryResult2476{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ImageRegistryResult2476")
	}
	r3 := SessionAffinityCfgResult2476{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("SessionAffinityCfgResult2476")
	}
	r4 := RSGenerationResult2477{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSGenerationResult2477")
	}
	r5 := STSPVCRetentionResult2477{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSPVCRetentionResult2477")
	}
	r6 := DSAffinityResult2477{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSAffinityResult2477")
	}
	r7 := NodeRuntimeCheckResult2478{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeRuntimeCheckResult2478")
	}
	r8 := PodPhaseDistResult2478{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("PodPhaseDistResult2478")
	}
	r9 := ProbeSummaryResult2478{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("ProbeSummaryResult2478")
	}
	r10 := RORootFSResult2479{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("RORootFSResult2479")
	}
	r11 := SecretSATokenResult2479{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretSATokenResult2479")
	}
	r12 := CRRulesTotalResult2479{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("CRRulesTotalResult2479")
	}
	r13 := NodeMachineIDResult2480{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeMachineIDResult2480")
	}
	r14 := PodHostnameResult2480{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("PodHostnameResult2480")
	}
	r15 := NSPhaseResult2480{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("NSPhaseResult2480")
	}
	r16 := TopNSByCMResult2481{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByCMResult2481")
	}
	r17 := NodeCPUVsCapResult2481{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeCPUVsCapResult2481")
	}
	r18 := NetPolicyTotalResult2481{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("NetPolicyTotalResult2481")
	}
}
