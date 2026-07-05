package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsCoreDNSPod(t *testing.T) {
	// CoreDNS in kube-system
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns-1", Namespace: "kube-system"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Image: "registry.k8s.io/coredns:v1.11.1"},
		}},
	}
	if !isCoreDNSPod(pod1) {
		t.Error("Expected true for coredns image in kube-system")
	}

	// Non-coredns pod
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Image: "nginx"},
		}},
	}
	if isCoreDNSPod(pod2) {
		t.Error("Expected false for nginx pod")
	}

	// Coredns in wrong namespace
	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "default"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Image: "coredns:v1.10"},
		}},
	}
	if isCoreDNSPod(pod3) {
		t.Error("Expected false for coredns in default namespace")
	}
}

func TestExtractCoreDNSVersion(t *testing.T) {
	tests := []struct {
		image  string
		expect string
	}{
		{"registry.k8s.io/coredns:v1.11.1", "v1.11.1"},
		{"coredns:1.10.1", "1.10.1"},
		{"coredns", ""},
		{"ghcr.io/coredns/coredns:v1.12.0@sha256:abc", "v1.12.0"},
	}

	for _, tt := range tests {
		got := extractCoreDNSVersion(tt.image)
		if got != tt.expect {
			t.Errorf("extractCoreDNSVersion(%q) = %q, want %q", tt.image, got, tt.expect)
		}
	}
}

func TestExtractDNSForwarders(t *testing.T) {
	corefile := `. {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    forward . 8.8.8.8 1.1.1.1
    cache 30
    loop
    reload
    loadbalance
}`

	forwarders := extractDNSForwarders(corefile)
	if len(forwarders) != 2 {
		t.Fatalf("Expected 2 forwarders, got %d: %v", len(forwarders), forwarders)
	}
	if forwarders[0] != "8.8.8.8" || forwarders[1] != "1.1.1.1" {
		t.Errorf("Expected [8.8.8.8, 1.1.1.1], got %v", forwarders)
	}
}

func TestExtractDNSForwardersEmpty(t *testing.T) {
	corefile := `. {
    errors
    kubernetes cluster.local
}`
	forwarders := extractDNSForwarders(corefile)
	if len(forwarders) != 0 {
		t.Errorf("Expected 0 forwarders, got %d", len(forwarders))
	}
}

func TestExtractCoreDNSPlugins(t *testing.T) {
	corefile := `. {
    errors
    health
    ready
    kubernetes cluster.local {
        fallthrough
    }
    forward . 8.8.8.8
    cache 30
    loop
    reload
    loadbalance
}`
	plugins := extractCoreDNSPlugins(corefile)

	expected := []string{"errors", "health", "ready", "kubernetes", "forward", "cache", "loop", "reload", "loadbalance"}
	if len(plugins) != len(expected) {
		t.Fatalf("Expected %d plugins, got %d: %v", len(expected), len(plugins), plugins)
	}
	for _, e := range expected {
		found := false
		for _, p := range plugins {
			if p == e {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected plugin %q not found in %v", e, plugins)
		}
	}
}

func TestCalculateDNSHealthScore(t *testing.T) {
	// Perfect
	s := DNSHealthSummary{CoreDNSPods: 2, CoreDNSReady: 2}
	if score := calculateDNSHealthScore(s, true); score != 100 {
		t.Errorf("Expected 100 for healthy, got %d", score)
	}

	// No CoreDNS
	s = DNSHealthSummary{CoreDNSPods: 0}
	if score := calculateDNSHealthScore(s, false); score != 0 {
		t.Errorf("Expected 0 for no coredns, got %d", score)
	}

	// Not all ready
	s = DNSHealthSummary{CoreDNSPods: 2, CoreDNSReady: 1, HeadlessNoEP: 3}
	// 100 - 40 - 9 = 51
	score := calculateDNSHealthScore(s, false)
	if score != 51 {
		t.Errorf("Expected 51 for unhealthy, got %d", score)
	}
}

func TestGenerateDNSRecommendations(t *testing.T) {
	s := DNSHealthSummary{
		CoreDNSPods:  1,
		CoreDNSReady: 1,
		HeadlessNoEP: 2,
		NDotsIssues:  3,
		HealthScore:  40,
	}
	coredns := CoreDNSStatus{HasCorefile: true, Forwarders: nil, Plugins: []string{"errors"}}
	dnsConfig := DNSConfigAnalysis{HasNodeLocalDNS: false}

	recs := generateDNSRecommendations(s, coredns, dnsConfig)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundSingleCoreDNS := false
	foundHeadless := false
	foundNodeLocal := false
	foundNoForwarders := false
	for _, r := range recs {
		if containsSubstr(r, "at least 2") {
			foundSingleCoreDNS = true
		}
		if containsSubstr(r, "headless") {
			foundHeadless = true
		}
		if containsSubstr(r, "NodeLocal") {
			foundNodeLocal = true
		}
		if containsSubstr(r, "forwarders") {
			foundNoForwarders = true
		}
	}
	if !foundSingleCoreDNS {
		t.Error("Expected recommendation about single CoreDNS replica")
	}
	if !foundHeadless {
		t.Error("Expected recommendation about headless services")
	}
	if !foundNodeLocal {
		t.Error("Expected recommendation about NodeLocal DNS")
	}
	if !foundNoForwarders {
		t.Error("Expected recommendation about missing forwarders")
	}
}

func TestGenerateDNSRecommendationsClean(t *testing.T) {
	s := DNSHealthSummary{
		CoreDNSPods:  2,
		CoreDNSReady: 2,
		HealthScore:  100,
	}
	coredns := CoreDNSStatus{
		HasCorefile: true,
		Forwarders:  []string{"8.8.8.8"},
		Plugins:     []string{"errors", "ready", "forward"},
	}
	dnsConfig := DNSConfigAnalysis{HasNodeLocalDNS: true}

	recs := generateDNSRecommendations(s, coredns, dnsConfig)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestDNSIssueRank(t *testing.T) {
	if dnsIssueRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if dnsIssueRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if dnsIssueRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}
