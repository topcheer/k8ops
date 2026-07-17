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

	// All v18.28-v18.36 APIs that must have frontend visibility
	requiredEndpoints := []string{
		"/api/operations/chaos-readiness",
		"/api/security/supply-chain",
		"/api/scalability/capacity-forecast-deep",
		"/api/operations/drain-impact",
		"/api/scalability/request-accuracy",
		"/api/security/hardening-score",
		"/api/security/fix-plan",
		"/api/docs/api-coverage-map",
		"/api/deployment/release-gate",
		"/api/product/service-catalog",
		"/api/operations/resource-topology",
		"/api/docs/api-explorer",
		"/api/scalability/orphan-cleanup",
		"/api/scalability/cost-anomaly",
		"/api/deployment/config-snapshot",
		"/api/operations/pod-health-index",
		"/api/product/namespace-quota-map",
		"/api/security/secret-exposure",
		"/api/docs/cluster-maturity",
		"/api/scalability/right-size-engine",
		"/api/deployment/deploy-risk",
		"/api/operations/pdb-generator",
		"/api/security/netpol-generator",
		"/api/product/service-dependency-map",
		"/api/scalability/quota-generator",
		"/api/deployment/probe-generator",
		"/api/docs/platform-insights",
		"/api/docs/action-priority-matrix",
		"/api/operations/health-trend",
		"/api/scalability/image-cleanup",
		"/api/operations/restart-analyzer",
		"/api/security/env-leak-scanner",
		"/api/deployment/update-strategy-auditor",
		"/api/product/label-score",
		"/api/scalability/storage-tier",
		"/api/security/trust-chain",
		"/api/operations/alert-fatigue",
		"/api/deployment/deploy-frequency",
		"/api/docs/platform-comparison",
		"/api/security/container-hardening",
		"/api/scalability/autoscale-readiness",
		"/api/product/workload-efficiency",
		"/api/operations/capacity-gap",
		"/api/deployment/revision-drift",
		"/api/docs/knowledge-base",
		"/api/security/compliance-gap",
		"/api/scalability/scheduler-fairness",
		"/api/product/workload-fingerprint",
		"/api/deployment/deploy-heatmap",
		"/api/operations/log-volume",
		"/api/docs/cluster-narrative",
		"/api/security/config-audit-trail",
		"/api/scalability/node-utilization-deep",
		"/api/security/secret-rotation-plan",
		"/api/operations/event-correlation-deep",
		"/api/deployment/rollback-simulator",
		"/api/docs/upgrade-planner",
		"/api/security/rbac-drift",
		"/api/scalability/resource-forecast",
		"/api/product/config-warmstart",
		"/api/operations/pod-slo",
		"/api/deployment/deploy-readiness-gate",
		"/api/docs/api-governance-score",
		"/api/security/disruption-budget-gap",
		"/api/product/cost-topology",
		"/api/scalability/binpack-efficiency",
		"/api/operations/slo-burn-rate",
		"/api/deployment/surge-capacity",
		"/api/docs/runbook-coverage",
		"/api/security/privilege-map",
		"/api/product/api-slo-correlation",
		"/api/scalability/eviction-risk",
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
