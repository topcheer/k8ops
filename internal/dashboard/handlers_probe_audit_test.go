package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func intOrString(i int) intstr.IntOrString {
	return intstr.FromInt(i)
}

func TestExtractProbeDetail(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intOrString(8080),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       15,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	d := extractProbeDetail(probe)

	if d.Type != "httpGet" {
		t.Errorf("Expected httpGet, got %s", d.Type)
	}
	if d.Path != "/healthz" {
		t.Errorf("Expected /healthz, got %s", d.Path)
	}
	if d.Port != 8080 {
		t.Errorf("Expected 8080, got %d", d.Port)
	}
	if d.PeriodSec != 15 {
		t.Errorf("Expected period 15, got %d", d.PeriodSec)
	}
}

func TestExtractProbeDetailDefaults(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intOrString(3306),
			},
		},
	}

	d := extractProbeDetail(probe)

	// Defaults should be applied
	if d.PeriodSec != 10 {
		t.Errorf("Expected default period 10, got %d", d.PeriodSec)
	}
	if d.TimeoutSec != 1 {
		t.Errorf("Expected default timeout 1, got %d", d.TimeoutSec)
	}
	if d.FailureThreshold != 3 {
		t.Errorf("Expected default failure threshold 3, got %d", d.FailureThreshold)
	}
}

func TestAnalyzeProbeConfigAggressive(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    3, // aggressive
		TimeoutSeconds:   1,
		FailureThreshold: 3,
	}

	findings := analyzeProbeConfig("liveness", probe, "app")

	found := false
	for _, f := range findings {
		if f.Check == "aggressive-probe" && f.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Error("Expected aggressive-probe finding")
	}
}

func TestAnalyzeProbeConfigShortTimeout(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    10,
		TimeoutSeconds:   1, // short
		FailureThreshold: 3,
	}

	findings := analyzeProbeConfig("liveness", probe, "app")

	found := false
	for _, f := range findings {
		if f.Check == "short-timeout" {
			found = true
		}
	}
	if !found {
		t.Error("Expected short-timeout finding")
	}
}

func TestAnalyzeProbeConfigLowFailureThreshold(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    10,
		TimeoutSeconds:   3,
		FailureThreshold: 1, // low
	}

	findings := analyzeProbeConfig("readiness", probe, "app")

	found := false
	for _, f := range findings {
		if f.Check == "low-failure-threshold" {
			found = true
		}
	}
	if !found {
		t.Error("Expected low-failure-threshold finding")
	}
}

func TestAnalyzeProbeConfigSlowReadiness(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    90, // slow
		TimeoutSeconds:   3,
		FailureThreshold: 3,
	}

	findings := analyzeProbeConfig("readiness", probe, "app")

	found := false
	for _, f := range findings {
		if f.Check == "slow-readiness" {
			found = true
		}
	}
	if !found {
		t.Error("Expected slow-readiness finding")
	}
}

func TestAnalyzeProbeConfigHighFailureThreshold(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    10,
		TimeoutSeconds:   3,
		FailureThreshold: 15, // high
	}

	findings := analyzeProbeConfig("liveness", probe, "app")

	found := false
	for _, f := range findings {
		if f.Check == "high-failure-threshold" {
			found = true
		}
	}
	if !found {
		t.Error("Expected high-failure-threshold finding")
	}
}

func TestAnalyzeProbeConfigGood(t *testing.T) {
	probe := &corev1.Probe{
		PeriodSeconds:    10,
		TimeoutSeconds:   3,
		FailureThreshold: 3,
	}

	findings := analyzeProbeConfig("liveness", probe, "app")

	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for good probe, got %d", len(findings))
	}
}

func TestProbesIdentical(t *testing.T) {
	tests := []struct {
		name   string
		a      *corev1.Probe
		b      *corev1.Probe
		expect bool
	}{
		{
			"same-http",
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intOrString(80)}}},
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intOrString(80)}}},
			true,
		},
		{
			"different-path",
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intOrString(80)}}},
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intOrString(80)}}},
			false,
		},
		{
			"same-tcp",
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intOrString(3306)}}},
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intOrString(3306)}}},
			true,
		},
		{
			"different-type",
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intOrString(80)}}},
			&corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intOrString(80)}}},
			false,
		},
	}

	for _, tt := range tests {
		got := probesIdentical(tt.a, tt.b)
		if got != tt.expect {
			t.Errorf("probesIdentical(%s) = %v, want %v", tt.name, got, tt.expect)
		}
	}
}

