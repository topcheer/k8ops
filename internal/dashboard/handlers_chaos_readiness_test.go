package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// Note: int32Ptr and int64Ptr helpers are defined in handlers_deploy_audit_test.go

// TestChaosReadinessEmpty verifies empty cluster behavior.
func TestChaosReadinessEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
	if result.ReadinessScore != 100 {
		t.Errorf("expected score 100 for empty cluster, got %d", result.ReadinessScore)
	}
}

// TestChaosReadinessResilientWorkload verifies a well-configured HA workload.
func TestChaosReadinessResilientWorkload(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "resilient-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "resilient"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "resilient"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "app:v1",
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8080)}},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt32(8080)}},
								},
								Lifecycle: &corev1.Lifecycle{
									PreStop: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"sleep", "5"}}},
								},
							},
						},
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{TopologyKey: "kubernetes.io/hostname"},
								},
							},
						},
						TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
							{MaxSkew: 1, TopologyKey: "topology.kubernetes.io/zone"},
						},
						TerminationGracePeriodSeconds: int64Ptr(60),
					},
				},
			},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "resilient-pdb", Namespace: "prod"},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "resilient"}},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", clientset)
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 1 {
		t.Fatalf("expected 1 workload, got %d", result.Summary.TotalWorkloads)
	}

	wl := result.Workloads[0]
	if wl.ReadinessLevel != "ready" {
		t.Errorf("expected ready, got %s (score: %d)", wl.ReadinessLevel, wl.Score)
	}
	if wl.Score < 70 {
		t.Errorf("expected score >=70, got %d", wl.Score)
	}

	// Should have safe chaos experiments
	hasSafeExperiment := false
	for _, exp := range result.Experiments {
		if exp.Safe && exp.Target == "resilient-app" {
			hasSafeExperiment = true
		}
	}
	if !hasSafeExperiment {
		t.Error("expected safe chaos experiment for resilient workload")
	}
}

// TestChaosReadinessFragileWorkload verifies detection of fragile workloads.
func TestChaosReadinessFragileWorkload(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "fragile-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "fragile"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "fragile"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:v1"},
						},
						TerminationGracePeriodSeconds: int64Ptr(5),
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", clientset)
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.FragileCount != 1 {
		t.Errorf("expected 1 fragile workload, got %d", result.Summary.FragileCount)
	}

	if len(result.FragileWorkloads) == 0 {
		t.Error("expected fragile workloads list to be populated")
	}

	// Should not recommend chaos experiments
	for _, exp := range result.Experiments {
		if exp.Target == "fragile-app" && exp.Safe {
			t.Error("should not recommend safe experiment for fragile workload")
		}
	}
}

// TestChaosReadinessPartialWorkload verifies partial readiness.
func TestChaosReadinessPartialWorkload(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		// Has HA but no probes/PDB/graceful shutdown
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "partial-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "partial"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "partial"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:v1"},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", clientset)
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	wl := result.Workloads[0]
	if wl.ReadinessLevel != "partial" {
		t.Errorf("expected partial, got %s (score: %d)", wl.ReadinessLevel, wl.Score)
	}
	if wl.Score < 40 || wl.Score >= 70 {
		t.Errorf("expected score 40-69, got %d", wl.Score)
	}
}

// TestChaosReadinessMultiZone verifies multi-zone spread detection.
func TestChaosReadinessMultiZone(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-a",
				Labels: map[string]string{"topology.kubernetes.io/zone": "us-east-1a"},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-b",
				Labels: map[string]string{"topology.kubernetes.io/zone": "us-east-1b"},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "multi-zone-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "mz"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "mz"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:v1"},
						},
					},
				},
			},
		},
		// Pods on different nodes/zones
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-zone-app-pod1",
				Namespace: "prod",
				Labels:    map[string]string{"app": "mz"},
			},
			Spec:   corev1.PodSpec{NodeName: "node-a"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-zone-app-pod2",
				Namespace: "prod",
				Labels:    map[string]string{"app": "mz"},
			},
			Spec:   corev1.PodSpec{NodeName: "node-b"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", clientset)
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect 2 zones
	wl := result.Workloads[0]
	hasZoneCheck := false
	for _, c := range wl.Checks {
		if c.Name == "Multi-Zone Spread" && c.Status == "pass" {
			hasZoneCheck = true
		}
	}
	if !hasZoneCheck {
		t.Error("expected multi-zone spread check to pass")
	}
}

// TestChaosReadinessRecommendations verifies recommendation generation.
func TestChaosReadinessRecommendations(t *testing.T) {
	result := ChaosReadinessResult{
		ReadinessScore: 35,
		Summary: ChaosSummary{
			TotalWorkloads:  10,
			FragileCount:    3,
			ReadyForChaos:   2,
			HasPDB:          4,
			HasProbes:       5,
			HasGracefulStop: 3,
			HasMultiReplica: 7,
			HasAntiAffinity: 4,
		},
	}

	recs := generateChaosRecommendations(result, 3)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundFragile := false
	foundPDB := false
	foundProbe := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "fragile") {
			foundFragile = true
		}
		if strings.Contains(lower, "pdb") {
			foundPDB = true
		}
		if strings.Contains(lower, "probe") {
			foundProbe = true
		}
	}
	if !foundFragile {
		t.Error("expected fragile workload recommendation")
	}
	if !foundPDB {
		t.Error("expected PDB gap recommendation")
	}
	if !foundProbe {
		t.Error("expected probe gap recommendation")
	}
}

// TestChaosReadinessStatefulSet verifies StatefulSet assessment.
func TestChaosReadinessStatefulSet(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ha-sts", Namespace: "data"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "sts"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "sts"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "sts",
								Image: "sts:v1",
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt32(8080)}},
								},
							},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/chaos-readiness", clientset)
	w := httptest.NewRecorder()
	s.handleChaosReadiness(w, req)

	var result ChaosReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	found := false
	for _, wl := range result.Workloads {
		if wl.Kind == "StatefulSet" && wl.Name == "ha-sts" {
			found = true
			if wl.Score < 40 {
				t.Errorf("expected higher score for 3-replica StatefulSet with probes, got %d", wl.Score)
			}
		}
	}
	if !found {
		t.Error("expected to find StatefulSet in results")
	}
}

// TestGenerateChaosExperiments verifies experiment generation.
func TestGenerateChaosExperiments(t *testing.T) {
	workloads := []ChaosWorkload{
		{
			Name: "ready-app", Namespace: "prod", Kind: "Deployment",
			Replicas: 3, Score: 85, ReadinessLevel: "ready",
			MaxTolerableFailure: 1,
		},
		{
			Name: "fragile-app", Namespace: "prod", Kind: "Deployment",
			Replicas: 1, Score: 20, ReadinessLevel: "fragile",
			MaxTolerableFailure: 0,
		},
	}

	experiments := generateChaosExperiments(workloads, 2)

	if len(experiments) == 0 {
		t.Fatal("expected experiments")
	}

	// Ready workload should have safe experiment
	foundSafe := false
	foundUnsafe := false
	for _, exp := range experiments {
		if exp.Target == "ready-app" && exp.Safe {
			foundSafe = true
		}
		if exp.Target == "fragile-app" {
			foundUnsafe = true
		}
	}
	if !foundSafe {
		t.Error("expected safe experiment for ready workload")
	}
	if foundUnsafe {
		t.Error("should not generate experiment for fragile workload")
	}
}
