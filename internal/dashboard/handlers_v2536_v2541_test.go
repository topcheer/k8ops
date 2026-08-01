package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2536_2541(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("CPUReqSummaryResult2536", (CPUReqSummaryResult2536{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("VolumeMountDetailResult2536", (VolumeMountDetailResult2536{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ServicePortsSummaryResult2536", (ServicePortsSummaryResult2536{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSLabelSelectorResult2537", (RSLabelSelectorResult2537{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSCurrentRevResult2537", (STSCurrentRevResult2537{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSUpdatedVsDesiredResult2537", (DSUpdatedVsDesiredResult2537{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocPodsDetailResult2538", (NodeAllocPodsDetailResult2538{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodCPULimitTotalResult2538", (PodCPULimitTotalResult2538{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RunningStateDetailResult2538", (RunningStateDetailResult2538{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CapAddVsDropResult2539", (CapAddVsDropResult2539{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretTypeVsKeysResult2539", (SecretTypeVsKeysResult2539{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBUserVsGroupResult2539", (RBUserVsGroupResult2539{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocVsCapCPUResult2540", (NodeAllocVsCapCPUResult2540{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DNSPolicyDetailResult2540", (DNSPolicyDetailResult2540{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSFinalizerSummaryResult2540", (NSFinalizerSummaryResult2540{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSBySvc2541Result", (TopNSBySvc2541Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemUsageRatioResult2541", (NodeMemUsageRatioResult2541{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PDBCountResult2541", (PDBCountResult2541{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
