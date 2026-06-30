package providermanager

import (
	"context"
	"fmt"
	"testing"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/provider"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- Fake k8s client helpers ---

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = aiv1alpha1.AddToScheme(s)
	return s
}

func fakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		Build()
}

// --- ReloadFromDirect tests ---

func TestReloadFromDirect_EmptyType(t *testing.T) {
	m := New(nil, testLogger())
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "",
		APIKey: "key",
	})
	if err == nil {
		t.Error("expected error for empty type")
	}
}

func TestReloadFromDirect_WithK8sClient(t *testing.T) {
	m := New(fakeClient(), testLogger())
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "test-key",
		Model:  "gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ConfigMap was persisted
	cm := &corev1.ConfigMap{}
	err = m.k8sClient.Get(context.Background(),
		types.NamespacedName{Name: providerConfigMapName, Namespace: "k8ops-system"}, cm)
	if err != nil {
		t.Fatalf("ConfigMap not persisted: %v", err)
	}
	if cm.Data["type"] != "openai" {
		t.Errorf("ConfigMap type = %q, want 'openai'", cm.Data["type"])
	}
	if cm.Data["model"] != "gpt-4o" {
		t.Errorf("ConfigMap model = %q, want 'gpt-4o'", cm.Data["model"])
	}

	// Verify Secret was persisted (fake client may store in StringData instead of Data)
	secret := &corev1.Secret{}
	err = m.k8sClient.Get(context.Background(),
		types.NamespacedName{Name: providerSecretName, Namespace: "k8ops-system"}, secret)
	if err != nil {
		t.Fatalf("Secret not persisted: %v", err)
	}
	apiKey := ""
	if v, ok := secret.Data["apiKey"]; ok {
		apiKey = string(v)
	} else if v, ok := secret.StringData["apiKey"]; ok {
		apiKey = v
	}
	if apiKey != "test-key" {
		t.Errorf("Secret apiKey = %q", apiKey)
	}
}

func TestReloadFromDirect_UpdateExistingConfigMap(t *testing.T) {
	// Pre-create ConfigMap + Secret
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: providerConfigMapName, Namespace: "k8ops-system"},
		Data:       map[string]string{"type": "old"},
	}
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: providerSecretName, Namespace: "k8ops-system"},
		Data:       map[string][]byte{"apiKey": []byte("old-key")},
	}

	m := New(fakeClient(existingCM, existingSecret), testLogger())
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "new-key",
		Model:  "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ConfigMap was updated
	cm := &corev1.ConfigMap{}
	_ = m.k8sClient.Get(context.Background(),
		types.NamespacedName{Name: providerConfigMapName, Namespace: "k8ops-system"}, cm)
	if cm.Data["type"] != "openai" {
		t.Errorf("ConfigMap type = %q, want 'openai'", cm.Data["type"])
	}

	// Verify Secret was updated (fake client may store in StringData instead of Data)
	secret := &corev1.Secret{}
	_ = m.k8sClient.Get(context.Background(),
		types.NamespacedName{Name: providerSecretName, Namespace: "k8ops-system"}, secret)
	// Prefer StringData (what the manager actually sets on update)
	apiKey := ""
	if v, ok := secret.StringData["apiKey"]; ok {
		apiKey = v
	} else if v, ok := secret.Data["apiKey"]; ok {
		apiKey = string(v)
	}
	if apiKey != "new-key" {
		t.Errorf("Secret apiKey = %q, want 'new-key'", apiKey)
	}
}

// --- Reload tests ---

func TestReload_NoK8opsConfig(t *testing.T) {
	m := New(fakeClient(), testLogger())
	err := m.Reload(context.Background())
	if err == nil {
		t.Error("expected error when no K8opsConfig found")
	}
}

func TestReload_WithK8opsConfig(t *testing.T) {
	cfg := &aiv1alpha1.K8opsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "k8ops-system"},
		Spec: aiv1alpha1.K8opsConfigSpec{
			Provider: aiv1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
			},
		},
	}
	m := New(fakeClient(cfg), testLogger())
	err := m.Reload(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Get() == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestReload_WithSecretRef(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "api-key", Namespace: "k8ops-system"},
		Data:       map[string][]byte{"apiKey": []byte("secret-value")},
	}
	cfg := &aiv1alpha1.K8opsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "k8ops-system"},
		Spec: aiv1alpha1.K8opsConfigSpec{
			Provider: aiv1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
				APIKeySecretRef: &aiv1alpha1.SecretKeySelector{
					Name: "api-key",
					Key:  "apiKey",
				},
			},
		},
	}
	m := New(fakeClient(cfg, secret), testLogger())
	err := m.Reload(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.GetConfig().APIKey != "secret-value" {
		t.Errorf("APIKey = %q, want 'secret-value'", m.GetConfig().APIKey)
	}
}

func TestReload_SecretRefMissing(t *testing.T) {
	cfg := &aiv1alpha1.K8opsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "k8ops-system"},
		Spec: aiv1alpha1.K8opsConfigSpec{
			Provider: aiv1alpha1.ProviderSpec{
				Type:  "openai",
				Model: "gpt-4o",
				APIKeySecretRef: &aiv1alpha1.SecretKeySelector{
					Name: "nonexistent",
					Key:  "apiKey",
				},
			},
		},
	}
	m := New(fakeClient(cfg), testLogger())
	err := m.Reload(context.Background())
	if err == nil {
		t.Error("expected error when secret ref not found")
	}
}

