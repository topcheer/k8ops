package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

func TestEEExposedLevel(t *testing.T) {
	// LoadBalancer = public
	svc := corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	if level := eeExposedLevel(svc); level != "public" {
		t.Errorf("Expected public for LB, got %s", level)
	}

	// NodePort = node
	svc = corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort}}
	if level := eeExposedLevel(svc); level != "node" {
		t.Errorf("Expected node for NodePort, got %s", level)
	}

	// ExternalIP = public
	svc = corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ExternalIPs: []string{"1.2.3.4"}}}
	if level := eeExposedLevel(svc); level != "public" {
		t.Errorf("Expected public for ExternalIP, got %s", level)
	}

	// ClusterIP = internal
	svc = corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1"}}
	if level := eeExposedLevel(svc); level != "internal" {
		t.Errorf("Expected internal for ClusterIP, got %s", level)
	}
}

func TestEEAssessRisk(t *testing.T) {
	// Public, no NP = critical
	entry := EEEntry{ExposedLevel: "public", HasNetworkPolicy: false}
	if level := eeAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	// Public, with NP = high
	entry = EEEntry{ExposedLevel: "public", HasNetworkPolicy: true}
	if level := eeAssessRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	// Node, no NP = high
	entry = EEEntry{ExposedLevel: "node", HasNetworkPolicy: false}
	if level := eeAssessRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	// Internal = low
	entry = EEEntry{ExposedLevel: "internal"}
	if level := eeAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestEEIngressRisk(t *testing.T) {
	entry := EEIngressEntry{HasTLS: false}
	if level := eeIngressRisk(entry); level != "high" {
		t.Errorf("Expected high for no TLS, got %s", level)
	}

	entry = EEIngressEntry{HasTLS: true}
	if level := eeIngressRisk(entry); level != "low" {
		t.Errorf("Expected low for TLS, got %s", level)
	}
}

func TestEEScore(t *testing.T) {
	// Empty
	if score := eeScore(EESummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := EESummary{TotalServices: 10, InternalOnly: 10}
	if score := eeScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Exposed with issues
	s = EESummary{
		TotalServices:   10,
		ExposedExternal: 3, // -15
		NodePorts:       2, // -6
		IngressNoTLS:    2, // -16
		NoNetworkPolicy: 3, // -18
	}
	// 100 - 15 - 6 - 16 - 18 = 45
	if score := eeScore(s); score != 45 {
		t.Errorf("Expected 45, got %d", score)
	}
}

func TestEEGenRecs(t *testing.T) {
	s := EESummary{
		TotalServices:      10,
		ExposedExternal:    3,
		LoadBalancers:      2,
		NodePorts:          1,
		IngressNoTLS:       2,
		NoNetworkPolicy:    3,
		AttackSurfaceScore: 35,
	}
	exposed := []EEEntry{
		{Namespace: "default", Name: "api-svc"},
	}
	ingress := []EEIngressEntry{
		{Namespace: "default", Name: "api-ingress"},
	}

	recs := eeGenRecs(s, exposed, ingress)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundExposed := false
	foundTLS := false
	foundNetPol := false
	for _, r := range recs {
		if strContains(r, "externally exposed") {
			foundExposed = true
		}
		if strContains(r, "NO TLS") {
			foundTLS = true
		}
		if strContains(r, "NetworkPolicy") {
			foundNetPol = true
		}
	}
	if !foundExposed {
		t.Error("Expected recommendation about exposed services")
	}
	if !foundTLS {
		t.Error("Expected recommendation about no TLS")
	}
	if !foundNetPol {
		t.Error("Expected recommendation about NetworkPolicy")
	}
}

func TestEEGenRecsClean(t *testing.T) {
	s := EESummary{TotalServices: 10, InternalOnly: 10}
	recs := eeGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestEENSRisk(t *testing.T) {
	if level := eeNSRisk(EENSEntry{ExposedCount: 5, NoTLSIngress: 2}); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}
	if level := eeNSRisk(EENSEntry{ExposedCount: 1, NoTLSIngress: 1}); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := eeNSRisk(EENSEntry{ExposedCount: 2}); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}
	if level := eeNSRisk(EENSEntry{}); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestEERiskRank(t *testing.T) {
	if eeRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if eeRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if eeRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if eeRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestEEIssueRank(t *testing.T) {
	if eeIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if eeIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}

func TestEEIngressBackend(t *testing.T) {
	ing := netv1.Ingress{
		Spec: netv1.IngressSpec{
			DefaultBackend: &netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{Name: "my-svc"},
			},
		},
	}
	if backend := eeIngressBackend(ing); backend != "my-svc" {
		t.Errorf("Expected my-svc, got %s", backend)
	}

	// Empty
	ing = netv1.Ingress{}
	if backend := eeIngressBackend(ing); backend != "" {
		t.Errorf("Expected empty, got %s", backend)
	}
}

func TestEEGetOrCreateNS(t *testing.T) {
	m := make(map[string]*EENSEntry)
	e1 := eeGetOrCreateNS(m, "default")
	e1.ServiceCount = 5

	e2 := eeGetOrCreateNS(m, "default")
	if e2.ServiceCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.ServiceCount)
	}

	e3 := eeGetOrCreateNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}
