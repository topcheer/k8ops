package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2530_2535(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("TermGraceDistResult2530", (TermGraceDistResult2530{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EphemeralLimitResult2530", (EphemeralLimitResult2530{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("IPFamilyPolicyDetailResult2530", (IPFamilyPolicyDetailResult2530{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSStatusDetailResult2531", (RSStatusDetailResult2531{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSReplicasDetailResult2531", (STSReplicasDetailResult2531{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSObservedGenResult2531", (DSObservedGenResult2531{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocMemResult2532", (NodeAllocMemResult2532{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodVolumesResult2532", (PodVolumesResult2532{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("MemReqSummaryResult2532", (MemReqSummaryResult2532{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("WindowsGMSAResult2533", (WindowsGMSAResult2533{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretCreationRateResult2533", (SecretCreationRateResult2533{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRBVerbsSummaryResult2533", (CRBVerbsSummaryResult2533{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeKernelDetailResult2534", (NodeKernelDetailResult2534{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodPriorityDetailResult2534", (PodPriorityDetailResult2534{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSResourceVersionResult2534", (NSResourceVersionResult2534{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByDSResult2535", (TopNSByDSResult2535{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemCapVsAllocResult2535", (NodeMemCapVsAllocResult2535{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EventTypeDistResult2535", (EventTypeDistResult2535{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
