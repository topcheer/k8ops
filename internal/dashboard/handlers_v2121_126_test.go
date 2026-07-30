package dashboard

import "testing"

func TestTermGraceResult2121(t *testing.T) {
	r := TermGraceResult2121{Summary: TermGraceSummary2121{TotalPods: 100, DefaultGP: 80, CustomGP: 20}}
	if r.Summary.CustomGP != 20 {
		t.Errorf("expected 20")
	}
}
func TestOSSelectorResult2122(t *testing.T) {
	r := OSSelectorResult2122{Summary: OSSelectorSummary2122{TotalPods: 100, WithOSSel: 5}}
	if r.Summary.WithOSSel != 5 {
		t.Errorf("expected 5")
	}
}
func TestQoSGuarResult2123(t *testing.T) {
	r := QoSGuarResult2123{Summary: QoSGuarSummary2123{TotalPods: 100, Guaranteed: 30}}
	if r.Summary.Guaranteed != 30 {
		t.Errorf("expected 30")
	}
}
func TestSysctlResult2124(t *testing.T) {
	r := SysctlResult2124{Summary: SysctlSummary2124{TotalPods: 100, AtRisk: 2}}
	if r.Summary.AtRisk != 2 {
		t.Errorf("expected 2")
	}
}
func TestFeatureLabelResult2125(t *testing.T) {
	r := FeatureLabelResult2125{Summary: FeatureLabelSummary2125{TotalNodes: 1, FeatureLabels: map[string]int{"node.kubernetes.io/instance-type": 1}}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestCPUThrottleResult2126(t *testing.T) {
	r := CPUThrottleResult2126{Summary: CPUThrottleSummary2126{TotalContainers: 200, HighLimit: 10}}
	if r.Summary.HighLimit != 10 {
		t.Errorf("expected 10")
	}
}
func TestNSReplicaResult2126(t *testing.T) {
	r := NSReplicaResult2126{Summary: NSReplicaSummary2126{TotalNS: 20, TotalReplicas: 150}}
	if r.Summary.TotalReplicas != 150 {
		t.Errorf("expected 150")
	}
}
