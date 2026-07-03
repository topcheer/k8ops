package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- Helper to build waste test requests ---

func wasteTestReq(objects ...runtime.Object) (*Server, *http.Request) {
	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/resources/waste", clientset)
	return &Server{}, req
}

// --- Dead Service tests ---

func TestWaste_DeadServiceClusterIP(t *testing.T) {
	// Service with no endpoints
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "dead-svc", Namespace: "app"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1"},
	}
	srv, req := wasteTestReq(svc)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, item := range result.Items {
		if item.Category == WasteDeadService && item.Name == "dead-svc" {
			found = true
			if item.Severity != WasteSeverityMedium {
				t.Errorf("expected medium severity for ClusterIP, got %s", item.Severity)
			}
		}
	}
	if !found {
		t.Error("expected dead-service waste item not found")
	}
}

func TestWaste_DeadServiceLoadBalancer(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-svc", Namespace: "app"},
		Spec: corev1.ServiceSpec{
			Type:        corev1.ServiceTypeLoadBalancer,
			ClusterIP:   "10.0.0.2",
			ExternalIPs: []string{"1.2.3.4"},
		},
	}
	srv, req := wasteTestReq(svc)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Category == WasteDeadService && item.Name == "lb-svc" {
			if item.Severity != WasteSeverityCritical {
				t.Errorf("expected critical for LoadBalancer, got %s", item.Severity)
			}
			return
		}
	}
	t.Error("expected critical dead-service for LoadBalancer not found")
}

func TestWaste_ServiceWithEndpointsNotWasted(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "alive-svc", Namespace: "app"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1"},
	}
	endpoint := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "alive-svc", Namespace: "app"},
		Subsets: []corev1.EndpointSubset{
			{Addresses: []corev1.EndpointAddress{{IP: "10.1.0.1"}}},
		},
	}
	srv, req := wasteTestReq(svc, endpoint)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Name == "alive-svc" {
			t.Errorf("service with endpoints should not be flagged as waste")
		}
	}
}

func TestWaste_ServiceInKubeSystemSkipped(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.10"},
	}
	srv, req := wasteTestReq(svc)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Namespace == "kube-system" && item.Category == WasteDeadService {
			t.Error("system namespace services should be skipped")
		}
	}
}

// --- Unused PVC tests ---

func TestWaste_UnusedPVC(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan-pvc", Namespace: "data"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	srv, req := wasteTestReq(pvc)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, item := range result.Items {
		if item.Category == WasteUnusedPVC && item.Name == "orphan-pvc" {
			found = true
			if item.Severity != WasteSeverityHigh {
				t.Errorf("expected high severity, got %s", item.Severity)
			}
		}
	}
	if !found {
		t.Error("expected unused-pvc waste not found")
	}
}

func TestWaste_UsedPVCNotWasted(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "in-use-pvc", Namespace: "data"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "data"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "db", Image: "postgres"}},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "in-use-pvc",
						},
					},
				},
			},
		},
	}
	srv, req := wasteTestReq(pvc, pod)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Category == WasteUnusedPVC && item.Name == "in-use-pvc" {
			t.Error("used PVC should not be flagged")
		}
	}
}

// --- Orphaned ConfigMap tests ---

func TestWaste_OrphanedConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "old-config", Namespace: "app",
			Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
		},
	}
	srv, req := wasteTestReq(cm)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, item := range result.Items {
		if item.Category == WasteOrphanedCM && item.Name == "old-config" {
			found = true
			if item.Severity != WasteSeverityMedium {
				t.Errorf("expected medium for user-created CM, got %s", item.Severity)
			}
		}
	}
	if !found {
		t.Error("expected orphaned-configmap not found")
	}
}

func TestWaste_SystemConfigMapSkipped(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "default"},
	}
	srv, req := wasteTestReq(cm)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Name == "kube-root-ca.crt" {
			t.Error("system CM should be skipped")
		}
	}
}

// --- Orphaned Secret tests ---

func TestWaste_OrphanedSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "old-secret", Namespace: "app"},
		Type:       corev1.SecretTypeOpaque,
	}
	srv, req := wasteTestReq(secret)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, item := range result.Items {
		if item.Category == WasteOrphanedSecret && item.Name == "old-secret" {
			found = true
			if item.Severity != WasteSeverityHigh {
				t.Errorf("expected high for orphaned secret, got %s", item.Severity)
			}
		}
	}
	if !found {
		t.Error("expected orphaned-secret not found")
	}
}

func TestWaste_ServiceAccountTokenSecretSkipped(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "default-token-abc", Namespace: "app"},
		Type:       corev1.SecretTypeServiceAccountToken,
	}
	srv, req := wasteTestReq(secret)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Name == "default-token-abc" {
			t.Error("SA token secret should be skipped")
		}
	}
}

func TestWaste_HelmReleaseSecretSkipped(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.app.v1", Namespace: "default"},
		Type:       "helm.sh/release.v1",
	}
	srv, req := wasteTestReq(secret)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Name == "sh.helm.release.v1.app.v1" {
			t.Error("Helm release secret should be skipped")
		}
	}
}

