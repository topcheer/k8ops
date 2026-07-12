// Package k8s — Cluster audit tool that exposes all dashboard audit endpoints to the LLM agent.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ggai/k8ops/internal/tools"
)

// auditEndpoint maps an audit name to its dashboard API path.
type auditEndpoint struct {
	name string
	path string
	desc string
}

// auditRegistry is the complete list of all audit/analysis endpoints available to the agent.
// When adding a new audit endpoint in server.go, also add it here so the agent can use it.
var auditRegistry = []auditEndpoint{
	// --- Product ---
	{"product:staleness", "/api/product/staleness", "Workload staleness & release cadence"},
	{"product:ingress-health", "/api/product/ingress-health", "Ingress traffic routing health"},
	{"product:namespace-lifecycle", "/api/product/namespaces/lifecycle", "Namespace governance & lifecycle"},
	{"product:dns-health", "/api/product/dns-health", "DNS resolution health checker"},
	{"product:config-audit", "/api/product/config-audit", "ConfigMap & Secret configuration audit"},
	{"product:network-policy", "/api/product/network-policy", "Network policy compliance & traffic isolation"},
	{"product:label-hygiene", "/api/product/label-hygiene", "Label & annotation hygiene auditor"},
	{"product:orphaned-resources", "/api/product/orphaned-resources", "Orphaned resource detector"},
	{"product:pvc-health", "/api/product/pvc-health", "PV/PVC storage health & capacity"},
	{"product:statefulset-audit", "/api/product/statefulset-audit", "StatefulSet health & ordered rollout audit"},
	{"product:affinity-conflict", "/api/product/affinity-conflict", "Affinity & anti-affinity conflict detector"},
	{"product:taint-toleration", "/api/product/taint-toleration", "Node taint & pod toleration impact analyzer"},
	{"product:configmap-size", "/api/product/configmap-size", "ConfigMap/Secret size & memory pressure auditor"},
	{"product:job-health", "/api/product/job-health", "Batch job execution health & completion analyzer"},
	{"product:hpa-health", "/api/product/hpa-health", "HPA health & scaling activity analyzer"},
	{"product:api-deprecation", "/api/product/api-deprecation", "Deprecated API version & upgrade readiness checker"},
	{"product:qos-priority", "/api/product/qos-priority", "Pod QoS & priority class distribution auditor"},
	{"product:service-connectivity", "/api/product/service-connectivity", "Service endpoint & connectivity health auditor"},
	{"product:topology-spread", "/api/product/topology-spread", "Topology spread constraint validator"},
	{"product:backup-compliance", "/api/product/backup-compliance", "Volume snapshot & PVC backup compliance auditor"},
	{"product:init-container-audit", "/api/product/init-container-audit", "Init container reliability & startup dependency auditor"},

	// --- Deployment ---
	{"deployment:image-hygiene", "/api/deployment/image-hygiene", "Container image deployment hygiene analyzer"},
	{"deployment:revision-history", "/api/deployment/revision-history", "Deployment revision history & rollback readiness"},
	{"deployment:disruption-impact", "/api/deployment/disruption-impact", "Deployment PDB disruption & maintenance impact"},
	{"deployment:workload-maturity", "/api/deployment/workload-maturity", "Workload maturity & best practices scorer"},
	{"deployment:ephemeral-storage", "/api/deployment/ephemeral-storage", "Ephemeral storage & emptyDir limit compliance"},
	{"deployment:config-sync", "/api/deployment/config-sync", "ConfigMap/Secret config sync & staleness detector"},
	{"deployment:sidecar-audit", "/api/deployment/sidecar-audit", "Sidecar container overhead & injection auditor"},
	{"deployment:restart-policy", "/api/deployment/restart-policy", "Restart policy & lifecycle hook auditor"},
	{"deployment:scale-readiness", "/api/deployment/scale-readiness", "Deployment scale readiness & autoscaling gap detector"},
	{"deployment:rollout-health", "/api/deployment/rollout-health", "Deployment rollout strategy & health analyzer"},
	{"deployment:probe-compliance", "/api/deployment/probe-compliance", "Health probe compliance auditor"},
	{"deployment:resource-limits", "/api/deployment/resource-limits", "Resource limit & enforcement gap audit"},
	{"deployment:graceful-shutdown", "/api/deployment/graceful-shutdown", "Graceful shutdown & termination compliance"},
	{"deployment:update-strategy", "/api/deployment/update-strategy", "Deployment update strategy & rollback readiness"},
	{"deployment:ref-integrity", "/api/deployment/ref-integrity", "Secret/ConfigMap reference integrity checker"},
	{"deployment:image-drift", "/api/deployment/image-drift", "Deployment image drift & version consistency detector"},
	{"deployment:replica-availability", "/api/deployment/replica-availability", "Deployment replica availability & ready pod ratio monitor"},

	// --- Operations ---
	{"operations:cronjobs-health", "/api/operations/cronjobs/health", "CronJob execution health"},
	{"operations:slo", "/api/operations/slo", "SLO/SLA error budget"},
	{"operations:event-storm", "/api/operations/event-storm", "Event storm & cascade detection"},
	{"operations:probes", "/api/operations/probes", "Health probe effectiveness audit"},
	{"operations:health-score", "/api/operations/health-score", "Cluster health score aggregator"},
	{"operations:node-pressure", "/api/operations/node-pressure", "Node condition & resource pressure"},
	{"operations:oom-tracker", "/api/operations/oom-tracker", "Container OOM kill tracker"},
	{"operations:crashloop", "/api/operations/crashloop", "CrashLoopBackOff detector & crash pattern analyzer"},
	{"operations:pdb-audit", "/api/operations/pdb-audit", "PDB compliance & voluntary disruption risk"},
	{"operations:topology-distribution", "/api/operations/topology-distribution", "Topology spread & pod distribution audit"},
	{"operations:image-pull-failures", "/api/operations/image-pull-failures", "Image pull & container start failure tracker"},
	{"operations:restart-reasons", "/api/operations/restart-reasons", "Pod restart reason analyzer"},
	{"operations:scheduling-latency", "/api/operations/scheduling-latency", "Pod scheduling latency analyzer"},
	{"operations:resource-contention", "/api/operations/resource-contention", "Resource contention & throttling detector"},
	{"operations:node-lease", "/api/operations/node-lease", "Node lease & heartbeat health monitor"},
	{"operations:control-plane", "/api/operations/control-plane", "Control plane health checker"},
	{"operations:pod-evictions", "/api/operations/pod-evictions", "Pod eviction & node pressure history tracker"},
	{"operations:api-latency", "/api/operations/api-latency", "API server responsiveness & pod start latency monitor"},
	{"operations:volume-mount-errors", "/api/operations/volume-mount-errors", "Volume mount & attach error tracker"},
	{"operations:pod-startup", "/api/operations/pod-startup", "Pod startup lifecycle & bottleneck analyzer"},
	{"operations:kubelet-health", "/api/operations/kubelet-health", "Kubelet & container runtime health monitor"},
	{"operations:dns-health", "/api/operations/dns-health", "DNS resolution health & CoreDNS monitor"},
	{"operations:csr-monitor", "/api/operations/csr-monitor", "Certificate signing request & node bootstrap cert monitor"},
	{"operations:etcd-health", "/api/operations/etcd-health", "etcd health & database pressure monitor"},

	// --- Security ---
	{"security:audit", "/api/security/audit", "Cluster-wide security scan"},
	{"security:secrets", "/api/security/secrets", "Secret data exposure & credential leak scanner"},
	{"security:network-policies", "/api/security/network-policies", "NetworkPolicy audit"},
	{"security:health", "/api/security/health", "Platform security health check"},
	{"security:compliance", "/api/security/compliance", "CIS benchmark compliance scan"},
	{"security:pods", "/api/security/pods", "Pod security posture scan"},
	{"security:secrets-rotation", "/api/security/secrets/rotation", "Secret lifecycle & rotation audit"},
	{"security:images", "/api/security/images", "Image supply chain security"},
	{"security:containers", "/api/security/containers", "Container security context audit"},
	{"security:rbac-risk", "/api/security/rbac-risk", "RBAC permission risk analysis"},
	{"security:service-accounts", "/api/security/service-accounts", "ServiceAccount security audit"},
	{"security:rbac-effective", "/api/security/rbac-effective", "RBAC effective permissions & escalation"},
	{"security:admission-audit", "/api/security/admission-audit", "Admission webhook configuration audit"},
	{"security:audit-policy", "/api/security/audit-policy", "API server audit logging configuration checker"},
	{"security:encryption-at-rest", "/api/security/encryption-at-rest", "Secret encryption at rest configuration checker"},
	{"security:host-namespace", "/api/security/host-namespace", "Container host namespace & privilege exposure auditor"},
	{"security:cert-expiry", "/api/security/cert-expiry", "Certificate & TLS expiry monitor"},
	{"security:volume-mounts", "/api/security/volume-mounts", "Volume & mount risk security audit"},
	{"security:endpoint-exposure", "/api/security/endpoint-exposure", "Service endpoint exposure & attack surface audit"},
	{"security:seccomp-audit", "/api/security/seccomp-audit", "Seccomp profile & PSS restricted compliance"},
	{"security:batch-audit", "/api/security/batch-audit", "CronJob & batch job security audit"},
	{"security:psa-audit", "/api/security/psa-audit", "Pod security admission enforcement auditor"},
	{"security:mac-audit", "/api/security/mac-audit", "AppArmor & SELinux MAC compliance auditor"},
	{"security:forensics", "/api/security/forensics", "Pod security forensics & incident evidence collector"},
	{"security:rbac-audit", "/api/security/rbac-audit", "RBAC overprivilege & wildcard permission auditor"},
	{"security:secret-scan", "/api/security/secret-scan", "Secret data exposure & env var credential leak scanner"},

	// --- Scalability ---
	{"scalability:overcommit", "/api/scalability/overcommit", "Resource over-commit & pressure"},
	{"scalability:autoscale-recommendations", "/api/scalability/autoscale-recommendations", "HPA/VPA right-sizing"},
	{"scalability:pvc-analysis", "/api/scalability/pvc-analysis", "PVC binding & storage performance"},
	{"scalability:storage-forecast", "/api/scalability/storage-forecast", "Storage capacity exhaustion predictor"},
	{"scalability:pod-density", "/api/scalability/pod-density", "Pod density & scheduling capacity analyzer"},
	{"scalability:ns-consumption", "/api/scalability/ns-consumption", "Namespace resource consumption & cost attribution"},
	{"scalability:capacity-headroom", "/api/scalability/capacity-headroom", "Cluster capacity headroom & scale-out readiness"},
	{"scalability:quota-utilization", "/api/scalability/quota-utilization", "Resource quota utilization & limit compliance"},
	{"scalability:ha-audit", "/api/scalability/ha-audit", "HA & single-point-of-failure detector"},
	{"scalability:node-failure-sim", "/api/scalability/node-failure-sim", "Node failure impact simulator"},
	{"scalability:crd-explosion", "/api/scalability/crd-explosion", "API object count & CRD explosion risk detector"},
	{"scalability:bottleneck-predictor", "/api/scalability/bottleneck-predictor", "K8s scalability bottleneck predictor"},
	{"scalability:namespace-isolation", "/api/scalability/namespace-isolation", "Namespace isolation & multi-tenancy audit"},
	{"scalability:csi-audit", "/api/scalability/csi-audit", "CSI driver & storage capability auditor"},
	{"scalability:scale-limits", "/api/scalability/scale-limits", "Cluster scalability limits & threshold monitor"},
	{"scalability:dr-readiness", "/api/scalability/dr-readiness", "Disaster recovery readiness & backup compliance auditor"},
	{"scalability:fragmentation", "/api/scalability/fragmentation", "Resource fragmentation & bin-packing efficiency analyzer"},
	{"scalability:ip-cidr-utilization", "/api/scalability/ip-cidr-utilization", "IP address & Pod CIDR utilization monitor"},
	{"scalability:node-topology", "/api/scalability/node-topology", "Node topology distribution & multi-AZ fault tolerance analyzer"},
	{"scalability:tenant-pressure", "/api/scalability/tenant-pressure", "Multi-tenant resource pressure & quota competition auditor"},

	// --- Other audits ---
	{"certificates:expiry", "/api/certificates/expiry", "Certificate & TLS expiry monitor"},
	{"compatibility", "/api/compatibility", "Cluster compatibility & upgrade readiness"},
	{"pdbs", "/api/pdbs", "Pod Disruption Budget list"},
	{"images", "/api/images", "Container image inventory"},
	{"addons:health", "/api/addons/health", "Add-on health scan"},
	{"deployments:rollout", "/api/deployments/rollout", "Deployment rollout health"},
	{"resources:waste", "/api/resources/waste", "Resource waste detection"},
	{"resources:quota", "/api/resources/quota", "ResourceQuota & LimitRange monitor"},
	{"scaling:bottlenecks", "/api/scaling/bottlenecks", "Scaling bottleneck detection"},
	{"dependencies", "/api/dependencies", "Resource dependency graph & blast radius"},
	{"topology:spread", "/api/topology/spread", "Topology spread compliance"},
	{"networking:health", "/api/networking/health", "Service & endpoint health"},
	{"storage:health", "/api/storage/health", "PV/PVC storage health"},
	{"scheduling:health", "/api/scheduling/health", "Scheduling health & fragmentation"},
	{"deployments:audit", "/api/deployments/audit", "Deployment config audit"},
	{"efficiency", "/api/efficiency", "Resource efficiency analysis"},
	{"cost:summary", "/api/cost/summary", "Cost summary"},
	{"cost:recommendations", "/api/cost/recommendations", "Cost optimization recommendations"},

	// --- Cluster overview & diagnostics ---
	{"cluster:overview", "/api/cluster/overview", "Cluster overview: nodes, pods, resources, health summary"},
	{"cluster:diagnostics", "/api/diagnostics", "Run cluster diagnostics (restart analysis, common issues)"},
	{"cluster:diagnostics-restarts", "/api/diagnostics/restarts", "Pod restart diagnosis"},
	{"cluster:events", "/api/events", "Recent cluster events"},
	{"cluster:events-summary", "/api/events/summary", "Event summary & statistics"},
	{"cluster:audit-log", "/api/audit", "Audit log entries"},
	{"cluster:audit-stats", "/api/audit/stats", "Audit log statistics"},
	{"cluster:audit-events", "/api/audit/events", "Audit log events"},
	{"cluster:remediations", "/api/remediations", "Remediation actions list"},
	{"cluster:optimizations", "/api/optimizations", "Optimization suggestions list"},

	// --- Resource browser ---
	{"cluster:resources", "/api/resources", "List all resources across the cluster"},
	{"cluster:crds", "/api/crds", "List Custom Resource Definitions"},
	{"cluster:images-inventory", "/api/images", "Container image inventory (reuse if distinct from security:images)"},

	// --- Infrastructure ---
	{"infra:storage-capacity", "/api/storage/capacity", "Storage capacity & usage"},
	{"infra:capacity-planning", "/api/capacity/planning", "Capacity planning & recommendations"},
	{"infra:capacity-forecast", "/api/capacity/forecast", "Capacity forecast & exhaustion prediction"},
	{"infra:system-info", "/api/system/info", "System information (Go version, memory, goroutines)"},
	{"infra:api-performance", "/api/system/performance", "API performance metrics (p50/p95/p99 latency)"},
	{"infra:namespaces-ranking", "/api/namespaces/ranking", "Namespace ranking by resource usage"},
	{"infra:hpas", "/api/hpa", "Horizontal Pod Autoscaler list"},
}

