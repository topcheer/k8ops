package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2518_2523(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("HostnameVsNodeResult2518", (HostnameVsNodeResult2518{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ImageLayerResult2518", (ImageLayerResult2518{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HCNodePortResult2518", (HCNodePortResult2518{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSConditionsResult2519", (RSConditionsResult2519{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSCurrentRepResult2519", (STSCurrentRepResult2519{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSConditionsResult2519", (DSConditionsResult2519{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeConditionsResult2520", (NodeConditionsResult2520{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodFailedCountResult2520", (PodFailedCountResult2520{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ExitCodeResult2520", (ExitCodeResult2520{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SeccompOnRootResult2521", (SeccompOnRootResult2521{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretOwnerKindResult2521", (SecretOwnerKindResult2521{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CRNonResourceResult2521", (CRNonResourceResult2521{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RuntimeVsKubeletResult2522", (RuntimeVsKubeletResult2522{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodSAResult2522", (PodSAResult2522{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSDeletionResult2522", (NSDeletionResult2522{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNodeMemReqResult2523", (TopNodeMemReqResult2523{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodePodsAllocRatioResult2523", (NodePodsAllocRatioResult2523{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("JobTotalResult2523", (JobTotalResult2523{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
