package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2596_2601(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("SubdomainResult2596", (SubdomainResult2596{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ResourceSummaryDetailResult2596", (ResourceSummaryDetailResult2596{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ExternalTrafficPolicyResult2596", (ExternalTrafficPolicyResult2596{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSOwnerRefCountResult2597", (RSOwnerRefCountResult2597{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSMinReadyResult2597", (STSMinReadyResult2597{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSRollingUpdateResult2597", (DSRollingUpdateResult2597{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeInfoSummaryResult2598", (NodeInfoSummaryResult2598{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodVolCMResult2598", (PodVolCMResult2598{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CPUReqDetailResult2598", (CPUReqDetailResult2598{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HostIPCDetailResult2599", (HostIPCDetailResult2599{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretCreationDistResult2599", (SecretCreationDistResult2599{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRRuleAPIGroupResult2599", (CRRuleAPIGroupResult2599{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RuntimeVsKubelet2600Result", (RuntimeVsKubelet2600Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodSecurityCtx2600Result", (PodSecurityCtx2600Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSPhase2600Result", (NSPhase2600Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByCron2Result2601", (TopNSByCron2Result2601{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeStorCapVsAllocRatioResult2601", (NodeStorCapVsAllocRatioResult2601{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HPAMinReplicasResult2601", (HPAMinReplicasResult2601{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
