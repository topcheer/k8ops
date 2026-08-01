package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2458_2463(t *testing.T) {
	r1 := DNSConfigResult2458{ScannedAt: time.Now(), HealthScore: 100}
	if r1.HealthScore != 100 {
		t.Fatal("DNSConfigResult2458")
	}
	r2 := ImageSizeEstResult2458{ScannedAt: time.Now(), HealthScore: 100}
	if r2.HealthScore != 100 {
		t.Fatal("ImageSizeEstResult2458")
	}
	r3 := ClusterIPDistResult2458{ScannedAt: time.Now(), HealthScore: 100}
	if r3.HealthScore != 100 {
		t.Fatal("ClusterIPDistResult2458")
	}
	r4 := RSOwnerRefResult2459{ScannedAt: time.Now(), HealthScore: 100}
	if r4.HealthScore != 100 {
		t.Fatal("RSOwnerRefResult2459")
	}
	r5 := STSVolClaimResult2459{ScannedAt: time.Now(), HealthScore: 100}
	if r5.HealthScore != 100 {
		t.Fatal("STSVolClaimResult2459")
	}
	r6 := DSTemplateGenResult2459{ScannedAt: time.Now(), HealthScore: 100}
	if r6.HealthScore != 100 {
		t.Fatal("DSTemplateGenResult2459")
	}
	r7 := NodeKubeletVerResult2460{ScannedAt: time.Now(), HealthScore: 100}
	if r7.HealthScore != 100 {
		t.Fatal("NodeKubeletVerResult2460")
	}
	r8 := PodReadyRatioResult2460{ScannedAt: time.Now(), HealthScore: 100}
	if r8.HealthScore != 100 {
		t.Fatal("PodReadyRatioResult2460")
	}
	r9 := CtnrPortExposureResult2460{ScannedAt: time.Now(), HealthScore: 100}
	if r9.HealthScore != 100 {
		t.Fatal("CtnrPortExposureResult2460")
	}
	r10 := FSGroupResult2461{ScannedAt: time.Now(), HealthScore: 100}
	if r10.HealthScore != 100 {
		t.Fatal("FSGroupResult2461")
	}
	r11 := SecretImmutableResult2461{ScannedAt: time.Now(), HealthScore: 100}
	if r11.HealthScore != 100 {
		t.Fatal("SecretImmutableResult2461")
	}
	r12 := RBClusterWideResult2461{ScannedAt: time.Now(), HealthScore: 100}
	if r12.HealthScore != 100 {
		t.Fatal("RBClusterWideResult2461")
	}
	r13 := NodeRuntimeResult2462{ScannedAt: time.Now(), HealthScore: 100}
	if r13.HealthScore != 100 {
		t.Fatal("NodeRuntimeResult2462")
	}
	r14 := SchedulerNameResult2462{ScannedAt: time.Now(), HealthScore: 100}
	if r14.HealthScore != 100 {
		t.Fatal("SchedulerNameResult2462")
	}
	r15 := IngressTLSSummaryResult2462{ScannedAt: time.Now(), HealthScore: 100}
	if r15.HealthScore != 100 {
		t.Fatal("IngressTLSSummaryResult2462")
	}
	r16 := TopNodeByPodResult2463{ScannedAt: time.Now(), HealthScore: 100}
	if r16.HealthScore != 100 {
		t.Fatal("TopNodeByPodResult2463")
	}
	r17 := NodeCPUCapTotalResult2463{ScannedAt: time.Now(), HealthScore: 100}
	if r17.HealthScore != 100 {
		t.Fatal("NodeCPUCapTotalResult2463")
	}
	r18 := PVTotalResult2463{ScannedAt: time.Now(), HealthScore: 100}
	if r18.HealthScore != 100 {
		t.Fatal("PVTotalResult2463")
	}
}
