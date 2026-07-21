package dashboard

import "testing"

func TestImageConsistResult1911(t *testing.T) {
	r := ImageConsistResult{
		Summary:     ImageConsistSummary{TotalContainers: 72, UniqueImages: 50, LatestTagCount: 15, PinnedImages: 57},
		HealthScore: 79,
	}
	if r.Summary.LatestTagCount != 15 {
		t.Errorf("expected 15, got %d", r.Summary.LatestTagCount)
	}
}

func TestConfigReloadResult1911(t *testing.T) {
	r := ConfigReloadResult{
		Summary:     ConfigReloadSummary{TotalCMRefs: 100, HotReloadReady: 60, NeedsRestart: 40, VolumeMounts: 50, EnvVarMounts: 40},
		HealthScore: 60,
	}
	if r.Summary.NeedsRestart != 40 {
		t.Errorf("expected 40, got %d", r.Summary.NeedsRestart)
	}
}

func TestDeployFreezeResult1911(t *testing.T) {
	r := DeployFreezeResult{
		Summary:     DeployFreezeSummary{CurrentlyFrozen: false, ChangesInWindow: 5, SafeToDeploy: true, NextFreezeHours: 24},
		HealthScore: 90,
	}
	if r.Summary.SafeToDeploy != true {
		t.Error("expected safe to deploy")
	}
}

func TestBuildImageConsistRecs1911(t *testing.T) {
	r := &ImageConsistResult{Summary: ImageConsistSummary{TotalContainers: 72, UniqueImages: 50, LatestTagCount: 15, PinnedImages: 57, DifferentRegistries: 3}}
	recs := buildImageConsistRecs1911(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildConfigReloadRecs1911(t *testing.T) {
	r := &ConfigReloadResult{Summary: ConfigReloadSummary{TotalCMRefs: 100, VolumeMounts: 50, ProjectedVolumes: 10, EnvVarMounts: 40, NeedsRestart: 40}}
	recs := buildConfigReloadRecs1911(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildDeployFreezeRecs1911(t *testing.T) {
	r := &DeployFreezeResult{Summary: DeployFreezeSummary{CurrentlyFrozen: true, ActiveFreezeCount: 2, ChangesInWindow: 3, NextFreezeHours: 0}}
	recs := buildDeployFreezeRecs1911(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
