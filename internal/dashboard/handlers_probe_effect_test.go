package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestProbeEffectEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/probe-effectiveness", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleProbeEffect(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result ProbeEffResult
	json.Unmarshal(w.Body.Bytes(), &result)
}

func TestProbeEffectNoProbes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "app:v1"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/probe-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleProbeEffect(w, req)
	var result ProbeEffResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.WithoutLiveness != 1 {
		t.Errorf("expected 1 without liveness, got %d", result.Summary.WithoutLiveness)
	}
	if result.Summary.LivenessCoverPct > 0 {
		t.Errorf("expected 0%% liveness, got %f", result.Summary.LivenessCoverPct)
	}
}

func TestProbeEffectWithProbes(t *testing.T) {
	delay := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c", Image: "app:v1",
				LivenessProbe: &corev1.Probe{
					InitialDelaySeconds: delay,
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
						Path: "/health", Port: intstr.FromInt(8080),
					}},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
						Path: "/ready", Port: intstr.FromInt(8080),
					}},
				},
			}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/probe-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleProbeEffect(w, req)
	var result ProbeEffResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.LivenessCoverPct != 100 {
		t.Errorf("expected 100%%, got %f", result.Summary.LivenessCoverPct)
	}
}

func TestProbeEffectHighRestarts(t *testing.T) {
	delay := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashy", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c", Image: "app:v1",
				LivenessProbe: &corev1.Probe{InitialDelaySeconds: delay,
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(80)}}},
			}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", RestartCount: 10}}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/probe-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleProbeEffect(w, req)
	var result ProbeEffResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.HighRiskCount == 0 {
		t.Error("expected high risk count for high restarts with liveness probe")
	}
}

func TestProbeEffectSameEndpoint(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c", Image: "app:v1",
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(80)}}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(80)}}},
			}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/probe-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleProbeEffect(w, req)
	var result ProbeEffResult
	json.Unmarshal(w.Body.Bytes(), &result)
	found := false
	for _, ip := range result.Ineffective {
		if strings.Contains(ip.Issue, "same endpoint") {
			found = true
		}
	}
	if !found {
		t.Error("expected same-endpoint ineffective probe")
	}
}

func TestProbeEffectRecs(t *testing.T) {
	result := ProbeEffResult{
		Summary: ProbeEffSummary{
			WithoutLiveness: 5, WithoutReadiness: 3, HighRiskCount: 2,
			LivenessCoverPct: 60, ReadinessCoverPct: 70,
		},
		Ineffective:         []IneffectiveProbeEntry{{}},
		NoProbeEffWorkloads: []ProbeEffWorkload{{}},
	}
	recs := generateProbeEffectRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
}
