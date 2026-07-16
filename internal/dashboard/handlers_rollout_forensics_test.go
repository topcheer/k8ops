package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sintstr "k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestRolloutForensicsEmptyCluster verifies handler on empty cluster.
func TestRolloutForensicsEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

// TestRolloutForensicsHealthyDeploy verifies scoring with a healthy deployment.
func TestRolloutForensicsHealthyDeploy(t *testing.T) {
	replicas := int32(3)
	revLimit := int32(10)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas:           &replicas,
				RevisionHistoryLimit: &revLimit,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("100m"),
								},
							},
							ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: k8sintstr.FromInt32(8080)}}},
							LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/live", Port: k8sintstr.FromInt32(8080)}}},
						},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:           3,
				UpdatedReplicas:    3,
				ReadyReplicas:      3,
				AvailableReplicas:  3,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, LastUpdateTime: metav1.Now()},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalDeployments != 1 {
		t.Errorf("expected 1 deployment, got %d", result.Summary.TotalDeployments)
	}

	// Healthy deployment should have high reliability
	if len(result.ReliabilityScore) > 0 {
		score := result.ReliabilityScore[0]
		if score.Score < 80 {
			t.Errorf("healthy deploy score should be >= 80, got %d", score.Score)
		}
	}
}

// TestRolloutForensicsAntiPatterns verifies anti-pattern detection.
func TestRolloutForensicsAntiPatterns(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		// Single replica, no probes, recreate strategy, no resources
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app"},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect multiple anti-patterns
	patternTypes := map[string]bool{}
	for _, ap := range result.AntiPatterns {
		patternTypes[ap.Type] = true
	}

	if !patternTypes["no-readiness-probe"] {
		t.Error("Should detect missing readiness probe")
	}
	if !patternTypes["no-liveness-probe"] {
		t.Error("Should detect missing liveness probe")
	}
	if !patternTypes["recreate-strategy"] {
		t.Error("Should detect recreate strategy")
	}
	if !patternTypes["no-resources"] {
		t.Error("Should detect missing resources")
	}
	if !patternTypes["single-replica"] {
		t.Error("Should detect single replica")
	}

	// Reliability score should be low
	if len(result.ReliabilityScore) > 0 {
		score := result.ReliabilityScore[0]
		if score.Score >= 60 {
			t.Errorf("bad deploy score should be < 60, got %d", score.Score)
		}
		if score.Grade == "A" || score.Grade == "B" {
			t.Errorf("bad deploy grade should be C or worse, got %s", score.Grade)
		}
	}
}

// TestRolloutForensicsFailedRollout verifies failed rollout detection.
func TestRolloutForensicsFailedRollout(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "failing-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app"},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 1, // Only 1 of 3 updated
				ReadyReplicas:   0, // None ready!
				AvailableReplicas: 0,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, LastUpdateTime: metav1.Time{Time: metav1.Now().Add(-15 * 1e9)}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.Failed == 0 {
		t.Error("Should detect failed rollout (0 ready replicas)")
	}

	if len(result.FailedRollouts) == 0 {
		t.Error("Should have at least one failed rollout entry")
	}
}

// TestRolloutForensicsRolloutForensicsRiskFactors verifies cluster-level risk detection.
func TestRolloutForensicsRolloutForensicsRiskFactors(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "risky-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have at least one risk factor
	if len(result.RolloutForensicsRiskFactors) == 0 {
		t.Error("Should detect risk factors with risky deployment")
	}

	for _, rf := range result.RolloutForensicsRiskFactors {
		if rf.Factor == "" || rf.Description == "" {
			t.Error("Risk factor should have factor and description")
		}
	}
}

// TestRolloutForensicsSystemNSExclusion verifies system namespaces excluded.
func TestRolloutForensicsSystemNSExclusion(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// kube-system deployments should be excluded
	if result.Summary.TotalDeployments != 0 {
		t.Errorf("kube-system should be excluded, got %d deployments", result.Summary.TotalDeployments)
	}
}

// TestRolloutForensicsRecommendations verifies recommendations.
func TestRolloutForensicsRecommendations(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollout-forensics", clientset)
	w := httptest.NewRecorder()

	s.handleRolloutForensics(w, req)

	var result RolloutForensicsResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}

// TestRolloutTakeFirst verifies the helper.
func TestRolloutTakeFirst(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e"}
	if len(takeFirst(s, 3)) != 3 {
		t.Error("takeFirst should return 3 elements")
	}
	if len(takeFirst(s, 10)) != 5 {
		t.Error("takeFirst should return all elements if n > len")
	}
	if len(takeFirst(s, 0)) != 0 {
		t.Error("takeFirst should return empty if n=0")
	}
}
