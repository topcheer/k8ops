package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScoreToGrade(t *testing.T) {
	tests := []struct {
		score  int
		expect string
	}{
		{100, "A"},
		{90, "A"},
		{89, "B"},
		{80, "B"},
		{79, "C"},
		{70, "C"},
		{69, "D"},
		{60, "D"},
		{59, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		got := scoreToGrade(tt.score)
		if got != tt.expect {
			t.Errorf("scoreToGrade(%d) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestScoreToStatus(t *testing.T) {
	tests := []struct {
		score  int
		expect string
	}{
		{100, "healthy"},
		{70, "healthy"},
		{69, "warning"},
		{40, "warning"},
		{39, "critical"},
		{0, "critical"},
	}

	for _, tt := range tests {
		got := scoreToStatus(tt.score)
		if got != tt.expect {
			t.Errorf("scoreToStatus(%d) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	if severityRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if severityRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if severityRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}

func TestAssessNodeHealth(t *testing.T) {
	// All ready
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "n2"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		},
	}
	cat := assessNodeHealth(nodes)
	if cat.Score != 100 {
		t.Errorf("Expected 100 for all ready, got %d", cat.Score)
	}
	if cat.Status != "healthy" {
		t.Errorf("Expected healthy, got %s", cat.Status)
	}
	if cat.IssueCount != 0 {
		t.Errorf("Expected 0 issues, got %d", cat.IssueCount)
	}

	// One not ready
	nodes.Items[1].Status.Conditions[0].Status = corev1.ConditionFalse
	cat = assessNodeHealth(nodes)
	if cat.Score != 50 {
		t.Errorf("Expected 50 for 1/2 not ready, got %d", cat.Score)
	}
	if cat.Status != "warning" {
		t.Errorf("Expected warning, got %s", cat.Status)
	}

	// Empty
	cat = assessNodeHealth(&corev1.NodeList{})
	if cat.Score != 0 {
		t.Errorf("Expected 0 for empty, got %d", cat.Score)
	}
	if cat.Status != "critical" {
		t.Errorf("Expected critical for empty, got %s", cat.Status)
	}
}

func TestAssessPodHealth(t *testing.T) {
	// All running
	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{Status: corev1.PodStatus{Phase: corev1.PodRunning}},
			{Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		},
	}
	cat, summary := assessPodHealth(pods)
	if cat.Score != 100 {
		t.Errorf("Expected 100 for all running, got %d", cat.Score)
	}
	if summary.TotalPods != 2 {
		t.Errorf("Expected 2 pods, got %d", summary.TotalPods)
	}
	if summary.RunningPods != 2 {
		t.Errorf("Expected 2 running, got %d", summary.RunningPods)
	}

	// With crash loop
	pods.Items[0].Status.ContainerStatuses = []corev1.ContainerStatus{
		{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
	}
	cat, _ = assessPodHealth(pods)
	if cat.Score >= 50 {
		t.Errorf("Expected score < 50 for crash loop, got %d", cat.Score)
	}
	if cat.Status != "critical" {
		t.Errorf("Expected critical for crash loop, got %s", cat.Status)
	}
}

func TestAssessEventHealth(t *testing.T) {
	now := time.Now()

	// No events
	cat := assessEventHealth(&corev1.EventList{}, now)
	if cat.Score != 100 {
		t.Errorf("Expected 100 for no events, got %d", cat.Score)
	}

	// Moderate events
	events := &corev1.EventList{
		Items: make([]corev1.Event, 15),
	}
	for i := range events.Items {
		events.Items[i].LastTimestamp = metav1.Time{Time: now.Add(-30 * time.Minute)}
	}
	cat = assessEventHealth(events, now)
	if cat.Score != 65 {
		t.Errorf("Expected 65 for 15 events, got %d", cat.Score)
	}
	if cat.Status != "warning" {
		t.Errorf("Expected warning, got %s", cat.Status)
	}

	// Event storm
	events.Items = make([]corev1.Event, 80)
	for i := range events.Items {
		events.Items[i].LastTimestamp = metav1.Time{Time: now.Add(-30 * time.Minute)}
	}
	cat = assessEventHealth(events, now)
	if cat.Score != 15 {
		t.Errorf("Expected 15 for 80 events, got %d", cat.Score)
	}
	if cat.Status != "critical" {
		t.Errorf("Expected critical, got %s", cat.Status)
	}
}
