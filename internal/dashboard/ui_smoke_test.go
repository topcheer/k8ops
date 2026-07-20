package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuditDashboardFrontendCoverage verifies that all key v18.28+ API endpoints
// are referenced in the frontend audit-dashboard.js file.
func TestAuditDashboardFrontendCoverage(t *testing.T) {
	// Find audit-dashboard.js relative to test file
	paths := []string{
		"audit-dashboard.js",
		filepath.Join("..", "..", "internal", "dashboard", "web", "audit-dashboard.js"),
		"/Volumes/new/ggai/k8ops/internal/dashboard/web/audit-dashboard.js",
	}

	var content string
	var found bool
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			content = string(data)
			found = true
			break
		}
	}
	if !found {
		t.Skip("audit-dashboard.js not found from test working directory")
	}

	// Validate key endpoints from the restructured dashboard are present.
	// Tests a representative sample across all dimensions and subcategories.
	requiredEndpoints := []string{
		// Security - RBAC
		"/api/security/rbac-audit",
		"/api/security/sa-token-lifecycle",
		"/api/security/rbac-effective",
		// Security - Secrets
		"/api/security/secret-scan",
		"/api/security/secret-rotation-v2",
		"/api/security/secret-exposure",
		// Security - Pod Security
		"/api/security/pss-scorecard",
		"/api/security/seccomp-audit",
		"/api/security/container-hardening",
		// Security - Network
		"/api/security/network-policies",
		"/api/security/netpol-generator",
		// Security - Compliance
		"/api/security/compliance-map",
		"/api/security/kyverno-compliance",
		"/api/security/opa-compliance",
		// Security - Supply Chain
		"/api/security/image-vuln",
		"/api/security/supply-chain",
		// Security - Runtime
		"/api/security/runtime-scan",
		// Security - Certs
		"/api/security/cert-expiry",
		// Security - Posture
		"/api/security/attack-surface",
		"/api/security/hardening-score",
		"/api/security/fix-plan",
		// Operations - Control Plane
		"/api/operations/etcd-health",
		"/api/operations/kubelet-health",
		"/api/operations/cni-health",
		// Operations - Observability
		"/api/operations/metrics-pipeline",
		"/api/operations/prom-health",
		"/api/operations/grafana-health",
		// Operations - Pod Health
		"/api/operations/pod-health-index",
		"/api/operations/crashloop",
		"/api/operations/crash-budget-tracker",
		"/api/operations/restart-analyzer",
		// Operations - Events
		"/api/operations/event-storm",
		"/api/operations/incident-correlation",
		// Operations - SLO
		"/api/operations/pod-slo",
		"/api/operations/slo-burn-rate",
		"/api/operations/health-score",
		// Operations - Node
		"/api/operations/node-pressure",
		"/api/operations/pdb-audit",
		// Operations - API
		"/api/operations/api-load",
		// Operations - Reliability
		"/api/operations/chaos-readiness",
		"/api/operations/throttle-risk",
		// Scalability - Cost
		"/api/scalability/cost-waste",
		"/api/scalability/cost-allocation",
		"/api/scalability/idle-waste",
		// Scalability - Autoscaling
		"/api/scalability/hpa-performance",
		"/api/scalability/autoscale-readiness",
		"/api/scalability/vpa-audit",
		// Scalability - Resource
		"/api/scalability/alloc-efficiency",
		"/api/scalability/overcommit-risk",
		"/api/scalability/right-size-engine",
		// Scalability - Node
		"/api/scalability/node-lifecycle",
		"/api/scalability/node-utilization-deep",
		"/api/scalability/node-life-forecast",
		// Scalability - Storage
		"/api/scalability/pv-reclaim",
		"/api/scalability/storage-performance",
		// Scalability - Scheduling
		"/api/scalability/scheduling-intel",
		"/api/scalability/scheduler-fairness",
		"/api/scalability/binpack-efficiency",
		// Scalability - HA
		"/api/scalability/dr-readiness",
		"/api/scalability/cluster-fault-tolerance",
		// Scalability - Capacity
		"/api/scalability/capacity-headroom",
		"/api/scalability/capacity-forecast-deep",
		// Scalability - Cleanup
		"/api/scalability/orphan-cleanup",
		"/api/scalability/green-computing",
		// Product - Service
		"/api/product/service-connectivity",
		"/api/product/service-catalog",
		"/api/product/service-dependency-map",
		// Product - Mesh
		"/api/product/mesh-health",
		"/api/product/ingress-health",
		// Product - Endpoints
		"/api/product/endpoint-dns-health",
		"/api/product/endpoint-health-deep",
		// Product - Workload
		"/api/product/workload-criticality",
		"/api/product/reliability-scorecard",
		// Product - Config
		"/api/product/configmap-size",
		"/api/product/label-hygiene",
		"/api/product/ownership-map",
		// Product - Placement
		"/api/product/placement-score",
		"/api/product/topology-spread",
		// Product - API
		"/api/product/api-version-governance",
		"/api/product/slo-compliance",
		// Deployment - GitOps
		"/api/deployment/helm-health",
		"/api/deployment/helm-drift-monitor",
		"/api/deployment/gitops-audit",
		// Deployment - Rollout
		"/api/deployment/progressive-delivery",
		"/api/deployment/rollout-health",
		// Deployment - Rollback
		"/api/deployment/rollback-risk",
		"/api/deployment/rollback-safety",
		// Deployment - Image
		"/api/deployment/image-hygiene",
		"/api/deployment/image-freshness",
		// Deployment - Readiness
		"/api/deployment/preflight-check",
		"/api/deployment/readiness-gate",
		// Deployment - DORA
		"/api/deployment/dora-metrics",
		"/api/deployment/deploy-frequency",
		// Deployment - Probes
		"/api/deployment/probe-compliance",
		// Deployment - Drift
		"/api/deployment/revision-drift",
		"/api/deployment/config-sync",
		// Docs
		"/api/docs/platform-scorecard",
		"/api/docs/api-coverage-map",
		"/api/docs/api-explorer",
		"/api/docs/action-priority-matrix",
		"/api/docs/oncall-readiness",
		"/api/docs/upgrade-planner",
	}

	missing := []string{}
	for _, ep := range requiredEndpoints {
		if !strings.Contains(content, ep) {
			missing = append(missing, ep)
		}
	}

	if len(missing) > 0 {
		t.Errorf("audit-dashboard.js is missing %d endpoints:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

// TestFrontendFilesExist verifies critical frontend files are present.
func TestFrontendFilesExist(t *testing.T) {
	basePaths := []string{
		".",
		filepath.Join("..", "..", "internal", "dashboard", "web"),
		"/Volumes/new/ggai/k8ops/internal/dashboard/web",
	}

	criticalFiles := []string{
		"index.html",
		"main.js",
		"core.js",
		"overview.js",
		"audit-dashboard.js",
		"modules/utils.js",
		"styles.css",
	}

	var baseDir string
	for _, bp := range basePaths {
		if _, err := os.Stat(filepath.Join(bp, "index.html")); err == nil {
			baseDir = bp
			break
		}
	}
	if baseDir == "" {
		t.Skip("frontend directory not found from test working directory")
	}

	for _, f := range criticalFiles {
		path := filepath.Join(baseDir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing critical frontend file: %s", f)
		}
	}
}
