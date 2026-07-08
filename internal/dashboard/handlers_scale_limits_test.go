package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScLimMaxPodsOnNode(t *testing.T) {
	nodes := []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}}
	pods := []corev1.Pod{
		{Spec: corev1.PodSpec{NodeName: "node-1"}},
		{Spec: corev1.PodSpec{NodeName: "node-1"}},
		{Spec: corev1.PodSpec{NodeName: "node-2"}},
	}
	if max := sclimMaxPodsOnNode(nodes, pods); max != 2 {
		t.Errorf("Expected 2, got %d", max)
	}
}

func TestScLimPctStatus(t *testing.T) {
	if sclimPctStatus(85) != "critical" {
		t.Error("Expected critical for >=80%")
	}
	if sclimPctStatus(65) != "warning" {
		t.Error("Expected warning for >=60%")
	}
	if sclimPctStatus(30) != "safe" {
		t.Error("Expected safe for <60%")
	}
}

func TestScLimMakeLimit(t *testing.T) {
	l := sclimMakeLimit("Pods", 120, 150, "test")
	if l.Percent != 80 {
		t.Errorf("Expected 80%%, got %.1f", l.Percent)
	}
	if l.Status != "critical" {
		t.Error("Expected critical")
	}

	// No hard limit
	l = sclimMakeLimit("ConfigMaps", 500, 0, "no limit")
	if l.Status != "info" {
		t.Error("Expected info for no limit")
	}
}

func TestScLimScore(t *testing.T) {
	limits := []ScLimEntry{
		{Status: "critical"}, // -20
		{Status: "warning"},  // -10
		{Status: "safe"},
	}
	if score := sclimScore(limits); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	limits = []ScLimEntry{{Status: "safe"}}
	if score := sclimScore(limits); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}
}

func TestScLimGenRecs(t *testing.T) {
	s := ScLimSummary{
		NodeCount:      5,
		PodCount:       400,
		TotalCapacity:  550,
		UtilizationPct: 72,
		ConfigMapCount: 1500,
		SecretCount:    1200,
		ScaleScore:     50,
	}
	limits := []ScLimEntry{
		{Name: "Pods per node", Percent: 75, Status: "warning", Current: 82, Maximum: 110},
	}

	recs := sclimGenRecs(s, limits)
	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundUtil := false
	foundCM := false
	for _, r := range recs {
		if strContains(r, "utilization") {
			foundUtil = true
		}
		if strContains(r, "ConfigMaps") {
			foundCM = true
		}
	}
	if !foundUtil {
		t.Error("Expected recommendation about capacity utilization")
	}
	if !foundCM {
		t.Error("Expected recommendation about ConfigMaps")
	}
}

func TestScLimGenRecsClean(t *testing.T) {
	s := ScLimSummary{NodeCount: 3, PodCount: 50, TotalCapacity: 330, UtilizationPct: 15}
	recs := sclimGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}