// --- Unattached PV tests ---

func TestWaste_UnattachedPV(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan-pv"},
		Status:     corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
	}
	srv, req := wasteTestReq(pv)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, item := range result.Items {
		if item.Category == WasteUnattachedPV && item.Name == "orphan-pv" {
			found = true
			if item.Severity != WasteSeverityCritical {
				t.Errorf("expected critical for unattached PV, got %s", item.Severity)
			}
		}
	}
	if !found {
		t.Error("expected unattached-pv not found")
	}
}

// --- Empty Namespace tests ---

func TestWaste_EmptyNamespace(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "abandoned-ns"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	srv, req := wasteTestReq(ns)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, item := range result.Items {
		if item.Category == WasteEmptyNamespace && item.Name == "abandoned-ns" {
			found = true
		}
	}
	if !found {
		t.Error("expected empty-namespace not found")
	}
}

func TestWaste_NamespaceWithPodsNotEmpty(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "active-ns"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "active-ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
	}
	srv, req := wasteTestReq(ns, pod)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, item := range result.Items {
		if item.Category == WasteEmptyNamespace && item.Name == "active-ns" {
			t.Error("namespace with pods should not be empty")
		}
	}
}

// --- Summary tests ---

func TestWaste_SummaryCostRisk(t *testing.T) {
	// Create 3 critical waste items (LoadBalancer services with no endpoints)
	objects := []runtime.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "lb1", Namespace: "app"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "lb2", Namespace: "app"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "lb3", Namespace: "app"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}},
	}
	srv, req := wasteTestReq(objects...)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.EstCostRisk != "high" {
		t.Errorf("expected high cost risk with 3 critical items, got %s", result.Summary.EstCostRisk)
	}
	if result.Summary.Total < 3 {
		t.Errorf("expected at least 3 items, got %d", result.Summary.Total)
	}
	if result.Summary.ByCategory[string(WasteDeadService)] < 3 {
		t.Errorf("expected 3 dead services, got %d", result.Summary.ByCategory[string(WasteDeadService)])
	}
}

func TestWaste_Sorting(t *testing.T) {
	// Critical PV + Low CM
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv1"},
		Status:     corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "app"},
	}
	srv, req := wasteTestReq(pv, cm)
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.Items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(result.Items))
	}
	// Critical should sort first
	if result.Items[0].Severity != WasteSeverityCritical {
		t.Errorf("expected critical first, got %s", result.Items[0].Severity)
	}
}

// --- Reference set builder unit tests ---

func TestWaste_BuildMountedPVCSet(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
					}},
				},
			},
		},
	}
	set := buildMountedPVCSet(pods)
	if !set["ns1/pvc1"] {
		t.Error("expected ns1/pvc1 in mounted set")
	}
	if set["ns1/pvc2"] {
		t.Error("pvc2 should not be in set")
	}
}

func TestWaste_BuildUsedConfigMapSet(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"},
						},
					}},
				},
				Containers: []corev1.Container{
					{
						Name: "c1",
						Env: []corev1.EnvVar{
							{
								Name: "KEY",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "cm2"},
										Key:                  "key",
									},
								},
							},
						},
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "cm3"},
							}},
						},
					},
				},
			},
		},
	}
	set := buildUsedConfigMapSet(pods)
	if !set["ns1/cm1"] || !set["ns1/cm2"] || !set["ns1/cm3"] {
		t.Errorf("expected cm1, cm2, cm3 in set, got %v", set)
	}
}

func TestWaste_BuildUsedSecretSet(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-secret"}},
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "tls-secret"},
					}},
				},
				Containers: []corev1.Container{
					{
						Name: "c1",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "db-secret"},
							}},
						},
					},
				},
			},
		},
	}
	set := buildUsedSecretSet(pods)
	if !set["ns1/reg-secret"] || !set["ns1/tls-secret"] || !set["ns1/db-secret"] {
		t.Errorf("expected all 3 secrets in set, got %v", set)
	}
}

func TestWaste_BuildEndpointMap(t *testing.T) {
	endpoints := []corev1.Endpoints{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns1"},
			Subsets: []corev1.EndpointSubset{
				{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc2", Namespace: "ns1"},
			Subsets:    []corev1.EndpointSubset{}, // empty
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "svc3", Namespace: "ns1"},
			Subsets: []corev1.EndpointSubset{
				{NotReadyAddresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}}},
			},
		},
	}
	m := buildEndpointMap(endpoints)
	if !m["ns1/svc1"] {
		t.Error("svc1 should have endpoints")
	}
	if m["ns1/svc2"] {
		t.Error("svc2 should NOT have endpoints")
	}
	if !m["ns1/svc3"] {
		t.Error("svc3 with NotReadyAddresses should count as having endpoints")
	}
}

func TestWaste_EmptyCluster(t *testing.T) {
	srv, req := wasteTestReq()
	rr := httptest.NewRecorder()
	srv.handleWasteDetection(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var result WasteResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.Total != 0 {
		t.Errorf("expected 0 waste items, got %d", result.Summary.Total)
	}
}

// Ensure unused import guard
var _ context.Context
