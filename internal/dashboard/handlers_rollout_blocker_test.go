package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRolloutBlocker_HealthyDeployment(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-healthy", Namespace: "app-prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				UpdatedReplicas:   3,
				ReadyReplicas:     3,
				AvailableReplicas: 3,
				Replicas:          3,
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/rollout-blocker", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRolloutBlocker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RolloutBlockerResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.HealthyDeployments != 1 {
		t.Errorf("expected 1 healthy deployment, got %d", result.Summary.HealthyDeployments)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestRolloutBlocker_BlockedDeployment(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-blocked", Namespace: "app-prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				UpdatedReplicas:   0,
				ReadyReplicas:     3,
				AvailableReplicas: 3,
				Replicas:          3,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:    appsv1.DeploymentProgressing,
						Status:  corev1.ConditionFalse,
						Reason:  "ProgressDeadlineExceeded",
						Message: "ReplicaSet has timed out progressing",
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/rollout-blocker", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRolloutBlocker(rec, req)

	var result RolloutBlockerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.BlockedRollouts < 1 {
		t.Errorf("expected at least 1 blocked rollout, got %d", result.Summary.BlockedRollouts)
	}
	found := false
	for _, b := range result.BlockedRollouts {
		if b.Name == "app-blocked" && b.Severity == "critical" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find blocked deployment with critical severity")
	}
	if result.HealthScore > 80 {
		t.Errorf("expected reduced health score for blocked rollout, got %d", result.HealthScore)
	}
}

func TestRolloutBlocker_CrashLoopPod(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-crash", Namespace: "app-prod"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 0, Replicas: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-crash-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-crash-abc"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "c1",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason:  "CrashLoopBackOff",
								Message: "Back-off restarting failed container",
							},
						},
						RestartCount: 5,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/rollout-blocker", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRolloutBlocker(rec, req)

	var result RolloutBlockerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PodsCrashLooping != 1 {
		t.Errorf("expected 1 CrashLoopBackOff pod, got %d", result.Summary.PodsCrashLooping)
	}
	found := false
	for _, pc := range result.PodConditions {
		if pc.Condition == "CrashLoopBackOff" && pc.Severity == "critical" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find CrashLoopBackOff pod condition")
	}
}

func TestRolloutBlocker_ImagePullBackOff(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-pull-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-pull-abc"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "c1",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason:  "ImagePullBackOff",
								Message: "Image pull failed",
							},
						},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/rollout-blocker", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRolloutBlocker(rec, req)

	var result RolloutBlockerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PodsImagePullBackOff != 1 {
		t.Errorf("expected 1 ImagePullBackOff, got %d", result.Summary.PodsImagePullBackOff)
	}
	if result.Summary.PodsPending != 1 {
		t.Errorf("expected 1 pending pod, got %d", result.Summary.PodsPending)
	}
}

func TestRolloutBlocker_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/deployment/rollout-blocker", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRolloutBlocker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result RolloutBlockerResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalDeployments != 0 {
		t.Errorf("expected 0 deployments, got %d", result.Summary.TotalDeployments)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100, got %d", result.HealthScore)
	}
}
