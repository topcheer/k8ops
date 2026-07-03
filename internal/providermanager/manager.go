// Package providermanager provides hot-reloadable provider management.
// It watches K8opsConfig CR changes and swaps the active provider atomically
// without restarting the manager process.
package providermanager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/provider"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager holds the currently active provider, swappable at runtime.
type Manager struct {
	mu         sync.RWMutex
	provider   provider.Provider
	config     provider.ProviderConfig
	k8sClient  client.Client
	log        *slog.Logger
	lastReload time.Time
	namespace  string
}

const (
	// providerConfigMapName is the ConfigMap storing non-secret provider settings.
	providerConfigMapName = "k8ops-provider-config"
	// providerSecretName is the Secret storing the API key.
	providerSecretName = "k8ops-provider-secret"
)

// New creates a provider manager. Initial config must be loaded via Reload().
func New(k8sClient client.Client, log *slog.Logger) *Manager {
	return &Manager{
		k8sClient: k8sClient,
		log:       log,
		namespace: "k8ops-system",
	}
}

// Get returns the current provider.
func (m *Manager) Get() provider.Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// GetConfig returns the current provider config.
func (m *Manager) GetConfig() provider.ProviderConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// LastReload returns the timestamp of the last successful reload.
func (m *Manager) LastReload() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastReload
}

// Reload fetches the latest K8opsConfig + Secret and swaps the provider.
func (m *Manager) Reload(ctx context.Context) error {
	configs := &aiv1alpha1.K8opsConfigList{}
	if err := m.k8sClient.List(ctx, configs); err != nil {
		return fmt.Errorf("failed to list K8opsConfig: %w", err)
	}
	if len(configs.Items) == 0 {
		return fmt.Errorf("no K8opsConfig found")
	}
	return m.ReloadFromConfig(ctx, &configs.Items[0])
}

// ReloadFromConfig builds a provider from a K8opsConfig CR and swaps it in.
func (m *Manager) ReloadFromConfig(ctx context.Context, cfg *aiv1alpha1.K8opsConfig) error {
	pCfg := provider.ProviderConfig{
		Type:        cfg.Spec.Provider.Type,
		Model:       cfg.Spec.Provider.Model,
		Endpoint:    cfg.Spec.Provider.Endpoint,
		MaxTokens:   cfg.Spec.Provider.MaxTokens,
		Temperature: cfg.Spec.Provider.Temperature,
	}

	// Resolve API key from Secret
	if cfg.Spec.Provider.APIKeySecretRef != nil {
		key, err := m.resolveSecretKey(ctx, cfg.Namespace, cfg.Spec.Provider.APIKeySecretRef)
		if err != nil {
			return fmt.Errorf("failed to resolve API key: %w", err)
		}
		pCfg.APIKey = key
	}

	newProvider, err := provider.New(pCfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	m.mu.Lock()
	old := m.provider
	m.provider = newProvider
	m.config = pCfg
	m.lastReload = time.Now()
	m.mu.Unlock()

	if old != nil {
		m.log.Info("provider hot-reloaded",
			"type", pCfg.Type, "model", pCfg.Model,
			"endpoint", pCfg.Endpoint)
	} else {
		m.log.Info("provider initialized",
			"type", pCfg.Type, "model", pCfg.Model)
	}

	return nil
}

// ReloadFromDirect allows direct config injection (e.g. from dashboard).
// The config is persisted to a ConfigMap (non-secret fields) + Secret (API key)
// so it survives Pod restarts and rescheduling.
func (m *Manager) ReloadFromDirect(ctx context.Context, pCfg provider.ProviderConfig) error {
	if pCfg.Type == "" {
		return fmt.Errorf("provider type is required")
	}

	// Create provider first to validate config
	newProvider, err := provider.New(pCfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Persist non-secret fields to ConfigMap (skip if no k8s client, e.g. in tests)
	if m.k8sClient != nil {
		if err := m.saveConfigMap(ctx, pCfg); err != nil {
			m.log.Warn("failed to persist provider ConfigMap", "error", err)
		}
		// Persist API key to Secret
		if err := m.saveSecret(ctx, pCfg.APIKey); err != nil {
			m.log.Warn("failed to persist provider Secret", "error", err)
		}
	}

	m.mu.Lock()
	old := m.provider
	m.provider = newProvider
	m.config = pCfg
	m.lastReload = time.Now()
	m.mu.Unlock()

	if old != nil {
		m.log.Info("provider hot-reloaded",
			"type", pCfg.Type, "model", pCfg.Model,
			"endpoint", pCfg.Endpoint)
	} else {
		m.log.Info("provider initialized",
			"type", pCfg.Type, "model", pCfg.Model)
	}
	return nil
}

// LoadPersisted tries to load provider config from ConfigMap + Secret.
// Called on startup before falling back to K8opsConfig CR.
func (m *Manager) LoadPersisted(ctx context.Context) error {
	pCfg, err := m.loadFromConfigMapAndSecret(ctx)
	if err != nil {
		return err
	}

	newProvider, err := provider.New(pCfg)
	if err != nil {
		return fmt.Errorf("failed to create provider from persisted config: %w", err)
	}

	m.mu.Lock()
	m.provider = newProvider
	m.config = pCfg
	m.lastReload = time.Now()
	m.mu.Unlock()

	m.log.Info("provider loaded from ConfigMap/Secret",
		"type", pCfg.Type, "model", pCfg.Model)
	return nil
}

// saveConfigMap writes non-secret provider fields to a ConfigMap.
func (m *Manager) saveConfigMap(ctx context.Context, pCfg provider.ProviderConfig) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerConfigMapName,
			Namespace: m.namespace,
		},
		Data: map[string]string{
			"type":        pCfg.Type,
			"model":       pCfg.Model,
			"endpoint":    pCfg.Endpoint,
			"maxTokens":   fmt.Sprintf("%d", pCfg.MaxTokens),
			"temperature": fmt.Sprintf("%v", pCfg.Temperature),
		},
	}

	existing := &corev1.ConfigMap{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{Name: providerConfigMapName, Namespace: m.namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return m.k8sClient.Create(ctx, cm)
		}
		return err
	}
	existing.Data = cm.Data
	return m.k8sClient.Update(ctx, existing)
}

