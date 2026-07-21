package dashboard

import (
	"strings"
	"testing"
)

func TestOwnershipRegistryResultStruct1897(t *testing.T) {
	r := OwnershipRegistryResult{
		Summary: OwnershipRegSummary{
			TotalWorkloads:  73,
			WithOwner:       30,
			WithoutOwner:    43,
			TeamsIdentified: 5,
			CriticalUnowned: 10,
		},
		HealthScore: 41,
	}
	if r.Summary.WithoutOwner != 43 {
		t.Errorf("expected 43 unowned, got %d", r.Summary.WithoutOwner)
	}
}

func TestReleaseNoteResultStruct1897(t *testing.T) {
	r := ReleaseNoteResult{
		Summary: ReleaseNoteSummary{
			TotalChanges:  120,
			ImageUpdates:  30,
			ConfigChanges: 15,
			ScalingEvents: 50,
		},
		HealthScore: 30,
	}
	if r.Summary.ImageUpdates != 30 {
		t.Errorf("expected 30 image updates, got %d", r.Summary.ImageUpdates)
	}
}

func TestPostmortemResultStruct1897(t *testing.T) {
	r := PostmortemResult{
		Summary: PostmortemSummary{
			DetectedIncidents: 5,
			CrashLoops:        2,
			OOMKills:          3,
			HighRestartPods:   8,
			AffectedServices:  4,
		},
		HealthScore: 50,
	}
	if r.Summary.OOMKills != 3 {
		t.Errorf("expected 3 OOM kills, got %d", r.Summary.OOMKills)
	}
}

func TestIsIncidentEvent1897(t *testing.T) {
	incidents := []string{"crash", "OOMKilled", "BackOff", "failed to start", "evicted"}
	for _, s := range incidents {
		if !isIncidentEvent1897(strings.ToLower(s), "") {
			t.Errorf("expected %q to be incident event", s)
		}
	}
	normal := []string{"Started", "Pulled", "Created", "normal event"}
	for _, s := range normal {
		if isIncidentEvent1897(strings.ToLower(s), "") {
			t.Errorf("expected %q to NOT be incident event", s)
		}
	}
}

func TestBuildOwnershipRecs1897(t *testing.T) {
	result := &OwnershipRegistryResult{
		Summary: OwnershipRegSummary{
			TotalWorkloads: 50, WithOwner: 30,
			WithoutOwner: 20, CriticalUnowned: 5, TeamsIdentified: 4,
		},
	}
	recs := buildOwnershipRecs1897(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildReleaseNoteRecs1897(t *testing.T) {
	result := &ReleaseNoteResult{
		Summary: ReleaseNoteSummary{
			TotalChanges: 150, ImageUpdates: 30, NamespacesAffected: 10,
		},
	}
	recs := buildReleaseNoteRecs1897(result)
	if len(recs) < 1 {
		t.Errorf("expected recs, got %d", len(recs))
	}
}

func TestBuildPostmortemRecs1897(t *testing.T) {
	result := &PostmortemResult{
		Summary: PostmortemSummary{
			DetectedIncidents: 5, OOMKills: 2, CrashLoops: 1, AffectedServices: 3,
		},
	}
	recs := buildPostmortemRecs1897(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
