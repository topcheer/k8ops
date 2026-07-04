package dashboard

import (
	"testing"
)

func TestIsSensitiveHostPath(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"/etc", true},
		{"/etc/kubernetes", true},
		{"/var/run", true},
		{"/root", true},
		{"/proc", true},
		{"/sys", true},
		{"/data", false},
		{"/tmp", false},
		{"/home/user", false},
		{"/boot", true},
		{"/usr/lib/go", true},
		{"/var/lib/data", false},
	}

	for _, tt := range tests {
		got := isSensitiveHostPath(tt.path)
		if got != tt.expect {
			t.Errorf("isSensitiveHostPath(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestAssessContainerRisk(t *testing.T) {
	// Low risk — no issues
	entry := ContainerSecEntry{
		Containers: []ContainerInfo{
			{Name: "app", RunAsNonRoot: true, ReadOnlyRootFS: true, HasSecurityContext: true},
		},
	}
	if level := assessContainerRisk(entry); level != "low" {
		t.Errorf("Expected low for clean container, got %s", level)
	}

	// Critical — privileged + hostPID + sensitive hostPath
	entry = ContainerSecEntry{
		Containers: []ContainerInfo{
			{Name: "app", Privileged: true}, // +30
		},
		HostPID:   true,                    // +15
		HostPaths: []string{"/etc/passwd"}, // +15
	}
	if level := assessContainerRisk(entry); level != "critical" {
		t.Errorf("Expected critical for privileged+hostPID+hostPath, got %s", level)
	}

	// High — runAsRoot + dangerous cap
	entry = ContainerSecEntry{
		Containers: []ContainerInfo{
			{Name: "app", RunAsRoot: true, DangerousCaps: []string{"NET_ADMIN"}}, // +10 + 10
		},
	}
	if level := assessContainerRisk(entry); level != "high" {
		t.Errorf("Expected high for root+dangerous cap, got %s", level)
	}

	// Medium — hostNetwork only
	entry = ContainerSecEntry{
		HostNetwork: true, // +10
		Containers: []ContainerInfo{
			{Name: "app", RunAsNonRoot: true, ReadOnlyRootFS: true, HasSecurityContext: true},
		},
	}
	if level := assessContainerRisk(entry); level != "medium" {
		t.Errorf("Expected medium for hostNetwork only, got %s", level)
	}
}

func TestCalculateContainerSecScore(t *testing.T) {
	// Perfect
	perfect := ContainerSecSummary{
		TotalContainers: 10,
	}
	if score := calculateContainerSecScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := ContainerSecSummary{
		TotalContainers:   20,
		Privileged:        2, // -16
		RunAsRoot:         5, // -10
		DangerousCaps:     3, // -9
		HasHostNetwork:    2, // -6
		NoSecurityContext: 5, // -5
	}
	// 100 - 16 - 10 - 9 - 6 - 5 = 54
	score := calculateContainerSecScore(withIssues)
	if score != 54 {
		t.Errorf("Expected 54, got %d", score)
	}

	// Floor at 0
	terrible := ContainerSecSummary{
		TotalContainers: 10,
		Privileged:      20, // -160
	}
	if score := calculateContainerSecScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty
	empty := ContainerSecSummary{}
	if score := calculateContainerSecScore(empty); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateContainerSecRecs(t *testing.T) {
	s := ContainerSecSummary{
		Privileged:        1,
		PrivEscalation:    2,
		RunAsRoot:         5,
		DangerousCaps:     3,
		HasHostPID:        1,
		HasHostNetwork:    2,
		HostPathMounts:    4,
		NoSecurityContext: 3,
		SecurityScore:     35,
	}

	recs := generateContainerSecRecs(s)

	if len(recs) < 8 {
		t.Errorf("Expected at least 8 recommendations, got %d", len(recs))
	}

	foundPriv := false
	foundRoot := false
	foundHostPath := false
	for _, r := range recs {
		if containsSubstr(r, "privileged") {
			foundPriv = true
		}
		if containsSubstr(r, "run as root") || containsSubstr(r, "runAsNonRoot") {
			foundRoot = true
		}
		if containsSubstr(r, "hostPath") {
			foundHostPath = true
		}
	}
	if !foundPriv {
		t.Error("Expected recommendation about privileged containers")
	}
	if !foundRoot {
		t.Error("Expected recommendation about running as root")
	}
	if !foundHostPath {
		t.Error("Expected recommendation about hostPath mounts")
	}
}

func TestGenerateContainerSecRecsClean(t *testing.T) {
	s := ContainerSecSummary{
		TotalContainers: 10,
		SecurityScore:   100,
	}

	recs := generateContainerSecRecs(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestGetOrCreateContainerSecNs(t *testing.T) {
	m := make(map[string]*ContainerSecNs)

	e1 := getOrCreateContainerSecNs(m, "default")
	e1.PodCount = 5

	e2 := getOrCreateContainerSecNs(m, "default")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry with PodCount=5, got %d", e2.PodCount)
	}

	e3 := getOrCreateContainerSecNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected kube-system, got %s", e3.Namespace)
	}
}

func TestContainerRiskRank(t *testing.T) {
	if containerRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if containerRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if containerRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if containerRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
