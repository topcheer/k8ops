package dashboard

import "testing"

func TestSecretExposureResult1919(t *testing.T) {
	r := SecretExposureResult1919{
		Summary:     SecretExposureSummary1919{TotalSecrets: 73, MountedSecrets: 20, UnusedSecrets: 53, OverExposedCount: 2},
		HealthScore: 97,
	}
	if r.Summary.UnusedSecrets != 53 {
		t.Errorf("expected 53, got %d", r.Summary.UnusedSecrets)
	}
}

func TestAdmissionExceptionResult1919(t *testing.T) {
	r := AdmissionExceptionResult{
		Summary:     AdmissionExcSummary{TotalNamespaces: 29, WithPSAEnforce: 0, WithoutPSA: 29, WebhookCount: 2, MissingGatekeeper: true},
		HealthScore: 0,
	}
	if r.Summary.WithoutPSA != 29 {
		t.Errorf("expected 29, got %d", r.Summary.WithoutPSA)
	}
}

func TestProcMountResult1919(t *testing.T) {
	r := ProcMountResult{
		Summary:     ProcMountSummary{TotalContainers: 72, DefaultProcMount: 72, UnmaskedProcMount: 0, HostPathWritable: 11},
		HealthScore: 84,
	}
	if r.Summary.HostPathWritable != 11 {
		t.Errorf("expected 11, got %d", r.Summary.HostPathWritable)
	}
}

func TestBuildSecretExposureRecs1919(t *testing.T) {
	r := &SecretExposureResult1919{Summary: SecretExposureSummary1919{TotalSecrets: 73, MountedSecrets: 20, UnusedSecrets: 53, OverExposedCount: 2, MaxMountCount: 5}}
	recs := buildSecretExposureRecs1919(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildAdmissionExcRecs1919(t *testing.T) {
	r := &AdmissionExceptionResult{Summary: AdmissionExcSummary{TotalNamespaces: 29, WithPSAEnforce: 0, WithoutPSA: 29, WebhookCount: 2, MissingGatekeeper: true}}
	recs := buildAdmissionExcRecs1919(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildProcMountRecs1919(t *testing.T) {
	r := &ProcMountResult{Summary: ProcMountSummary{TotalContainers: 72, UnmaskedProcMount: 0, WritableTmpfs: 3, HostPathWritable: 11}}
	recs := buildProcMountRecs1919(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
