package dashboard

import "testing"

func TestReadyLatResult2063(t *testing.T) {
	r := ReadyLatResult2063{Summary: ReadyLatSummary2063{TotalPods: 100, SlowPods: 5}}
	if r.Summary.SlowPods != 5 {
		t.Errorf("expected 5")
	}
}
func TestRestartVelResult2063(t *testing.T) {
	r := RestartVelResult2063{Summary: RestartVelSummary2063{TotalPods: 100, HighVelocity: 3}}
	if r.Summary.HighVelocity != 3 {
		t.Errorf("expected 3")
	}
}
func TestEventNoiseResult2063(t *testing.T) {
	r := EventNoiseResult2063{Summary: EventNoiseSummary2063{TotalEvents: 1000, NoisyReasons: 5}}
	if r.Summary.NoisyReasons != 5 {
		t.Errorf("expected 5")
	}
}