func TestAuditWorkloadProbesMissing(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "app", Image: "nginx:v1"},
		},
	}

	pw := auditWorkloadProbes("Deployment", &metav1.ObjectMeta{Name: "test", Namespace: "default"}, spec)

	if len(pw.Containers) != 1 {
		t.Fatalf("Expected 1 container, got %d", len(pw.Containers))
	}

	// Should have missing-liveness and missing-readiness
	checks := make(map[string]bool)
	for _, f := range pw.Containers[0].Findings {
		checks[f.Check] = true
	}

	if !checks["missing-liveness"] {
		t.Error("Expected missing-liveness finding")
	}
	if !checks["missing-readiness"] {
		t.Error("Expected missing-readiness finding")
	}
	if pw.RiskScore < 16 {
		t.Errorf("Expected risk score >= 16 (2 warnings), got %d", pw.RiskScore)
	}
}

func TestAuditWorkloadProbesComplete(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "app",
				Image: "app:v1@sha256:abc123",
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intOrString(8080)},
					},
					PeriodSeconds:    10,
					TimeoutSeconds:   3,
					FailureThreshold: 3,
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intOrString(8080)},
					},
					PeriodSeconds:    10,
					TimeoutSeconds:   3,
					FailureThreshold: 3,
				},
			},
		},
	}

	pw := auditWorkloadProbes("Deployment", &metav1.ObjectMeta{Name: "good", Namespace: "default"}, spec)

	// Should have no critical or warning findings
	for _, c := range pw.Containers {
		for _, f := range c.Findings {
			if f.Severity == "critical" || f.Severity == "warning" {
				t.Errorf("Expected no critical/warning findings, got [%s] %s: %s", f.Severity, f.Check, f.Message)
			}
		}
	}

	if pw.RiskScore > 5 {
		t.Errorf("Expected low risk score for well-configured probes, got %d", pw.RiskScore)
	}
}

func TestGenerateProbeRecommendations(t *testing.T) {
	result := ProbeAuditResult{
		Summary: ProbeAuditSummary{
			MissingReadiness: 3,
			MissingLiveness:  2,
			AggressiveProbes: 1,
			Score:            40,
		},
	}

	recs := generateProbeRecommendations(result)

	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundReadiness := false
	foundScore := false
	for _, r := range recs {
		if containsSubstr(r, "readiness") {
			foundReadiness = true
		}
		if containsSubstr(r, "effectiveness score") {
			foundScore = true
		}
	}
	if !foundReadiness {
		t.Error("Expected recommendation about missing readiness probes")
	}
	if !foundScore {
		t.Error("Expected recommendation about score")
	}
}

func TestGenerateProbeRecommendationsClean(t *testing.T) {
	result := ProbeAuditResult{
		Summary: ProbeAuditSummary{
			Score: 100,
		},
	}

	recs := generateProbeRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for perfect cluster, got %d", len(recs))
	}
}

func TestAggregateFindings(t *testing.T) {
	m := make(map[string]*ProbeFindingAgg)

	pw := ProbeWorkload{
		Containers: []ProbeContainer{
			{
				Findings: []ProbeIssue{
					{Check: "missing-liveness", Severity: "warning"},
					{Check: "missing-readiness", Severity: "warning"},
					{Check: "aggressive-probe", Severity: "warning"},
				},
			},
			{
				Findings: []ProbeIssue{
					{Check: "missing-liveness", Severity: "warning"},
				},
			},
		},
	}

	aggregateFindings(m, pw)

	if m["missing-liveness"].Count != 2 {
		t.Errorf("Expected missing-liveness count 2, got %d", m["missing-liveness"].Count)
	}
	if m["missing-readiness"].Count != 1 {
		t.Errorf("Expected missing-readiness count 1, got %d", m["missing-readiness"].Count)
	}
}
