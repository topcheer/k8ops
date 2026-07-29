package dashboard

import "testing"

func TestKubeProxyResult2051(t *testing.T) {
	r := KubeProxyResult2051{Summary: KubeProxySummary2051{ProxyPodsFound: 3, HealthyPods: 2, RestartedPods: 1}}
	if r.Summary.RestartedPods != 1 {
		t.Errorf("expected 1")
	}
}
func TestKubeProxyEntry2051(t *testing.T) {
	e := KubeProxyEntry2051{Pod: "kube-proxy-abc", Status: "running", Restarts: 3}
	if e.Restarts != 3 {
		t.Errorf("expected 3")
	}
}
func TestCNIResult2051(t *testing.T) {
	r := CNIResult2051{Summary: CNISummary2051{CNIPodsFound: 2, HealthyPods: 2, CNIDetected: "flannel"}}
	if r.Summary.CNIDetected != "flannel" {
		t.Errorf("expected flannel")
	}
}
func TestStorOpResult2051(t *testing.T) {
	r := StorOpResult2051{Summary: StorOpSummary2051{TotalPVCs: 20, PendingPVCs: 2, BoundPVCs: 17, StuckPVCs: 1}}
	if r.Summary.StuckPVCs != 1 {
		t.Errorf("expected 1")
	}
}
func TestStorOpEntry2051(t *testing.T) {
	e := StorOpEntry2051{Name: "data", Namespace: "prod", Phase: "pending-stuck"}
	if e.Phase != "pending-stuck" {
		t.Errorf("expected pending-stuck")
	}
}
