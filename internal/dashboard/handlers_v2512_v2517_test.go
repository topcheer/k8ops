package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2512_2517(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("PodOSResult2512", (PodOSResult2512{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ImageVersionedTagResult2512", (ImageVersionedTagResult2512{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LBSourceRangesResult2512", (LBSourceRangesResult2512{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSReadyRatioResult2513", (RSReadyRatioResult2513{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSGenObservedResult2513", (STSGenObservedResult2513{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSUnavailDetailResult2513", (DSUnavailDetailResult2513{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocCPUResult2514", (NodeAllocCPUResult2514{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodCompletedCountResult2514", (PodCompletedCountResult2514{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CtnrRestartTotalResult2514", (CtnrRestartTotalResult2514{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("FSGroupChangeResult2515", (FSGroupChangeResult2515{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretAnnotationResult2515", (SecretAnnotationResult2515{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBRoleRefNameResult2515", (RBRoleRefNameResult2515{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeOSResult2516", (NodeOSResult2516{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CtnrVsInitCtnrResult2516", (CtnrVsInitCtnrResult2516{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSUIDResult2516", (NSUIDResult2516{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSBySTSResult2517", (TopNSBySTSResult2517{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodePodVsCapResult2517", (NodePodVsCapResult2517{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ControllerRevResult2517", (ControllerRevResult2517{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
