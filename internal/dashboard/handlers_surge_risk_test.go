package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestSurgeRisk_HealthyRollingUpdate(t *testing.T) {
	replicas := int32(3)
	surge := intstr.FromString("25%")
	unavail := intstr.FromString("25%")
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-good", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge,
						MaxUnavailable: &unavail,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/surge-risk", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSurgeRisk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SurgeRiskResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalDeployments != 1 {
		t.Errorf("expected 1 deployment, got %d", result.Summary.TotalDeployments)
	}
	if result.HealthScore < 95 {
		t.Errorf("expected high health score for good config, got %d", result.HealthScore)
	}
}

func TestSurgeRisk_HighSurge(t *testing.T) {
	replicas := int32(3)
	surge := intstr.FromString("100%")
	unavail := intstr.FromString("0%")
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-surge", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge,
						MaxUnavailable: &unavail,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/surge-risk", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSurgeRisk(rec, req)

	var result SurgeRiskResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighSurge != 1 {
		t.Errorf("expected 1 high surge, got %d", result.Summary.HighSurge)
	}
	found := false
	for _, d := range result.Deployments {
		if d.Name == "app-surge" && d.RiskLevel == "high" {
			found = true
		}
	}
	if !found {
		t.Error("expected app-surge to have high risk level")
	}
}

func TestSurgeRisk_RecreateStrategy(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-recreate", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/surge-risk", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSurgeRisk(rec, req)

	var result SurgeRiskResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.RecreateStrategy != 1 {
		t.Errorf("expected 1 recreate strategy, got %d", result.Summary.RecreateStrategy)
	}
}

func TestSurgeRisk_HighUnavailable(t *testing.T) {
	replicas := int32(3)
	surge := intstr.FromString("0%")
	unavail := intstr.FromString("100%")
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-unavail", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge,
						MaxUnavailable: &unavail,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/surge-risk", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSurgeRisk(rec, req)

	var result SurgeRiskResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighUnavailable != 1 {
		t.Errorf("expected 1 high unavailable, got %d", result.Summary.HighUnavailable)
	}
}

func TestSurgeRisk_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/deployment/surge-risk", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSurgeRisk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result SurgeRiskResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalDeployments != 0 {
		t.Errorf("expected 0 deployments, got %d", result.Summary.TotalDeployments)
	}
}
