package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2482_2487(t *testing.T) {
	r1 := InitContainerResult2482{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("InitContainerResult2482")
	}
	r2 := TermMsgPathResult2482{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("TermMsgPathResult2482")
	}
	r3 := IPFamilyPolicyResult2482{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("IPFamilyPolicyResult2482")
	}
	r4 := RSImageSummaryResult2483{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSImageSummaryResult2483")
	}
	r5 := STSMinReadyResult2483{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSMinReadyResult2483")
	}
	r6 := DSTemplateHashResult2483{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSTemplateHashResult2483")
	}
	r7 := NodeTaintResult2484{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeTaintResult2484")
	}
	r8 := PodConditionDistResult2484{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("PodConditionDistResult2484")
	}
	r9 := ImagePullCountResult2484{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("ImagePullCountResult2484")
	}
	r10 := HostUsersResult2485{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("HostUsersResult2485")
	}
	r11 := SecretSSHAuthResult2485{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretSSHAuthResult2485")
	}
	r12 := RBRoleRefKindResult2485{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("RBRoleRefKindResult2485")
	}
	r13 := NodeKubeProxyResult2486{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeKubeProxyResult2486")
	}
	r14 := PodNodeNameDistResult2486{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("PodNodeNameDistResult2486")
	}
	r15 := SvcClusterIPResult2486{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("SvcClusterIPResult2486")
	}
	r16 := TopNodeCPULimitResult2487{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNodeCPULimitResult2487")
	}
	r17 := NodeMemCapTotalResult2487{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeMemCapTotalResult2487")
	}
	r18 := EPSliceEndpointTotalResult2487{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("EPSliceEndpointTotalResult2487")
	}
}
