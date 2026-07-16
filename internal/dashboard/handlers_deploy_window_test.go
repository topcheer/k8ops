package dashboard

import (
	"testing"
)

func TestTotalEventsThreshold(t *testing.T) {
	if th := totalEventsThreshold(0); th != 1 {
		t.Errorf("expected 1 for 0 events, got %d", th)
	}
	if th := totalEventsThreshold(100); th != 30 {
		t.Errorf("expected 30 for 100 events, got %d", th)
	}
	if th := totalEventsThreshold(3); th != 1 {
		t.Errorf("expected 1 for 3 events, got %d", th)
	}
}

func TestGenerateDWRecs(t *testing.T) {
	r := DeployWindowResult{
		CurrentRisk: "low",
		Verdict:     "safe-to-deploy",
		BestWindow:  "02:00-04:00 (risk: 5%)",
		WorstWindow: "14:00-16:00 (risk: 85%)",
		Summary: DWSummary{
			CrashLoopPods:     0,
			PendingPods:       0,
			WarningEvents:     5,
			TotalEvents:       50,
			CriticalWorkloads: 3,
		},
		RecommendedWindows: []DWWindow{
			{StartHour: 2, EndHour: 4, RiskScore: 5, DayOfWeek: "off-hours", Reason: "test"},
		},
	}
	recs := generateDWRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recs, got %d", len(recs))
	}

	// With crash loops → should say WAIT
	r2 := r
	r2.Summary.CrashLoopPods = 2
	recs2 := generateDWRecs(r2)
	found := false
	for _, rec := range recs2 {
		if len(rec) > 4 && rec[:4] == "WAIT" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected WAIT recommendation with crash loops")
	}
}
