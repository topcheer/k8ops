package dashboard

import (
	"testing"
	"time"
)

func TestHandlers2560_2565(t *testing.T) {
	check := func(name string, hs int) {
		if hs != 100 {
			t.Fatal(name)
		}
	}
	check("TermGraceSummaryResult2560", (TermGraceSummaryResult2560{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("MemLimitContainerResult2560", (MemLimitContainerResult2560{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("ClusterIPNoneResult2560", (ClusterIPNoneResult2560{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RSPausedResult2561", (RSPausedResult2561{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("STSPartitionResult2561", (STSPartitionResult2561{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("DSDeletionResult2561", (DSDeletionResult2561{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeAllocVsRunningResult2562", (NodeAllocVsRunningResult2562{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("PodVolumeSizeResult2562", (PodVolumeSizeResult2562{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("StartupProbeResult2562", (StartupProbeResult2562{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("HostPIDDetailResult2563", (HostPIDDetailResult2563{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("SecretImmutableResult2563", (SecretImmutableResult2563{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("RBSubjectsCountResult2563", (RBSubjectsCountResult2563{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeCapVsAllocStorResult2564", (NodeCapVsAllocStorResult2564{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeSelectorDetailResult2564", (NodeSelectorDetailResult2564{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NSLabelVsAnnotResult2564", (NSLabelVsAnnotResult2564{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("TopNSByPVC2Result2565", (TopNSByPVC2Result2565{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("NodeMemAllocDetailResult2565", (NodeMemAllocDetailResult2565{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
	check("JobActiveResult2565", (JobActiveResult2565{ScannedAt: time.Now(), HealthScore: 100}).HealthScore)
}
