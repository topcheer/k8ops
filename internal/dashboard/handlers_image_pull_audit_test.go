package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestImagePullAudit_PolicyIssues(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		// Pod with Never pull policy (should be flagged)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-never", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25", ImagePullPolicy: corev1.PullNever},
				},
			},
		},
		// Pod with IfNotPresent (good)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-good", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/nginx:1.25", ImagePullPolicy: corev1.PullIfNotPresent},
				},
			},
		},
		// Pod with Always on pinned image (should be flagged as wasteful)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-always", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/nginx:1.25", ImagePullPolicy: corev1.PullAlways},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/image-pull-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImagePullAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ImagePullAuditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.NeverPull != 1 {
		t.Errorf("expected 1 Never pull, got %d", result.Summary.NeverPull)
	}
	if result.Summary.AlwaysPull != 1 {
		t.Errorf("expected 1 Always pull, got %d", result.Summary.AlwaysPull)
	}
	if result.Summary.IfNotPresent != 1 {
		t.Errorf("expected 1 IfNotPresent, got %d", result.Summary.IfNotPresent)
	}

	// Check that Never pull is flagged
	foundNever := false
	foundWasteful := false
	for _, issue := range result.PolicyIssues {
		if issue.Issue == "imagePullPolicy: Never prevents pulling updated images" {
			foundNever = true
		}
		if issue.Issue == "imagePullPolicy: Always on pinned image wastes bandwidth; consider IfNotPresent" {
			foundWasteful = true
		}
	}
	if !foundNever {
		t.Error("expected to find Never pull policy issue")
	}
	if !foundWasteful {
		t.Error("expected to find Always pull on pinned image issue")
	}
}

func TestImagePullAudit_PrivateImageNoSecret(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		// Pod with private image but no pull secrets (should be flagged)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-private", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.iot2.win/app:v1.0", ImagePullPolicy: corev1.PullIfNotPresent},
				},
			},
		},
		// Pod with private image and pull secrets (good)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-with-secret", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.iot2.win/app:v1.0", ImagePullPolicy: corev1.PullIfNotPresent},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/image-pull-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImagePullAudit(rec, req)

	var result ImagePullAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	foundNoSecret := false
	for _, issue := range result.SecretIssues {
		if issue.Issue == "Private image registry.iot2.win/app:v1.0 used without imagePullSecrets" {
			foundNoSecret = true
		}
	}
	if !foundNoSecret {
		t.Error("expected to find private image without pull secrets issue")
	}
}

func TestImagePullAudit_StaleSecret(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		// Stale dockerconfigjson secret (not referenced)
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "stale-regcred", Namespace: "app-prod"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte(`{"auths":{"registry.iot2.win":{"auth":"dXNlcjpwYXNz"}}}`),
			},
		},
		// Referenced secret
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "active-regcred", Namespace: "app-prod"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte(`{"auths":{"registry.iot2.win":{"auth":"dXNlcjpwYXNz"}}}`),
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "active-regcred"}},
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25", ImagePullPolicy: corev1.PullIfNotPresent},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/image-pull-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImagePullAudit(rec, req)

	var result ImagePullAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.StaleSecrets != 1 {
		t.Errorf("expected 1 stale secret, got %d", result.Summary.StaleSecrets)
	}

	foundStale := false
	for _, issue := range result.SecretIssues {
		if issue.SecretName == "stale-regcred" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Error("expected to find stale-regcred secret issue")
	}
}

func TestImagePullAudit_HealthScore(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-clean", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/nginx:1.25.3", ImagePullPolicy: corev1.PullIfNotPresent},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/image-pull-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImagePullAudit(rec, req)

	var result ImagePullAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.HealthScore < 95 {
		t.Errorf("expected high health score for clean config, got %d", result.HealthScore)
	}
}

func TestImagePullAudit_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/deployment/image-pull-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImagePullAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result ImagePullAuditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalPods != 0 {
		t.Errorf("expected 0 pods, got %d", result.Summary.TotalPods)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100, got %d", result.HealthScore)
	}
}
