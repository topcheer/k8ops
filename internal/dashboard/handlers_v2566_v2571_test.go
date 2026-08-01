package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2566_2571(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("DNSConfigResult2566", (DNSConfigResult2566{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("LimitVsReqRatioResult2566", (LimitVsReqRatioResult2566{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ExternalNameResult2566", (ExternalNameResult2566{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSAnnotationsResult2567", (RSAnnotationsResult2567{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSServiceNameResult2567", (STSServiceNameResult2567{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSNodeSelectorResult2567", (DSNodeSelectorResult2567{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCondSummaryResult2568", (NodeCondSummaryResult2568{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodTolerationsCountResult2568", (PodTolerationsCountResult2568{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("EphemeralReqResult2568", (EphemeralReqResult2568{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PrivEscResult2569", (PrivEscResult2569{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretLabelCountResult2569", (SecretLabelCountResult2569{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBSubjectKindResult2569", (RBSubjectKindResult2569{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeKubeProxyDistResult2570", (NodeKubeProxyDistResult2570{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodNodeAffinityResult2570", (PodNodeAffinityResult2570{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSAnnotVsFinResult2570", (NSAnnotVsFinResult2570{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSBySecret2Result2571", (TopNSBySecret2Result2571{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCPUReqVsAllocResult2571", (NodeCPUReqVsAllocResult2571{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("CronJobScheduleResult2571", (CronJobScheduleResult2571{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
