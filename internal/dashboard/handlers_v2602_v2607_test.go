package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2602_2607(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodOverheadResult2602", (PodOverheadResult2602{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CPULimitDetail2Result2602", (CPULimitDetail2Result2602{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SvcPublishNotReadyResult2602", (SvcPublishNotReadyResult2602{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSSpecVsStatus2603Result", (RSSpecVsStatus2603Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSStrategyDetail2603Result", (STSStrategyDetail2603Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSMinReady2603Result", (DSMinReady2603Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUvsMemResult2604", (NodeCPUvsMemResult2604{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("InitContainerCountResult2604", (InitContainerCountResult2604{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("StdinOnceResult2604", (StdinOnceResult2604{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SeccompLocalhost2605Result", (SeccompLocalhost2605Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretStringDataResult2605", (SecretStringDataResult2605{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRBRoleRefKindResult2605", (CRBRoleRefKindResult2605{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeTaintsSummary2606Result", (NodeTaintsSummary2606Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodNodeName2606Result", (PodNodeName2606Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSPhaseSummary2606Result", (NSPhaseSummary2606Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByEPSResult2607", (TopNSByEPSResult2607{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUvsMemLimit2607Result", (NodeCPUvsMemLimit2607Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PVTotalByPhase2Result2607", (PVTotalByPhase2Result2607{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
