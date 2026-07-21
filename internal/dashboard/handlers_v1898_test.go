package dashboard

import (
	"testing"
)

func TestStorageClassAuditResultStruct1898(t *testing.T) {
	r := StorageClassAuditResult{
		Summary: StorageClassSummary{
			TotalPVCs: 15, TotalStorageGB: 200, StorageClasses: 3,
			BoundPVCs: 14, PendingPVCs: 1,
		},
		HealthScore: 80,
	}
	if r.Summary.PendingPVCs != 1 {
		t.Errorf("expected 1 pending, got %d", r.Summary.PendingPVCs)
	}
}

func TestInterdependencyResultStruct1898(t *testing.T) {
	r := InterdependencyResult{
		Summary: InterdepSummary{
			TotalServices: 100, WithDependencies: 60,
			HubServices: 5, MaxFanIn: 8,
		},
		HealthScore: 60,
	}
	if r.Summary.MaxFanIn != 8 {
		t.Errorf("expected 8, got %d", r.Summary.MaxFanIn)
	}
}

func TestDNSHealthResultStruct1898(t *testing.T) {
	r := DNSHealthResult1898{
		Summary: DNSHealthSummary1898{
			CoreDNSServers: 2, TotalServices: 100,
			HeadlessServices: 5, ConfigIssues: 1,
		},
		HealthScore: 80,
	}
	if r.Summary.CoreDNSServers != 2 {
		t.Errorf("expected 2, got %d", r.Summary.CoreDNSServers)
	}
}

func TestBuildStorageClassRecs1898(t *testing.T) {
	result := &StorageClassAuditResult{
		Summary: StorageClassSummary{
			TotalPVCs: 15, PendingPVCs: 2, NoStorageClass: 3,
		},
	}
	recs := buildStorageClassRecs1898(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildInterdepRecs1898(t *testing.T) {
	result := &InterdependencyResult{
		Summary: InterdepSummary{
			TotalServices: 80, WithDependencies: 50,
			HubServices: 5, OrphanedServices: 10, MaxFanIn: 6,
		},
	}
	recs := buildInterdepRecs1898(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildDNSHealthRecs1898(t *testing.T) {
	result := &DNSHealthResult1898{
		Summary: DNSHealthSummary1898{
			CoreDNSServers: 1, ConfigIssues: 2, LongNames: 3,
		},
	}
	recs := buildDNSHealthRecs1898(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
