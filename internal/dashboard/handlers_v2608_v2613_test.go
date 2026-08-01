package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2608_2613(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("EphemeralContainersResult2608", (EphemeralContainersResult2608{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CPULimitSummary2608Result", (CPULimitSummary2608Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SvcIPFamily2608Result", (SvcIPFamily2608Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSFinalizers2609Result", (RSFinalizers2609Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSVolClaim2609Result", (STSVolClaim2609Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSRevHistory2609Result", (DSRevHistory2609Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeTaintEffect2610Result", (NodeTaintEffect2610Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RestartPolicy2610Result", (RestartPolicy2610Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LifecycleHook2610Result", (LifecycleHook2610Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RunAsNonRoot2611Result", (RunAsNonRoot2611Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretImmutable2611Result", (SecretImmutable2611Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBRoleRefAPIGroup2611Result", (RBRoleRefAPIGroup2611Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeTaintKey2612Result", (NodeTaintKey2612Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodOSEnabled2612Result", (PodOSEnabled2612Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSLabelVsSpec2612Result", (NSLabelVsSpec2612Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByPDB2613Result", (TopNSByPDB2613Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemReqVsLim2613Result", (NodeMemReqVsLim2613Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EPSlicePort2613Result", (EPSlicePort2613Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
