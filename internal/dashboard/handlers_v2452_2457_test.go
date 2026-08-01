package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2452_2457(t *testing.T) {
	r1 := HostIPCResult2452{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("HostIPCResult2452")
	}
	r2 := PullPolicyDistResult2452{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("PullPolicyDistResult2452")
	}
	r3 := LBIPCountResult2452{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("LBIPCountResult2452")
	}
	r4 := RSReplicasDistResult2453{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSReplicasDistResult2453")
	}
	r5 := STSServiceNameResult2453{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSServiceNameResult2453")
	}
	r6 := DSNodeSelectorResult2453{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSNodeSelectorResult2453")
	}
	r7 := NodeDiskPressureResult2454{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeDiskPressureResult2454")
	}
	r8 := HostAliasesCountResult2454{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("HostAliasesCountResult2454")
	}
	r9 := StdinOnceUsageResult2454{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("StdinOnceUsageResult2454")
	}
	r10 := PrivilegedCtnrResult2455{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("PrivilegedCtnrResult2455")
	}
	r11 := SecretTLSResult2455{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretTLSResult2455")
	}
	r12 := RBSubjectNSResult2455{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("RBSubjectNSResult2455")
	}
	r13 := NodeKernelResult2456{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeKernelResult2456")
	}
	r14 := DNSPolicyDistResult2456{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("DNSPolicyDistResult2456")
	}
	r15 := ServicePortSummaryResult2456{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("ServicePortSummaryResult2456")
	}
	r16 := TopNSByMemResult2457{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNSByMemResult2457")
	}
	r17 := NodePodPressureResult2457{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodePodPressureResult2457")
	}
	r18 := EventTotalResult2457{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("EventTotalResult2457")
	}
}
