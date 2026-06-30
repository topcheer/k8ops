package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// === handleResources tests ===

func TestHandleResources_Deployments(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptrInt32Ptr(3),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:1.21"}}},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=deployments", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	items, ok := result["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items wrong: %v", result["items"])
	}
	first := items[0].(map[string]any)
	if first["name"] != "web" {
		t.Errorf("name = %v, want web", first["name"])
	}
	if first["ready"] != "2/3" {
		t.Errorf("ready = %v, want 2/3", first["ready"])
	}
}

func TestHandleResources_DeploymentsDefaultKind(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["count"].(float64) != 0 {
		t.Errorf("count = %v, want 0", result["count"])
	}
}

func TestHandleResources_Services(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=services", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	items := result["items"].([]any)
	first := items[0].(map[string]any)
	if first["type"] != "ClusterIP" {
		t.Errorf("type = %v, want ClusterIP", first["type"])
	}
}

func TestHandleResources_Ingresses(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "web-ing", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: "example.com"}},
			},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=ingresses", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
}

func TestHandleResources_ConfigMaps(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
			Data:       map[string]string{"key": "value"},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=configmaps", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleResources_Secrets(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "app-secret", Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=secrets", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleResources_DaemonSets(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
			Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=daemonsets", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleResources_StatefulSets(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "mysql", Namespace: "default"},
			Status:     appsv1.StatefulSetStatus{ReadyReplicas: 2},
			Spec:       appsv1.StatefulSetSpec{Replicas: ptrInt32Ptr(3)},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=statefulsets", cs)

	s.handleResources(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleResources_UnknownKind(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/resources?kind=unknown", cs)

	s.handleResources(w, r)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// === handleNodePods tests ===

func TestHandleNodePods_Basic(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-2"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/nodes/node-1/pods", cs)

	s.handleNodePods(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// === CRD/YAML handler tests ===

func TestBuiltinGVR_Known(t *testing.T) {
	gvr, err := builtinGVR("pods")
	if err != nil {
		t.Fatalf("builtinGVR error: %v", err)
	}
	if gvr.Resource != "pods" {
		t.Errorf("resource = %q, want pods", gvr.Resource)
	}
}

func TestBuiltinGVR_Unknown(t *testing.T) {
	_, err := builtinGVR("unknownthing")
	if err == nil {
		t.Error("expected error for unknown resource")
	}
}

// === Cost handler tests (with method check only to avoid nil panic) ===

func TestHandleCostSummary_MethodCheck(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/cost/summary", nil)

	s.handleCostSummary(w, r)

	if w.Code != 405 {
		t.Errorf("POST should return 405, got %d", w.Code)
	}
}

func TestHandleCostRecommendations_MethodCheck(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/cost/recommendations", nil)

	s.handleCostRecommendations(w, r)

	if w.Code != 405 {
		t.Errorf("POST should return 405, got %d", w.Code)
	}
}

// === RBAC utility tests ===

func TestContains_Basic(t *testing.T) {
	if !contains("hello.go", ".go") {
		t.Error("contains('hello.go', '.go') should be true")
	}
	if contains("hello.txt", ".go") {
		t.Error("contains('hello.txt', '.go') should be false")
	}
}

func TestExtractGroup_Basic(t *testing.T) {
	g := extractGroup("rbac.authorization.k8s.io/v1")
	if g != "rbac.authorization.k8s.io" {
		t.Errorf("group = %q, want rbac.authorization.k8s.io", g)
	}
}

func TestExtractVersion_Basic(t *testing.T) {
	v := extractVersion("rbac.authorization.k8s.io/v1")
	if v != "v1" {
		t.Errorf("version = %q, want v1", v)
	}
}

// === handleClusterOverview with version ===

func TestHandleClusterOverview_Version(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		makeNode("node-1", true),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/cluster/overview", cs)

	s.handleClusterOverview(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// fake clientset returns a version
	if result["clusterVersion"] == nil {
		t.Error("expected clusterVersion in response")
	}
}

// === Additional Pod tests ===

func TestHandlePods_MultipleContainers(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", RestartCount: 1},
					{Name: "c2", RestartCount: 3},
					{Name: "c3", RestartCount: 0},
				},
			},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/pods", cs)

	s.handlePods(w, r)

	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	items := result["items"].([]any)
	first := items[0].(map[string]any)
	if first["restarts"].(float64) != 4 {
		t.Errorf("restarts = %v, want 4", first["restarts"])
	}
}

// === Events with long message truncation ===

func TestHandleEvents_LongMessage(t *testing.T) {
	longMsg := ""
	for i := 0; i < 500; i++ {
		longMsg += "x"
	}
	cs := k8sfake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-1", Namespace: "default"},
			Type:           "Normal",
			Message:        longMsg,
			LastTimestamp:  metav1.Now(),
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p1", Namespace: "default"},
		},
	)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := newReqWithClients(http.MethodGet, "/api/events", cs)

	s.handleEvents(w, r)

	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	items := result["items"].([]any)
	first := items[0].(map[string]any)
	msg := first["message"].(string)
	if len(msg) > 303 {
		t.Errorf("message not truncated, len = %d", len(msg))
	}
	if msg[len(msg)-3:] != "..." {
		t.Errorf("message should end with ..., got %q", msg[len(msg)-3:])
	}
}

// === Container port helper ===

func TestIntstrBasic(t *testing.T) {
	v := intstr.FromInt(80)
	if v.IntVal != 80 {
		t.Errorf("IntVal = %d, want 80", v.IntVal)
	}
}

// ptrInt32Ptr returns a pointer to the given int32.
func ptrInt32Ptr(v int32) *int32 { return &v }
