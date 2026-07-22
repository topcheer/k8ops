package dashboard

import "testing"

func TestManifestDriftResult1917(t *testing.T) {
	r := ManifestDriftResult1917{
		Summary:     DriftSummary1917{TotalWorkloads: 64, DriftDetected: 5, ReplicaDrift: 3, ImageDrift: 1, LabelDrift: 1, NoDrift: 59},
		HealthScore: 92,
	}
	if r.Summary.ReplicaDrift != 3 {
		t.Errorf("expected 3, got %d", r.Summary.ReplicaDrift)
	}
}

func TestPreFlightResult1917(t *testing.T) {
	r := PreFlightResult{
		Summary:     PreFlightSummary{TotalChecks: 7, Passed: 5, Failed: 2, BlockingCount: 1, SafeToDeploy: false},
		HealthScore: 51,
	}
	if r.Summary.SafeToDeploy != false {
		t.Error("expected false")
	}
}

func TestHelmHealthResult1917(t *testing.T) {
	r := HelmHealthResult1917{
		Summary:     HelmHealthSummary1916{TotalReleases: 10, DeployedReleases: 8, FailedReleases: 1, StaleReleases: 2},
		HealthScore: 74,
	}
	if r.Summary.StaleReleases != 2 {
		t.Errorf("expected 2, got %d", r.Summary.StaleReleases)
	}
}

func TestBuildDriftRecs1917(t *testing.T) {
	r := &ManifestDriftResult1917{Summary: DriftSummary1917{DriftDetected: 5, TotalWorkloads: 64, ReplicaDrift: 3, ImageDrift: 1, LabelDrift: 1}}
	recs := buildDriftRecs1917(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildPreFlightRecs1917(t *testing.T) {
	r := &PreFlightResult{Summary: PreFlightSummary{Passed: 5, TotalChecks: 7, BlockingCount: 1, SafeToDeploy: false}}
	recs := buildPreFlightRecs1917(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildHelmHealthRecs1917(t *testing.T) {
	r := &HelmHealthResult1917{Summary: HelmHealthSummary1916{TotalReleases: 10, DeployedReleases: 8, FailedReleases: 1, StaleReleases: 2}}
	recs := buildHelmHealthRecs1917(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
