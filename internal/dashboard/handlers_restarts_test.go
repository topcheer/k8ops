package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func makeRestartPod(name, ns string, restartCount int32, lastTermReason string, waitingReason string, age time.Duration) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-age)},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Image:        "nginx:latest",
					RestartCount: restartCount,
				},
			},
		},
	}

	cs := &pod.Status.ContainerStatuses[0]

	if lastTermReason != "" {
		cs.LastTerminationState.Terminated = &corev1.ContainerStateTerminated{
			Reason:   lastTermReason,
			ExitCode: 137,
		}
	}

	if waitingReason != "" {
		cs.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  waitingReason,
				Message: "container is waiting",
			},
		}
	} else {
		cs.State = corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{},
		}
	}

	return pod
}

// --- classifyRestartPattern tests ---

func TestClassifyRestartPattern_None(t *testing.T) {
	p := classifyRestartPattern(0, time.Now(), nil, nil)
	if p != RestartPatternNone {
		t.Errorf("got %s, want none", p)
	}
}

func TestClassifyRestartPattern_CrashLoop(t *testing.T) {
	// 5+ restarts within 1 hour
	p := classifyRestartPattern(6, time.Now().Add(-30*time.Minute), nil, nil)
	if p != RestartPatternCrashLoop {
		t.Errorf("got %s, want crash-loop", p)
	}
}

func TestClassifyRestartPattern_CrashLoopByWaiting(t *testing.T) {
	p := classifyRestartPattern(2, time.Now(), nil, &WaitingDetail{Reason: "CrashLoopBackOff"})
	if p != RestartPatternCrashLoop {
		t.Errorf("got %s, want crash-loop", p)
	}
}

func TestClassifyRestartPattern_PostDeploy(t *testing.T) {
	// 1-3 restarts within 30 min
	p := classifyRestartPattern(2, time.Now().Add(-10*time.Minute), nil, nil)
	if p != RestartPatternPostDeploy {
		t.Errorf("got %s, want post-deploy", p)
	}
}

func TestClassifyRestartPattern_Occasional(t *testing.T) {
	// Few restarts over a long time
	p := classifyRestartPattern(2, time.Now().Add(-48*time.Hour), nil, nil)
	if p != RestartPatternOccasional {
		t.Errorf("got %s, want occasional", p)
	}
}

// --- classifyRestartRisk tests ---

func TestClassifyRestartRisk_CrashLoopBackOff(t *testing.T) {
	r := classifyRestartRisk(RestartPatternCrashLoop, 5, nil, &WaitingDetail{Reason: "CrashLoopBackOff"})
	if r != "critical" {
		t.Errorf("got %s, want critical", r)
	}
}

func TestClassifyRestartRisk_ImagePullBackOff(t *testing.T) {
	r := classifyRestartRisk(RestartPatternNone, 0, nil, &WaitingDetail{Reason: "ImagePullBackOff"})
	if r != "high" {
		t.Errorf("got %s, want high", r)
	}
}

func TestClassifyRestartRisk_CrashLoopHighRestarts(t *testing.T) {
	r := classifyRestartRisk(RestartPatternCrashLoop, 12, &TerminationDetail{Reason: "OOMKilled"}, nil)
	if r != "critical" {
		t.Errorf("got %s, want critical (>=10 restarts)", r)
	}
}

func TestClassifyRestartRisk_CrashLoopModerate(t *testing.T) {
	r := classifyRestartRisk(RestartPatternCrashLoop, 6, nil, nil)
	if r != "high" {
		t.Errorf("got %s, want high", r)
	}
}

func TestClassifyRestartRisk_Occasional(t *testing.T) {
	r := classifyRestartRisk(RestartPatternOccasional, 3, nil, nil)
	if r != "medium" {
		t.Errorf("got %s, want medium", r)
	}
}

func TestClassifyRestartRisk_Healthy(t *testing.T) {
	r := classifyRestartRisk(RestartPatternNone, 0, nil, nil)
	if r != "healthy" {
		t.Errorf("got %s, want healthy", r)
	}
}

// --- buildPodRestartInfo tests ---

func TestBuildPodRestartInfo_HealthyPod(t *testing.T) {
	pod := makeRestartPod("web-1", "default", 0, "", "", 24*time.Hour)
	info := buildPodRestartInfo(&pod)

	if info.TotalRestarts != 0 {
		t.Errorf("TotalRestarts = %d, want 0", info.TotalRestarts)
	}
	if info.OverallRisk != "healthy" {
		t.Errorf("OverallRisk = %s, want healthy", info.OverallRisk)
	}
}

func TestBuildPodRestartInfo_OOMKilled(t *testing.T) {
	pod := makeRestartPod("worker-1", "prod", 5, "OOMKilled", "", 2*time.Hour)
	info := buildPodRestartInfo(&pod)

	if info.TotalRestarts != 5 {
		t.Errorf("TotalRestarts = %d, want 5", info.TotalRestarts)
	}
	if info.OverallRisk != "high" {
		t.Errorf("OverallRisk = %s, want high", info.OverallRisk)
	}
	if len(info.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(info.Containers))
	}
	if info.Containers[0].LastTermination == nil {
		t.Fatal("expected LastTermination")
	}
	if info.Containers[0].LastTermination.Reason != "OOMKilled" {
		t.Errorf("termination reason = %s, want OOMKilled", info.Containers[0].LastTermination.Reason)
	}
}

