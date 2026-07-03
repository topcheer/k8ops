package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// ptrInt32Ptr is defined in handlers_coverage2_test.go

// --- Deployment rollout tests ---

func TestRollout_DeploymentComplete(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: "default", Generation: 2, Labels: map[string]string{"pod-template-hash": "abc123"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(3),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}}},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           3,
			ReadyReplicas:      3,
			UpdatedReplicas:    3,
			AvailableReplicas:  3,
			ObservedGeneration: 2,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(result.Workloads))
	}
	wr := result.Workloads[0]
	if wr.Status != RolloutComplete {
		t.Errorf("expected complete, got %s", wr.Status)
	}
	if wr.TemplateHash != "abc123" {
		t.Errorf("expected template hash abc123, got %s", wr.TemplateHash)
	}
	if len(wr.Images) != 1 || wr.Images[0] != "nginx:1.25" {
		t.Errorf("unexpected images: %v", wr.Images)
	}
	if result.Summary.Complete != 1 {
		t.Errorf("expected summary complete=1, got %d", result.Summary.Complete)
	}
}

func TestRollout_DeploymentInProgress(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "rolling-app", Namespace: "prod", Generation: 5},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(4),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           4,
			ReadyReplicas:      3,
			UpdatedReplicas:    2,
			AvailableReplicas:  3,
			ObservedGeneration: 5,
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutInProgress {
		t.Errorf("expected in-progress, got %s (issues: %v)", wr.Status, wr.Issues)
	}
	if len(wr.Issues) == 0 {
		t.Error("expected at least one issue message for in-progress deployment")
	}
}

func TestRollout_DeploymentFailed(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-app", Namespace: "default", Generation: 3},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(2),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:        2,
			ReadyReplicas:   0,
			UpdatedReplicas: 0,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "ProgressDeadlineExceeded",
					Message: "ReplicaSet timed out progressing",
				},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutFailed {
		t.Errorf("expected failed, got %s", wr.Status)
	}
	if len(wr.Issues) == 0 {
		t.Error("expected issues for failed deployment")
	}
}

func TestRollout_DeploymentDegraded(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "degraded-app", Namespace: "default", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(5),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           5,
			ReadyReplicas:      5,
			UpdatedReplicas:    5,
			AvailableReplicas:  3,
			ObservedGeneration: 1,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutDegraded {
		t.Errorf("expected degraded, got %s", wr.Status)
	}
}

func TestRollout_DeploymentPaused(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "paused-app", Namespace: "default", Generation: 1,
			Annotations: map[string]string{"deployment.kubernetes.io/paused": "true"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           3,
			ReadyReplicas:      3,
			UpdatedReplicas:    2,
			ObservedGeneration: 1,
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutPaused {
		t.Errorf("expected paused, got %s", wr.Status)
	}
}

func TestRollout_DeploymentScaledToZero(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "scaled-down", Namespace: "default", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(0),
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
		},
	}

	clientset := k8sfake.NewSimpleClientset(dep)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutScaledZero {
		t.Errorf("expected scaled-to-zero, got %s", wr.Status)
	}
}

func TestRollout_DeploymentStalledGenerationMismatch(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "stuck-app", Namespace: "default", Generation: 10},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           2,
			UpdatedReplicas:    1,
			ReadyReplicas:      2,
			ObservedGeneration: 8, // stale
		},
	}

	wr := analyzeDeploymentRollout(dep)
	if wr.Status != RolloutStalled {
		t.Errorf("expected stalled, got %s", wr.Status)
	}
}

func TestRollout_DeploymentReplicaFailure(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "fail-rs", Namespace: "default", Generation: 2},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32Ptr(3),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:           2,
			UpdatedReplicas:    2,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
			ObservedGeneration: 2,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentReplicaFailure,
					Status: corev1.ConditionTrue,
					Reason: "FailedCreate",
				},
			},
		},
	}

	wr := analyzeDeploymentRollout(dep)
	if wr.Status != RolloutDegraded {
		t.Errorf("expected degraded from ReplicaFailure, got %s", wr.Status)
	}
}

