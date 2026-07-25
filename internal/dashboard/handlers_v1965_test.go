package dashboard

import "testing"

func TestDNSPolicyResult1965(t *testing.T) {
	r := DNSPolicyResult1965{Summary: DNSPolicySummary1965{TotalPods: 100, ClusterFirst: 90, WithCustomDNS: 5}}
	if r.Summary.TotalPods != 100 {
		t.Errorf("expected 100")
	}
	if r.Summary.WithCustomDNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestDNSPolicyEntry1965(t *testing.T) {
	e := DNSPolicyEntry1965{Name: "app-1", Namespace: "prod", Policy: "None", HasCustom: true, Nameservers: []string{"8.8.8.8"}}
	if e.Policy != "None" {
		t.Errorf("expected None")
	}
	if !e.HasCustom {
		t.Errorf("expected true")
	}
}
func TestPodPriorityResult1965(t *testing.T) {
	r := PodPriorityResult1965{Summary: PodPrioritySummary1965{TotalPods: 50, WithPriorityClass: 30, WithoutPriority: 20, SystemCritical: 5}}
	if r.Summary.WithoutPriority != 20 {
		t.Errorf("expected 20")
	}
	if r.Summary.SystemCritical != 5 {
		t.Errorf("expected 5")
	}
}
func TestPriorityClassEntry1965(t *testing.T) {
	e := PriorityClassEntry1965{Name: "high-priority", Value: 1000000, IsDefault: false, PodCount: 10}
	if e.Value != 1000000 {
		t.Errorf("expected 1000000")
	}
}
func TestUnassignedPodEntry1965(t *testing.T) {
	e := UnassignedPodEntry1965{Name: "app-1", Namespace: "default"}
	if e.Name != "app-1" {
		t.Errorf("expected app-1")
	}
}
func TestSecretEnvResult1965(t *testing.T) {
	r := SecretEnvResult1965{Summary: SecretEnvSummary1965{TotalPods: 80, PodsWithSecretEnv: 30, TotalSecretRefs: 45, AllKeysExposed: 10, MissingSecrets: 2}}
	if r.Summary.TotalSecretRefs != 45 {
		t.Errorf("expected 45")
	}
	if r.Summary.MissingSecrets != 2 {
		t.Errorf("expected 2")
	}
}
func TestSecretEnvEntry1965(t *testing.T) {
	e := SecretEnvEntry1965{Pod: "app-1", Namespace: "prod", Secret: "db-password", Keys: []string{"password"}, AllKeys: false}
	if e.Secret != "db-password" {
		t.Errorf("expected db-password")
	}
	if e.AllKeys {
		t.Errorf("expected false")
	}
}
func TestSecretEnvEntry1965AllKeys(t *testing.T) {
	e := SecretEnvEntry1965{Pod: "app-2", Namespace: "prod", Secret: "config", AllKeys: true}
	if !e.AllKeys {
		t.Errorf("expected true")
	}
}
