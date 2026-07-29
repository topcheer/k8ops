package dashboard

import "testing"

func TestSecretMountResult2027(t *testing.T) {
	r := SecretMountResult2027{Summary: SecretMountSummary2027{TotalPods: 100, PodsWithSecrets: 30, EnvVarSecrets: 10, VolumeSecrets: 20}}
	if r.Summary.PodsWithSecrets != 30 {
		t.Errorf("expected 30")
	}
}
func TestSecretMountEntry2027(t *testing.T) {
	e := SecretMountEntry2027{Pod: "app", Namespace: "prod", SecretName: "db-password", ExposureType: "env-var", Container: "web"}
	if e.ExposureType != "env-var" {
		t.Errorf("expected env-var")
	}
}
func TestBareNSResult2027(t *testing.T) {
	r := BareNSResult2027{Summary: BareNSSummary2027{TotalNamespaces: 10, NamespacesWithNetPol: 3, BareNamespaces: 7}}
	if r.Summary.BareNamespaces != 7 {
		t.Errorf("expected 7")
	}
}
func TestBareNSEntry2027(t *testing.T) {
	e := BareNSEntry2027{Namespace: "prod", PodCount: 15}
	if e.PodCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestPrivEscResult2027(t *testing.T) {
	r := PrivEscResult2027{Summary: PrivEscSummary2027{TotalContainers: 200, Privileged: 5, WithSysAdmin: 2, WithHostPID: 1, WithHostNetwork: 1}}
	if r.Summary.Privileged != 5 {
		t.Errorf("expected 5")
	}
}
func TestPrivEscEntry2027(t *testing.T) {
	e := PrivEscEntry2027{Pod: "app", Namespace: "prod", Container: "web", Issue: "CAP_SYS_ADMIN"}
	if e.Issue != "CAP_SYS_ADMIN" {
		t.Errorf("expected CAP_SYS_ADMIN")
	}
}
