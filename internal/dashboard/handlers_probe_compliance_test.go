package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPCProbeToDetail(t *testing.T) {
	port := intstr.FromInt(8080)
	p := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: port},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		TimeoutSeconds:      2,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}
	d := pcProbeToDetail(p)
	if d.Type != "httpGet" {
		t.Errorf("Expected httpGet, got %s", d.Type)
	}
	if d.Path != "/healthz" {
		t.Errorf("Expected /healthz, got %s", d.Path)
	}
	if d.Port != 8080 {
		t.Errorf("Expected 8080, got %d", d.Port)
	}

	p = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: port},
		},
	}
	d = pcProbeToDetail(p)
	if d.Type != "tcpSocket" {
		t.Errorf("Expected tcpSocket, got %s", d.Type)
	}

	p = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: []string{"/bin/healthcheck"}},
		},
	}
	d = pcProbeToDetail(p)
	if d.Type != "exec" {
		t.Errorf("Expected exec, got %s", d.Type)
	}
}

func TestPCCheckMisconfigured(t *testing.T) {
	c := corev1.Container{
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 180,
			PeriodSeconds:       70,
			FailureThreshold:    15,
			TimeoutSeconds:      15,
			SuccessThreshold:    2,
		},
	}
	issues := pcCheckMisconfigured(c)
	if len(issues) < 5 {
		t.Errorf("Expected at least 5 issues, got %d", len(issues))
	}

	c = corev1.Container{
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			FailureThreshold:    3,
			TimeoutSeconds:      1,
			SuccessThreshold:    1,
		},
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      3,
		},
	}
	issues = pcCheckMisconfigured(c)
	if len(issues) != 0 {
		t.Errorf("Expected 0 issues for clean config, got %d", len(issues))
	}
}

func TestPCAssessRisk(t *testing.T) {
	entry := PCEntry{HasLiveness: false, HasReadiness: false}
	if level := pcAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	entry = PCEntry{HasLiveness: true, HasReadiness: false}
	if level := pcAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	entry = PCEntry{HasLiveness: false, HasReadiness: true}
	if level := pcAssessRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	entry = PCEntry{HasLiveness: true, HasReadiness: true, Issues: []string{"a", "b", "c"}}
	if level := pcAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}

	entry = PCEntry{HasLiveness: true, HasReadiness: true}
	if level := pcAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestPCScore(t *testing.T) {
	if score := pcScore(PCSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := PCSummary{TotalContainers: 10, HasLiveness: 10, HasReadiness: 10}
	if score := pcScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s = PCSummary{
		TotalContainers:    10,
		NoProbeAtAll:       2,
		MissingReadiness:   3,
		MissingLiveness:    2,
		MisconfiguredCount: 2,
	}
	if score := pcScore(s); score != 8 {
		t.Errorf("Expected 8, got %d", score)
	}
}

func TestPCGenRecs(t *testing.T) {
	s := PCSummary{
		TotalContainers:    10,
		NoProbeAtAll:       2,
		MissingReadiness:   3,
		MissingLiveness:    2,
		TCPSocketProbes:    1,
		HasStartup:         0,
		HasLiveness:        8,
		MisconfiguredCount: 1,
		HealthScore:        25,
	}

	recs := pcGenRecs(s, nil, nil)
	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}
}

func TestPCGenRecsClean(t *testing.T) {
	s := PCSummary{TotalContainers: 10, HasLiveness: 10, HasReadiness: 10, HasStartup: 5}
	recs := pcGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestPCRiskRank(t *testing.T) {
	if pcRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if pcRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestPCIssueRank(t *testing.T) {
	if pcIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if pcIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