// saveSecret writes the API key to a Kubernetes Secret.
func (m *Manager) saveSecret(ctx context.Context, apiKey string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerSecretName,
			Namespace: m.namespace,
		},
		Data: map[string][]byte{
			"apiKey": []byte(apiKey),
		},
	}

	existing := &corev1.Secret{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{Name: providerSecretName, Namespace: m.namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return m.k8sClient.Create(ctx, secret)
		}
		return err
	}
	existing.Data = secret.Data
	return m.k8sClient.Update(ctx, existing)
}

// loadFromConfigMapAndSecret reconstructs ProviderConfig from ConfigMap + Secret.
func (m *Manager) loadFromConfigMapAndSecret(ctx context.Context) (provider.ProviderConfig, error) {
	cm := &corev1.ConfigMap{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{Name: providerConfigMapName, Namespace: m.namespace}, cm); err != nil {
		return provider.ProviderConfig{}, fmt.Errorf("ConfigMap %s not found: %w", providerConfigMapName, err)
	}

	secret := &corev1.Secret{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{Name: providerSecretName, Namespace: m.namespace}, secret); err != nil {
		return provider.ProviderConfig{}, fmt.Errorf("Secret %s not found: %w", providerSecretName, err)
	}

	pCfg := provider.ProviderConfig{
		Type:     cm.Data["type"],
		Model:    cm.Data["model"],
		Endpoint: cm.Data["endpoint"],
	}

	if v, ok := secret.Data["apiKey"]; ok {
		pCfg.APIKey = string(v)
	}

	if pCfg.Type == "" || pCfg.APIKey == "" {
		return provider.ProviderConfig{}, fmt.Errorf("persisted config is incomplete")
	}

	return pCfg, nil
}

func (m *Manager) resolveSecretKey(ctx context.Context, namespace string, ref *aiv1alpha1.SecretKeySelector) (string, error) {
	key := ref.Key
	if key == "" {
		key = "apiKey"
	}

	secret := &corev1.Secret{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, ref.Name, err)
	}

	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key '%s' not found in secret %s/%s", key, namespace, ref.Name)
	}
	return string(val), nil
}

// Status returns the current status for dashboard display.
type Status struct {
	Type       string `json:"type"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint,omitempty"`
	HasAPIKey  bool   `json:"hasApiKey"`
	LastReload string `json:"lastReload"`
	Active     bool   `json:"active"`
}

// Status returns the current provider status.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := Status{Active: m.provider != nil}
	if m.lastReload.IsZero() {
		status.LastReload = ""
	} else {
		status.LastReload = m.lastReload.Format(time.RFC3339)
	}
	status.Type = m.config.Type
	status.Model = m.config.Model
	status.Endpoint = m.config.Endpoint
	status.HasAPIKey = m.config.APIKey != ""
	return status
}
