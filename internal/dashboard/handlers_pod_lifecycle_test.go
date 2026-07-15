package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestLifecycleEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-lifecycle", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handlePodLifecycle(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result LifecycleResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected 100, got %d", result.HealthScore)
	}
}

func TestLifecycleRunningPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-1", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			},
			Spec:   corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-2", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-48 * time.Hour)},
			},
			Spec:   corev1.PodSpec{NodeName: "node-2"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-lifecycle", clientset)
	w := httptest.NewRecorder()
	s.handlePodLifecycle(w, req)
	var result LifecycleResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.Running != 2 {
		t.Errorf("expected 2 running, got %d", result.Summary.Running)
	}
	if result.Summary.OldestPodAgeHr < 47 {
		t.Errorf("expected oldest >=47hr, got %.1f", result.Summary.OldestPodAgeHr)
	}
}

func TestLifecycleStuckPod(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stuck-pod", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-45 * time.Minute)},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "Image not found"},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-lifecycle", clientset)
	w := httptest.NewRecorder()
	s.handlePodLifecycle(w, req)
	var result LifecycleResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.StuckPods) == 0 {
		t.Fatal("expected stuck pods")
	}
	sp := result.StuckPods[0]
	if sp.Name != "stuck-pod" {
		t.Errorf("expected stuck-pod, got %s", sp.Name)
	}
	if sp.Severity != "critical" {
		t.Errorf("expected critical, got %s", sp.Severity)
	}
	if !strings.Contains(sp.Reason, "ImagePullBackOff") {
		t.Errorf("expected ImagePullBackOff reason, got %s", sp.Reason)
	}
}

func TestLifecycleFailedPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "failed-job", Namespace: "ci",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-lifecycle", clientset)
	w := httptest.NewRecorder()
	s.handlePodLifecycle(w, req)
	var result LifecycleResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Summary.Failed)
	}
}

func TestLifecycleTerminating(t *testing.T) {
	now := metav1.NewTime(time.Now())
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dying-pod", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Hour)},
				DeletionTimestamp: &now,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/pod-lifecycle", clientset)
	w := httptest.NewRecorder()
	s.handlePodLifecycle(w, req)
	var result LifecycleResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.Terminating != 1 {
		t.Errorf("expected 1 terminating, got %d", result.Summary.Terminating)
	}
}

func TestLifecycleRecommendations(t *testing.T) {
	result := LifecycleResult{
		Summary: LifecycleSummary{
			StuckCount: 3, Pending: 5, Failed: 2, AvgPendingMin: 8.5, MaxPendingMin: 25,
		},
		DwellTime: DwellTimeStats{PendingP90: 7.5, TerminatingP90: 3.2},
	}
	recs := generateLifecycleRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundStuck := false
	foundPending := false
	for _, r := range recs {
		l := strings.ToLower(r)
		if strings.Contains(l, "stuck") {
			foundStuck = true
		}
		if strings.Contains(l, "pending") {
			foundPending = true
		}
	}
	if !foundStuck {
		t.Error("expected stuck recommendation")
	}
	if !foundPending {
		t.Error("expected pending recommendation")
	}
}

func TestLifecyclePercentileEmpty(t *testing.T) {
	if p := lifecyclePercentile(nil, 50); p != 0 {
		t.Errorf("expected 0 for empty, got %f", p)
	}
}

func TestLifecyclePercentile(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := lifecyclePercentile(data, 50)
	if p50 < 4 || p50 > 6 {
		t.Errorf("P50 out of range: %f", p50)
	}
	p90 := lifecyclePercentile(data, 90)
	if p90 < 8 {
		t.Errorf("P90 too low: %f", p90)
	}
}
