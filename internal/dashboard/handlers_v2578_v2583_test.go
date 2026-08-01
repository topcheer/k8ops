package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2578_2583(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("TopologyCountResult2578", (TopologyCountResult2578{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("StdinConfigResult2578", (StdinConfigResult2578{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SvcClusterIPDetailResult2578", (SvcClusterIPDetailResult2578{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSTemplateHashResult2579", (RSTemplateHashResult2579{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSCurVsRepResult2579", (STSCurVsRepResult2579{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSUpdateStrategyResult2579", (DSUpdateStrategyResult2579{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocCapRatioResult2580", (NodeAllocCapRatioResult2580{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ImagePullSecretsDetailResult2580", (ImagePullSecretsDetailResult2580{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EnvFromCountResult2580", (EnvFromCountResult2580{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SeccompDetailResult2581", (SeccompDetailResult2581{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretDataKeyResult2581", (SecretDataKeyResult2581{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRBSubjectNameResult2581", (CRBSubjectNameResult2581{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeStorVsCapResult2582", (NodeStorVsCapResult2582{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CtnrPortSummaryResult2582", (CtnrPortSummaryResult2582{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSAgedistResult2582", (NSAgedistResult2582{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByDS2Result2583", (TopNSByDS2Result2583{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPULimVsAllocResult2583", (NodeCPULimVsAllocResult2583{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EPSliceTotal2583Result", (EPSliceTotal2583Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
