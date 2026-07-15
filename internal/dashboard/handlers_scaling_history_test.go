package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestScalingHistory_NoEvents(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/scalability/scaling-history", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleScalingHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ScalingHistoryResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalEvents != 0 {
		t.Errorf("expected 0 events, got %d", result.Summary.TotalEvents)
	}
	if result.HealthScore < 80 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestScalingHistory_ScaleUpEvent(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "scale-up-1", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Deployment", Name: "myapp", Namespace: "default",
			},
			Reason:        "SuccessfulRescale",
			Message:       "Deployment scaled up from 2 to 4 replicas",
			LastTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scaling-history", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleScalingHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ScalingHistoryResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalEvents != 1 {
		t.Errorf("expected 1 event, got %d", result.Summary.TotalEvents)
	}
	if result.Summary.ScaleUpEvents != 1 {
		t.Errorf("expected 1 scale-up, got %d", result.Summary.ScaleUpEvents)
	}
	if result.Summary.Last1h != 1 {
		t.Errorf("expected 1 event in last 1h, got %d", result.Summary.Last1h)
	}
}

func TestScalingHistory_FailedScale(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "failed-1", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Deployment", Name: "app", Namespace: "default",
			},
			Reason:        "FailedScale",
			Message:       "failed to scale up — insufficient CPU",
			LastTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scaling-history", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleScalingHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ScalingHistoryResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.FailedScales != 1 {
		t.Errorf("expected 1 failed scale, got %d", result.Summary.FailedScales)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestScalingHistory_HPAEvent(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa-1", Namespace: "api"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "HorizontalPodAutoscaler", Name: "api-hpa", Namespace: "api",
			},
			Reason:        "SuccessfulRescale",
			Message:       "Horizontal Pod Autoscaler scaled up from 3 to 5 replicas",
			LastTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa-2", Namespace: "api"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "HorizontalPodAutoscaler", Name: "api-hpa", Namespace: "api",
			},
			Reason:        "SuccessfulRescale",
			Message:       "Horizontal Pod Autoscaler scaled down from 5 to 3 replicas",
			LastTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scaling-history", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleScalingHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ScalingHistoryResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalEvents != 2 {
		t.Errorf("expected 2 events, got %d", result.Summary.TotalEvents)
	}
	if result.Summary.HPAEvents != 2 {
		t.Errorf("expected 2 HPA events, got %d", result.Summary.HPAEvents)
	}
	if result.Summary.ScaleUpEvents != 1 || result.Summary.ScaleDownEvents != 1 {
		t.Errorf("expected 1 scale-up + 1 scale-down, got up=%d down=%d", result.Summary.ScaleUpEvents, result.Summary.ScaleDownEvents)
	}
}
