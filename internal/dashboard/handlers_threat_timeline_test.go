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

func TestThreatTimeline_NoEvents(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/security/threat-timeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleThreatTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ThreatTimelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalEvents != 0 {
		t.Errorf("expected 0 events, got %d", result.Summary.TotalEvents)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestThreatTimeline_RBACChange(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "rbac-1", Namespace: "kube-system"},
			InvolvedObject: corev1.ObjectReference{Kind: "ClusterRole", Name: "admin-role"},
			Reason:         "Created",
			Message:        "ClusterRole admin-role created",
			LastTimestamp:  metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/threat-timeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleThreatTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ThreatTimelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.RBACChanges != 1 {
		t.Errorf("expected 1 RBAC change, got %d", result.Summary.RBACChanges)
	}
}

func TestThreatTimeline_ForbiddenAccess(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "forbidden-1", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "app-1", Namespace: "default"},
			Reason:         "Forbidden",
			Message:        "forbidden: user cannot create pods",
			LastTimestamp:  metav1.NewTime(time.Now().Add(-3 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/threat-timeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleThreatTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ThreatTimelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.Forbidden != 1 {
		t.Errorf("expected 1 forbidden, got %d", result.Summary.Forbidden)
	}
	if result.HealthScore >= 90 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestThreatTimeline_AdmissionDenied(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "denied-1", Namespace: "production"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "bad-pod", Namespace: "production"},
			Reason:         "AdmissionDenied",
			Message:        "admission webhook denied pod creation: privileged container not allowed",
			LastTimestamp:  metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/threat-timeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleThreatTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ThreatTimelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.AdmissionDenied != 1 {
		t.Errorf("expected 1 admission denied, got %d", result.Summary.AdmissionDenied)
	}
}
