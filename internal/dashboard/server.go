// Package dashboard provides an embedded HTTP dashboard for k8ops.
// It serves a single-page web UI and REST APIs for querying diagnostics,
// remediations, optimizations, and cluster health.
package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"github.com/ggai/k8ops/internal/audit"
	"github.com/ggai/k8ops/internal/auth"
	"github.com/ggai/k8ops/internal/chat"
	_ "github.com/ggai/k8ops/internal/metrics" // register Prometheus metrics (promauto)
	"github.com/ggai/k8ops/internal/providermanager"
	"github.com/ggai/k8ops/internal/tools/k8s"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed web/*
var webFS embed.FS

// Server is the dashboard HTTP server.
type Server struct {
	k8sClient          client.Client
	clientset          *kubernetes.Clientset
	restConfig         *rest.Config
	scheme             *runtime.Scheme
	auditLog           *audit.Logger
	chatEngine         *chat.Engine
	providerMgr        *providermanager.Manager
	k8sClientTool      *k8s.KubeClient
	cache              *responseCache
	chatLimiter        *userRateLimiter // per-user rate limiter for LLM calls
	auth               *auth.Authenticator
	authRequired       bool   // true if auth was requested but failed to init (fail-closed)
	authFailedMsg      string // error message when auth init failed
	log                *slog.Logger
	server             *http.Server
	corsAllowedOrigins []string
	tlsCert            string
	tlsKey             string
	startTime          *time.Time
	perfTracker        *apiPerformanceTracker

	// Graceful shutdown state
	draining       atomic.Bool  // true when server is draining (SIGTERM received)
	activeConns    atomic.Int64 // number of in-flight HTTP connections
	shutdownSignal atomic.Bool  // true when graceful shutdown has been initiated
}

// New creates a new dashboard server.
func New(k8sClient client.Client, config *rest.Config, scheme *runtime.Scheme, auditLog *audit.Logger, log *slog.Logger) (*Server, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	kubeClient, err := k8s.NewKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %w", err)
	}
	allowedOrigins := parseCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if len(allowedOrigins) > 0 {
		log.Info("CORS: allowed origins configured", "origins", allowedOrigins)
	} else {
		log.Info("CORS: no allowed origins configured (same-origin only)")
	}

	now := time.Now()
	return &Server{
		k8sClient:          k8sClient,
		clientset:          clientset,
		restConfig:         config,
		scheme:             scheme,
		auditLog:           auditLog,
		k8sClientTool:      kubeClient,
		cache:              newResponseCache(10 * time.Minute),
		log:                log,
		corsAllowedOrigins: allowedOrigins,
		startTime:          &now,
		perfTracker:        newAPIPerformanceTracker(5000),
	}, nil
}

// Start starts the dashboard HTTP server.
// If TLS cert and key files are configured (via DASHBOARD_TLS_CERT/DASHBOARD_TLS_KEY
// env vars or SetTLS), the server uses HTTPS; otherwise it falls back to plain HTTP.
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// Serve embedded frontend
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("failed to get web subfs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	// API routes
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealthz) // K8s liveness probe
	mux.HandleFunc("/readyz", s.handleReadyz)   // K8s readiness probe
	mux.HandleFunc("/api/version", s.handleVersion)

	// System & log management
	mux.HandleFunc("/api/system/info", s.handleSystemInfo)
	mux.HandleFunc("/api/system/log/rotate", s.adminOnlyMiddleware(s.handleLogRotate))
	mux.HandleFunc("/api/system/log/cleanup", s.adminOnlyMiddleware(s.handleLogCleanup))
	mux.HandleFunc("/api/system/performance", s.cacheMiddleware(15*time.Second, s.handleAPIPerformance))

	// Backup management
	mux.HandleFunc("/api/system/backup", s.handleBackupDispatch)
	mux.HandleFunc("/api/exec", s.handleQuickExec) // NL-to-kubectl quick command execution
	mux.HandleFunc("/api/cluster/overview", s.cacheMiddleware(30*time.Second, s.handleClusterOverview))
	mux.HandleFunc("/api/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("/api/diagnostics/restarts", s.cacheMiddleware(30*time.Second, s.handleRestartDiagnosis)) // pod restart diagnosis
	mux.HandleFunc("/api/diagnostics/history", s.handleDiagnosticsHistory)                                   // must be before catch-all
	mux.HandleFunc("/api/diagnostics/", s.handleDiagnosticDetail)
	mux.HandleFunc("/api/remediations", s.handleRemediations)
	mux.HandleFunc("/api/remediation/", s.handleRemediationAction)
	mux.HandleFunc("/api/optimizations", s.handleOptimizations)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/nodes", s.cacheMiddleware(30*time.Second, s.handleNodes))
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/events/summary", s.cacheMiddleware(30*time.Second, s.handleEventSummary)) // 30s cache
	mux.HandleFunc("/api/events/stream", s.handleEventsStream)                                     // SSE real-time
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/audit/stats", s.handleAuditStats)
	mux.HandleFunc("/api/audit/events", s.handleAuditEvents)
	mux.HandleFunc("/api/audit/export", s.handleAuditExport)
	mux.HandleFunc("/api/audit/events/", s.handleAuditEventDetail)
	mux.HandleFunc("/api/pods", s.cacheMiddleware(30*time.Second, s.handlePods))
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/conversations", s.handleConversations)
	mux.HandleFunc("/api/provider/status", s.handleProviderStatus)
	mux.HandleFunc("/api/provider/update", s.handleProviderUpdate)
	mux.HandleFunc("/api/provider/reload", s.handleProviderReload)
	mux.HandleFunc("/api/tools", s.handleToolList)

	// Resource browser + drill-down
	mux.HandleFunc("/api/nodes/", s.handleNodePods)                                               // /api/nodes/{node}/pods
	mux.HandleFunc("/api/pods/", s.handlePodActions)                                              // /api/pods/{ns}/{name}/logs|exec|containers
	mux.HandleFunc("/api/resources", s.cacheMiddleware(60*time.Second, s.handleResources))        // 1min cache
	mux.HandleFunc("/api/crds", s.cacheMiddleware(10*time.Minute, s.handleCRDs))                  // 10min cache (expensive with_counts)
	mux.HandleFunc("/api/crd-resources", s.cacheMiddleware(60*time.Second, s.handleCRDResources)) // 1min cache
	mux.HandleFunc("/api/yaml", s.handleYAML)                                                     // view YAML of any resource
	mux.HandleFunc("/api/yaml/apply", s.handleYAMLApply)                                          // apply YAML (kubectl apply)
	mux.HandleFunc("/api/scale", s.handleScale)                                                   // scale deployment/statefulset
	mux.HandleFunc("/api/pod/delete", s.handlePodDelete)                                          // delete a single pod
	mux.HandleFunc("/api/rollout/restart", s.handleRolloutRestart)                                // restart deployment/daemonset/statefulset
	mux.HandleFunc("/api/node/cordon", s.handleNodeCordon)                                        // cordon/uncordon node
	mux.HandleFunc("/api/resource/data", s.handleResourceData)                                    // configmap/secret data viewer

	// Security audit
	mux.HandleFunc("/api/security/audit", s.handleSecurityAudit)
	mux.HandleFunc("/api/security/secrets", s.cacheMiddleware(60*time.Second, s.handleSecretScan))           // 1min cache                // cluster-wide security scan
	mux.HandleFunc("/api/security/network-policies", s.cacheMiddleware(60*time.Second, s.handleNetPolAudit)) // NetworkPolicy audit
	mux.HandleFunc("/api/security/health", s.handleSecurityHealth)                                           // platform security health check
	mux.HandleFunc("/api/security/compliance", s.handleComplianceScan)                                       // CIS benchmark compliance scan
	mux.HandleFunc("/api/security/compliance/report", s.handleComplianceReport)                              // downloadable compliance report

	// OpenAPI documentation
	mux.HandleFunc("/api/openapi.json", s.handleOpenAPISpec)                                                    // OpenAPI 3.0 spec
	mux.HandleFunc("/api/docs", s.handleAPIDocs)                                                                // API documentation (JSON + metadata)
	mux.HandleFunc("/api/docs/platform-maturity", s.cacheMiddleware(300*time.Second, s.handlePlatformMaturity)) // platform maturity assessment & capability matrix (5min cache)

	// Cost / FinOps
	mux.HandleFunc("/api/cost/summary", s.cacheMiddleware(60*time.Second, s.handleCostSummary))                 // 1min cache
	mux.HandleFunc("/api/cost/recommendations", s.cacheMiddleware(60*time.Second, s.handleCostRecommendations)) // 1min cache

	// Namespace resource ranking
	mux.HandleFunc("/api/namespaces/ranking", s.cacheMiddleware(60*time.Second, s.handleNamespaceRanking)) // 1min cache
	mux.HandleFunc("/api/namespaces/", s.handleNamespaceDetail)                                            // /api/namespaces/{name}/detail

	// HPA visualization
	mux.HandleFunc("/api/hpa", s.cacheMiddleware(30*time.Second, s.handleHPAList)) // 30s cache

	// Container image inventory
	mux.HandleFunc("/api/images", s.cacheMiddleware(60*time.Second, s.handleImageInventory)) // 1min cache

	// Storage & Capacity Planning
	mux.HandleFunc("/api/storage/capacity", s.cacheMiddleware(60*time.Second, s.handleStorageCapacity)) // 1min cache
	mux.HandleFunc("/api/capacity/planning", s.cacheMiddleware(60*time.Second, s.handleCapacityPlanning))
	mux.HandleFunc("/api/capacity/forecast", s.cacheMiddleware(120*time.Second, s.handleCapacityForecast)) // 2min cache

	// Cluster efficiency analysis
	mux.HandleFunc("/api/efficiency", s.cacheMiddleware(60*time.Second, s.handleEfficiency))

	// Pod Disruption Budgets
	mux.HandleFunc("/api/pdbs", s.cacheMiddleware(30*time.Second, s.handlePDBList))                                             // 1min cache
	mux.HandleFunc("/api/compatibility", s.cacheMiddleware(60*time.Second, s.handleCompatibility))                              // 1min cache
	mux.HandleFunc("/api/certificates/expiry", s.cacheMiddleware(120*time.Second, s.handleCertExpiryScan))                      // 2min cache
	mux.HandleFunc("/api/system/drain-status", s.handleDrainStatus)                                                             // server draining/shutdown observability
	mux.HandleFunc("/api/addons/health", s.cacheMiddleware(120*time.Second, s.handleAddonScan))                                 // 2min cache
	mux.HandleFunc("/api/deployments/rollout", s.cacheMiddleware(30*time.Second, s.handleRolloutStatus))                        // deployment rollout health
	mux.HandleFunc("/api/resources/waste", s.cacheMiddleware(60*time.Second, s.handleWasteDetection))                           // resource waste detection
	mux.HandleFunc("/api/resources/quota", s.cacheMiddleware(60*time.Second, s.handleQuotaMonitor))                             // ResourceQuota & LimitRange monitor
	mux.HandleFunc("/api/scaling/bottlenecks", s.cacheMiddleware(60*time.Second, s.handleScalingBottlenecks))                   // scaling bottleneck detection
	mux.HandleFunc("/api/security/rbac-risk", s.cacheMiddleware(120*time.Second, s.handleRBACRiskScan))                         // RBAC permission risk analysis
	mux.HandleFunc("/api/security/service-accounts", s.cacheMiddleware(120*time.Second, s.handleSAAudit))                       // ServiceAccount security audit
	mux.HandleFunc("/api/operations/cronjobs/health", s.cacheMiddleware(60*time.Second, s.handleCronJobHealth))                 // cronjob execution health
	mux.HandleFunc("/api/operations/slo", s.cacheMiddleware(15*time.Second, s.handleSLOReport))                                 // SLO/SLA error budget
	mux.HandleFunc("/api/operations/event-storm", s.cacheMiddleware(30*time.Second, s.handleEventStorm))                        // event storm & cascade detection
	mux.HandleFunc("/api/operations/probes", s.cacheMiddleware(60*time.Second, s.handleProbeAudit))                             // health probe effectiveness audit
	mux.HandleFunc("/api/operations/health-score", s.cacheMiddleware(30*time.Second, s.handleHealthScore))                      // cluster health score aggregator
	mux.HandleFunc("/api/operations/node-pressure", s.cacheMiddleware(30*time.Second, s.handleNodePressure))                    // node condition & resource pressure
	mux.HandleFunc("/api/operations/oom-tracker", s.cacheMiddleware(30*time.Second, s.handleOOMTracker))                        // container OOM kill tracker
	mux.HandleFunc("/api/operations/crashloop", s.cacheMiddleware(30*time.Second, s.handleCrashLoop))                           // CrashLoopBackOff detector & crash pattern analyzer
	mux.HandleFunc("/api/operations/pdb-audit", s.cacheMiddleware(60*time.Second, s.handlePDBAudit))                            // PDB compliance & voluntary disruption risk
	mux.HandleFunc("/api/operations/topology-distribution", s.cacheMiddleware(60*time.Second, s.handleTopologySpread))          // topology spread & pod distribution audit
	mux.HandleFunc("/api/operations/image-pull-failures", s.cacheMiddleware(30*time.Second, s.handleImagePullFailures))         // image pull & container start failure tracker
	mux.HandleFunc("/api/operations/restart-reasons", s.cacheMiddleware(30*time.Second, s.handleRestartReasons))                // pod restart reason analyzer
	mux.HandleFunc("/api/operations/scheduling-latency", s.cacheMiddleware(30*time.Second, s.handleSchedulingLatency))          // pod scheduling latency analyzer
	mux.HandleFunc("/api/operations/resource-contention", s.cacheMiddleware(30*time.Second, s.handleResourceContention))        // resource contention & throttling detector
	mux.HandleFunc("/api/operations/node-lease", s.cacheMiddleware(30*time.Second, s.handleNodeLease))                          // node lease & heartbeat health monitor
	mux.HandleFunc("/api/operations/control-plane", s.cacheMiddleware(30*time.Second, s.handleControlPlaneHealth))              // control plane health checker
	mux.HandleFunc("/api/operations/pod-evictions", s.cacheMiddleware(30*time.Second, s.handlePodEviction))                     // pod eviction & node pressure history tracker
	mux.HandleFunc("/api/operations/api-latency", s.cacheMiddleware(30*time.Second, s.handleResponsiveness))                    // API server responsiveness & pod start latency monitor
	mux.HandleFunc("/api/operations/volume-mount-errors", s.cacheMiddleware(30*time.Second, s.handleVolumeMountErrors))         // volume mount & attach error tracker
	mux.HandleFunc("/api/operations/pod-startup", s.cacheMiddleware(30*time.Second, s.handlePodStartup))                        // pod startup lifecycle & bottleneck analyzer
	mux.HandleFunc("/api/operations/kubelet-health", s.cacheMiddleware(30*time.Second, s.handleKubeletHealth))                  // kubelet & container runtime health monitor
	mux.HandleFunc("/api/operations/dns-health", s.cacheMiddleware(30*time.Second, s.handleDNSHealth))                          // DNS resolution health & CoreDNS monitor
	mux.HandleFunc("/api/operations/csr-monitor", s.cacheMiddleware(30*time.Second, s.handleCSRMonitor))                        // certificate signing request & node bootstrap cert monitor
	mux.HandleFunc("/api/operations/etcd-health", s.cacheMiddleware(60*time.Second, s.handleEtcdHealth))                        // etcd health & database pressure monitor
	mux.HandleFunc("/api/operations/api-load", s.cacheMiddleware(30*time.Second, s.handleAPILoad))                              // API server request throughput & load pressure monitor
	mux.HandleFunc("/api/operations/prom-health", s.cacheMiddleware(120*time.Second, s.handlePromHealth))                       // Prometheus rule health & alert coverage auditor
	mux.HandleFunc("/api/operations/alertmanager-health", s.cacheMiddleware(120*time.Second, s.handleAlertmanager))             // Alertmanager config & alert routing health auditor
	mux.HandleFunc("/api/operations/grafana-health", s.cacheMiddleware(120*time.Second, s.handleGrafanaHealth))                 // Grafana dashboard availability & datasource health auditor
	mux.HandleFunc("/api/operations/metrics-pipeline", s.cacheMiddleware(120*time.Second, s.handleMetricsPipeline))             // metrics pipeline & kube-state-metrics health auditor
	mux.HandleFunc("/api/operations/audit-log-health", s.cacheMiddleware(120*time.Second, s.handleAuditLogHealth))              // audit log pipeline & event export health auditor
	mux.HandleFunc("/api/operations/alert-noise", s.cacheMiddleware(30*time.Second, s.handleAlertNoise))                        // alert noise & fatigue detection auditor
	mux.HandleFunc("/api/operations/apf-audit", s.cacheMiddleware(120*time.Second, s.handleAPFAudit))                           // API Priority & Fairness configuration auditor
	mux.HandleFunc("/api/networking/health", s.cacheMiddleware(30*time.Second, s.handleNetworkingHealth))                       // service & endpoint health
	mux.HandleFunc("/api/storage/health", s.cacheMiddleware(60*time.Second, s.handleStorageHealth))                             // PV/PVC storage health
	mux.HandleFunc("/api/deployments/audit", s.cacheMiddleware(60*time.Second, s.handleDeployAudit))                            // deployment config audit
	mux.HandleFunc("/api/scheduling/health", s.cacheMiddleware(30*time.Second, s.handleSchedulingHealth))                       // scheduling health & fragmentation
	mux.HandleFunc("/api/security/pods", s.cacheMiddleware(60*time.Second, s.handlePodSecurityScan))                            // pod security posture scan
	mux.HandleFunc("/api/security/secrets/rotation", s.cacheMiddleware(120*time.Second, s.handleSecretRotationAudit))           // secret lifecycle & rotation audit
	mux.HandleFunc("/api/security/secret-rotation-v2", s.cacheMiddleware(120*time.Second, s.handleSecretCompliance))            // secret rotation compliance & staleness tracker
	mux.HandleFunc("/api/security/images", s.cacheMiddleware(120*time.Second, s.handleImageSecurityAudit))                      // image supply chain security
	mux.HandleFunc("/api/security/containers", s.cacheMiddleware(120*time.Second, s.handleContainerSecurityAudit))              // container security context audit
	mux.HandleFunc("/api/security/rbac-effective", s.cacheMiddleware(120*time.Second, s.handleRBACEffective))                   // RBAC effective permissions & escalation
	mux.HandleFunc("/api/security/admission-audit", s.cacheMiddleware(120*time.Second, s.handleAdmissionAudit))                 // admission webhook configuration audit
	mux.HandleFunc("/api/security/audit-policy", s.cacheMiddleware(120*time.Second, s.handleAuditPolicy))                       // API server audit logging configuration checker
	mux.HandleFunc("/api/security/encryption-at-rest", s.cacheMiddleware(120*time.Second, s.handleEncryptionAtRest))            // secret encryption at rest configuration checker
	mux.HandleFunc("/api/security/host-namespace", s.cacheMiddleware(120*time.Second, s.handleHostNamespace))                   // container host namespace & privilege exposure auditor
	mux.HandleFunc("/api/security/cert-expiry", s.cacheMiddleware(120*time.Second, s.handleCertExpiry))                         // certificate & TLS expiry monitor
	mux.HandleFunc("/api/security/volume-mounts", s.cacheMiddleware(120*time.Second, s.handleVolumeSecurity))                   // volume & mount risk security audit
	mux.HandleFunc("/api/security/endpoint-exposure", s.cacheMiddleware(120*time.Second, s.handleEndpointExposure))             // service endpoint exposure & attack surface audit
	mux.HandleFunc("/api/security/seccomp-audit", s.cacheMiddleware(120*time.Second, s.handleSeccompAudit))                     // seccomp profile & PSS restricted compliance
	mux.HandleFunc("/api/security/batch-audit", s.cacheMiddleware(120*time.Second, s.handleBatchSecurity))                      // CronJob & batch job security audit
	mux.HandleFunc("/api/security/psa-audit", s.cacheMiddleware(120*time.Second, s.handlePSAAudit))                             // pod security admission enforcement auditor
	mux.HandleFunc("/api/security/mac-audit", s.cacheMiddleware(120*time.Second, s.handleMACAudit))                             // AppArmor & SELinux MAC compliance auditor
	mux.HandleFunc("/api/security/forensics", s.cacheMiddleware(30*time.Second, s.handleForensics))                             // pod security forensics & incident evidence collector
	mux.HandleFunc("/api/security/rbac-audit", s.cacheMiddleware(120*time.Second, s.handleRBACOverprivilege))                   // RBAC overprivilege & wildcard permission auditor
	mux.HandleFunc("/api/security/secret-scan", s.cacheMiddleware(120*time.Second, s.handleSecretScan))                         // secret data exposure & env var credential leak scanner
	mux.HandleFunc("/api/security/sec-drift", s.cacheMiddleware(120*time.Second, s.handleSecDrift))                             // security context drift & runtime policy compliance auditor
	mux.HandleFunc("/api/security/opa-compliance", s.cacheMiddleware(120*time.Second, s.handleOPACompliance))                   // OPA/Gatekeeper policy compliance & constraint violation auditor
	mux.HandleFunc("/api/security/image-vuln", s.cacheMiddleware(120*time.Second, s.handleImageVuln))                           // container image vulnerability & patch lag auditor
	mux.HandleFunc("/api/security/kyverno-compliance", s.cacheMiddleware(120*time.Second, s.handleKyvernoCompliance))           // Kyverno policy compliance & cluster policy audit
	mux.HandleFunc("/api/security/pss-scorecard", s.cacheMiddleware(120*time.Second, s.handlePSSScorecard))                     // Pod Security Standards compliance scorecard
	mux.HandleFunc("/api/security/sa-token-audit", s.cacheMiddleware(120*time.Second, s.handleSATokenAudit))                    // SA token rotation & access risk audit
	mux.HandleFunc("/api/security/supply-chain", s.cacheMiddleware(120*time.Second, s.handleSupplyChain))                       // supply chain & SBOM coverage security auditor
	mux.HandleFunc("/api/security/quota-security", s.cacheMiddleware(120*time.Second, s.handleQuotaSecurity))                   // resource quota & limit range security auditor
	mux.HandleFunc("/api/security/policy-drift", s.cacheMiddleware(120*time.Second, s.handlePolicyDrift))                       // security policy drift & baseline configuration auditor
	mux.HandleFunc("/api/operations/log-pipeline", s.cacheMiddleware(120*time.Second, s.handleLogPipeline))                     // log aggregation & forwarding pipeline health auditor
	mux.HandleFunc("/api/product/runtime-class", s.cacheMiddleware(120*time.Second, s.handleRuntimeClass))                      // container runtime class & OCI image compliance auditor
	mux.HandleFunc("/api/deployment/image-pull-audit", s.cacheMiddleware(120*time.Second, s.handleImagePullAudit))              // image pull policy & secret management auditor
	mux.HandleFunc("/api/scalability/vpa-audit", s.cacheMiddleware(120*time.Second, s.handleVPAAudit))                          // VPA configuration & resource recommendation quality auditor
	mux.HandleFunc("/api/product/mesh-traffic", s.cacheMiddleware(120*time.Second, s.handleMeshTraffic))                        // service mesh traffic management & circuit breaker health auditor
	mux.HandleFunc("/api/deployment/rollout-blocker", s.cacheMiddleware(120*time.Second, s.handleRolloutBlocker))               // deployment rollout blocker & pod condition auditor
	mux.HandleFunc("/api/security/pss-hardening", s.cacheMiddleware(120*time.Second, s.handlePSSHardening))                     // PSS enforcement gap & workload hardening auditor
	mux.HandleFunc("/api/operations/node-trend", s.cacheMiddleware(120*time.Second, s.handleNodeTrend))                         // node condition trend & hardware failure prediction auditor
	mux.HandleFunc("/api/product/endpoint-slice", s.cacheMiddleware(120*time.Second, s.handleEndpointSlice))                    // endpoint slice health & topology-aware routing auditor
	mux.HandleFunc("/api/scalability/saturation", s.cacheMiddleware(120*time.Second, s.handleSaturation))                       // resource saturation & CPU/memory throttling risk predictor
	mux.HandleFunc("/api/operations/registry-rate-limit", s.cacheMiddleware(120*time.Second, s.handleRegistryRateLimit))        // container image registry rate limit & pull reliability auditor
	mux.HandleFunc("/api/product/cert-manager", s.cacheMiddleware(120*time.Second, s.handleCertManager))                        // cert-manager health & certificate renewal pipeline auditor
	mux.HandleFunc("/api/deployment/quota-impact", s.cacheMiddleware(120*time.Second, s.handleDeployQuota))                     // deployment resource quota impact & namespace deployment capacity auditor
	mux.HandleFunc("/api/security/runtime-threat", s.cacheMiddleware(120*time.Second, s.handleRuntimeThreat))                   // runtime threat detection & container anomaly auditor
	mux.HandleFunc("/api/security/secret-posture", s.cacheMiddleware(120*time.Second, s.handleSecretPosture))                   // secret management posture & external secret integration auditor
	mux.HandleFunc("/api/security/namespace-posture", s.cacheMiddleware(120*time.Second, s.handleNamespaceSecurity))            // namespace security posture & trust boundary auditor
	mux.HandleFunc("/api/security/image-provenance", s.cacheMiddleware(120*time.Second, s.handleImageProvenance))               // container image provenance & registry trust auditor
	mux.HandleFunc("/api/security/threat-timeline", s.cacheMiddleware(60*time.Second, s.handleThreatTimeline))                  // security event timeline & threat detection pattern auditor
	mux.HandleFunc("/api/security/secret-age", s.cacheMiddleware(120*time.Second, s.handleSecretAge))                           // secret age & stale credential tracker
	mux.HandleFunc("/api/security/blast-radius", s.cacheMiddleware(120*time.Second, s.handleBlastRadius))                       // workload attack surface & blast radius analyzer
	mux.HandleFunc("/api/operations/cni-health", s.cacheMiddleware(120*time.Second, s.handleCNIHealth))                         // CNI plugin health & network stack configuration auditor
	mux.HandleFunc("/api/operations/observability-stack", s.cacheMiddleware(120*time.Second, s.handleObservabilityStack))       // observability stack integration health auditor
	mux.HandleFunc("/api/operations/incident-correlation", s.cacheMiddleware(30*time.Second, s.handleIncidentCorrelation))      // multi-signal incident correlation & root cause engine
	mux.HandleFunc("/api/product/service-topology", s.cacheMiddleware(120*time.Second, s.handleServiceTopology))                // cluster-wide service dependency topology & cascade risk analyzer
	mux.HandleFunc("/api/deployment/chaos-readiness", s.cacheMiddleware(120*time.Second, s.handleChaosReadiness))               // chaos engineering readiness assessment & experiment recommender
	mux.HandleFunc("/api/scalability/carbon-footprint", s.cacheMiddleware(300*time.Second, s.handleCarbonFootprint))            // cluster carbon footprint & sustainability analyzer
	mux.HandleFunc("/api/security/admission-policy-audit", s.cacheMiddleware(120*time.Second, s.handleAdmissionPolicyAudit))    // admission control policy gap & CEL expression auditor
	mux.HandleFunc("/api/operations/pod-anomaly", s.cacheMiddleware(60*time.Second, s.handlePodAnomaly))                        // pod performance anomaly & noisy neighbor detector
	mux.HandleFunc("/api/product/exposure-map", s.cacheMiddleware(120*time.Second, s.handleExposureMap))                        // cluster external exposure surface risk map
	mux.HandleFunc("/api/scalability/scale-simulator", s.cacheMiddleware(60*time.Second, s.handleScaleSimulator))               // workload scaling impact simulator
	mux.HandleFunc("/api/deployment/rollback-risk", s.cacheMiddleware(120*time.Second, s.handleRollbackRisk))                   // rollback risk & revision integrity assessor
	mux.HandleFunc("/api/operations/pod-lifecycle", s.cacheMiddleware(60*time.Second, s.handlePodLifecycle))                    // pod lifecycle stage analyzer & dwell-time tracker
	mux.HandleFunc("/api/security/rbac-graph", s.cacheMiddleware(120*time.Second, s.handleRBACGraph))                           // RBAC permission graph & escalation path analyzer
	mux.HandleFunc("/api/product/gateway-audit", s.cacheMiddleware(120*time.Second, s.handleGatewayAudit))                      // gateway API & ingress controller health audit
	mux.HandleFunc("/api/scalability/cost-allocation", s.cacheMiddleware(300*time.Second, s.handleCostAllocation))              // namespace cost allocation & chargeback report
	mux.HandleFunc("/api/deployment/gitops-audit", s.cacheMiddleware(120*time.Second, s.handleGitOpsAudit))                     // GitOps/CD pipeline health & config drift auditor
	mux.HandleFunc("/api/operations/metrics-pipeline-audit", s.cacheMiddleware(120*time.Second, s.handleMetricsPipelineHealth)) // metrics collection pipeline integrity audit
	mux.HandleFunc("/api/security/compliance-map", s.cacheMiddleware(120*time.Second, s.handleComplianceMap))                   // SOC2/PCI-DSS/HIPAA compliance mapping
	mux.HandleFunc("/api/product/probe-effectiveness", s.cacheMiddleware(120*time.Second, s.handleProbeEffect))                 // health probe effectiveness analyzer
	mux.HandleFunc("/api/scalability/node-upgrade-audit", s.cacheMiddleware(120*time.Second, s.handleNodeUpgrade))              // node upgrade readiness & K8s version compatibility auditor
	mux.HandleFunc("/api/operations/operator-health", s.cacheMiddleware(120*time.Second, s.handleOperatorHealth))               // cluster operator & OLM health auditor
	mux.HandleFunc("/api/operations/restart-storm", s.cacheMiddleware(60*time.Second, s.handleRestartStorm))                    // pod restart pattern & crashloop clustering auditor
	mux.HandleFunc("/api/operations/webhook-health", s.cacheMiddleware(120*time.Second, s.handleWebhookHealth))                 // admission webhook configuration health & performance risk auditor
	mux.HandleFunc("/api/operations/kube-proxy-health", s.cacheMiddleware(120*time.Second, s.handleKubeProxyHealth))            // kube-proxy & network routing stability auditor
	mux.HandleFunc("/api/operations/coredns-health", s.cacheMiddleware(120*time.Second, s.handleCoreDNSHealth))                 // CoreDNS configuration & resolution health auditor
	mux.HandleFunc("/api/scalability/budget-alert", s.cacheMiddleware(120*time.Second, s.handleBudgetAlert))                    // cost budget alert & namespace spending limit auditor
	mux.HandleFunc("/api/scalability/node-drain-readiness", s.cacheMiddleware(120*time.Second, s.handleNodeDrainReadiness))     // node drain & rotation readiness auditor
	mux.HandleFunc("/api/scalability/scaling-history", s.cacheMiddleware(120*time.Second, s.handleScalingHistory))              // cluster scaling history & autoscaler event timeline auditor
	mux.HandleFunc("/api/scalability/scheduling-fit", s.cacheMiddleware(120*time.Second, s.handleSchedulingFit))                // pod resource request density & scheduling fit auditor
	mux.HandleFunc("/api/scalability/quota-saturation", s.cacheMiddleware(120*time.Second, s.handleQuotaSaturation))
	mux.HandleFunc("/api/scalability/ext-resource-health", s.cacheMiddleware(120*time.Second, s.handleExtResourceHealth)) // extended resource & device plugin health auditor                  // namespace resource quota saturation & limit exhaustion predictor
	mux.HandleFunc("/api/scalability/reservation-audit", s.cacheMiddleware(120*time.Second, s.handleResvAudit))           // node resource reservation & allocatable gap analyzer
	mux.HandleFunc("/api/product/ingress-tls", s.cacheMiddleware(120*time.Second, s.handleIngressTLS))                    // ingress TLS certificate & HTTPS enforcement auditor
	mux.HandleFunc("/api/product/east-west-traffic", s.cacheMiddleware(120*time.Second, s.handleEastWestTraffic))         // east-west traffic & service-to-service connectivity auditor
	mux.HandleFunc("/api/product/port-exposure", s.cacheMiddleware(120*time.Second, s.handlePortExposure))                // container port exposure & named port consistency auditor
	mux.HandleFunc("/api/product/endpoint-mismatch", s.cacheMiddleware(60*time.Second, s.handleEndpointMismatch))         // service endpoint vs pod readiness mismatch auditor
	mux.HandleFunc("/api/deployment/env-config-drift", s.cacheMiddleware(120*time.Second, s.handleEnvConfigDrift))        // deployment env config drift & ConfigMap/Secret reference auditor
	mux.HandleFunc("/api/deployment/traceability", s.cacheMiddleware(120*time.Second, s.handleDeployTraceability))        // deployment reproducibility & CI/CD traceability auditor
	mux.HandleFunc("/api/deployment/termination-audit", s.cacheMiddleware(120*time.Second, s.handleTerminationAudit))     // pod termination message & exit code pattern auditor
	mux.HandleFunc("/api/deployment/readiness-gate", s.cacheMiddleware(60*time.Second, s.handleReadinessGate))            // pod readiness gate compliance & custom condition auditor
	mux.HandleFunc("/api/dependencies", s.cacheMiddleware(60*time.Second, s.handleDependencyGraph))                       // resource dependency graph & blast radius
	mux.HandleFunc("/api/topology/spread", s.cacheMiddleware(60*time.Second, s.handleTopologySpreadAudit))                // topology spread compliance
	mux.HandleFunc("/api/product/staleness", s.cacheMiddleware(60*time.Second, s.handleStalenessCheck))                   // workload staleness & release cadence
	mux.HandleFunc("/api/product/ingress-health", s.cacheMiddleware(60*time.Second, s.handleIngressHealth))               // ingress traffic routing health
	mux.HandleFunc("/api/product/namespaces/lifecycle", s.cacheMiddleware(60*time.Second, s.handleNamespaceLifecycle))    // namespace governance & lifecycle
	mux.HandleFunc("/api/product/dns-health", s.cacheMiddleware(60*time.Second, s.handleDNSHealth))                       // DNS resolution health checker
	mux.HandleFunc("/api/product/config-audit", s.cacheMiddleware(60*time.Second, s.handleConfigAudit))                   // ConfigMap & Secret configuration audit
	mux.HandleFunc("/api/product/network-policy", s.cacheMiddleware(60*time.Second, s.handleNetworkPolicyAudit))          // network policy compliance & traffic isolation
	mux.HandleFunc("/api/product/label-hygiene", s.cacheMiddleware(60*time.Second, s.handleLabelHygiene))                 // label & annotation hygiene auditor
	mux.HandleFunc("/api/product/orphaned-resources", s.cacheMiddleware(60*time.Second, s.handleOrphanedResources))       // orphaned resource detector
	mux.HandleFunc("/api/product/pvc-health", s.cacheMiddleware(60*time.Second, s.handlePVCHealth))                       // PV/PVC storage health & capacity
	mux.HandleFunc("/api/product/statefulset-audit", s.cacheMiddleware(60*time.Second, s.handleStatefulSetAudit))         // StatefulSet health & ordered rollout audit
	mux.HandleFunc("/api/product/affinity-conflict", s.cacheMiddleware(60*time.Second, s.handleAffinityConflict))         // affinity & anti-affinity conflict detector
	mux.HandleFunc("/api/product/taint-toleration", s.cacheMiddleware(60*time.Second, s.handleTaintToleration))           // node taint & pod toleration impact analyzer
	mux.HandleFunc("/api/product/configmap-size", s.cacheMiddleware(120*time.Second, s.handleConfigMapSize))              // ConfigMap/Secret size & memory pressure auditor
	mux.HandleFunc("/api/product/job-health", s.cacheMiddleware(60*time.Second, s.handleJobHealth))                       // batch job execution health & completion analyzer
	mux.HandleFunc("/api/product/hpa-health", s.cacheMiddleware(60*time.Second, s.handleHPAHealth))                       // HPA health & scaling activity analyzer
	mux.HandleFunc("/api/product/api-deprecation", s.cacheMiddleware(120*time.Second, s.handleDeprecatedAPI))             // deprecated API version & upgrade readiness checker
	mux.HandleFunc("/api/product/qos-priority", s.cacheMiddleware(60*time.Second, s.handleQoSAudit))                      // pod QoS & priority class distribution auditor
	mux.HandleFunc("/api/product/service-connectivity", s.cacheMiddleware(60*time.Second, s.handleServiceConnectivity))   // service endpoint & connectivity health auditor
	mux.HandleFunc("/api/product/topology-spread", s.cacheMiddleware(60*time.Second, s.handleTopologySpreadAudit))        // topology spread constraint validator
	mux.HandleFunc("/api/product/backup-compliance", s.cacheMiddleware(120*time.Second, s.handleBackupCompliance))        // volume snapshot & PVC backup compliance auditor
	mux.HandleFunc("/api/product/init-container-audit", s.cacheMiddleware(60*time.Second, s.handleInitContainerAudit))    // init container reliability & startup dependency auditor
	mux.HandleFunc("/api/product/hpa-gap", s.cacheMiddleware(60*time.Second, s.handleHPAGap))                             // HPA target utilization gap & scaling behavior auditor
	mux.HandleFunc("/api/product/mesh-health", s.cacheMiddleware(120*time.Second, s.handleMeshHealth))
	mux.HandleFunc("/api/product/mesh-injection", s.cacheMiddleware(120*time.Second, s.handleMeshInjection))                          // service mesh injection coverage & namespace adoption analyzer                                // service mesh sidecar health & mTLS coverage auditor
	mux.HandleFunc("/api/product/replica-distribution", s.cacheMiddleware(120*time.Second, s.handleReplicaDistribution))              // workload replica distribution & anti-affinity coverage analyzer
	mux.HandleFunc("/api/product/cronjob-schedule", s.cacheMiddleware(60*time.Second, s.handleCronJobSchedule))                       // CronJob schedule conflict & resource configuration auditor
	mux.HandleFunc("/api/product/external-secret-health", s.cacheMiddleware(120*time.Second, s.handleExternalSecretHealth))           // external secrets & secret store CSI health auditor
	mux.HandleFunc("/api/product/endpoint-dns-health", s.cacheMiddleware(60*time.Second, s.handleEndpointDNSHealth))                  // service endpoint & DNS resolution health auditor
	mux.HandleFunc("/api/product/config-mount-risk", s.cacheMiddleware(60*time.Second, s.handleConfigMountRisk))                      // ConfigMap & Secret mount injection risk auditor
	mux.HandleFunc("/api/product/pv-access", s.cacheMiddleware(120*time.Second, s.handlePVAccess))                                    // PV access mode & multi-attach risk auditor
	mux.HandleFunc("/api/product/traffic-policy", s.cacheMiddleware(120*time.Second, s.handleTrafficPolicy))                          // service traffic policy & routing configuration auditor
	mux.HandleFunc("/api/product/priority-preemption", s.cacheMiddleware(60*time.Second, s.handlePriorityPreemption))                 // pod priority preemption & scheduling starvation risk analyzer
	mux.HandleFunc("/api/scalability/overcommit", s.cacheMiddleware(60*time.Second, s.handleOvercommitAnalysis))                      // resource over-commit & pressure
	mux.HandleFunc("/api/scalability/autoscale-recommendations", s.cacheMiddleware(60*time.Second, s.handleAutoscaleRecommendations)) // HPA/VPA right-sizing
	mux.HandleFunc("/api/scalability/pvc-analysis", s.cacheMiddleware(60*time.Second, s.handlePVCAnalysis))                           // PVC binding & storage performance
	mux.HandleFunc("/api/scalability/storage-forecast", s.cacheMiddleware(120*time.Second, s.handleStorageForecast))                  // storage capacity exhaustion predictor
	mux.HandleFunc("/api/scalability/pod-density", s.cacheMiddleware(60*time.Second, s.handlePodDensity))                             // pod density & scheduling capacity analyzer
	mux.HandleFunc("/api/scalability/ns-consumption", s.cacheMiddleware(60*time.Second, s.handleNSConsumption))                       // namespace resource consumption & cost attribution
	mux.HandleFunc("/api/scalability/capacity-headroom", s.cacheMiddleware(60*time.Second, s.handleCapacityHeadroom))                 // cluster capacity headroom & scale-out readiness
	mux.HandleFunc("/api/scalability/quota-utilization", s.cacheMiddleware(60*time.Second, s.handleQuotaUtilization))                 // resource quota utilization & limit compliance
	mux.HandleFunc("/api/scalability/ha-audit", s.cacheMiddleware(60*time.Second, s.handleHASPOFDetector))                            // HA & single-point-of-failure detector
	mux.HandleFunc("/api/scalability/node-failure-sim", s.cacheMiddleware(60*time.Second, s.handleNodeFailureSim))                    // node failure impact simulator
	mux.HandleFunc("/api/scalability/crd-explosion", s.cacheMiddleware(120*time.Second, s.handleCRDExplosion))                        // API object count & CRD explosion risk detector
	mux.HandleFunc("/api/scalability/bottleneck-predictor", s.cacheMiddleware(120*time.Second, s.handleScalabilityBottleneck))        // K8s scalability bottleneck predictor
	mux.HandleFunc("/api/scalability/namespace-isolation", s.cacheMiddleware(120*time.Second, s.handleNamespaceIsolation))            // namespace isolation & multi-tenancy audit
	mux.HandleFunc("/api/scalability/csi-audit", s.cacheMiddleware(120*time.Second, s.handleCSIAudit))                                // CSI driver & storage capability auditor
	mux.HandleFunc("/api/scalability/scale-limits", s.cacheMiddleware(60*time.Second, s.handleScaleLimits))                           // cluster scalability limits & threshold monitor
	mux.HandleFunc("/api/scalability/dr-readiness", s.cacheMiddleware(120*time.Second, s.handleDRReadiness))                          // disaster recovery readiness & backup compliance auditor
	mux.HandleFunc("/api/scalability/fragmentation", s.cacheMiddleware(60*time.Second, s.handleFragmentation))                        // resource fragmentation & bin-packing efficiency analyzer
	mux.HandleFunc("/api/scalability/ip-cidr-utilization", s.cacheMiddleware(60*time.Second, s.handleIPCIDRAudit))                    // IP address & Pod CIDR utilization monitor
	mux.HandleFunc("/api/scalability/node-topology", s.cacheMiddleware(60*time.Second, s.handleNodeTopology))                         // node topology distribution & multi-AZ fault tolerance analyzer
	mux.HandleFunc("/api/scalability/tenant-pressure", s.cacheMiddleware(60*time.Second, s.handleTenantPressure))                     // multi-tenant resource pressure & quota competition auditor
	mux.HandleFunc("/api/scalability/node-pool-health", s.cacheMiddleware(60*time.Second, s.handleNodePool))                          // node pool & cluster autoscaler health monitor
	mux.HandleFunc("/api/scalability/cost-waste", s.cacheMiddleware(120*time.Second, s.handleCostWaste))                              // idle resource cost waste & namespace cost attribution auditor
	mux.HandleFunc("/api/scalability/node-lifecycle", s.cacheMiddleware(120*time.Second, s.handleNodeLifecycle))                      // node OS patch, kernel drift, GPU resources & node rotation auditor
	mux.HandleFunc("/api/scalability/alloc-efficiency", s.cacheMiddleware(60*time.Second, s.handleAllocEfficiency))                   // resource request vs limit allocation efficiency auditor
	mux.HandleFunc("/api/scalability/hpa-performance", s.cacheMiddleware(60*time.Second, s.handleHPAPerformance))                     // HPA autoscaling performance & scaling event auditor
	mux.HandleFunc("/api/scalability/pv-reclaim", s.cacheMiddleware(120*time.Second, s.handlePVReclaim))                              // PV reclaim policy & storage class waste auditor
	mux.HandleFunc("/api/scalability/capacity-plan", s.cacheMiddleware(60*time.Second, s.handleCapacityPlan))                         // capacity planning & growth trend predictor
	mux.HandleFunc("/api/scalability/spot-readiness", s.cacheMiddleware(120*time.Second, s.handleSpotReadiness))                      // spot/preemptible instance readiness & cost optimization auditor
	mux.HandleFunc("/api/deployment/image-hygiene", s.cacheMiddleware(60*time.Second, s.handleImageHygiene))                          // container image deployment hygiene analyzer
	mux.HandleFunc("/api/deployment/revision-history", s.cacheMiddleware(60*time.Second, s.handleRevisionHistory))                    // deployment revision history & rollback readiness
	mux.HandleFunc("/api/deployment/disruption-impact", s.cacheMiddleware(60*time.Second, s.handleDisruptionImpact))                  // deployment PDB disruption & maintenance impact
	mux.HandleFunc("/api/deployment/workload-maturity", s.cacheMiddleware(60*time.Second, s.handleWorkloadMaturity))                  // workload maturity & best practices scorer
	mux.HandleFunc("/api/deployment/config-consistency", s.cacheMiddleware(60*time.Second, s.handleConfigConsistency))                // configuration consistency & standardization auditor
	mux.HandleFunc("/api/deployment/ephemeral-storage", s.cacheMiddleware(60*time.Second, s.handleEphemeralStorage))                  // ephemeral storage & emptyDir limit compliance
	mux.HandleFunc("/api/deployment/config-sync", s.cacheMiddleware(60*time.Second, s.handleConfigSync))                              // ConfigMap/Secret config sync & staleness detector
	mux.HandleFunc("/api/deployment/sidecar-audit", s.cacheMiddleware(60*time.Second, s.handleSidecarAudit))
	mux.HandleFunc("/api/deployment/restart-policy", s.cacheMiddleware(60*time.Second, s.handleRestartPolicy))                 // restart policy & lifecycle hook auditor                          // sidecar container overhead & injection auditor
	mux.HandleFunc("/api/deployment/scale-readiness", s.cacheMiddleware(60*time.Second, s.handleScaleReadiness))               // deployment scale readiness & autoscaling gap detector
	mux.HandleFunc("/api/deployment/rollout-health", s.cacheMiddleware(30*time.Second, s.handleRolloutHealth))                 // deployment rollout strategy & health analyzer
	mux.HandleFunc("/api/deployment/probe-compliance", s.cacheMiddleware(60*time.Second, s.handleProbeCompliance))             // health probe compliance auditor
	mux.HandleFunc("/api/deployment/resource-limits", s.cacheMiddleware(60*time.Second, s.handleResourceLimitsAudit))          // resource limit & enforcement gap audit
	mux.HandleFunc("/api/deployment/graceful-shutdown", s.cacheMiddleware(60*time.Second, s.handleGracefulShutdown))           // graceful shutdown & termination compliance
	mux.HandleFunc("/api/deployment/update-strategy", s.cacheMiddleware(60*time.Second, s.handleUpdateStrategy))               // deployment update strategy & rollback readiness
	mux.HandleFunc("/api/deployment/ref-integrity", s.cacheMiddleware(60*time.Second, s.handleRefIntegrity))                   // Secret/ConfigMap reference integrity checker
	mux.HandleFunc("/api/deployment/image-drift", s.cacheMiddleware(60*time.Second, s.handleImageDrift))                       // deployment image drift & version consistency detector
	mux.HandleFunc("/api/deployment/replica-availability", s.cacheMiddleware(30*time.Second, s.handleReplicaAvailability))     // deployment replica availability & ready pod ratio monitor
	mux.HandleFunc("/api/deployment/helm-health", s.cacheMiddleware(120*time.Second, s.handleHelmHealth))                      // Helm release health & GitOps drift detector
	mux.HandleFunc("/api/deployment/surge-risk", s.cacheMiddleware(60*time.Second, s.handleSurgeRisk))                         // rolling update risk & surge configuration analyzer
	mux.HandleFunc("/api/deployment/startup-latency", s.cacheMiddleware(60*time.Second, s.handleStartupLatency))               // pod startup latency & readiness performance auditor
	mux.HandleFunc("/api/deployment/progressive-delivery", s.cacheMiddleware(60*time.Second, s.handleProgressiveDelivery))     // progressive delivery & canary rollout health auditor
	mux.HandleFunc("/api/deployment/rs-staleness", s.cacheMiddleware(60*time.Second, s.handleRSStaleness))                     // ReplicaSet staleness & rollout history auditor
	mux.HandleFunc("/api/deployment/gitops-sync-deep", s.cacheMiddleware(60*time.Second, s.handleGitOpsSync))                  // ArgoCD & Flux GitOps sync status & drift auditor
	mux.HandleFunc("/api/deployment/dora-metrics", s.cacheMiddleware(60*time.Second, s.handleDORAMetrics))                     // DORA metrics: deployment frequency, lead time, MTTR, change failure rate
	mux.HandleFunc("/api/deployment/daemonset-audit", s.cacheMiddleware(60*time.Second, s.handleDaemonSetAudit))               // DaemonSet rollout & node coverage auditor
	mux.HandleFunc("/api/deployment/concurrency-guard", s.cacheMiddleware(30*time.Second, s.handleDeploymentConcurrencyGuard)) // deployment concurrency & rolling update collision detector
	mux.HandleFunc("/api/deployment/revision-diff", s.cacheMiddleware(120*time.Second, s.handleRevisionDiff))                  // deployment revision diff & pod template change impact analyzer
	mux.HandleFunc("/api/operations/predictive-health", s.cacheMiddleware(60*time.Second, s.handlePredictiveHealth))           // cluster predictive health & risk forecast engine
	mux.HandleFunc("/api/deployment/change-readiness", s.cacheMiddleware(30*time.Second, s.handleChangeReadiness))             // deployment change readiness pre-flight gate
	mux.HandleFunc("/api/scalability/request-intelligence", s.cacheMiddleware(120*time.Second, s.handleRequestIntelligence))   // resource request intelligence & right-sizing engine
	mux.HandleFunc("/api/product/reliability-scorecard", s.cacheMiddleware(120*time.Second, s.handleReliabilityScorecard))     // per-workload reliability posture scorecard (A-F grading)
	mux.HandleFunc("/api/security/posture-scorecard", s.cacheMiddleware(120*time.Second, s.handleSecurityPosture))
	mux.HandleFunc("/api/operations/triage", s.cacheMiddleware(30*time.Second, s.handleTriage))
	mux.HandleFunc("/api/deployment/impact-simulator", s.cacheMiddleware(60*time.Second, s.handleDeployImpact))                     // cluster-wide security posture scorecard (A-F grading)
	mux.HandleFunc("/api/deployment/rollout-forensics", s.cacheMiddleware(60*time.Second, s.handleRolloutForensics))                // rollout failure forensics & deployment pattern detector
	mux.HandleFunc("/api/deployment/resource-governance", s.cacheMiddleware(60*time.Second, s.handleResourceGovernance))            // resource governance & namespace quota effectiveness
	mux.HandleFunc("/api/scalability/cost-intelligence", s.cacheMiddleware(120*time.Second, s.handleCostIntelligence))              // cost intelligence & spend forecast engine
	mux.HandleFunc("/api/scalability/autoscaling-intel", s.cacheMiddleware(120*time.Second, s.handleAutoscalingIntel))              // autoscaling intelligence & scaling behavior profiler
	mux.HandleFunc("/api/scalability/scheduling-intel", s.cacheMiddleware(60*time.Second, s.handleSchedulingIntel))                 // scheduling intelligence & bin-packing efficiency analyzer
	mux.HandleFunc("/api/product/golden-signals", s.cacheMiddleware(60*time.Second, s.handleGoldenSignals))                         // SRE four golden signals unified health engine
	mux.HandleFunc("/api/product/dependency-resilience", s.cacheMiddleware(60*time.Second, s.handleDependencyResilience))           // service dependency resilience & cascade failure risk analyzer
	mux.HandleFunc("/api/product/ownership-map", s.cacheMiddleware(60*time.Second, s.handleOwnershipMap))                           // workload ownership & accountability governance engine
	mux.HandleFunc("/api/security/remediation-matrix", s.cacheMiddleware(120*time.Second, s.handleRemediationMatrix))               // security remediation priority & risk-effort matrix
	mux.HandleFunc("/api/security/compliance-posture", s.cacheMiddleware(120*time.Second, s.handleCompliancePosture))               // multi-framework compliance posture & control mapping (SOC2/PCI-DSS/HIPAA/NIST/GDPR)
	mux.HandleFunc("/api/security/net-policy-effectiveness", s.cacheMiddleware(120*time.Second, s.handleNetPolicyEffectiveness))    // network policy effectiveness & zero-trust isolation scorer
	mux.HandleFunc("/api/operations/mttr", s.cacheMiddleware(60*time.Second, s.handleMTTR))                                         // mean time to recovery & incident lifecycle analytics
	mux.HandleFunc("/api/operations/change-intel", s.cacheMiddleware(60*time.Second, s.handleChangeIntel))                          // change intelligence & blast radius analyzer
	mux.HandleFunc("/api/operations/obs-coverage", s.cacheMiddleware(120*time.Second, s.handleObsCoverage))                         // observability coverage & blind spot detector
	mux.HandleFunc("/api/operations/obs-cardinality", s.cacheMiddleware(120*time.Second, s.handleObsCardinality))                   // observability data cardinality & volume cost analyzer
	mux.HandleFunc("/api/deployment/gitops-drift", s.cacheMiddleware(120*time.Second, s.handleGitOpsDrift))                         // GitOps sync health & configuration drift analyzer
	mux.HandleFunc("/api/product/api-version-governance", s.cacheMiddleware(120*time.Second, s.handleAPIVersionGov))                // K8s API version governance & deprecation tracker
	mux.HandleFunc("/api/security/secret-lifecycle", s.cacheMiddleware(120*time.Second, s.handleSecretLifecycle))                   // secret management lifecycle & rotation tracker
	mux.HandleFunc("/api/scalability/dr-backup-verify", s.cacheMiddleware(120*time.Second, s.handleDRBackup))                       // disaster recovery & backup verification assessor
	mux.HandleFunc("/api/docs/training-readiness", s.cacheMiddleware(120*time.Second, s.handleTrainingReadiness))                   // platform onboarding & documentation quality assessor
	mux.HandleFunc("/api/operations/cert-expiry", s.cacheMiddleware(120*time.Second, s.handleCertExpiry))                           // TLS certificate expiry & lifecycle monitor
	mux.HandleFunc("/api/security/image-supply-chain", s.cacheMiddleware(120*time.Second, s.handleSupplyChain))                     // container image supply chain security scanner
	mux.HandleFunc("/api/scalability/node-os-drift", s.cacheMiddleware(120*time.Second, s.handleNodeOSDrift))                       // node OS lifecycle & kernel drift deep analyzer
	mux.HandleFunc("/api/product/traffic-flow", s.cacheMiddleware(60*time.Second, s.handleTrafficFlow))                             // east-west traffic flow & service communication map
	mux.HandleFunc("/api/deployment/pipeline-health", s.cacheMiddleware(60*time.Second, s.handlePipelineHealth))                    // CI/CD pipeline health & DORA maturity analyzer
	mux.HandleFunc("/api/operations/alert-rule-quality", s.cacheMiddleware(120*time.Second, s.handleAlertRuleQuality))              // alerting rule quality & coverage gap analyzer
	mux.HandleFunc("/api/scalability/chargeback", s.cacheMiddleware(300*time.Second, s.handleChargeback))                           // cost chargeback & team budget allocation report
	mux.HandleFunc("/api/security/runtime-scan", s.cacheMiddleware(60*time.Second, s.handleRuntimeThreat))                          // runtime threat detection & behavioral anomaly scanner
	mux.HandleFunc("/api/docs/exec-dashboard", s.cacheMiddleware(60*time.Second, s.handleExecDashboard))                            // executive platform health summary & scorecard
	mux.HandleFunc("/api/product/slo-compliance", s.cacheMiddleware(30*time.Second, s.handleSLOCompliance))                         // service SLO compliance & error budget burn rate
	mux.HandleFunc("/api/operations/probe-latency", s.cacheMiddleware(60*time.Second, s.handleProbeLatency))                        // health probe latency & readiness performance analyzer
	mux.HandleFunc("/api/deployment/helm-health-deep", s.cacheMiddleware(120*time.Second, s.handleHelmHealthDeep))                  // deep Helm release health & chart staleness analyzer
	mux.HandleFunc("/api/scalability/spot-readiness-deep", s.cacheMiddleware(120*time.Second, s.handleSpotReadinessDeep))           // spot/preemptible instance readiness deep analyzer
	mux.HandleFunc("/api/security/rbac-blast", s.cacheMiddleware(120*time.Second, s.handleRBACBlast))                               // RBAC privilege escalation & blast radius analyzer
	mux.HandleFunc("/api/product/api-gateway-health", s.cacheMiddleware(60*time.Second, s.handleGatewayHealth))                     // API gateway & ingress controller health analyzer
	mux.HandleFunc("/api/operations/throttle-risk", s.cacheMiddleware(60*time.Second, s.handleThrottleRisk))                        // pod resource throttling risk & CPU pressure detector
	mux.HandleFunc("/api/security/audit-trail", s.cacheMiddleware(120*time.Second, s.handleAuditTrail))                             // audit logging coverage & compliance trail analyzer
	mux.HandleFunc("/api/deployment/image-freshness", s.cacheMiddleware(120*time.Second, s.handleImageFreshness))                   // container image freshness & update tracking
	mux.HandleFunc("/api/scalability/multi-cluster-conn", s.cacheMiddleware(120*time.Second, s.handleMultiClusterConn))             // multi-cluster connectivity & federation health
	mux.HandleFunc("/api/security/admission-posture", s.cacheMiddleware(120*time.Second, s.handleAdmissionAudit))                   // admission controller posture & policy engine audit
	mux.HandleFunc("/api/operations/dashboard-availability", s.cacheMiddleware(120*time.Second, s.handleDashAvail))                 // Grafana dashboard availability & observability UI coverage
	mux.HandleFunc("/api/scalability/storage-orphan", s.cacheMiddleware(120*time.Second, s.handleStorageOrphan))                    // orphaned PVC & storage waste analyzer
	mux.HandleFunc("/api/deployment/workload-deps", s.cacheMiddleware(120*time.Second, s.handleWLDeps))                             // workload dependency graph analyzer
	mux.HandleFunc("/api/operations/metrics-pipe", s.cacheMiddleware(120*time.Second, s.handleMetricsPipe))                         // metrics pipeline integrity & scraping coverage
	mux.HandleFunc("/api/docs/platform-changelog", s.cacheMiddleware(30*time.Second, s.handleChangeLog))                            // platform changelog from recent resource changes
	mux.HandleFunc("/api/scalability/capacity-forecast-deep", s.cacheMiddleware(60*time.Second, s.handleCapacityForecastDeep))      // cluster capacity exhaustion forecast
	mux.HandleFunc("/api/security/compliance-framework", s.cacheMiddleware(120*time.Second, s.handleComplianceMap))                 // SOC2/PCI-DSS/CIS compliance framework mapping
	mux.HandleFunc("/api/product/mttr-analysis", s.cacheMiddleware(60*time.Second, s.handleMTTR))                                   // mean time to recovery from restart patterns
	mux.HandleFunc("/api/deployment/gitops-sync-status", s.cacheMiddleware(120*time.Second, s.handleGitOpsSync))                    // GitOps sync state & drift detection
	mux.HandleFunc("/api/operations/endpoint-probe", s.cacheMiddleware(60*time.Second, s.handleEndpointProbe))                      // service endpoint readiness probe
	mux.HandleFunc("/api/scalability/node-decomm", s.cacheMiddleware(120*time.Second, s.handleNodeDecomm))                          // node decommissioning & lifecycle rotation
	mux.HandleFunc("/api/operations/backup-coverage", s.cacheMiddleware(120*time.Second, s.handleBackupCoverage))                   // backup & disaster recovery posture analyzer
	mux.HandleFunc("/api/deployment/idle-zombie", s.cacheMiddleware(120*time.Second, s.handleIdleZombie))                           // idle/zombie workload detector
	mux.HandleFunc("/api/product/service-mesh", s.cacheMiddleware(120*time.Second, s.handleServiceMesh))                            // service mesh coverage & mTLS analyzer
	mux.HandleFunc("/api/product/mesh-readiness", s.cacheMiddleware(120*time.Second, s.handleMeshReadiness))                        // service mesh readiness & mTLS coverage gap analyzer
	mux.HandleFunc("/api/scalability/idle-waste", s.cacheMiddleware(120*time.Second, s.handleIdleWaste))                            // idle resource waste quantification & cost recovery engine
	mux.HandleFunc("/api/security/policy-governance", s.cacheMiddleware(120*time.Second, s.handlePolicyGovernance))                 // admission policy governance & enforcement auditor
	mux.HandleFunc("/api/docs/api-quality", s.cacheMiddleware(120*time.Second, s.handleAPIQuality))                                 // platform API endpoint quality & coverage gap analyzer
	mux.HandleFunc("/api/product/cloud-portability", s.cacheMiddleware(120*time.Second, s.handleCloudPortability))                  // cloud vendor lock-in & workload portability assessor
	mux.HandleFunc("/api/scalability/storage-performance", s.cacheMiddleware(120*time.Second, s.handleStoragePerf))                 // storage performance tier classification & mismatch detector
	mux.HandleFunc("/api/deployment/workload-lifecycle", s.cacheMiddleware(120*time.Second, s.handleWorkloadLifecycle))             // workload lifecycle stage classifier & cleanup advisor
	mux.HandleFunc("/api/deployment/upgrade-impact", s.cacheMiddleware(120*time.Second, s.handleUpgradeImpact))                     // K8s version upgrade impact simulator & readiness assessor
	mux.HandleFunc("/api/docs/resource-inventory", s.cacheMiddleware(120*time.Second, s.handleResourceInventory))                   // comprehensive cluster resource catalog & inventory
	mux.HandleFunc("/api/scalability/unit-economics", s.cacheMiddleware(120*time.Second, s.handleUnitEconomics))                    // FinOps unit economics: cost per pod/service/namespace
	mux.HandleFunc("/api/docs/platform-scorecard", s.cacheMiddleware(120*time.Second, s.handlePlatformScorecard))                   // unified platform engineering scorecard
	mux.HandleFunc("/api/operations/signal-correlation", s.cacheMiddleware(30*time.Second, s.handleSignalCorrelation))              // proactive multi-signal anomaly correlation engine
	mux.HandleFunc("/api/scalability/green-computing", s.cacheMiddleware(120*time.Second, s.handleGreenComputing))                  // green computing & sustainability scorecard
	mux.HandleFunc("/api/deployment/deploy-window", s.cacheMiddleware(60*time.Second, s.handleDeployWindow))                        // optimal deployment window analyzer
	mux.HandleFunc("/api/product/workload-criticality", s.cacheMiddleware(120*time.Second, s.handleCriticality))                    // workload criticality scoring & tier classification
	mux.HandleFunc("/api/scalability/commit-optimizer", s.cacheMiddleware(120*time.Second, s.handleCommitOptimizer))                // resource commitment & reserved instance optimizer
	mux.HandleFunc("/api/deployment/change-freeze", s.cacheMiddleware(30*time.Second, s.handleChangeFreeze))                        // change freeze detector & deployment risk gate
	mux.HandleFunc("/api/security/attack-surface", s.cacheMiddleware(120*time.Second, s.handleAttackSurface))                       // external attack surface mapper & TLS gap analyzer
	mux.HandleFunc("/api/scalability/density-balance", s.cacheMiddleware(60*time.Second, s.handleDensityBalance))                   // pod scheduling density & node balance analyzer
	mux.HandleFunc("/api/security/secret-rotation", s.cacheMiddleware(120*time.Second, s.handleSecretRotationAudit))                // secret rotation compliance & staleness tracker
	mux.HandleFunc("/api/scalability/hpa-behavior", s.cacheMiddleware(60*time.Second, s.handleHPABehavior))                         // HPA scaling behavior & flapping risk analyzer
	mux.HandleFunc("/api/operations/api-access-pattern", s.cacheMiddleware(30*time.Second, s.handleAPIAccess))                      // API server access pattern & anomaly detector
	mux.HandleFunc("/api/scalability/volume-budget", s.cacheMiddleware(120*time.Second, s.handleVolumeBudget))                      // PVC storage budget & orphan detector
	mux.HandleFunc("/api/operations/restart-pattern", s.cacheMiddleware(60*time.Second, s.handleRestartPattern))                    // pod restart pattern & chronic issue analyzer
	mux.HandleFunc("/api/security/cert-inventory", s.cacheMiddleware(120*time.Second, s.handleCertInventory))                       // TLS certificate inventory & expiry tracker
	mux.HandleFunc("/api/product/env-var-audit", s.cacheMiddleware(120*time.Second, s.handleEnvVarAudit))                           // environment variable security & sprawl auditor
	mux.HandleFunc("/api/scalability/scaling-simulator", s.cacheMiddleware(120*time.Second, s.handleScalingSimulator))              // cluster scaling scenario simulator
	mux.HandleFunc("/api/product/placement-score", s.cacheMiddleware(120*time.Second, s.handlePlacementScore))                      // pod scheduling placement quality scorer
	mux.HandleFunc("/api/operations/chaos-readiness", s.cacheMiddleware(120*time.Second, s.handleChaosReadiness))                   // chaos engineering readiness & resilience auditor
	mux.HandleFunc("/api/operations/drain-impact", s.cacheMiddleware(60*time.Second, s.handleDrainImpact))                          // node drain impact simulator
	mux.HandleFunc("/api/scalability/request-accuracy", s.cacheMiddleware(120*time.Second, s.handleRequestAccuracy))                // resource request accuracy & right-sizing analyzer
	mux.HandleFunc("/api/security/hardening-score", s.cacheMiddleware(120*time.Second, s.handleHardeningScore))                     // comprehensive security hardening posture score
	mux.HandleFunc("/api/security/fix-plan", s.cacheMiddleware(120*time.Second, s.handleSecurityFixPlan))                           // security remediation action plan generator
	mux.HandleFunc("/api/docs/api-coverage-map", s.cacheMiddleware(300*time.Second, s.handleAPICoverageMap))                        // API endpoint coverage map by dimension
	mux.HandleFunc("/api/deployment/release-gate", s.cacheMiddleware(60*time.Second, s.handleReleaseGate))                          // pre-deployment release gate evaluator
	mux.HandleFunc("/api/product/service-catalog", s.cacheMiddleware(60*time.Second, s.handleServiceCatalog))                       // cluster service catalog & discovery map
	mux.HandleFunc("/api/operations/resource-topology", s.cacheMiddleware(120*time.Second, s.handleResourceTopology))               // resource dependency graph & orphan detector
	mux.HandleFunc("/api/docs/api-explorer", s.cacheMiddleware(300*time.Second, s.handleAPIExplorer))                               // interactive API endpoint browser with search
	mux.HandleFunc("/api/scalability/orphan-cleanup", s.cacheMiddleware(120*time.Second, s.handleOrphanCleanup))                    // orphaned resource cleanup planner
	mux.HandleFunc("/api/scalability/cost-anomaly", s.cacheMiddleware(120*time.Second, s.handleCostAnomaly))                        // cost anomaly detector
	mux.HandleFunc("/api/deployment/config-snapshot", s.cacheMiddleware(60*time.Second, s.handleConfigSnapshot))                    // cluster config snapshot for drift detection
	mux.HandleFunc("/api/operations/pod-health-index", s.cacheMiddleware(60*time.Second, s.handlePodHealthIndex))                   // per-pod health score & issue detector
	mux.HandleFunc("/api/product/namespace-quota-map", s.cacheMiddleware(120*time.Second, s.handleNamespaceQuotaMap))               // namespace quota & limit range coverage map
	mux.HandleFunc("/api/security/secret-exposure", s.cacheMiddleware(120*time.Second, s.handleSecretExposure))                     // secret exposure & plaintext scanner
	mux.HandleFunc("/api/docs/cluster-maturity", s.cacheMiddleware(300*time.Second, s.handleClusterMaturity))                       // cluster maturity model assessment
	mux.HandleFunc("/api/scalability/right-size-engine", s.cacheMiddleware(120*time.Second, s.handleRightSizeEngine))               // resource right-sizing engine with patches
	mux.HandleFunc("/api/deployment/deploy-risk", s.cacheMiddleware(60*time.Second, s.handleDeployRisk))                            // pre-deployment risk assessment
	mux.HandleFunc("/api/operations/pdb-generator", s.cacheMiddleware(120*time.Second, s.handlePDBGenerator))                       // PDB manifest generator
	mux.HandleFunc("/api/security/netpol-generator", s.cacheMiddleware(120*time.Second, s.handleNetpolGenerator))                   // NetworkPolicy manifest generator
	mux.HandleFunc("/api/product/service-dependency-map", s.cacheMiddleware(120*time.Second, s.handleServiceDependencyMap))         // service dependency graph
	mux.HandleFunc("/api/scalability/quota-generator", s.cacheMiddleware(120*time.Second, s.handleQuotaGenerator))                  // ResourceQuota & LimitRange manifest generator
	mux.HandleFunc("/api/deployment/probe-generator", s.cacheMiddleware(120*time.Second, s.handleProbeGenerator))                   // health probe patch generator
	mux.HandleFunc("/api/docs/platform-insights", s.cacheMiddleware(60*time.Second, s.handlePlatformInsights))                      // unified executive platform insights
	mux.HandleFunc("/api/docs/action-priority-matrix", s.cacheMiddleware(120*time.Second, s.handleActionPriorityMatrix))            // prioritized remediation action queue
	mux.HandleFunc("/api/operations/health-trend", s.cacheMiddleware(60*time.Second, s.handleHealthTrend))                          // cluster health trend over time
	mux.HandleFunc("/api/scalability/image-cleanup", s.cacheMiddleware(120*time.Second, s.handleImageCleanup))                      // unused image cleanup advisor
	mux.HandleFunc("/api/operations/restart-analyzer", s.cacheMiddleware(60*time.Second, s.handleRestartAnalyzer))                  // pod restart pattern analyzer & root cause
	mux.HandleFunc("/api/security/env-leak-scanner", s.cacheMiddleware(120*time.Second, s.handleEnvLeakScanner))                    // plaintext env var leak scanner
	mux.HandleFunc("/api/deployment/update-strategy-auditor", s.cacheMiddleware(120*time.Second, s.handleUpdateStrategyAuditor))    // update strategy risk auditor
	mux.HandleFunc("/api/product/label-score", s.cacheMiddleware(120*time.Second, s.handleLabelScore))                              // label hygiene score
	mux.HandleFunc("/api/scalability/storage-tier", s.cacheMiddleware(120*time.Second, s.handleStorageTier))                        // storage tier analyzer
	mux.HandleFunc("/api/security/trust-chain", s.cacheMiddleware(120*time.Second, s.handleTrustChain))                             // trust chain auditor
	mux.HandleFunc("/api/operations/alert-fatigue", s.cacheMiddleware(60*time.Second, s.handleAlertFatigue))                        // event noise & alert fatigue analyzer
	mux.HandleFunc("/api/deployment/deploy-frequency", s.cacheMiddleware(60*time.Second, s.handleDeployFrequency))                  // deployment frequency tracker (DORA)
	mux.HandleFunc("/api/docs/platform-comparison", s.cacheMiddleware(60*time.Second, s.handlePlatformComparison))                  // platform comparison & trend snapshot
	mux.HandleFunc("/api/security/container-hardening", s.cacheMiddleware(120*time.Second, s.handleContainerHardening))             // container security hardening scanner
	mux.HandleFunc("/api/scalability/autoscale-readiness", s.cacheMiddleware(120*time.Second, s.handleAutoscaleReadiness))          // HPA autoscale readiness & generator
	mux.HandleFunc("/api/product/workload-efficiency", s.cacheMiddleware(120*time.Second, s.handleWorkloadEfficiency))              // workload resource efficiency scorer
	mux.HandleFunc("/api/operations/capacity-gap", s.cacheMiddleware(120*time.Second, s.handleCapacityGap))                         // capacity gap & node loss survival analyzer
	mux.HandleFunc("/api/deployment/revision-drift", s.cacheMiddleware(120*time.Second, s.handleRevisionDrift))                     // ReplicaSet revision drift detector
	mux.HandleFunc("/api/docs/knowledge-base", s.cacheMiddleware(300*time.Second, s.handleKnowledgeBase))                           // auto-generated cluster knowledge base
	mux.HandleFunc("/api/security/compliance-gap", s.cacheMiddleware(120*time.Second, s.handleComplianceGap))                       // compliance framework gap analysis (CIS/NIST/SOC2)
	mux.HandleFunc("/api/scalability/scheduler-fairness", s.cacheMiddleware(120*time.Second, s.handleSchedulerFairness))            // pod scheduling fairness & node balance analyzer
	mux.HandleFunc("/api/product/workload-fingerprint", s.cacheMiddleware(120*time.Second, s.handleWorkloadFingerprint))            // workload fingerprint & duplicate detector
	mux.HandleFunc("/api/deployment/deploy-heatmap", s.cacheMiddleware(60*time.Second, s.handleDeployHeatmap))                      // deployment activity heatmap
	mux.HandleFunc("/api/operations/log-volume", s.cacheMiddleware(120*time.Second, s.handleLogVolume))                             // log volume estimator & noisy logger finder
	mux.HandleFunc("/api/docs/cluster-narrative", s.cacheMiddleware(60*time.Second, s.handleClusterNarrative))                      // human-readable cluster narrative report
	mux.HandleFunc("/api/security/config-audit-trail", s.cacheMiddleware(60*time.Second, s.handleConfigAuditTrail))                 // configuration change audit trail
	mux.HandleFunc("/api/scalability/node-utilization-deep", s.cacheMiddleware(120*time.Second, s.handleNodeUtilizationDeep))       // deep node utilization & top consumer analysis
	mux.HandleFunc("/api/security/secret-rotation-plan", s.cacheMiddleware(120*time.Second, s.handleSecretRotationPlan))            // secret rotation plan generator
	mux.HandleFunc("/api/operations/event-correlation-deep", s.cacheMiddleware(60*time.Second, s.handleEventCorrelationDeep))       // deep event correlation & root cause
	mux.HandleFunc("/api/deployment/rollback-simulator", s.cacheMiddleware(120*time.Second, s.handleRollbackSimulator))             // rollback risk simulator
	mux.HandleFunc("/api/docs/upgrade-planner", s.cacheMiddleware(300*time.Second, s.handleUpgradePlanner))                         // k8s upgrade planner
	mux.HandleFunc("/api/security/rbac-drift", s.cacheMiddleware(120*time.Second, s.handleRBACDrift))                               // RBAC drift & over-permissive role detector
	mux.HandleFunc("/api/scalability/resource-forecast", s.cacheMiddleware(300*time.Second, s.handleResourceForecast))              // resource capacity forecast
	mux.HandleFunc("/api/product/config-warmstart", s.cacheMiddleware(120*time.Second, s.handleConfigWarmstart))                    // startup optimization analyzer
	mux.HandleFunc("/api/operations/pod-slo", s.cacheMiddleware(60*time.Second, s.handlePodSLO))                                    // pod SLO compliance tracker
	mux.HandleFunc("/api/deployment/deploy-readiness-gate", s.cacheMiddleware(120*time.Second, s.handleDeployReadinessGate))        // deployment readiness gate composite evaluator
	mux.HandleFunc("/api/docs/api-governance-score", s.cacheMiddleware(300*time.Second, s.handleAPIGovernanceScore))                // API version governance score
	mux.HandleFunc("/api/security/disruption-budget-gap", s.cacheMiddleware(120*time.Second, s.handleDisruptionBudgetGap))          // PDB gap & disruption risk analyzer
	mux.HandleFunc("/api/product/cost-topology", s.cacheMiddleware(300*time.Second, s.handleCostTopology))                          // per-namespace cost topology
	mux.HandleFunc("/api/scalability/binpack-efficiency", s.cacheMiddleware(120*time.Second, s.handleBinpackEfficiency))            // node bin-packing efficiency & consolidation
	mux.HandleFunc("/api/operations/slo-burn-rate", s.cacheMiddleware(60*time.Second, s.handleSLOBurnRate))                         // SLO error budget burn rate analyzer
	mux.HandleFunc("/api/deployment/surge-capacity", s.cacheMiddleware(120*time.Second, s.handleSurgeCapacity))                     // rolling update surge capacity checker
	mux.HandleFunc("/api/docs/runbook-coverage", s.cacheMiddleware(300*time.Second, s.handleRunbookCoverage))                       // runbook annotation coverage scanner
	mux.HandleFunc("/api/security/privilege-map", s.cacheMiddleware(120*time.Second, s.handlePrivilegeMap))                         // cluster-wide privilege exposure map
	mux.HandleFunc("/api/product/api-slo-correlation", s.cacheMiddleware(120*time.Second, s.handleAPISLOCorrelation))               // API endpoint SLO correlation analyzer
	mux.HandleFunc("/api/scalability/eviction-risk", s.cacheMiddleware(60*time.Second, s.handleEvictionRisk))                       // pod eviction risk predictor
	mux.HandleFunc("/api/operations/golden-signal-budget", s.cacheMiddleware(60*time.Second, s.handleGoldenSignalBudget))           // golden signal composite health budget
	mux.HandleFunc("/api/deployment/preflight-check", s.cacheMiddleware(120*time.Second, s.handlePreflightCheck))                   // deployment preflight check suite
	mux.HandleFunc("/api/docs/capacity-runbook", s.cacheMiddleware(300*time.Second, s.handleCapacityRunbook))                       // capacity planning runbook generator
	mux.HandleFunc("/api/security/secret-spray", s.cacheMiddleware(120*time.Second, s.handleSecretSpray))                           // secret mount spray exposure analyzer
	mux.HandleFunc("/api/product/traffic-cost-split", s.cacheMiddleware(300*time.Second, s.handleTrafficCostSplit))                 // traffic cost split by service/ingress
	mux.HandleFunc("/api/scalability/node-failure-blast", s.cacheMiddleware(120*time.Second, s.handleNodeFailureBlast))             // node failure blast radius simulator
	mux.HandleFunc("/api/operations/incident-timeline", s.cacheMiddleware(60*time.Second, s.handleIncidentTimeline))                // incident timeline reconstructor
	mux.HandleFunc("/api/deployment/rollback-safety", s.cacheMiddleware(120*time.Second, s.handleRollbackSafety))                   // rollback safety auditor
	mux.HandleFunc("/api/docs/api-semantic-version", s.cacheMiddleware(300*time.Second, s.handleAPISemanticVersion))                // API semantic version tracker
	mux.HandleFunc("/api/security/cert-chain-validator", s.cacheMiddleware(120*time.Second, s.handleCertChainValidator))            // TLS certificate chain validator
	mux.HandleFunc("/api/product/feature-flag-audit", s.cacheMiddleware(120*time.Second, s.handleFeatureFlagAudit))                 // feature flag coverage audit
	mux.HandleFunc("/api/scalability/autoscaler-gap", s.cacheMiddleware(120*time.Second, s.handleAutoscalerGap))                    // cluster autoscaler gap analyzer
	mux.HandleFunc("/api/operations/resource-saturation-watch", s.cacheMiddleware(60*time.Second, s.handleResourceSaturationWatch)) // resource saturation watchdog
	mux.HandleFunc("/api/deployment/deploy-frequency-trend", s.cacheMiddleware(300*time.Second, s.handleDeployFrequencyTrend))      // DORA deploy frequency trend
	mux.HandleFunc("/api/docs/oncall-readiness", s.cacheMiddleware(300*time.Second, s.handleOncallReadiness))                       // on-call readiness evaluator
	mux.HandleFunc("/api/security/mtls-trust-domain", s.cacheMiddleware(120*time.Second, s.handleMTLSTrustDomain))                  // mTLS trust domain auditor
	mux.HandleFunc("/api/product/latency-budget", s.cacheMiddleware(120*time.Second, s.handleLatencyBudget))                        // latency budget allocator
	mux.HandleFunc("/api/scalability/pod-disruption-tolerance", s.cacheMiddleware(120*time.Second, s.handlePodDisruptionTolerance)) // pod disruption tolerance analyzer
	// /api/security/supply-chain already registered at line ~280
	// /api/scalability/capacity-forecast-deep already registered above
	// Prometheus /metrics — restricted to localhost only (Prometheus scrapes from inside the cluster)
	mux.Handle("/metrics", s.localOnlyMiddleware(promhttp.Handler()))

	// Slack webhook — admin-only endpoint
	mux.Handle("/api/webhooks/slack", s.adminOnlyMiddleware(s.handleSlackWebhook))
	mux.HandleFunc("/api/webhooks/alertmanager", s.handleAlertmanagerWebhook) // Prometheus Alertmanager
	mux.HandleFunc("/api/webhooks/alertmanager/test", s.handleAlertTest)      // Test endpoint

	// Auth routes
	if s.auth != nil {
		s.auth.RegisterRoutes(mux)
	}

	// RBAC management routes (admin only)
	s.registerRBACRoutes(mux)

	// Wrap all routes with auth middleware (if enabled)
	// Order: AuthMiddleware (validates JWT, sets user) → ImpersonationMiddleware (creates per-user K8s client) → mux
	var handler http.Handler = mux
	if s.auth != nil {
		handler = s.auth.Middleware(s.ImpersonationMiddleware(mux))
	} else if s.authRequired {
		// Auth was requested but failed to initialize — fail closed.
		// Block all API requests; allow only static assets (HTML/CSS/JS) so the login page can render.
		handler = s.authFailClosedMiddleware(mux)
	}

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.requestIDMiddleware(s.httpMetricsMiddleware(s.gzipMiddleware(s.securityHeadersMiddleware(s.corsMiddleware(handler))))),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // no WriteTimeout: SSE streaming can take arbitrarily long
		IdleTimeout:  120 * time.Second,
		ConnState:    s.connStateTracker, // track active connections for graceful draining
	}

	// TLS support: use HTTPS if cert/key are configured
	if s.tlsCert != "" && s.tlsKey != "" {
		s.log.Info("starting dashboard with TLS", "address", addr, "cert", s.tlsCert)
		return s.server.ListenAndServeTLS(s.tlsCert, s.tlsKey)
	}

	s.log.Info("starting dashboard", "address", addr, "tls", false)
	return s.server.ListenAndServe()
}

// SetChatEngine injects the chat engine (called after provider is ready).
func (s *Server) SetChatEngine(engine *chat.Engine) {
	s.chatEngine = engine
}

// SetAuthRequired marks that authentication was requested but failed.
// The server will fail-closed: all API requests return 503.
func (s *Server) SetAuthRequired(errMsg string) {
	s.authRequired = true
	s.authFailedMsg = errMsg
}

// authFailClosedMiddleware blocks all /api/ requests when auth was requested
// but failed to initialize. Static assets (HTML/CSS/JS) are still served
// so the login page can render with an error message.
func (s *Server) authFailClosedMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow static assets (non-API paths)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		// Allow health/readiness probes
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		// Block all API requests
		s.log.Error("auth fail-closed: blocking API request", "path", r.URL.Path, "reason", s.authFailedMsg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSON(w, map[string]any{
			"error":  "Authentication system unavailable",
			"detail": "The authentication database failed to initialize. Access is blocked for security. Check pod logs for details.",
		})
	})
}

// SetProviderManager injects the provider manager.
func (s *Server) SetProviderManager(mgr *providermanager.Manager) {
	s.providerMgr = mgr
}

// SetAuthenticator injects the authenticator (enables login).
func (s *Server) SetAuthenticator(a *auth.Authenticator) {
	s.auth = a
}

// SetTLS configures TLS for the dashboard server.
// If both cert and key are non-empty, the server will use HTTPS.
func (s *Server) SetTLS(cert, key string) {
	s.tlsCert = cert
	s.tlsKey = key
}

// IsTLS returns true if TLS is configured.
func (s *Server) IsTLS() bool {
	return s.tlsCert != "" && s.tlsKey != ""
}

// localOnlyMiddleware restricts access to requests from localhost (127.0.0.1, ::1).
// Used for /metrics which should only be scraped by Prometheus from inside the cluster.
func (s *Server) localOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.RemoteAddr
		// Strip port: handle both "IP:port" and "[IPv6]:port" formats
		if strings.HasPrefix(host, "[") {
			// IPv6 format: [::1]:port → strip after last ]
			if idx := strings.LastIndex(host, "]"); idx > 0 {
				host = host[1:idx] // remove brackets
			}
		} else if idx := strings.LastIndex(host, ":"); idx > 0 {
			host = host[:idx]
		}
		if host != "127.0.0.1" && host != "::1" && host != "localhost" {
			http.Error(w, `{"error":"forbidden: metrics endpoint is accessible from localhost only"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminOnlyMiddleware requires the authenticated user to have the "admin" role.
func (s *Server) adminOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromRequest(r)
		if user == nil || user.Role != "admin" {
			writeError(w, 403, "admin role required")
			return
		}
		next(w, r)
	}
}

// Stop gracefully shuts down the server.
// It first marks the server as draining (so /readyz returns 503 and kubelet
// removes this pod from Service endpoints), then waits for in-flight requests
// to complete up to the given context deadline.
func (s *Server) Stop(ctx context.Context) error {
	// Step 1: mark as draining — readiness probe immediately returns 503.
	s.draining.Store(true)
	s.shutdownSignal.Store(true)
	s.log.Info("server marked as draining, /readyz now returns 503",
		"active_connections", s.activeConns.Load())

	// Step 2: wait briefly for kubelet to notice /readyz=503 and remove
	// this pod from Service endpoints (typically 1-5s depending on poll interval).
	// This prevents new connections from arriving during the drain.
	drainWait := 3 * time.Second
	select {
	case <-time.After(drainWait):
	case <-ctx.Done():
		// Context expired during drain wait — proceed to shutdown anyway.
	}

	s.log.Info("proceeding with HTTP server shutdown",
		"remaining_connections", s.activeConns.Load())

	// Step 3: gracefully shut down (drain remaining in-flight requests).
	return s.server.Shutdown(ctx)
}

// connStateTracker tracks active HTTP connections for graceful draining.
func (s *Server) connStateTracker(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew, http.StateActive:
		s.activeConns.Add(1)
	case http.StateIdle, http.StateClosed, http.StateHijacked:
		s.activeConns.Add(-1)
	}
}

// DrainStatus returns the current draining state and active connection count.
// Used by /api/system/drain-status for observability.
type DrainStatus struct {
	Draining          bool  `json:"draining"`
	ShutdownInitiated bool  `json:"shutdownInitiated"`
	ActiveConnections int64 `json:"activeConnections"`
	UptimeSeconds     int64 `json:"uptimeSeconds"`
}

// handleDrainStatus reports the server's draining/shutdown state.
// GET /api/system/drain-status
func (s *Server) handleDrainStatus(w http.ResponseWriter, r *http.Request) {
	var uptime int64
	if s.startTime != nil {
		uptime = int64(time.Since(*s.startTime).Seconds())
	}
	writeJSON(w, DrainStatus{
		Draining:          s.draining.Load(),
		ShutdownInitiated: s.shutdownSignal.Load(),
		ActiveConnections: s.activeConns.Load(),
		UptimeSeconds:     uptime,
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only set CORS headers when the request Origin matches the allowlist.
		// When no origins are configured (default), no CORS headers are emitted,
		// meaning the dashboard is same-origin only — the secure default.
		origin := r.Header.Get("Origin")
		if origin != "" && s.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin") // cache correctly per origin
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed reports whether the given origin is in the configured allowlist.
func (s *Server) isOriginAllowed(origin string) bool {
	for _, allowed := range s.corsAllowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

// parseCORSOrigins parses a comma-separated list of origins from the
// CORS_ALLOWED_ORIGINS environment variable (e.g.
// "https://k8ops.iot2.win,https://k8ops.example.com").
func parseCORSOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var origins []string
	for _, p := range parts {
		o := strings.TrimSpace(p)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": msg})
}

// writeK8sError inspects a K8s API error and writes the appropriate HTTP status.
// Forbidden -> 403, Unauthorized -> 401, NotFound -> 404, else -> 500.
func writeK8sError(w http.ResponseWriter, err error) {
	if err == nil {
		writeError(w, 500, "unknown error")
		return
	}
	errStr := err.Error()
	if strings.Contains(errStr, "forbidden") {
		writeError(w, 403, extractK8sErrMessage(errStr))
		return
	}
	if strings.Contains(errStr, "unauthorized") {
		writeError(w, 401, "unauthorized")
		return
	}
	if strings.Contains(errStr, "not found") || strings.Contains(errStr, "NotFound") {
		writeError(w, 404, errStr)
		return
	}
	writeError(w, 500, errStr)
}

// extractK8sErrMessage extracts the human-readable message from a K8s status error.
func extractK8sErrMessage(s string) string {
	// K8s errors look like: "deployments.apps is forbidden: User \"nsviewer1\" cannot list ..."
	// We want the full message as it's useful for the user
	return s
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "time": time.Now().Format(time.RFC3339)})
}

// handleHealthz is the K8s liveness probe endpoint.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	_, _ = w.Write([]byte("ok\n"))
}

// handleReadyz is the K8s readiness probe endpoint.
// Returns 503 if the k8s API is unreachable OR if the server is draining.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	// During graceful shutdown, immediately return 503 so the kubelet
	// removes this pod from Service endpoints and stops sending new traffic.
	if s.draining.Load() {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(503)
		w.Write([]byte("draining\n"))
		return
	}
	if s.clientset == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(503)
		w.Write([]byte("k8s client not initialized\n"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if _, err := s.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(503)
		w.Write([]byte("k8s API unreachable\n"))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	_, _ = w.Write([]byte("ok\n"))
}

// handleVersion is defined in middleware.go.

func (s *Server) handleClusterOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	overview := map[string]any{}

	// Node count and status
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		ready, notReady := 0, 0
		for _, n := range nodes.Items {
			isReady := false
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady {
					isReady = c.Status == corev1.ConditionTrue
				}
			}
			if isReady {
				ready++
			} else {
				notReady++
			}
		}
		overview["nodes"] = map[string]any{"total": len(nodes.Items), "ready": ready, "notReady": notReady}
	}

	// Namespace count
	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		overview["namespaces"] = len(nss.Items)
	}

	// Diagnostic reports
	diagList := &aiv1alpha1.DiagnosticReportList{}
	if err := rc.ctrlClient.List(ctx, diagList); err == nil {
		byPhase := map[string]int{}
		for _, d := range diagList.Items {
			phase := d.Status.Phase
			if phase == "" {
				phase = "Pending"
			}
			byPhase[phase]++
		}
		overview["diagnostics"] = map[string]any{"total": len(diagList.Items), "byPhase": byPhase}
	}

	// Remediation plans
	remList := &aiv1alpha1.RemediationPlanList{}
	if err := rc.ctrlClient.List(ctx, remList); err == nil {
		byPhase := map[string]int{}
		for _, r := range remList.Items {
			phase := r.Status.Phase
			if phase == "" {
				phase = "Pending"
			}
			byPhase[phase]++
		}
		overview["remediations"] = map[string]any{"total": len(remList.Items), "byPhase": byPhase}
	}

	// Recent warnings
	events, err := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         100,
	})
	if err == nil {
		overview["recentWarnings"] = len(events.Items)
	}

	// Version info + cluster compatibility detection
	info, err := rc.clientset.Discovery().ServerVersion()
	if err == nil {
		overview["clusterVersion"] = info.GitVersion

		// Detect cloud provider, distribution, and version compatibility
		var nodeList []corev1.Node
		if nodes != nil {
			nodeList = nodes.Items
		}
		compat := detectClusterCompat(info.GitVersion, nodeList)
		overview["compatibility"] = compat
	}

	writeJSON(w, overview)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	list := &aiv1alpha1.K8opsConfigList{}
	if err := rc.ctrlClient.List(ctx, list); err != nil {
		writeK8sError(w, err)
		return
	}
	if len(list.Items) == 0 {
		writeJSON(w, map[string]any{"configured": false})
		return
	}
	cfg := list.Items[0]
	writeJSON(w, map[string]any{
		"configured":      true,
		"name":            cfg.Name,
		"provider":        cfg.Spec.Provider.Type,
		"model":           cfg.Spec.Provider.Model,
		"autoRemediation": cfg.Spec.AutoRemediation.Enabled,
		"maxRiskLevel":    cfg.Spec.AutoRemediation.MaxRiskLevel,
		"dryRun":          cfg.Spec.AutoRemediation.DryRun,
	})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get all pods to calculate per-node resource utilization
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodeUsage := make(map[string]struct {
		cpuReq int64 // milli-cores
		memReq int64 // bytes
		pods   int
	})
	for _, p := range allPods.Items {
		if p.Spec.NodeName == "" || p.Status.Phase == "Succeeded" || p.Status.Phase == "Failed" {
			continue
		}
		u := nodeUsage[p.Spec.NodeName]
		u.pods++
		for _, c := range p.Spec.Containers {
			if cpuQ := c.Resources.Requests.Cpu(); cpuQ != nil {
				u.cpuReq += cpuQ.MilliValue()
			}
			if memQ := c.Resources.Requests.Memory(); memQ != nil {
				u.memReq += memQ.Value()
			}
		}
		nodeUsage[p.Spec.NodeName] = u
	}

	type nodeInfo struct {
		Name          string            `json:"name"`
		Status        string            `json:"status"`
		Role          string            `json:"role"`
		Version       string            `json:"version"`
		CPU           string            `json:"cpu"`
		Memory        string            `json:"memory"`
		OS            string            `json:"os"`
		Arch          string            `json:"arch"`
		Conditions    map[string]string `json:"conditions"`
		Unschedulable bool              `json:"unschedulable"`
		// Utilization (requested / allocatable as percentage)
		CPURequested float64 `json:"cpuRequestedPct"`
		MemRequested float64 `json:"memRequestedPct"`
		CPURequests  string  `json:"cpuRequests"`
		MemRequests  string  `json:"memRequests"`
		PodCount     int     `json:"podCount"`
		PodCapacity  int     `json:"podCapacity"`
	}

	results := make([]nodeInfo, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		info := nodeInfo{
			Name:          n.Name,
			Status:        "Ready",
			Version:       n.Status.NodeInfo.KubeletVersion,
			OS:            n.Status.NodeInfo.OperatingSystem,
			Arch:          n.Status.NodeInfo.Architecture,
			CPU:           n.Status.Allocatable.Cpu().String(),
			Memory:        n.Status.Allocatable.Memory().String(),
			Conditions:    make(map[string]string),
			PodCapacity:   int(n.Status.Allocatable.Pods().Value()),
			Unschedulable: n.Spec.Unschedulable,
		}
		for _, c := range n.Status.Conditions {
			info.Conditions[string(c.Type)] = string(c.Status)
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionFalse {
				info.Status = "NotReady"
			}
		}
		for k := range n.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				info.Role = strings.TrimPrefix(k, "node-role.kubernetes.io/")
			}
		}
		if info.Role == "" {
			info.Role = "worker"
		}
		// Calculate utilization from pod requests
		usage := nodeUsage[n.Name]
		info.PodCount = usage.pods
		allocatableCPU := n.Status.Allocatable.Cpu().MilliValue()
		allocatableMem := n.Status.Allocatable.Memory().Value()
		if allocatableCPU > 0 {
			info.CPURequested = float64(usage.cpuReq) / float64(allocatableCPU) * 100
			info.CPURequests = fmt.Sprintf("%dm / %dm", usage.cpuReq, allocatableCPU)
		}
		if allocatableMem > 0 {
			info.MemRequested = float64(usage.memReq) / float64(allocatableMem) * 100
			info.MemRequests = fmt.Sprintf("%.1fGi / %.1fGi", float64(usage.memReq)/1024/1024/1024, float64(allocatableMem)/1024/1024/1024)
		}
		results = append(results, info)
	}

	// Sort by name
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	namespace := r.URL.Query().Get("namespace")
	warning := r.URL.Query().Get("warning") == "true"
	limit := 50

	fieldSelector := ""
	if warning {
		fieldSelector = "type=Warning"
	}

	var events *corev1.EventList
	var err error
	if namespace != "" {
		events, err = rc.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fieldSelector,
			Limit:         int64(limit),
		})
	} else {
		events, err = rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
			FieldSelector: fieldSelector,
			Limit:         int64(limit),
		})
	}
	if err != nil {
		writeK8sError(w, err)
		return
	}

	type eventInfo struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Object    string `json:"object"`
		Namespace string `json:"namespace"`
		Count     int32  `json:"count"`
		LastTime  string `json:"lastTime"`
	}

	results := make([]eventInfo, 0, len(events.Items))
	for _, e := range events.Items {
		results = append(results, eventInfo{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   truncate(e.Message, 300),
			Object:    fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			Namespace: e.InvolvedObject.Namespace,
			Count:     e.Count,
			LastTime:  e.LastTimestamp.Format(time.RFC3339),
		})
	}

	// Sort by last seen time, newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].LastTime > results[j].LastTime
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// parseInt parses an integer from a string, returning fallback on error.
func parseInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

// userName extracts the current user's name from the request, falling back to "unknown".
func userName(r *http.Request) string {
	u := auth.UserFromRequest(r)
	if u == nil {
		return "unknown"
	}
	return u.Username
}

// --- Pods endpoint (lightweight listing) ---

func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	namespace := r.URL.Query().Get("namespace")
	fieldSelector := ""

	pods, err := rc.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector, Limit: 200})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	type podInfo struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Phase     string `json:"phase"`
		Node      string `json:"node"`
		Restarts  int32  `json:"restarts"`
		Age       string `json:"age"`
	}

	results := make([]podInfo, 0, len(pods.Items))
	for _, p := range pods.Items {
		restarts := int32(0)
		for _, c := range p.Status.ContainerStatuses {
			restarts += c.RestartCount
		}
		results = append(results, podInfo{
			Name: p.Name, Namespace: p.Namespace,
			Phase: string(p.Status.Phase), Node: p.Spec.NodeName,
			Restarts: restarts,
			Age:      formatDuration(time.Since(p.CreationTimestamp.Time)),
		})
	}

	// Sort by namespace, then name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Namespace != results[j].Namespace {
			return results[i].Namespace < results[j].Namespace
		}
		return results[i].Name < results[j].Name
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

func formatDuration(d time.Duration) string {
	if d > 24*time.Hour {
		return fmt.Sprintf("%.0fd", d.Hours()/24)
	}
	if d > time.Hour {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
	return fmt.Sprintf("%.0fm", d.Minutes())
}

// Slack webhook handler moved to handlers_slack.go
