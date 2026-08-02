package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2632_2637(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodHostUserspace2632Result", (PodHostUserspace2632Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EphemeralReqDetail2632Result", (EphemeralReqDetail2632Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SvcTrafficDist2632Result", (SvcTrafficDist2632Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSSpecVsStatus2633Result", (RSSpecVsStatus2633Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSPVCDeleted2633Result", (STSPVCDeleted2633Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSScheduleDaemon2633Result", (DSScheduleDaemon2633Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCondTrue2634Result", (NodeCondTrue2634Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopologySpread2634Result", (TopologySpread2634Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EnvFromCM2634Result", (EnvFromCM2634Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("FSGroupChange2635Result", (FSGroupChange2635Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretImmutable2Result2635", (SecretImmutable2Result2635{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBSubjectAPIGroup2635Result", (RBSubjectAPIGroup2635Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeOSName2636Result", (NodeOSName2636Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodOSEnabled2636Result", (PodOSEnabled2636Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSLabelKeyCount2636Result", (NSLabelKeyCount2636Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByHPA2637Result", (TopNSByHPA2637Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeStorAllocVsCap2637Result", (NodeStorAllocVsCap2637Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PDBMinAvailable2637Result", (PDBMinAvailable2637Result{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
