package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2620_2625(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PreemptionPolicy2620Result", (PreemptionPolicy2620Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("MemReqDetail2620Result", (MemReqDetail2620Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SvcAllocLBNP2620Result", (SvcAllocLBNP2620Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSAnnotDetail2621Result", (RSAnnotDetail2621Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSAvailRep2621Result", (STSAvailRep2621Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSNumUnavail2621Result", (DSNumUnavail2621Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeKubelet2622Result", (NodeKubelet2622Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CtnrVsInit2622Result", (CtnrVsInit2622Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EphemeralLimit2622Result", (EphemeralLimit2622Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CapAddCount2623Result", (CapAddCount2623Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretOwnerRefCount2623Result", (SecretOwnerRefCount2623Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRWildcardVerbs2623Result", (CRWildcardVerbs2623Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeSysUUID2624Result", (NodeSysUUID2624Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodQOSClass2624Result", (PodQOSClass2624Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSTypeLabel2624Result", (NSTypeLabel2624Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByEPS2Result2625", (TopNSByEPS2Result2625{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUReqTotal2625Result", (NodeCPUReqTotal2625Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HPAMaxReplicas2625Result", (HPAMaxReplicas2625Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
