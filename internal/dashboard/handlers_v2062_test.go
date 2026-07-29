package dashboard

import "testing"

func TestGenTrackResult2062(t *testing.T) {
	r := GenTrackResult2062{Summary: GenTrackSummary2062{TotalDeploys: 50, HighGenCount: 3, AvgGen: 15}}
	if r.Summary.HighGenCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestStdinResult2062(t *testing.T) {
	r := StdinResult2062{Summary: StdinSummary2062{TotalContainers: 200, StdinEnabled: 5, TTYEnabled: 3}}
	if r.Summary.StdinEnabled != 5 {
		t.Errorf("expected 5")
	}
}
func TestSpreadResult2062(t *testing.T) {
	r := SpreadResult2062{Summary: SpreadSummary2062{TotalMultiReplica: 30, WithSpread: 10, MissingSpread: 20}}
	if r.Summary.MissingSpread != 20 {
		t.Errorf("expected 20")
	}
}
