package dashboard

import (
	"testing"
)

func TestBuildPDBGenRecs(t *testing.T) {
	r := &PDBGeneratorResult{Summary: PDBGenSummary{MissingPDB: 5}}
	recs := buildPDBGenRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildNetpolGenRecs(t *testing.T) {
	r := &NetpolGeneratorResult{Summary: NetpolGenSummary{MissingNetpol: 3, TotalNamespaces: 10}}
	recs := buildNetpolGenRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestExtractServiceRefs(t *testing.T) {
	nsSvcMap := map[string]map[string]bool{
		"default": {"api-svc": true, "db-svc": true},
	}
	refs := extractServiceRefs("http://api-svc:8080/path", "default", nsSvcMap)
	if len(refs) != 1 || refs[0] != "default/api-svc" {
		t.Errorf("expected default/api-svc, got %v", refs)
	}

	refs2 := extractServiceRefs("not-a-url", "default", nsSvcMap)
	if len(refs2) != 0 {
		t.Errorf("expected 0 refs, got %v", refs2)
	}
}

func TestBuildSvcDepMapRecs(t *testing.T) {
	r := &ServiceDependencyMapResult{
		Summary:       SvcDepMapSummary{Isolated: 5, CriticalSvcs: 2},
		CriticalPaths: []SvcCriticalPath{{Length: 3}},
	}
	recs := buildSvcDepMapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
