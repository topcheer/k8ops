package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2572_2577(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodGMSAResult2572", (PodGMSAResult2572{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("MemReqVsLimResult2572", (MemReqVsLimResult2572{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LBClassResult2572", (LBClassResult2572{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSMinReadyResult2573", (RSMinReadyResult2573{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSPodMgmtResult2573", (STSPodMgmtResult2573{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSHostNetResult2573", (DSHostNetResult2573{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUAllocVsReqResult2574", (NodeCPUAllocVsReqResult2574{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PriorityClassDistResult2574", (PriorityClassDistResult2574{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TermSignalResult2574", (TermSignalResult2574{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("FSGroupDetailResult2575", (FSGroupDetailResult2575{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretRevisionResult2575", (SecretRevisionResult2575{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRBSubjectNSResult2575", (CRBSubjectNSResult2575{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeOSImageDistResult2576", (NodeOSImageDistResult2576{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HostAliasesSummaryResult2576", (HostAliasesSummaryResult2576{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSLabelCountResult2576", (NSLabelCountResult2576{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByDeploy2Result2577", (TopNSByDeploy2Result2577{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemReqTotalResult2577", (NodeMemReqTotalResult2577{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ServiceTotalByTypeResult2577", (ServiceTotalByTypeResult2577{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
