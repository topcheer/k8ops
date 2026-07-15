package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestServiceTopologyEmptyCluster verifies behavior with no workloads.
func TestServiceTopologyEmptyCluster(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100, got %d", result.HealthScore)
	}
}

// TestServiceTopologyWithDependencies verifies dependency detection.
func TestServiceTopologyWithDependencies(t *testing.T) {
	replicas := int32(1)

	clientset := k8sfake.NewSimpleClientset(
		// Backend service
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "db-service", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "db"},
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
		// Backend workload (single replica = no HA)
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "db", Image: "postgres:15"},
						},
					},
				},
			},
		},
		// Frontend workload that depends on db-service
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "web",
								Image: "nginx:1.25",
								Env: []corev1.EnvVar{
									{Name: "DATABASE_URL", Value: "postgres://db-service.prod.svc:5432/mydb"},
								},
							},
						},
					},
				},
			},
		},
		// Frontend service
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend-svc", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "frontend"},
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", clientset)
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have 2 workloads + 2 services = 4 nodes
	if result.Summary.TotalWorkloads != 2 {
		t.Errorf("expected 2 workloads, got %d", result.Summary.TotalWorkloads)
	}

	// Should detect at least 1 edge (frontend → db-service)
	if result.Summary.TotalEdges < 1 {
		t.Errorf("expected >=1 edge, got %d", result.Summary.TotalEdges)
	}

	// db-service should be a critical hub with fan-in >= 1
	hasDBHub := false
	for _, hub := range result.CriticalHubs {
		if hub.Name == "db-service" {
			hasDBHub = true
			if hub.FanIn < 1 {
				t.Errorf("expected db-service fan-in >=1, got %d", hub.FanIn)
			}
		}
	}
	if !hasDBHub {
		t.Error("expected db-service to be a critical hub")
	}

	// Should detect single point of failure risk (db has 1 replica but is depended on)
	hasSPOFRisk := false
	for _, risk := range result.Risks {
		if risk.Category == "single-point-of-failure" {
			hasSPOFRisk = true
		}
	}
	if !hasSPOFRisk {
		t.Error("expected single-point-of-failure risk for db-service")
	}
}

// TestServiceTopologyOrphanServices detects services with no backing workload.
func TestServiceTopologyOrphanServices(t *testing.T) {
	replicas := int32(1)

	clientset := k8sfake.NewSimpleClientset(
		// Service with selector but no matching workload
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-svc", Namespace: "test"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "nonexistent"},
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
		// Normal workload and service
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "test"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "app"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-svc", Namespace: "test"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "app"}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", clientset)
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.OrphanServices) == 0 {
		t.Error("expected orphan services to be detected")
	}

	foundOrphan := false
	for _, os := range result.OrphanServices {
		if os.Name == "orphan-svc" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Error("expected orphan-svc to be in orphan services list")
	}
}

// TestServiceTopologyCrossNamespace verifies cross-namespace dependency detection.
func TestServiceTopologyCrossNamespace(t *testing.T) {
	replicas := int32(2)

	clientset := k8sfake.NewSimpleClientset(
		// Service in ns-backend
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api-service", Namespace: "ns-backend"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns-backend"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:v1"}}},
				},
			},
		},
		// Workload in ns-frontend that depends on service in ns-backend
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-frontend"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "web",
								Image: "web:v1",
								Env: []corev1.EnvVar{
									{Name: "API_URL", Value: "http://api-service.ns-backend.svc.cluster.local:8080"},
								},
							},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", clientset)
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect cross-namespace dependency
	if result.Summary.CrossNamespace == 0 {
		t.Error("expected cross-namespace edges to be detected")
	}

	// Should have a cross-namespace risk
	hasCrossNSRisk := false
	for _, risk := range result.Risks {
		if risk.Category == "cross-namespace-dependency" {
			hasCrossNSRisk = true
		}
	}
	if !hasCrossNSRisk {
		t.Error("expected cross-namespace-dependency risk")
	}
}

// TestServiceTopologyHealthScore verifies health score calculation.
func TestServiceTopologyHealthScore(t *testing.T) {
	replicas := int32(1) // Single replica = no HA

	clientset := k8sfake.NewSimpleClientset(
		// Critical hub service with single replica
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "critical-svc", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "critical"},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "critical", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "critical"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "critical"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
				},
			},
		},
		// 3 dependents
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep1"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep1"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "c", Image: "nginx", Env: []corev1.EnvVar{{Name: "SVC", Value: "critical-svc"}}},
						},
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep2", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep2"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep2"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "c", Image: "nginx", Env: []corev1.EnvVar{{Name: "SVC", Value: "critical-svc"}}},
						},
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep3", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep3"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep3"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "c", Image: "nginx", Env: []corev1.EnvVar{{Name: "SVC", Value: "critical-svc"}}},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", clientset)
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Health score should be < 100 due to single point of failure
	if result.HealthScore >= 100 {
		t.Errorf("expected health score < 100 with SPOF, got %d", result.HealthScore)
	}

	// Should have SPOF recommendation
	foundSPOF := false
	for _, r := range result.Recommendations {
		if strings.Contains(strings.ToLower(r), "single point") {
			foundSPOF = true
		}
	}
	if !foundSPOF {
		t.Error("expected single point of failure recommendation")
	}
}