func TestBuildPodRestartInfo_CrashLoopBackOff(t *testing.T) {
	pod := makeRestartPod("api-1", "default", 8, "Error", "CrashLoopBackOff", 30*time.Minute)
	info := buildPodRestartInfo(&pod)

	if info.OverallRisk != "critical" {
		t.Errorf("OverallRisk = %s, want critical", info.OverallRisk)
	}
	if info.OverallPattern != RestartPatternCrashLoop {
		t.Errorf("OverallPattern = %s, want crash-loop", info.OverallPattern)
	}
}

func TestBuildPodRestartInfo_ImagePullBackOff(t *testing.T) {
	pod := makeRestartPod("app-1", "default", 0, "", "ImagePullBackOff", 5*time.Minute)
	info := buildPodRestartInfo(&pod)

	if info.OverallRisk != "high" {
		t.Errorf("OverallRisk = %s, want high", info.OverallRisk)
	}
}

// --- diagnoseRestarts tests ---

func TestDiagnoseRestarts_EmptyCluster(t *testing.T) {
	result := diagnoseRestarts(nil)
	if result.Summary.TotalPods != 0 {
		t.Errorf("TotalPods = %d, want 0", result.Summary.TotalPods)
	}
}

func TestDiagnoseRestarts_MixedPods(t *testing.T) {
	pods := []corev1.Pod{
		makeRestartPod("healthy-1", "default", 0, "", "", 48*time.Hour),
		makeRestartPod("crash-1", "default", 10, "OOMKilled", "CrashLoopBackOff", 30*time.Minute),
		makeRestartPod("occasional-1", "prod", 2, "Error", "", 72*time.Hour),
	}

	result := diagnoseRestarts(pods)

	if result.Summary.TotalPods != 3 {
		t.Errorf("TotalPods = %d, want 3", result.Summary.TotalPods)
	}
	if result.Summary.PodsWithRestarts != 2 {
		t.Errorf("PodsWithRestarts = %d, want 2", result.Summary.PodsWithRestarts)
	}
	if result.Summary.CrashLoops != 1 {
		t.Errorf("CrashLoops = %d, want 1", result.Summary.CrashLoops)
	}
	if result.Summary.HighRisk != 1 {
		t.Errorf("HighRisk = %d, want 1", result.Summary.HighRisk)
	}
	if result.Summary.OOMKills != 1 {
		t.Errorf("OOMKills = %d, want 1", result.Summary.OOMKills)
	}
	if result.Summary.WaitingPods != 1 {
		t.Errorf("WaitingPods = %d, want 1", result.Summary.WaitingPods)
	}
}

func TestDiagnoseRestarts_SortOrder(t *testing.T) {
	pods := []corev1.Pod{
		makeRestartPod("low-risk", "default", 1, "", "", 48*time.Hour),   // occasional, low
		makeRestartPod("critical", "default", 10, "Error", "CrashLoopBackOff", 30*time.Minute),
		makeRestartPod("medium", "default", 3, "Error", "", 72*time.Hour), // occasional, medium
	}

	result := diagnoseRestarts(pods)

	if len(result.Pods) < 3 {
		t.Fatalf("expected 3 pods with restarts, got %d", len(result.Pods))
	}
	// Critical should be first
	if result.Pods[0].Name != "critical" {
		t.Errorf("first pod = %s, want critical (sorted by risk)", result.Pods[0].Name)
	}
}

func TestDiagnoseRestarts_HasRestartDiagnosisHint(t *testing.T) {
	pods := []corev1.Pod{
		makeRestartPod("oom-1", "default", 5, "OOMKilled", "", 2*time.Hour),
	}

	result := diagnoseRestarts(pods)

	if len(result.Pods) != 1 {
		t.Fatal("expected 1 pod")
	}
	hint := RestartDiagnosisHint(&result.Pods[0])
	if !strings.Contains(hint, "OOMKilled") {
		t.Errorf("hint should mention OOMKilled: %s", hint)
	}
}

// --- Handler integration ---

func TestHandleRestartDiagnosis_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/diagnostics/restarts", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRestartDiagnosis(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHandleRestartDiagnosis_WithPods(t *testing.T) {
	pod := makeRestartPod("crash-1", "default", 5, "OOMKilled", "CrashLoopBackOff", 30*time.Minute)
	clientset := k8sfake.NewSimpleClientset(&pod)

	req := newReqWithClients(http.MethodGet, "/api/diagnostics/restarts", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRestartDiagnosis(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "\"summary\"") {
		t.Error("response missing summary")
	}
	if !strings.Contains(body, "\"pods\"") {
		t.Error("response missing pods array")
	}
	if !strings.Contains(body, "crash-loop") {
		t.Error("response should contain crash-loop pattern")
	}
}

// --- Utility tests ---

func TestIsCrashLoopReason(t *testing.T) {
	if !isCrashLoopReason("CrashLoopBackOff") {
		t.Error("CrashLoopBackOff should be true")
	}
	if isCrashLoopReason("ImagePullBackOff") {
		t.Error("ImagePullBackOff should be false")
	}
}

func TestRiskScore(t *testing.T) {
	if riskScore("critical") >= riskScore("high") {
		t.Error("critical should score lower than high")
	}
}

func TestPatternPriority(t *testing.T) {
	if patternPriority(RestartPatternCrashLoop) >= patternPriority(RestartPatternOccasional) {
		t.Error("crash-loop should have higher priority than occasional")
	}
}

func TestFormatTimePtr_Nil(t *testing.T) {
	if formatTimePtr(nil) != "" {
		t.Error("nil time should return empty string")
	}
}

func TestFormatTimePtr_Valid(t *testing.T) {
	now := metav1.Now()
	result := formatTimePtr(&now)
	if result == "" {
		t.Error("valid time should return non-empty string")
	}
}