// AuditTool lets the LLM agent run any registered cluster audit/analysis endpoint.
type AuditTool struct {
	DashboardAddr string // e.g. "localhost:9090"
}

func (t *AuditTool) Name() string { return "k8s_run_audit" }

func (t *AuditTool) Description() string {
	var names []string
	for _, a := range auditRegistry {
		names = append(names, a.name)
	}
	return fmt.Sprintf("Run a cluster audit or analysis tool. Returns structured JSON with findings, scores, and recommendations. "+
		"Available audit_name values: %s. "+
		"Use this to quickly get health scores, compliance status, risk analysis, and operational insights without manually inspecting individual resources.",
		strings.Join(names, ", "))
}

func (t *AuditTool) Parameters() map[string]any {
	names := make([]string, len(auditRegistry))
	for i, a := range auditRegistry {
		names[i] = a.name
	}
	return tools.Schema(map[string]tools.Property{
		"audit_name": {
			Type:        "string",
			Description: "The audit to run (see available values above)",
			Enum:        names,
		},
	}, []string{"audit_name"})
}

func (t *AuditTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	auditName, err := tools.GetString(args, "audit_name")
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Find the audit endpoint
	var endpoint string
	var desc string
	found := false
	for _, a := range auditRegistry {
		if a.name == auditName {
			endpoint = a.path
			desc = a.desc
			found = true
			break
		}
	}
	if !found {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown audit: %s. Use k8s_list_audits to see available options.", auditName),
		}, nil
	}

	// Build URL
	addr := t.DashboardAddr
	if addr == "" {
		addr = "localhost:9090"
	}
	url := fmt.Sprintf("http://%s%s", addr, endpoint)

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to create request: %v", err)}, nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to call audit endpoint: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to read response: %v", err)}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("audit returned HTTP %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// Try to pretty-print if not already
	var pretty map[string]any
	if json.Unmarshal(body, &pretty) == nil {
		prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
		return &tools.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Audit: %s (%s)\n\n%s", auditName, desc, string(prettyBytes)),
		}, nil
	}

	return &tools.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Audit: %s (%s)\n\n%s", auditName, desc, string(body)),
	}, nil
}

// --- ListAuditsTool: list all available audits ---

type ListAuditsTool struct{}

func (t *ListAuditsTool) Name() string { return "k8s_list_audits" }
func (t *ListAuditsTool) Description() string {
	return "List all available cluster audit and analysis tools that can be run via k8s_run_audit. " +
		"Returns audit names, descriptions, and categories."
}
func (t *ListAuditsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *ListAuditsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	type auditInfo struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Desc     string `json:"description"`
	}

	audits := make([]auditInfo, 0, len(auditRegistry))
	for _, a := range auditRegistry {
		cat := "other"
		if idx := strings.Index(a.name, ":"); idx > 0 {
			cat = a.name[:idx]
		}
		audits = append(audits, auditInfo{Name: a.name, Category: cat, Desc: a.desc})
	}

	data, _ := json.MarshalIndent(map[string]any{
		"count":  len(audits),
		"audits": audits,
	}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}
