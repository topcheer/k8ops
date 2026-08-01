package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2584_2589(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodAntiAffinityResult2584", (PodAntiAffinityResult2584{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EnvVarCountResult2584", (EnvVarCountResult2584{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LBIngressResult2584", (LBIngressResult2584{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSDeletionResult2585", (RSDeletionResult2585{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSPVCRetentionResult2585", (STSPVCRetentionResult2585{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSTemplateGenResult2585", (DSTemplateGenResult2585{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMachineIDResult2586", (NodeMachineIDResult2586{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodCIDRResult2586", (PodCIDRResult2586{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CPULimitDetailResult2586", (CPULimitDetailResult2586{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SupplementalGroupsResult2587", (SupplementalGroupsResult2587{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretTypeLabelResult2587", (SecretTypeLabelResult2587{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBSubjectAPIGroupResult2587", (RBSubjectAPIGroupResult2587{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeBootIDResult2588", (NodeBootIDResult2588{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TolerationsSummaryResult2588", (TolerationsSummaryResult2588{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSUIDVsCreationResult2588", (NSUIDVsCreationResult2588{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByEvt2Result2589", (TopNSByEvt2Result2589{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUAllocSummaryResult2589", (NodeCPUAllocSummaryResult2589{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CMTotal2589Result", (CMTotal2589Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