// TestExtractServiceDependencies verifies service dependency extraction from env vars.
func TestExtractServiceDependencies(t *testing.T) {
	services := []corev1.Service{
		{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "prod"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
		{ObjectMeta: metav1.ObjectMeta{Name: "queue", Namespace: "infra"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
	}

	wl := workloadInfo{
		id:        "Deployment/prod/app",
		namespace: "prod",
		podSpec: &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					Env: []corev1.EnvVar{
						{Name: "DB_HOST", Value: "db.prod.svc:5432"},
						{Name: "CACHE_URL", Value: "redis://cache.prod.svc.cluster.local:6379"},
						{Name: "QUEUE_URL", Value: "amqp://queue.infra.svc:5672"},
						{Name: "API_KEY", Value: "secret123"},
					},
				},
			},
		},
	}

	deps := extractServiceDependencies(wl, services)

	if len(deps) < 3 {
		t.Errorf("expected >=3 dependencies, got %d", len(deps))
	}

	// Verify specific services detected
	foundDB, foundCache, foundQueue := false, false, false
	for _, d := range deps {
		switch d.name {
		case "db":
			foundDB = true
		case "cache":
			foundCache = true
		case "queue":
			foundQueue = true
		}
	}
	if !foundDB {
		t.Error("expected to detect 'db' dependency")
	}
	if !foundCache {
		t.Error("expected to detect 'cache' dependency")
	}
	if !foundQueue {
		t.Error("expected to detect 'queue' dependency (cross-namespace)")
	}
}

// TestComputeMaxDepth verifies the longest-path calculation.
func TestComputeMaxDepth(t *testing.T) {
	edges := []TopologyEdge{
		{From: "A", To: "B"},
		{From: "B", To: "C"},
		{From: "C", To: "D"},
		{From: "A", To: "D"}, // shortcut
	}

	depth := computeMaxDepth(edges)
	if depth < 3 {
		t.Errorf("expected max depth >=3 (A→B→C→D), got %d", depth)
	}
}

// TestComputeMaxDepthCycle handles cycles gracefully.
func TestComputeMaxDepthCycle(t *testing.T) {
	edges := []TopologyEdge{
		{From: "A", To: "B"},
		{From: "B", To: "C"},
		{From: "C", To: "A"}, // cycle!
	}

	// Should not hang or panic
	depth := computeMaxDepth(edges)
	if depth < 1 {
		t.Errorf("expected depth >=1 even with cycle, got %d", depth)
	}
}

// TestServiceTopologyStatefulSet verifies StatefulSet topology scanning.
func TestServiceTopologyStatefulSet(t *testing.T) {
	replicas := int32(3)

	clientset := k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "es-cluster", Namespace: "data"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "es"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "es"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "es", Image: "elasticsearch:8"}}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "es-svc", Namespace: "data"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "es"}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", clientset)
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect StatefulSet
	found := false
	for _, node := range result.Nodes {
		if node.Kind == "StatefulSet" && node.Name == "es-cluster" {
			found = true
			if !node.HasHA {
				t.Error("expected StatefulSet with 3 replicas to have HA=true")
			}
		}
	}
	if !found {
		t.Error("expected to find StatefulSet node")
	}
}

// TestGenerateServiceTopologyRecommendations verifies recommendation generation.
func TestGenerateServiceTopologyRecommendations(t *testing.T) {
	result := ServiceTopologyResult{
		Summary: ServiceTopologySummary{
			TotalWorkloads: 10,
			CrossNamespace: 3,
			MaxDepth:       6,
		},
		CriticalHubs: []CriticalHub{
			{Name: "db", Namespace: "prod", HasHA: false, FanIn: 5, Replicas: 1},
		},
		OrphanServices: []OrphanService{
			{Name: "old-svc", Namespace: "prod"},
		},
	}

	recs := generateServiceTopologyRecommendations(result)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundSPOF := false
	foundCrossNS := false
	foundOrphan := false
	foundDepth := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "single point") {
			foundSPOF = true
		}
		if strings.Contains(lower, "cross-namespace") {
			foundCrossNS = true
		}
		if strings.Contains(lower, "orphan") {
			foundOrphan = true
		}
		if strings.Contains(lower, "depth") {
			foundDepth = true
		}
	}

	if !foundSPOF {
		t.Error("expected SPOF recommendation")
	}
	if !foundCrossNS {
		t.Error("expected cross-namespace recommendation")
	}
	if !foundOrphan {
		t.Error("expected orphan service recommendation")
	}
	if !foundDepth {
		t.Error("expected deep dependency chain recommendation")
	}
}

// TestServiceTopologyTimestamp verifies scan timestamp.
func TestServiceTopologyTimestamp(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/service-topology", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleServiceTopology(w, req)

	var result ServiceTopologyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("expected non-zero scan timestamp")
	}
	if time.Since(result.ScannedAt) > 5*time.Second {
		t.Error("scan timestamp should be recent")
	}
}
