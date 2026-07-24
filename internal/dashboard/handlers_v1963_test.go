package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestWLInsightsResult1963(t *testing.T) {
	r := WLInsightsResult1963{Summary: WLInsightsSummary1963{TotalWorkloads: 20, HealthyCount: 15, WarningCount: 3, CriticalCount: 2}}
	if r.Summary.CriticalCount != 2 {
		t.Errorf("expected 2")
	}
}
func TestWLInsightsEntry1963(t *testing.T) {
	e := WLInsightsEntry1963{Name: "api", Namespace: "prod", Kind: "Deployment", Replicas: 3, Ready: 2, HealthScore: 66.7, Status: "warning"}
	if e.HealthScore != 66.7 {
		t.Errorf("expected 66.7")
	}
	if e.Status != "warning" {
		t.Errorf("expected warning")
	}
}
func TestStorageSummaryResult1963(t *testing.T) {
	r := StorageSummaryResult1963{Summary: StorageSummarySummary1963{TotalPVCs: 30, BoundPVCs: 28, UnboundPVCs: 2, TotalCapacityGB: 500.5}}
	if r.Summary.UnboundPVCs != 2 {
		t.Errorf("expected 2")
	}
}
func TestStorageClassEntry1963(t *testing.T) {
	e := StorageClassEntry1963{Name: "fast-ssd", Provisioner: "csi-driver", PVCCount: 10, IsDefault: true}
	if !e.IsDefault {
		t.Errorf("expected true")
	}
}
func TestPVCSummaryEntry1963(t *testing.T) {
	e := PVCSummaryEntry1963{Name: "data-vol", Namespace: "prod", SizeGB: 100.0, StorageClass: "fast-ssd", Status: "Bound"}
	if e.SizeGB != 100.0 {
		t.Errorf("expected 100.0")
	}
}
func TestNetTopoResult1963(t *testing.T) {
	r := NetTopoResult1963{Summary: NetTopoSummary1963{TotalServices: 50, ClusterIPSvc: 40, WithEndpoints: 45, WithoutEndpoints: 5}}
	if r.Summary.WithoutEndpoints != 5 {
		t.Errorf("expected 5")
	}
}
func TestNetTopoService1963(t *testing.T) {
	e := NetTopoService1963{Name: "api-svc", Namespace: "prod", Type: "ClusterIP", ClusterIP: "10.43.1.5", HasPorts: 2}
	if e.HasPorts != 2 {
		t.Errorf("expected 2")
	}
}
func TestNetTopoEndpoint1963(t *testing.T) {
	e := NetTopoEndpoint1963{Service: "api-svc", Namespace: "prod", Addresses: 3, Ready: true}
	if e.Addresses != 3 {
		t.Errorf("expected 3 addresses")
	}
}
func TestSubCountAddr1963(t *testing.T) {
	ep := corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{
			{Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}}},
			{Addresses: []corev1.EndpointAddress{{IP: "1.2.3.5"}}},
		},
	}
	addrs := subCountAddr(ep)
	if len(addrs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addrs))
	}
}