// --- StatefulSet rollout tests ---

func TestRollout_StatefulSetComplete(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "data"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       ptrInt32Ptr(3),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        3,
			ReadyReplicas:   3,
			UpdatedReplicas: 3,
			CurrentRevision: "db-v2",
			UpdateRevision:  "db-v2",
		},
	}

	clientset := k8sfake.NewSimpleClientset(sts)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutComplete {
		t.Errorf("expected complete, got %s", wr.Status)
	}
	if wr.Kind != "StatefulSet" {
		t.Errorf("expected StatefulSet, got %s", wr.Kind)
	}
}

func TestRollout_StatefulSetInProgress(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rolling-sts", Namespace: "data"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrInt32Ptr(5),
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        5,
			ReadyReplicas:   5,
			UpdatedReplicas: 2,
			CurrentRevision: "sts-v1",
			UpdateRevision:  "sts-v2",
		},
	}

	wr := analyzeStatefulSetRollout(sts)
	if wr.Status != RolloutInProgress {
		t.Errorf("expected in-progress, got %s", wr.Status)
	}
}

func TestRollout_StatefulSetDegraded(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "partial-sts", Namespace: "data"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrInt32Ptr(3),
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        3,
			ReadyReplicas:   1,
			UpdatedReplicas: 3,
			CurrentRevision: "sts-v1",
			UpdateRevision:  "sts-v1",
		},
	}

	wr := analyzeStatefulSetRollout(sts)
	if wr.Status != RolloutDegraded {
		t.Errorf("expected degraded, got %s", wr.Status)
	}
}

func TestRollout_StatefulSetOnDelete(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ondelete-sts", Namespace: "data"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       ptrInt32Ptr(3),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        3,
			ReadyReplicas:   3,
			UpdatedReplicas: 0,
			CurrentRevision: "sts-v1",
			UpdateRevision:  "sts-v2",
		},
	}

	wr := analyzeStatefulSetRollout(sts)
	if wr.Status != RolloutInProgress {
		t.Errorf("expected in-progress for OnDelete with pending update, got %s", wr.Status)
	}
}

// --- DaemonSet rollout tests ---

func TestRollout_DaemonSetComplete(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "log-agent", Namespace: "kube-system"},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            5,
			NumberAvailable:        5,
			UpdatedNumberScheduled: 5,
			NumberUnavailable:      0,
		},
	}

	clientset := k8sfake.NewSimpleClientset(ds)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	wr := result.Workloads[0]
	if wr.Status != RolloutComplete {
		t.Errorf("expected complete, got %s", wr.Status)
	}
	if wr.Kind != "DaemonSet" {
		t.Errorf("expected DaemonSet, got %s", wr.Kind)
	}
}

func TestRollout_DaemonSetInProgress(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "updating-ds", Namespace: "kube-system"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 10,
			CurrentNumberScheduled: 10,
			NumberReady:            8,
			UpdatedNumberScheduled: 5,
			NumberUnavailable:      2,
		},
	}

	wr := analyzeDaemonSetRollout(ds)
	if wr.Status != RolloutInProgress {
		t.Errorf("expected in-progress, got %s", wr.Status)
	}
}

func TestRollout_DaemonSetDegraded(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "degraded-ds", Namespace: "kube-system"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			CurrentNumberScheduled: 5,
			NumberReady:            3,
			UpdatedNumberScheduled: 5,
			NumberUnavailable:      2,
		},
	}

	wr := analyzeDaemonSetRollout(ds)
	if wr.Status != RolloutDegraded {
		t.Errorf("expected degraded, got %s", wr.Status)
	}
}

// --- Summary and filtering tests ---

