package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2590_2595(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("ReadinessGatesResult2590", (ReadinessGatesResult2590{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("WorkingDirResult2590", (WorkingDirResult2590{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SessionAffinityResult2590", (SessionAffinityResult2590{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSLabelCountResult2591", (RSLabelCountResult2591{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSCollisionDetailResult2591", (STSCollisionDetailResult2591{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSNodeSelectorCountResult2591", (DSNodeSelectorCountResult2591{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RuntimeDistResult2592", (RuntimeDistResult2592{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeSelectorCountResult2592", (NodeSelectorCountResult2592{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ImageLatestResult2592", (ImageLatestResult2592{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ReadOnlyRootFSResult2593", (ReadOnlyRootFSResult2593{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretOwnerUIDResult2593", (SecretOwnerUIDResult2593{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBSubjectNameResult2593", (RBSubjectNameResult2593{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeArchStableResult2594", (NodeArchStableResult2594{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodAffinityResult2594", (PodAffinityResult2594{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSFinalizerDetailResult2594", (NSFinalizerDetailResult2594{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByJob2Result2595", (TopNSByJob2Result2595{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemAllocVsLimResult2595", (NodeMemAllocVsLimResult2595{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NetPolicyTotal2Result2595", (NetPolicyTotal2Result2595{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
