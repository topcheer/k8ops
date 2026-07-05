package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestVSAssessHostPath(t *testing.T) {
	// Docker socket - RW
	level, reason := vsAssessHostPath("/var/run/docker.sock", false)
	if level != "critical" {
		t.Errorf("Expected critical for docker.sock RW, got %s", level)
	}
	if reason == "" {
		t.Error("Expected non-empty reason")
	}

	// Docker socket - RO
	level, _ = vsAssessHostPath("/var/run/docker.sock", true)
	if level != "high" {
		t.Errorf("Expected high for docker.sock RO, got %s", level)
	}

	// Root path
	level, _ = vsAssessHostPath("/", false)
	if level != "critical" {
		t.Errorf("Expected critical for root RW, got %s", level)
	}

	// Kubernetes path
	level, _ = vsAssessHostPath("/etc/kubernetes", false)
	if level != "critical" {
		t.Errorf("Expected critical for kubernetes path, got %s", level)
	}

	// Generic hostPath RW
	level, _ = vsAssessHostPath("/data/app", false)
	if level != "medium" {
		t.Errorf("Expected medium for generic RW, got %s", level)
	}

	// Generic hostPath RO
	level, _ = vsAssessHostPath("/data/app", true)
	if level != "low" {
		t.Errorf("Expected low for generic RO, got %s", level)
	}
}

func TestVSVolumeType(t *testing.T) {
	vol := corev1.Volume{Name: "vol1", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}}}
	if vsVolumeType(vol) != "hostPath" {
		t.Error("Expected hostPath")
	}

	vol = corev1.Volume{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"}}}
	if vsVolumeType(vol) != "secret" {
		t.Error("Expected secret")
	}

	vol = corev1.Volume{VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}}}
	if vsVolumeType(vol) != "configMap" {
		t.Error("Expected configMap")
	}

	vol = corev1.Volume{VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
	if vsVolumeType(vol) != "emptyDir" {
		t.Error("Expected emptyDir")
	}
}

func TestFindVolume(t *testing.T) {
	volumes := []corev1.Volume{
		{Name: "vol1", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}}},
		{Name: "vol2", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}

	vol := findVolume(volumes, "vol1")
	if vol == nil || vol.Name != "vol1" {
		t.Error("Expected to find vol1")
	}

	vol = findVolume(volumes, "missing")
	if vol != nil {
		t.Error("Expected nil for missing volume")
	}
}

func TestVSScore(t *testing.T) {
	// No pods
	if score := vsScore(VolSecSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// Clean
	s := VolSecSummary{TotalPods: 10}
	if score := vsScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With issues
	s = VolSecSummary{
		TotalPods:          10,
		CriticalMounts:     2, // -24
		PodsWithPrivileged: 1, // -15
		PodsWithHostNet:    2, // -10
		PodsWithHostPID:    1, // -5
		ReadWriteHostPath:  3, // -12
	}
	// 100 - 24 - 15 - 10 - 5 - 12 = 34
	if score := vsScore(s); score != 34 {
		t.Errorf("Expected 34, got %d", score)
	}

	// Floor at 0
	s = VolSecSummary{TotalPods: 5, CriticalMounts: 20}
	if score := vsScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestVSRecs(t *testing.T) {
	s := VolSecSummary{
		TotalPods:          10,
		CriticalMounts:     3,
		PodsWithPrivileged: 1,
		ReadWriteHostPath:  2,
		PodsWithHostNet:    1,
		PodsWithHostPID:    1,
		SecurityScore:      30,
	}
	dangerous := []VolSecEntry{
		{Namespace: "default", PodName: "app1", MountPath: "/var/run/docker.sock"},
	}

	recs := vsRecs(s, dangerous)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundPriv := false
	foundCritical := false
	foundRW := false
	for _, r := range recs {
		if strContains(r, "privileged") {
			foundPriv = true
		}
		if strContains(r, "critical volume mount") {
			foundCritical = true
		}
		if strContains(r, "read-write") {
			foundRW = true
		}
	}
	if !foundPriv {
		t.Error("Expected recommendation about privileged containers")
	}
	if !foundCritical {
		t.Error("Expected recommendation about critical mounts")
	}
	if !foundRW {
		t.Error("Expected recommendation about read-write hostPath")
	}
}

func TestVSRecsClean(t *testing.T) {
	s := VolSecSummary{TotalPods: 10}
	recs := vsRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestVSRiskRank(t *testing.T) {
	if vsRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if vsRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if vsRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if vsRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestVSIssueRank(t *testing.T) {
	if vsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if vsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}

func TestVSGetOrCreateNS(t *testing.T) {
	m := make(map[string]*VolSecNSStat)
	e1 := vsGetOrCreateNS(m, "default")
	e1.PodCount = 5

	e2 := vsGetOrCreateNS(m, "default")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.PodCount)
	}

	e3 := vsGetOrCreateNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestVSNSRisk(t *testing.T) {
	if level := vsNSRisk(&VolSecNSStat{CriticalCount: 1}); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}
	if level := vsNSRisk(&VolSecNSStat{PrivilegedPods: 1}); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := vsNSRisk(&VolSecNSStat{HostPathPods: 1}); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}
	if level := vsNSRisk(&VolSecNSStat{}); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}
