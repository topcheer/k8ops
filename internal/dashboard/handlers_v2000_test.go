package dashboard

import "testing"

func TestCPHAAResult2000(t *testing.T) {
	r := CPHAAResult2000{Summary: CPHAASummary2000{APIServerCount: 3, EtcdCount: 3, HALevel: "high"}}
	if r.Summary.HALevel != "high" {
		t.Errorf("expected high")
	}
}
func TestCPHAAEntry2000(t *testing.T) {
	e := CPHAAEntry2000{Component: "etcd", Count: 3, Status: "running"}
	if e.Count != 3 {
		t.Errorf("expected 3")
	}
}
func TestAntiAffResult2000(t *testing.T) {
	r := AntiAffResult2000{Summary: AntiAffSummary2000{TotalDeployments: 20, WithAntiAffinity: 5, WithoutAny: 15}}
	if r.Summary.WithoutAny != 15 {
		t.Errorf("expected 15")
	}
}
func TestAntiAffEntry2000(t *testing.T) {
	e := AntiAffEntry2000{Name: "api", Namespace: "prod", Replicas: 3, Type: "required"}
	if e.Type != "required" {
		t.Errorf("expected required")
	}
}
func TestHeadroomResult2000(t *testing.T) {
	r := HeadroomResult2000{Summary: HeadroomSummary2000{TotalNodes: 5, CPUHeadroomPct: 60.0, MemHeadroomPct: 50.0}}
	if r.Summary.CPUHeadroomPct != 60.0 {
		t.Errorf("expected 60")
	}
}
func TestHeadroomEntry2000(t *testing.T) {
	e := HeadroomEntry2000{Node: "node-1", AllocCPU: 16.0, ReqCPU: 8.0, HeadroomPct: 50.0}
	if e.HeadroomPct != 50.0 {
		t.Errorf("expected 50")
	}
}
func TestContainsStr2000(t *testing.T) {
	if !containsStr2000("kube-apiserver:v1.28", "kube-apiserver") {
		t.Errorf("expected true")
	}
	if containsStr2000("nginx", "etcd") {
		t.Errorf("expected false")
	}
}
func TestCPHAASummary2000(t *testing.T) {
	s := CPHAASummary2000{FaultTolerance: 1}
	if s.FaultTolerance != 1 {
		t.Errorf("expected 1")
	}
}
