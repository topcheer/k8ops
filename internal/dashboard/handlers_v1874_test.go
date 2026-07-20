package dashboard

import (
	"testing"
)

func TestPrivilegeEscalationPathResultStruct1874(t *testing.T) {
	r := PrivilegeEscalationPathResult{
		Summary: PrivEscSummary{
			TotalSubjects: 30,
			ClusterAdmins: 7,
			EscalateUsers: 2,
			WildcardUsers: 5,
		},
		HealthScore: 53,
	}
	if r.Summary.ClusterAdmins != 7 {
		t.Errorf("expected 7, got %d", r.Summary.ClusterAdmins)
	}
}

func TestNetworkSegmentGapResultStruct1874(t *testing.T) {
	r := NetworkSegmentGapResult{
		Summary: NetworkSegmentSummary{
			TotalNamespaces: 29,
			WithNetPol:      0,
			WithoutNetPol:   29,
		},
		HealthScore: 0,
	}
	if r.Summary.WithoutNetPol != 29 {
		t.Errorf("expected 29, got %d", r.Summary.WithoutNetPol)
	}
}

func TestImageBaselineResultStruct1874(t *testing.T) {
	r := ImageBaselineResult{
		Summary: ImageBaselineSummary{
			TotalImages:    83,
			UniqueImages:   63,
			UsingLatestTag: 26,
			UsingDigest:    0,
		},
		HealthScore: 0,
	}
	if r.Summary.UsingDigest != 0 {
		t.Errorf("expected 0, got %d", r.Summary.UsingDigest)
	}
}