func TestRollout_MixedWorkloadsSummary(t *testing.T) {
	objects := []runtime.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-dep", Namespace: "default", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32Ptr(2)},
			Status: appsv1.DeploymentStatus{
				Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2,
				ObservedGeneration: 1,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "failing-dep", Namespace: "default", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32Ptr(3)},
			Status: appsv1.DeploymentStatus{
				Replicas: 1, ReadyReplicas: 0, UpdatedReplicas: 0,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"},
				},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-sts", Namespace: "data"},
			Spec:       appsv1.StatefulSetSpec{Replicas: ptrInt32Ptr(1)},
			Status: appsv1.StatefulSetStatus{
				Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1,
				CurrentRevision: "v1", UpdateRevision: "v1",
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-ds", Namespace: "kube-system"},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3, NumberReady: 3, UpdatedNumberScheduled: 3,
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.Total != 4 {
		t.Errorf("expected total=4, got %d", result.Summary.Total)
	}
	if result.Summary.Deployments != 2 {
		t.Errorf("expected deployments=2, got %d", result.Summary.Deployments)
	}
	if result.Summary.StatefulSets != 1 {
		t.Errorf("expected statefulSets=1, got %d", result.Summary.StatefulSets)
	}
	if result.Summary.DaemonSets != 1 {
		t.Errorf("expected daemonSets=1, got %d", result.Summary.DaemonSets)
	}
	if result.Summary.Complete != 3 {
		t.Errorf("expected complete=3, got %d", result.Summary.Complete)
	}
	if result.Summary.Failed != 1 {
		t.Errorf("expected failed=1, got %d", result.Summary.Failed)
	}

	// Failed should sort first
	if result.Workloads[0].Status != RolloutFailed {
		t.Errorf("expected failed workload first, got %s: %s", result.Workloads[0].Status, result.Workloads[0].Name)
	}
}

func TestRollout_StatusFilter(t *testing.T) {
	objects := []runtime.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "ok-dep", Namespace: "default", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32Ptr(1)},
			Status: appsv1.DeploymentStatus{
				Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1,
				ObservedGeneration: 1,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-dep", Namespace: "default", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32Ptr(1)},
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"},
				},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout?status=failed", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	if len(result.Workloads) != 1 {
		t.Fatalf("expected 1 filtered workload, got %d", len(result.Workloads))
	}
	if result.Workloads[0].Name != "bad-dep" {
		t.Errorf("expected bad-dep, got %s", result.Workloads[0].Name)
	}
	// Summary should still reflect all workloads
	if result.Summary.Total != 2 {
		t.Errorf("expected summary total=2 (unfiltered), got %d", result.Summary.Total)
	}
}

func TestRollout_ExtractImages(t *testing.T) {
	containers := []corev1.Container{
		{Name: "app", Image: "nginx:1.25"},
		{Name: "sidecar", Image: "busybox:latest"},
		{Name: "app2", Image: "nginx:1.25"},
	}
	images := extractImages(containers)
	if len(images) != 2 {
		t.Errorf("expected 2 unique images, got %d: %v", len(images), images)
	}
}

func TestRollout_SeverityLabel(t *testing.T) {
	tests := []struct {
		status RolloutStatus
		label  string
	}{
		{RolloutFailed, "critical"},
		{RolloutStalled, "critical"},
		{RolloutDegraded, "warning"},
		{RolloutPaused, "warning"},
		{RolloutInProgress, "info"},
		{RolloutComplete, "ok"},
		{RolloutScaledZero, "info"},
	}
	for _, tc := range tests {
		got := RolloutSeverityLabel(tc.status)
		if got != tc.label {
			t.Errorf("RolloutSeverityLabel(%s) = %s, want %s", tc.status, got, tc.label)
		}
	}
}

func TestRollout_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	req := newReqWithClients(http.MethodGet, "/api/deployments/rollout", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleRolloutStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var result RolloutResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.Total != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.Total)
	}
}
