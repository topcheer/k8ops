/*
Copyright 2024 ggai.dev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Import all provider packages for side-effect registration
	_ "github.com/ggai/k8ops/internal/provider/anthropic"
	_ "github.com/ggai/k8ops/internal/provider/gemini"
	_ "github.com/ggai/k8ops/internal/provider/openai"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/audit"
	"github.com/ggai/k8ops/internal/auth"
	"github.com/ggai/k8ops/internal/chat"
	"github.com/ggai/k8ops/internal/collector"
	"github.com/ggai/k8ops/internal/dashboard"
	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/providermanager"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
	"github.com/ggai/k8ops/internal/controller/diagnostic"
	"github.com/ggai/k8ops/internal/controller/optimization"
	"github.com/ggai/k8ops/internal/controller/remediation"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(aiv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		providerType         string
		providerModel        string
		providerEndpoint     string
		providerAPIKey       string
		disableEventCollector bool
		dashboardAddr        string
		authDBPath           string
		authDBDriver         string
		authDBDSN            string
		authJWTSecret        string
	)

	const chatSystemPrompt = `You are k8ops AI, a Kubernetes AIOps assistant integrated into a dashboard.
	You can diagnose cluster issues, analyze resources, and suggest remediations.
	You have access to Kubernetes API tools and host node tools.

	You understand natural language queries about the cluster. When users ask questions like:
	- "what pods are running in default?" → use get_pods tool with namespace "default"
	- "show me nodes with high CPU" → use get_nodes, then analyze CPU usage
	- "why is my pod crashing?" → use get_pods, get_events, get_pod_status to investigate
	- "check the nginx deployment" → use get_deployments or get_pods with label selector
	- "what's wrong with the cluster?" → run a comprehensive diagnostic using multiple tools
	Always translate natural language into the appropriate tool calls automatically.

	When the user asks you to diagnose or investigate:
	1. Start by gathering relevant data using the available tools
	2. Analyze the data and identify root causes
	3. Provide clear, actionable recommendations

	Be concise but thorough. Use bullet points and code blocks for clarity.
	If you need more information, ask the user.
	If you discover a critical issue, highlight it with **CRITICAL**.`

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Bind address for the metrics server")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Bind address for the probe server")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election (not needed for DaemonSet mode)")
	flag.StringVar(&providerType, "provider-type", "openai", "AI provider type: openai, anthropic, gemini")
	flag.StringVar(&providerModel, "provider-model", "", "AI model name")
	flag.StringVar(&providerEndpoint, "provider-endpoint", "", "Custom AI API endpoint")
	flag.StringVar(&providerAPIKey, "provider-api-key", os.Getenv("AIOPS_API_KEY"), "AI provider API key")
	flag.BoolVar(&disableEventCollector, "disable-event-collector", false, "Disable automatic event-triggered diagnostics")
	flag.StringVar(&dashboardAddr, "dashboard-address", ":9090", "Address for the dashboard web UI")
	flag.StringVar(&authDBPath, "auth-db-path", os.Getenv("AUTH_DB_PATH"), "Path to auth database (default: /data/k8ops.db)")
	flag.StringVar(&authDBDriver, "auth-db-driver", func() string { d := os.Getenv("AUTH_DB_DRIVER"); if d == "" { return "sqlite" }; return d }(), "Database driver: sqlite (default), mysql, postgres")
	flag.StringVar(&authDBDSN, "auth-db-dsn", os.Getenv("AUTH_DB_DSN"), "Database DSN for mysql/postgres (e.g. 'user:pass@tcp(host:3306)/dbname' or 'host=localhost user=postgres dbname=k8ops')")
	flag.StringVar(&authJWTSecret, "auth-jwt-secret", os.Getenv("AUTH_JWT_SECRET"), "JWT signing secret for auth")
	flag.Parse()

	opts := zap.Options{
		Development: false,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "k8ops.ggai.dev",
	})
	if err != nil {
		logger.Error("unable to start manager", "error", err)
		os.Exit(1)
	}

	// Build default provider config (can be overridden by K8opsConfig CRD)
	providerCfg := provider.ProviderConfig{
		Type:     providerType,
		Model:    providerModel,
		APIKey:   providerAPIKey,
		Endpoint: providerEndpoint,
	}

	// Setup controllers
	if err := (&diagnostic.DiagnosticReconciler{
		Client:       mgr.GetClient(),
		Scheme:       scheme,
		Config:       mgr.GetConfig(),
		Log:          logger,
		ProviderCfg:  providerCfg,
	}).SetupWithManager(mgr); err != nil {
		logger.Error("unable to create diagnostic controller", "error", err)
		os.Exit(1)
	}

	if err := (&remediation.RemediationReconciler{
		Client: mgr.GetClient(),
		Scheme: scheme,
		Config: mgr.GetConfig(),
		Log:    logger,
	}).SetupWithManager(mgr); err != nil {
		logger.Error("unable to create remediation controller", "error", err)
		os.Exit(1)
	}

	if err := (&optimization.OptimizationReconciler{
		Client:      mgr.GetClient(),
		Scheme:      scheme,
		Log:         logger,
		ProviderCfg: providerCfg,
	}).SetupWithManager(mgr); err != nil {
		logger.Error("unable to create optimization controller", "error", err)
		os.Exit(1)
	}

	// Custom signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Hoisted so the signal handler goroutine can access them.
	var dash *dashboard.Server
	var authn *auth.Authenticator

	go func() {
		sig := <-sigCh
		logger.Info("received signal, initiating graceful shutdown", "signal", sig.String())

		// 1. Dashboard HTTP server graceful shutdown (drains SSE connections).
		if dash != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := dash.Stop(shutdownCtx); err != nil {
				logger.Error("dashboard graceful shutdown error", "error", err)
			}
			shutdownCancel()
			logger.Info("dashboard server stopped")
		}

		// 2. Close auth DB (flush SQLite WAL).
		if authn != nil {
			if err := authn.Close(); err != nil {
				logger.Error("auth DB close error", "error", err)
			}
			logger.Info("auth database closed (WAL flushed)")
		}

		// 3. Signal the controller-runtime manager to stop.
		cancel()

		// 4. A second signal forces immediate exit.
		<-sigCh
		logger.Warn("second signal received, forcing exit")
		os.Exit(1)
	}()

	if !disableEventCollector {
		go func() {
			// Wait for provider to be loaded from ConfigMap/Secret
			time.Sleep(10 * time.Second)
			collectorInstance, err := collector.NewEventCollector(mgr.GetClient(), mgr.GetConfig(), logger)
			if err != nil {
				logger.Error("unable to create event collector", "error", err)
				return
			}
			collectorInstance.Start(ctx)
			logger.Info("event collector started")
		}()
	}

	// Start dashboard server
	if dashboardAddr != "" {
		auditLogPath := os.Getenv("K8OPS_AUDIT_LOG")
		if auditLogPath == "" {
			auditLogPath = "/tmp/k8ops-audit.log"
		}
		auditLog, err := audit.NewLogger(auditLogPath, logger)
		if err != nil {
			logger.Error("unable to create audit logger", "error", err)
			auditLog = audit.NoopLogger(logger)
		}

		// Provider manager with hot-reload
		providerMgr := providermanager.New(mgr.GetClient(), logger)

		// Initial provider load: try ConfigMap/Secret first, then K8opsConfig CR
		go func() {
			time.Sleep(3 * time.Second) // wait for cache to sync

			// 1. Try ConfigMap + Secret (dashboard-configured provider)
			if err := providerMgr.LoadPersisted(context.Background()); err != nil {
				logger.Info("no persisted provider config, trying K8opsConfig CR", "reason", err)

				// 2. Fall back to K8opsConfig CR
				if err := providerMgr.Reload(context.Background()); err != nil {
					logger.Warn("provider load failed; configure via dashboard Settings tab",
						"error", err)
				}
			}
		}()

		// Build tool registry for chat
		kubeClient, err := k8s.NewKubeClientFromConfig(mgr.GetConfig())
		if err != nil {
			logger.Error("unable to create kube client for chat", "error", err)
		}

		var chatEngine *chat.Engine
		if kubeClient != nil {
			registry := tools.NewRegistry()
			for _, t := range []tools.Tool{
				&k8s.GetResourceTool{Client: kubeClient},
				&k8s.ListResourcesTool{Client: kubeClient},
				&k8s.DescribeResourceTool{Client: kubeClient},
				&k8s.GetPodLogsTool{Client: kubeClient},
				&k8s.GetEventsTool{Client: kubeClient},
				&k8s.GetNamespacesTool{Client: kubeClient},
				&k8s.GetTopTool{Client: kubeClient},
				&k8s.GetPodStatusTool{Client: kubeClient},
				&k8s.GetServicesTool{Client: kubeClient},
				&k8s.GetNodesTool{Client: kubeClient},
				&k8s.GetStorageTool{Client: kubeClient},
				&k8s.GetConfigMapTool{Client: kubeClient},
				&k8s.GetIngressTool{Client: kubeClient},
				&k8s.GetClusterVersionTool{Client: kubeClient},
				&host.HostInfoTool{},
				&host.HostDiskUsageTool{},
				&host.HostNetworkTool{},
				&host.HostProcessTool{},
				&host.HostDmesgTool{},
			} {
				registry.Register(t)
			}

			chatEngine = chat.NewEngine(
				func() provider.Provider { return providerMgr.Get() },
				registry,
				auditLog,
				chatSystemPrompt,
				logger,
			)
		}

		dash, err = dashboard.New(mgr.GetClient(), mgr.GetConfig(), scheme, auditLog, logger)
		if err != nil {
			logger.Error("unable to create dashboard", "error", err)
		} else {
			dash.SetChatEngine(chatEngine)
			dash.SetProviderManager(providerMgr)

			// TLS support: enable HTTPS if cert/key are configured
			tlsCert := os.Getenv("DASHBOARD_TLS_CERT")
			tlsKey := os.Getenv("DASHBOARD_TLS_KEY")
			if tlsCert != "" && tlsKey != "" {
				dash.SetTLS(tlsCert, tlsKey)
				logger.Info("dashboard TLS enabled", "cert", tlsCert)
			} else {
				logger.Warn("dashboard running without TLS; set DASHBOARD_TLS_CERT/DASHBOARD_TLS_KEY to enable HTTPS")
			}

			// Initialize auth if enabled
			authCfg := &auth.Config{
				DBDriver:    authDBDriver,
				DBDSN:       authDBDSN,
				DBPath:      authDBPath,
				JWTSecret:   authJWTSecret,
				JWTExpiry:   24 * time.Hour,
				DefaultRole: os.Getenv("AUTH_DEFAULT_ROLE"),
			}
			if authCfg.DBPath == "" {
				authCfg.DBPath = "/data/k8ops.db"
			}
			if authCfg.JWTSecret == "" {
				// Generate a random secret if none provided
				authCfg.JWTSecret = generateJWTSecret()
				logger.Info("auth: generated ephemeral JWT secret (set AUTH_JWT_SECRET for persistence)")
			}

			authn, err = auth.New(authCfg)
			if err != nil {
				logger.Error("unable to initialize auth", "error", err)
				dash.SetAuthRequired(err.Error()) // Fail-closed: block API access
			} else {
				dash.SetAuthenticator(authn)
				// Wire up RBAC syncer for namespace-scoped users
				if kubeClientset, err := kubernetes.NewForConfig(mgr.GetConfig()); err == nil {
					authn.SetRBACSyncer(auth.NewRBACSyncer(kubeClientset))
				}
				logger.Info("auth initialized",
					"driver", func() string { d := authCfg.DBDriver; if d == "" { return "sqlite" }; return d }())
			}

			go func() {
				if err := dash.Start(dashboardAddr); err != nil {
					logger.Error("dashboard server error", "error", err)
				}
			}()
		}
	}

	// Health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error("unable to set up health check", "error", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error("unable to set up ready check", "error", err)
		os.Exit(1)
	}

	logger.Info("starting k8ops manager",
		"provider", providerType,
		"model", providerModel,
		"metrics", metricsAddr,
		"probe", probeAddr,
		"dashboard", dashboardAddr,
	)

	if err := mgr.Start(ctx); err != nil {
		logger.Error("problem running manager", "error", err)
		os.Exit(1)
	}

	logger.Info("graceful shutdown complete")
	_ = fmt.Sprintf
	_ = time.Now
}

// generateJWTSecret creates a random 32-byte hex string for JWT signing.
func generateJWTSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-based secret (not ideal but won't crash)
		return fmt.Sprintf("k8ops-fallback-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
