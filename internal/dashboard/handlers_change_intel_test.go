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

func TestChangeIntelEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestChangeIntelWithRecentDeployment(t *testing.T) {
	replicas := int32(3)
	now := metav1.Now()
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status: appsv1.DeploymentStatus{
				Replicas: 3, ReadyReplicas: 3,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, LastUpdateTime: now},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", clientset)
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.TotalChanges == 0 {
		t.Error("Should detect recent changes")
	}
	if result.Summary.DeploymentChanges == 0 {
		t.Error("Should count deployment change")
	}
}

func TestChangeIntelRiskyChange(t *testing.T) {
	replicas := int32(5)
	now := metav1.Now()
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "risky", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, LastUpdateTime: now},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", RestartCount: 15},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", clientset)
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.RiskyChangeCount == 0 {
		t.Error("Should detect risky change due to restarts")
	}
}

func TestChangeIntelHourlyBuckets(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.ByHour) != 24 {
		t.Errorf("expected 24 hourly buckets, got %d", len(result.ByHour))
	}
}

func TestChangeIntelSystemNSExcluded(t *testing.T) {
	now := metav1.Now()
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system"},
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, LastUpdateTime: now},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", clientset)
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.TotalChanges != 0 {
		t.Errorf("kube-system should be excluded, got %d changes", result.Summary.TotalChanges)
	}
}

func TestChangeIntelRecommendations(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/change-intel", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleChangeIntel(w, req)

	var result ChangeIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}
