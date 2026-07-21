package dashboard

import "testing"

func TestEnvSecretLeakResult1906(t *testing.T) {
	r := EnvSecretLeakResult{
		Summary:     EnvSecretLeakSummary{TotalContainers: 75, HardcodedSecrets: 10, HighRisk: 3, MediumRisk: 5},
		HealthScore: 86,
	}
	if r.Summary.HardcodedSecrets != 10 {
		t.Errorf("expected 10, got %d", r.Summary.HardcodedSecrets)
	}
}

func TestProbeCoverageResult1906(t *testing.T) {
	r := ProbeCoverageResult{
		Summary:     ProbeCoverageSummary{TotalContainers: 75, WithLiveness: 30, WithoutLiveness: 45, WithReadiness: 25, WithoutReadiness: 50, CriticalMissing: 40},
		HealthScore: 36,
	}
	if r.Summary.WithoutReadiness != 50 {
		t.Errorf("expected 50, got %d", r.Summary.WithoutReadiness)
	}
}

func TestGPUAuditResult1906(t *testing.T) {
	r := GPUAuditResult{
		Summary:     GPUAuditSummary{TotalNodes: 1, NodesWithGPU: 0, WorkloadsWithGPU: 0, GPUCapacity: 0},
		HealthScore: 100,
	}
	if r.Summary.NodesWithGPU != 0 {
		t.Errorf("expected 0, got %d", r.Summary.NodesWithGPU)
	}
}

func TestBuildEnvSecretLeakRecs1906(t *testing.T) {
	r := &EnvSecretLeakResult{Summary: EnvSecretLeakSummary{TotalContainers: 75, HardcodedSecrets: 10, HighRisk: 3, MediumRisk: 5, WithSecretRef: 20}}
	recs := buildEnvSecretLeakRecs1906(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildProbeCoverageRecs1906(t *testing.T) {
	r := &ProbeCoverageResult{Summary: ProbeCoverageSummary{TotalContainers: 75, WithLiveness: 30, WithoutLiveness: 45, WithReadiness: 25, WithoutReadiness: 50, WithStartup: 10, CriticalMissing: 40}}
	recs := buildProbeCoverageRecs1906(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildGPUAuditRecs1906(t *testing.T) {
	r := &GPUAuditResult{Summary: GPUAuditSummary{NodesWithGPU: 0, TotalNodes: 1, WorkloadsWithGPU: 3, GPUAllocated: 3, GPUCapacity: 0, GPUAvailable: -3}}
	recs := buildGPUAuditRecs1906(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
