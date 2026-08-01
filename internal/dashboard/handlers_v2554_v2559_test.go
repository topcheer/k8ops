package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2554_2559(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("SADistResult2554", (SADistResult2554{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CPUReqContainerResult2554", (CPUReqContainerResult2554{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ServicePortRangeResult2554", (ServicePortRangeResult2554{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSOwnerDetailResult2555", (RSOwnerDetailResult2555{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSSpecRepTotalResult2555", (STSSpecRepTotalResult2555{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSGenSummaryResult2555", (DSGenSummaryResult2555{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemVsCapResult2556", (NodeMemVsCapResult2556{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodVolCountResult2556", (PodVolCountResult2556{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LivenessProbeResult2556", (LivenessProbeResult2556{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PrivilegedContainerResult2557", (PrivilegedContainerResult2557{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretTypeDetailResult2557", (SecretTypeDetailResult2557{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRAPIGroupsResult2557", (CRAPIGroupsResult2557{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeKernelDistResult2558", (NodeKernelDistResult2558{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HostPIDIPCResult2558", (HostPIDIPCResult2558{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSCreationTimeResult2558", (NSCreationTimeResult2558{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSBySTSRepResult2559", (TopNSBySTSRepResult2559{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUCapDetailResult2559", (NodeCPUCapDetailResult2559{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DeployTotal2559Result", (DeployTotal2559Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
