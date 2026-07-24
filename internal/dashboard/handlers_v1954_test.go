package dashboard

import "testing"

func TestOOMForecastResult1954(t *testing.T) {
	r := OOMForecastResult1954{Summary: OOMForecastSummary1954{TotalContainers: 89, OOMHistory: 3, HighRiskCount: 5}}
	if r.Summary.OOMHistory != 3 {
		t.Errorf("expected 3")
	}
}
func TestOOMForecastEntry1954(t *testing.T) {
	e := OOMForecastEntry1954{Container: "app", MemLimitMB: 256, WasOOMKilled: true, RiskLevel: "high"}
	if !e.WasOOMKilled {
		t.Errorf("expected OOM")
	}
}
func TestAPIPatternResult1954(t *testing.T) {
	r := APIPatternResult1954{Summary: APIPatternSummary1954{TotalResources: 222, TotalVerbs: 8}}
	if r.Summary.TotalVerbs != 8 {
		t.Errorf("expected 8")
	}
}
func TestAPIPatternVerbEntry1954(t *testing.T) {
	e := APIPatternVerbEntry1954{Verb: "get", Percentage: 97.3}
	if e.Percentage != 97.3 {
		t.Errorf("expected 97.3")
	}
}
func TestTermCatalogResult1954(t *testing.T) {
	r := TermCatalogResult1954{Summary: TermCatalogSummary1954{TotalTerminated: 15, OOMKilled: 3, ErrorExit: 5}}
	if r.Summary.ErrorExit != 5 {
		t.Errorf("expected 5")
	}
}
func TestTermCatalogEntry1954(t *testing.T) {
	e := TermCatalogEntry1954{PodName: "web", Reason: "OOMKilled", ExitCode: 137}
	if e.ExitCode != 137 {
		t.Errorf("expected 137")
	}
}
func TestTermCatalogReasonEntry1954(t *testing.T) {
	e := TermCatalogReasonEntry1954{Reason: "OOMKilled", Count: 3}
	if e.Count != 3 {
		t.Errorf("expected 3")
	}
}
func TestAPIPatternResourceEntry1954(t *testing.T) {
	e := APIPatternResourceEntry1954{Resource: "pods", VerbCount: 7, Group: "core"}
	if e.VerbCount != 7 {
		t.Errorf("expected 7")
	}
}