func TestReloadFromConfig_InvalidProviderType(t *testing.T) {
	cfg := &aiv1alpha1.K8opsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "k8ops-system"},
		Spec: aiv1alpha1.K8opsConfigSpec{
			Provider: aiv1alpha1.ProviderSpec{
				Type:  "nonexistent",
				Model: "test",
			},
		},
	}
	m := New(fakeClient(cfg), testLogger())
	err := m.ReloadFromConfig(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for invalid provider type")
	}
}

// --- resolveSecretKey tests ---

func TestResolveSecretKey_DefaultKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"apiKey": []byte("default-key-value")},
	}
	m := New(fakeClient(secret), testLogger())
	val, err := m.resolveSecretKey(context.Background(), "default", &aiv1alpha1.SecretKeySelector{
		Name: "my-secret",
		// Key empty — should default to "apiKey"
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "default-key-value" {
		t.Errorf("val = %q, want 'default-key-value'", val)
	}
}

func TestResolveSecretKey_CustomKey(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "ns1"},
		Data:       map[string][]byte{"custom-key": []byte("custom-value")},
	}
	m := New(fakeClient(secret), testLogger())
	val, err := m.resolveSecretKey(context.Background(), "ns1", &aiv1alpha1.SecretKeySelector{
		Name: "my-secret",
		Key:  "custom-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "custom-value" {
		t.Errorf("val = %q, want 'custom-value'", val)
	}
}

func TestResolveSecretKey_KeyNotFound(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "ns1"},
		Data:       map[string][]byte{"other": []byte("val")},
	}
	m := New(fakeClient(secret), testLogger())
	_, err := m.resolveSecretKey(context.Background(), "ns1", &aiv1alpha1.SecretKeySelector{
		Name: "my-secret",
		Key:  "missing-key",
	})
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// --- LoadPersisted tests ---

func TestLoadPersisted_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: providerConfigMapName, Namespace: "k8ops-system"},
		Data: map[string]string{
			"type":  "openai",
			"model": "gpt-4o",
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: providerSecretName, Namespace: "k8ops-system"},
		Data:       map[string][]byte{"apiKey": []byte("persisted-key")},
	}
	m := New(fakeClient(cm, secret), testLogger())
	err := m.LoadPersisted(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Get() == nil {
		t.Fatal("expected non-nil provider")
	}
	if m.GetConfig().Type != "openai" {
		t.Errorf("type = %q", m.GetConfig().Type)
	}
}

func TestLoadPersisted_NoConfigMap(t *testing.T) {
	m := New(fakeClient(), testLogger())
	err := m.LoadPersisted(context.Background())
	if err == nil {
		t.Error("expected error when ConfigMap missing")
	}
}

func TestLoadPersisted_NoSecret(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: providerConfigMapName, Namespace: "k8ops-system"},
		Data:       map[string]string{"type": "openai"},
	}
	m := New(fakeClient(cm), testLogger())
	err := m.LoadPersisted(context.Background())
	if err == nil {
		t.Error("expected error when Secret missing")
	}
}

func TestLoadPersisted_IncompleteConfig(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: providerConfigMapName, Namespace: "k8ops-system"},
		Data:       map[string]string{"type": ""}, // empty type
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: providerSecretName, Namespace: "k8ops-system"},
		Data:       map[string][]byte{"apiKey": []byte("key")},
	}
	m := New(fakeClient(cm, secret), testLogger())
	err := m.LoadPersisted(context.Background())
	if err == nil {
		t.Error("expected error for incomplete config")
	}
}

// --- Status tests ---

func TestStatus_AfterReload(t *testing.T) {
	m := New(nil, testLogger())
	m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:     "openai",
		APIKey:   "key",
		Model:    "gpt-4o",
		Endpoint: "https://api.openai.com",
	})
	s := m.Status()
	if !s.Active {
		t.Error("expected active")
	}
	if s.Type != "openai" {
		t.Errorf("type = %q", s.Type)
	}
	if s.Model != "gpt-4o" {
		t.Errorf("model = %q", s.Model)
	}
	if s.Endpoint != "https://api.openai.com" {
		t.Errorf("endpoint = %q", s.Endpoint)
	}
	if s.LastReload == "" {
		t.Error("expected non-empty LastReload")
	}
}

func TestStatus_WithReloadTimestamp(t *testing.T) {
	m := New(nil, testLogger())
	m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "k",
		Model:  "m",
	})
	s := m.Status()
	if s.LastReload == "" {
		t.Error("expected non-empty LastReload after reload")
	}
}

// --- GetConfig tests ---

func TestGetConfig_InitialState(t *testing.T) {
	m := New(nil, testLogger())
	cfg := m.GetConfig()
	if cfg.Type != "" {
		t.Errorf("expected empty type, got %q", cfg.Type)
	}
}

func TestGetConfig_AfterReload(t *testing.T) {
	m := New(nil, testLogger())
	m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:        "openai",
		APIKey:      "k",
		Model:       "gpt-4o",
		MaxTokens:   4096,
		Temperature: 0.7,
	})
	cfg := m.GetConfig()
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %v", cfg.Temperature)
	}
}

// --- Get tests ---

func TestGet_InitiallyNil(t *testing.T) {
	m := New(nil, testLogger())
	if m.Get() != nil {
		t.Error("expected nil provider initially")
	}
}

// --- Concurrent access test ---

func TestManager_ConcurrentGetAndReload(t *testing.T) {
	m := New(nil, testLogger())

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
				Type:   "openai",
				APIKey: fmt.Sprintf("key-%d", i),
				Model:  "gpt-4o",
			})
		}
	}()

	// Reader goroutines
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = m.Get()
				_ = m.Status()
				_ = m.GetConfig()
				_ = m.LastReload()
			}
		}()
	}

	<-done
}
