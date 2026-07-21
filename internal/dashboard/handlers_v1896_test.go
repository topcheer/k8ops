package dashboard

import (
	"testing"
)

func TestPodEscapeResultStruct1896(t *testing.T) {
	r := PodEscapeResult{
		Summary: PodEscapeSummary{
			TotalWorkloads: 60,
			SafeWorkloads:  30,
			HighRiskCount:  10,
			Privileged:     5,
			HostPathMounts: 8,
			RunAsRoot:      20,
		},
		HealthScore: 50,
	}
	if r.Summary.Privileged != 5 {
		t.Errorf("expected 5 privileged, got %d", r.Summary.Privileged)
	}
}

func TestEgressPolicyResultStruct1896(t *testing.T) {
	r := EgressPolicyResult{
		Summary: EgressPolicySummary{
			TotalNamespaces:  29,
			WithEgressPolicy: 3,
			WithoutEgress:    26,
			TotalNetPols:     5,
		},
		HealthScore: 10,
	}
	if r.Summary.WithoutEgress != 26 {
		t.Errorf("expected 26 without egress, got %d", r.Summary.WithoutEgress)
	}
}

func TestCISBenchmarkResultStruct1896(t *testing.T) {
	r := CISBenchmarkResult{
		Summary: CISMBSummary{
			TotalChecks: 11,
			Passed:      5,
			Failed:      3,
			Warn:        3,
			Score:       45,
		},
		HealthScore: 45,
	}
	if r.Summary.Failed != 3 {
		t.Errorf("expected 3 failed, got %d", r.Summary.Failed)
	}
}

func TestIsDangerousCap1896(t *testing.T) {
	dangerous := []string{"SYS_ADMIN", "NET_ADMIN", "SYS_MODULE", "SYS_PTRACE"}
	for _, c := range dangerous {
		if !isDangerousCap1896(c) {
			t.Errorf("expected %q to be dangerous", c)
		}
	}
	safe := []string{"CHOWN_OK", "FAKE_CAP"}
	for _, c := range safe {
		if isDangerousCap1896(c) {
			t.Errorf("expected %q to NOT be dangerous", c)
		}
	}
}

func TestIsSensitiveHostPath1896(t *testing.T) {
	sensitive := []string{"/etc/passwd", "/var/run/docker.sock", "/proc/1", "/sys/kernel", "/root/.ssh"}
	for _, p := range sensitive {
		if !isSensitiveHostPath1896(p) {
			t.Errorf("expected %q to be sensitive", p)
		}
	}
	safe := []string{"/tmp/data", "/var/log/app", "/home/user/config"}
	for _, p := range safe {
		if isSensitiveHostPath1896(p) {
			t.Errorf("expected %q to NOT be sensitive", p)
		}
	}
}

func TestBuildPodEscapeRecs1896(t *testing.T) {
	result := &PodEscapeResult{
		Summary: PodEscapeSummary{
			TotalWorkloads: 50, HighRiskCount: 10,
			MediumRiskCount: 5, SafeWorkloads: 35,
			Privileged: 3, HostPathMounts: 5, RunAsRoot: 15,
		},
	}
	recs := buildPodEscapeRecs1896(result)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recs, got %d", len(recs))
	}
}

func TestBuildEgressRecs1896(t *testing.T) {
	result := &EgressPolicyResult{
		Summary: EgressPolicySummary{
			TotalNamespaces: 29, WithEgressPolicy: 5,
			WithoutEgress: 24, TotalNetPols: 8, EgressNetPols: 3,
		},
	}
	recs := buildEgressRecs1896(result)
	if len(recs) < 1 {
		t.Errorf("expected recs, got %d", len(recs))
	}
}

func TestBuildCISRecs1896(t *testing.T) {
	result := &CISBenchmarkResult{
		Summary: CISMBSummary{
			TotalChecks: 11, Passed: 5, Failed: 4, Warn: 2, Score: 45,
		},
	}
	recs := buildCISRecs1896(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
