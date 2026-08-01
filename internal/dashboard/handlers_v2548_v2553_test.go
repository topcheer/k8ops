package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2548_2553(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodOSNameResult2548", (PodOSNameResult2548{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ReqVsLimitResult2548", (ReqVsLimitResult2548{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ServiceTypeSummaryResult2548", (ServiceTypeSummaryResult2548{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSSpecReplicasResult2549", (RSSpecReplicasResult2549{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSRollingUpdateResult2549", (STSRollingUpdateResult2549{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSNumberAvailResult2549", (DSNumberAvailResult2549{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCapMemResult2550", (NodeCapMemResult2550{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodPrioritySummaryResult2550", (PodPrioritySummaryResult2550{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ResourceSummaryResult2550", (ResourceSummaryResult2550{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RunAsUserDetailResult2551", (RunAsUserDetailResult2551{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretNSDistResult2551", (SecretNSDistResult2551{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBRulesSummaryResult2551", (RBRulesSummaryResult2551{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodePodsVsCapResult2552", (NodePodsVsCapResult2552{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RestartPolicyResult2552", (RestartPolicyResult2552{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSAnnotKeyResult2552", (NSAnnotKeyResult2552{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByEvtResult2553", (TopNSByEvtResult2553{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeStorAllocResult2553", (NodeStorAllocResult2553{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSTotal2553Result", (STSTotal2553Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
