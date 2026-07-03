package dashboard

import (
	"encoding/json"
	"net/http"
	"sort"
)

// OpenAPIInfo describes the API.
type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// OpenAPIServer describes a server URL.
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// OpenAPIParam describes a single parameter.
type OpenAPIParam struct {
	Name        string                 `json:"name"`
	In          string                 `json:"in"`
	Description string                 `json:"description"`
	Required    bool                   `json:"required"`
	Schema      map[string]interface{} `json:"schema"`
}

// OpenAPIResponse describes an HTTP response.
type OpenAPIResponse struct {
	Description string                 `json:"description"`
	Content     map[string]interface{} `json:"content,omitempty"`
}

// OpenAPIOperation describes a single API operation.
type OpenAPIOperation struct {
	Summary     string                     `json:"summary"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId"`
	Tags        []string                   `json:"tags"`
	Parameters  []OpenAPIParam             `json:"parameters,omitempty"`
	RequestBody map[string]interface{}     `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
}

// OpenAPISpec is the top-level OpenAPI 3.0 document.
type OpenAPISpec struct {
	OpenAPI    string                                 `json:"openapi"`
	Info       OpenAPIInfo                            `json:"info"`
	Servers    []OpenAPIServer                        `json:"servers"`
	Paths      map[string]map[string]OpenAPIOperation `json:"paths"`
	Components map[string]interface{}                 `json:"components,omitempty"`
}

// handleOpenAPISpec serves the OpenAPI 3.0 specification as JSON.
// GET /api/openapi.json
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := buildOpenAPISpec()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}

func jsonContent(example interface{}) map[string]interface{} {
	return map[string]interface{}{
		"application/json": map[string]interface{}{
			"schema": map[string]interface{}{
				"type":    "object",
				"example": example,
			},
		},
	}
}

func okResponse(desc string, example interface{}) OpenAPIResponse {
	return OpenAPIResponse{
		Description: desc,
		Content:     jsonContent(example),
	}
}

func errResponse(desc string) OpenAPIResponse {
	return OpenAPIResponse{
		Description: desc,
		Content: jsonContent(map[string]string{
			"error": "error description",
		}),
	}
}

func queryParam(name, desc string) OpenAPIParam {
	return OpenAPIParam{
		Name: name, In: "query", Description: desc,
		Schema: map[string]interface{}{"type": "string"},
	}
}

func bodyParam(desc string, example interface{}) map[string]interface{} {
	return map[string]interface{}{
		"description": desc,
		"required":    true,
		"content":     jsonContent(example),
	}
}

func buildOpenAPISpec() OpenAPISpec {
	spec := OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       "k8ops API",
			Description: "AIOps platform for Kubernetes management — diagnostics, remediations, resources, cost analysis, and security audit.",
			Version:     Version,
		},
		Servers: []OpenAPIServer{
			{URL: "/", Description: "Relative to server"},
		},
		Paths: map[string]map[string]OpenAPIOperation{},
	}

	add := func(path, method string, op OpenAPIOperation) {
		if spec.Paths[path] == nil {
			spec.Paths[path] = map[string]OpenAPIOperation{}
		}
		spec.Paths[path][method] = op
	}

	// --- Health & Version ---
	add("/api/health", "get", OpenAPIOperation{
		Summary: "Health check", OperationID: "health", Tags: []string{"System"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Server is healthy", map[string]string{"status": "ok"}),
		},
	})
	add("/api/version", "get", OpenAPIOperation{
		Summary: "Get version info", OperationID: "version", Tags: []string{"System"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Version details", map[string]string{"version": "v14.x", "gitCommit": "abc1234"}),
		},
	})
	add("/healthz", "get", OpenAPIOperation{
		Summary: "K8s liveness probe", OperationID: "healthz", Tags: []string{"System"},
		Responses: map[string]OpenAPIResponse{"200": {Description: "ok"}},
	})
	add("/readyz", "get", OpenAPIOperation{
		Summary: "K8s readiness probe", OperationID: "readyz", Tags: []string{"System"},
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "Ready"}, "503": {Description: "Not ready"},
		},
	})

	// --- Cluster Overview ---
	add("/api/cluster/overview", "get", OpenAPIOperation{
		Summary: "Cluster overview", OperationID: "clusterOverview", Tags: []string{"Cluster"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cluster summary with nodes, namespaces, diagnostics", map[string]interface{}{
				"nodes": map[string]int{"total": 3, "ready": 3, "notReady": 0},
			}),
		},
	})

	// --- Nodes ---
	add("/api/nodes", "get", OpenAPIOperation{
		Summary: "List nodes", OperationID: "listNodes", Tags: []string{"Nodes"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node list with utilization", map[string]interface{}{
				"count": 1, "items": []map[string]interface{}{{"name": "node-1", "status": "Ready"}},
			}),
		},
	})
	add("/api/nodes/{node}/pods", "get", OpenAPIOperation{
		Summary: "List pods on a node", OperationID: "nodePods", Tags: []string{"Nodes"},
		Parameters: []OpenAPIParam{
			{Name: "node", In: "path", Required: true, Description: "Node name",
				Schema: map[string]interface{}{"type": "string"}},
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pods running on the node", map[string]interface{}{}),
		},
	})
	add("/api/node/cordon", "post", OpenAPIOperation{
		Summary: "Cordon or uncordon a node", OperationID: "nodeCordon", Tags: []string{"Nodes", "Write Ops"},
		RequestBody: bodyParam("Node cordon request", map[string]interface{}{
			"node": "worker-1", "uncordon": false,
		}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node cordon status updated", map[string]bool{"success": true}),
			"400": errResponse("Invalid request"),
		},
	})

	// --- Pods ---
	add("/api/pods", "get", OpenAPIOperation{
		Summary: "List pods", OperationID: "listPods", Tags: []string{"Pods"},
		Parameters: []OpenAPIParam{queryParam("namespace", "Filter by namespace")},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod list", map[string]interface{}{
				"count": 10, "items": []map[string]interface{}{},
			}),
		},
	})
	add("/api/pods/{namespace}/{name}/logs", "get", OpenAPIOperation{
		Summary: "Get pod logs", OperationID: "podLogs", Tags: []string{"Pods"},
		Parameters: []OpenAPIParam{
			{Name: "namespace", In: "path", Required: true, Schema: map[string]interface{}{"type": "string"}},
			{Name: "name", In: "path", Required: true, Schema: map[string]interface{}{"type": "string"}},
			queryParam("container", "Container name"),
		},
		Responses: map[string]OpenAPIResponse{"200": {Description: "Pod logs (text/plain)"}},
	})
	add("/api/pod/delete", "post", OpenAPIOperation{
		Summary: "Delete a single pod", OperationID: "podDelete", Tags: []string{"Pods", "Write Ops"},
		RequestBody: bodyParam("Pod delete request", map[string]string{
			"namespace": "default", "name": "nginx-abc123",
		}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod deleted", map[string]bool{"success": true}),
		},
	})

	// --- Events ---
	add("/api/events", "get", OpenAPIOperation{
		Summary: "List Kubernetes events", OperationID: "listEvents", Tags: []string{"Events"},
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace"),
			queryParam("warning", "Show only warnings (true/false)"),
			queryParam("q", "Search query"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Event list", map[string]interface{}{"count": 5, "items": []interface{}{}}),
		},
	})
	add("/api/events/stream", "get", OpenAPIOperation{
		Summary: "Stream events via SSE", OperationID: "eventsStream", Tags: []string{"Events"},
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "Server-Sent Events stream (text/event-stream)"},
		},
	})

	// --- Resources ---
	add("/api/resources", "get", OpenAPIOperation{
		Summary: "Browse resources by kind", OperationID: "listResources", Tags: []string{"Resources"},
		Parameters: []OpenAPIParam{
			{Name: "kind", In: "query", Required: true, Description: "Resource kind (deployments, services, etc.)",
				Schema: map[string]interface{}{"type": "string"}},
			queryParam("namespace", "Filter by namespace"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Resource list", map[string]interface{}{}),
		},
	})
	add("/api/yaml", "get", OpenAPIOperation{
		Summary: "View resource YAML", OperationID: "viewYAML", Tags: []string{"Resources"},
		Parameters: []OpenAPIParam{
			queryParam("kind", "Resource kind"),
			queryParam("namespace", "Namespace"),
			queryParam("name", "Resource name"),
		},
		Responses: map[string]OpenAPIResponse{"200": {Description: "YAML content (text/plain)"}},
	})
	add("/api/yaml/apply", "post", OpenAPIOperation{
		Summary: "Apply YAML (kubectl apply)", OperationID: "applyYAML", Tags: []string{"Resources", "Write Ops"},
		RequestBody: bodyParam("YAML apply request", map[string]string{"yaml": "apiVersion: v1\nkind: ConfigMap\n..."}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("YAML applied", map[string]bool{"success": true}),
		},
	})
	add("/api/scale", "post", OpenAPIOperation{
		Summary: "Scale deployment or statefulset", OperationID: "scale", Tags: []string{"Resources", "Write Ops"},
		RequestBody: bodyParam("Scale request", map[string]interface{}{
			"namespace": "default", "kind": "deployment", "name": "nginx", "replicas": 3,
		}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scaled successfully", map[string]bool{"success": true}),
		},
	})
	add("/api/rollout/restart", "post", OpenAPIOperation{
		Summary: "Trigger rolling restart", OperationID: "rolloutRestart", Tags: []string{"Resources", "Write Ops"},
		RequestBody: bodyParam("Restart request", map[string]string{
			"namespace": "default", "kind": "deployment", "name": "nginx",
		}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollout restart triggered", map[string]bool{"success": true}),
		},
	})
	add("/api/resource/data", "get", OpenAPIOperation{
		Summary: "View ConfigMap/Secret data", OperationID: "resourceData", Tags: []string{"Resources"},
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Namespace"),
			queryParam("name", "Resource name"),
			queryParam("kind", "configmap or secret"),
		},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Resource data", map[string]interface{}{})},
	})

	// --- Cost / FinOps ---
	add("/api/cost/summary", "get", OpenAPIOperation{
		Summary: "Cost summary", OperationID: "costSummary", Tags: []string{"Cost"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cost breakdown by namespace", map[string]interface{}{}),
		},
	})
	add("/api/cost/recommendations", "get", OpenAPIOperation{
		Summary: "Cost optimization recommendations", OperationID: "costRecommendations", Tags: []string{"Cost"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cost saving recommendations", map[string]interface{}{}),
		},
	})

	// --- Security ---
	add("/api/security/audit", "get", OpenAPIOperation{
		Summary: "Security audit scan", OperationID: "securityAudit", Tags: []string{"Security"},
		Description: "Scans the cluster for Pod Security Standards violations, RBAC issues, network policy gaps, and other security concerns.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Security findings", map[string]interface{}{
				"summary":  map[string]int{"critical": 0, "high": 2, "total": 5},
				"findings": []interface{}{},
			}),
		},
	})
	add("/api/security/health", "get", OpenAPIOperation{
		Summary: "Platform security health", OperationID: "securityHealth", Tags: []string{"Security"},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Security posture", map[string]string{"status": "healthy"}),
		},
	})

	// --- Diagnostics & Remediations ---
	add("/api/diagnostics", "get", OpenAPIOperation{
		Summary: "List diagnostic reports", OperationID: "listDiagnostics", Tags: []string{"Diagnostics"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Diagnostic reports", map[string]interface{}{})},
	})
	add("/api/diagnostics/{name}", "get", OpenAPIOperation{
		Summary: "Get diagnostic report detail", OperationID: "diagnosticDetail", Tags: []string{"Diagnostics"},
		Parameters: []OpenAPIParam{
			{Name: "name", In: "path", Required: true, Schema: map[string]interface{}{"type": "string"}},
		},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Diagnostic detail", map[string]interface{}{})},
	})
	add("/api/remediations", "get", OpenAPIOperation{
		Summary: "List remediation plans", OperationID: "listRemediations", Tags: []string{"Remediations"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Remediation plans", map[string]interface{}{})},
	})
	add("/api/optimizations", "get", OpenAPIOperation{
		Summary: "List optimization recommendations", OperationID: "listOptimizations", Tags: []string{"Optimizations"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Optimizations", map[string]interface{}{})},
	})

	// --- Audit ---
	add("/api/audit", "get", OpenAPIOperation{
		Summary: "List audit log entries", OperationID: "listAudit", Tags: []string{"Audit"},
		Parameters: []OpenAPIParam{
			queryParam("severity", "Filter by severity"),
			queryParam("limit", "Max entries"),
		},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Audit entries", map[string]interface{}{})},
	})
	add("/api/audit/stats", "get", OpenAPIOperation{
		Summary: "Audit statistics", OperationID: "auditStats", Tags: []string{"Audit"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Audit stats", map[string]interface{}{})},
	})

	// --- Chat ---
	add("/api/chat", "post", OpenAPIOperation{
		Summary: "AI chat (streaming SSE)", OperationID: "chat", Tags: []string{"AI"},
		RequestBody: bodyParam("Chat request", map[string]string{"message": "What pods are crashing?"}),
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "SSE stream of AI response chunks"},
		},
	})
	add("/api/chat/conversations", "get", OpenAPIOperation{
		Summary: "List chat conversations", OperationID: "chatConversations", Tags: []string{"AI"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Conversation list", map[string]interface{}{})},
	})

	// --- Provider ---
	add("/api/provider/status", "get", OpenAPIOperation{
		Summary: "Get AI provider status", OperationID: "providerStatus", Tags: []string{"Settings"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Provider status", map[string]interface{}{})},
	})
	add("/api/provider/update", "post", OpenAPIOperation{
		Summary: "Update AI provider config", OperationID: "providerUpdate", Tags: []string{"Settings"},
		RequestBody: bodyParam("Provider update", map[string]string{"provider": "openai", "apiKey": "..."}),
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Updated", map[string]bool{"success": true})},
	})

	// --- CRDs ---
	add("/api/crds", "get", OpenAPIOperation{
		Summary: "List Custom Resource Definitions", OperationID: "listCRDs", Tags: []string{"CRDs"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("CRD list", map[string]interface{}{})},
	})
	add("/api/crd-resources", "get", OpenAPIOperation{
		Summary: "List CR instances", OperationID: "listCRDResources", Tags: []string{"CRDs"},
		Parameters: []OpenAPIParam{queryParam("crd", "CRD name")},
		Responses:  map[string]OpenAPIResponse{"200": okResponse("CR instances", map[string]interface{}{})},
	})

	// --- RBAC ---
	add("/api/rbac/clusterroles", "get", OpenAPIOperation{
		Summary: "List cluster roles", OperationID: "listClusterRoles", Tags: []string{"RBAC"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Cluster roles", map[string]interface{}{})},
	})
	add("/api/rbac/roles", "get", OpenAPIOperation{
		Summary: "List namespace roles", OperationID: "listRoles", Tags: []string{"RBAC"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Roles", map[string]interface{}{})},
	})
	add("/api/rbac/rolebindings", "get", OpenAPIOperation{
		Summary: "List role bindings", OperationID: "listRoleBindings", Tags: []string{"RBAC"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Role bindings", map[string]interface{}{})},
	})

	// --- Tools ---
	add("/api/tools", "get", OpenAPIOperation{
		Summary: "List available AI tools", OperationID: "listTools", Tags: []string{"AI"},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Tool list", map[string]interface{}{})},
	})

	// --- Metrics ---
	add("/metrics", "get", OpenAPIOperation{
		Summary: "Prometheus metrics", OperationID: "metrics", Tags: []string{"System"},
		Description: "Prometheus-format metrics. Restricted to localhost only.",
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "Prometheus metrics (text/plain)"},
			"403": {Description: "Forbidden (not localhost)"},
		},
	})

	// --- Namespace Ranking (v14.33+) ---
	add("/api/namespaces/ranking", "get", OpenAPIOperation{
		Summary: "Namespace resource ranking", OperationID: "namespaceRanking", Tags: []string{"Cost", "Capacity"},
		Description: "Per-namespace CPU/memory requests, limits, pod counts, and PVC storage, sorted by CPU consumption.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Namespace ranking with summary", map[string]interface{}{
				"count":   10,
				"summary": map[string]interface{}{"totalNamespaces": 10, "totalPods": 50},
				"items":   []interface{}{},
			}),
		},
	})
	add("/api/namespaces/{name}/detail", "get", OpenAPIOperation{
		Summary: "Namespace detail", OperationID: "namespaceDetail", Tags: []string{"Capacity"},
		Parameters: []OpenAPIParam{
			{Name: "name", In: "path", Required: true, Description: "Namespace name",
				Schema: map[string]interface{}{"type": "string"}},
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("ResourceQuota usage, LimitRanges, recent warnings", map[string]interface{}{}),
		},
	})

	// --- Storage & Capacity (v14.34+) ---
	add("/api/storage/capacity", "get", OpenAPIOperation{
		Summary: "Storage capacity (PVCs)", OperationID: "storageCapacity", Tags: []string{"Capacity", "Storage"},
		Description: "PVC overview with capacity, status, storage class, and requested size.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PVC capacity data", map[string]interface{}{
				"summary": map[string]interface{}{"totalPVCs": 5, "bound": 4, "totalCapacityGB": 100.0},
				"items":   []interface{}{},
			}),
		},
	})
	add("/api/capacity/planning", "get", OpenAPIOperation{
		Summary: "Capacity planning", OperationID: "capacityPlanning", Tags: []string{"Capacity"},
		Description: "Node capacity vs requested resources with per-node utilization percentages and expansion recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Capacity planning with recommendations", map[string]interface{}{
				"summary":         map[string]interface{}{"clusterCPUUtilPct": 45.2, "nodeCount": 3},
				"recommendations": []string{},
				"nodes":           []interface{}{},
			}),
		},
	})

	// --- HPA Visualization (v14.39+) ---
	add("/api/hpa", "get", OpenAPIOperation{
		Summary: "List HPAs with metrics", OperationID: "listHPA", Tags: []string{"HPA", "Autoscaling"},
		Description: "Detailed HPA data with scaling metrics (CPU/memory utilization, pods, external), replica status, and scaling state.",
		Parameters:  []OpenAPIParam{queryParam("namespace", "Filter by namespace")},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("HPA list with metrics", map[string]interface{}{
				"summary": map[string]interface{}{"totalHPAs": 3, "scalingActive": 1},
				"items":   []interface{}{},
			}),
		},
	})

	// --- Compliance (v14.35+) ---
	add("/api/security/compliance", "get", OpenAPIOperation{
		Summary: "CIS compliance scan", OperationID: "complianceScan", Tags: []string{"Security", "Compliance"},
		Description: "Runs CIS Kubernetes Benchmark checks (RBAC, Pod Security, Network, Secrets) and returns pass/warn/fail status per control.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Compliance scan results", map[string]interface{}{
				"score": 85, "summary": map[string]int{"pass": 8, "warn": 2, "fail": 0, "total": 10},
			}),
		},
	})
	add("/api/security/compliance/report", "get", OpenAPIOperation{
		Summary: "Download compliance report", OperationID: "complianceReport", Tags: []string{"Security", "Compliance"},
		Description: "Downloads a text compliance report with scores, per-check results, and remediation guidance.",
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "Text report (text/plain, attachment)"},
		},
	})

	// --- System & Operations (v14.38+) ---
	add("/api/system/info", "get", OpenAPIOperation{
		Summary: "System info", OperationID: "systemInfo", Tags: []string{"System", "Operations"},
		Description: "Version, Go runtime, memory stats, goroutine count, uptime, and audit log size.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("System info", map[string]interface{}{
				"version": "v14.41", "goVersion": "go1.26", "uptime": "5h30m",
			}),
		},
	})
	add("/api/system/performance", "get", OpenAPIOperation{
		Summary: "API performance stats", OperationID: "apiPerformance", Tags: []string{"System", "Performance"},
		Description: "Per-endpoint latency percentiles (p50, p95, p99), average, max, and error rate from in-memory tracking.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Performance stats", map[string]interface{}{
				"summary":   map[string]interface{}{"totalRequests": 1000, "errorRate": 0.5},
				"endpoints": []interface{}{},
			}),
		},
	})
	add("/api/system/log/rotate", "post", OpenAPIOperation{
		Summary: "Rotate audit log", OperationID: "logRotate", Tags: []string{"System", "Operations"},
		Description: "Manually triggers audit log rotation. Admin only.",
		RequestBody: bodyParam("Empty body", map[string]interface{}{}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rotation successful", map[string]bool{"success": true}),
		},
	})
	add("/api/system/log/cleanup", "post", OpenAPIOperation{
		Summary: "Cleanup old audit logs", OperationID: "logCleanup", Tags: []string{"System", "Operations"},
		Description: "Removes rotated audit log files older than 30 days. Admin only.",
		RequestBody: bodyParam("Empty body", map[string]interface{}{}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cleanup completed", map[string]interface{}{"removed": 3}),
		},
	})

	// Images
	add("/api/images", "get", OpenAPIOperation{
		Summary: "Container image inventory", OperationID: "getImages", Tags: []string{"Images"},
		Description: "Lists all container images in the cluster with usage, resource limit auditing, and :latest tag detection.",
		Parameters:  []OpenAPIParam{},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image list", map[string]interface{}{
				"count": 15, "items": []interface{}{}, "summary": map[string]interface{}{},
			}),
		},
	})

	// Events summary
	add("/api/events/summary", "get", OpenAPIOperation{
		Summary: "Warning event summary by reason", OperationID: "getEventSummary", Tags: []string{"Events"},
		Description: "Aggregates all cluster Warning events by reason, with severity classification (critical/warning) and affected namespace tracking.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Event summary", map[string]interface{}{
				"summary": map[string]interface{}{"totalReasons": 5, "totalWarnings": 42, "criticalCount": 2},
				"items":   []interface{}{},
			}),
		},
	})

	// Efficiency
	add("/api/efficiency", "get", OpenAPIOperation{
		Summary: "Cluster efficiency analysis", OperationID: "getEfficiency", Tags: []string{"Scalability"},
		Description: "Analyzes cluster for resource waste: pods without limits, over-provisioned containers (limit/request >10x), underutilized nodes (<20%). Returns efficiency score (0-100) and recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Efficiency report", map[string]interface{}{
				"score": 85.0, "recommendations": []interface{}{}, "stats": map[string]interface{}{},
			}),
		},
	})

	// Security: Secret exposure
	add("/api/security/secrets", "get", OpenAPIOperation{
		Summary: "Secret exposure scanner", OperationID: "getSecretExposure", Tags: []string{"Security", "Secrets"},
		Description: "Scans for hardcoded credentials in pod env vars, tracks secret rotation (90d), detects unused secrets, and identifies sensitive key names in Opaque secrets.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Secret exposure report", map[string]interface{}{
				"summary": map[string]interface{}{"totalSecrets": 8, "exposedEnvVars": 2, "unusedSecrets": 1},
			}),
		},
	})

	// Backup management
	add("/api/system/backup", "get", OpenAPIOperation{
		Summary: "List database backups", OperationID: "listBackups", Tags: []string{"System", "Backup"},
		Description: "Lists available database backup files with size, age, and type information.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Backup list", map[string]interface{}{
				"backups": []interface{}{}, "summary": map[string]interface{}{"count": 3},
			}),
		},
	})
	add("/api/system/backup", "post", OpenAPIOperation{
		Summary: "Create database backup", OperationID: "createBackup", Tags: []string{"System", "Backup"},
		Description: "Creates a timestamped database backup by copying the SQLite DB to /data/backups/.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Backup created", map[string]interface{}{"success": true}),
		},
	})

	// Alertmanager webhook
	add("/api/webhooks/alertmanager", "post", OpenAPIOperation{
		Summary: "Receive Prometheus Alertmanager alerts", OperationID: "receiveAlerts", Tags: []string{"Alerts", "Operations"},
		Description: "Receives Alertmanager v4 webhook payloads. Parses alerts, generates investigation hints based on alert type, and logs to audit trail. Configure in Alertmanager: webhook_configs url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager",
		RequestBody: bodyParam("Alertmanager webhook payload", map[string]interface{}{
			"status": "firing", "alerts": []interface{}{},
		}),
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Alerts received", map[string]interface{}{
				"received": true, "firing": 1, "resolved": 0,
			}),
		},
	})

	// Audit search and export
	add("/api/audit/events", "get", OpenAPIOperation{
		Summary: "Search audit events", OperationID: "searchAuditEvents", Tags: []string{"Audit", "Security"},
		Description: "Searches audit events with filters: severity, actor, action, full-text search (q), date range (from/to), pagination.",
		Parameters: []OpenAPIParam{
			{Name: "page", In: "query", Required: false, Schema: map[string]interface{}{"type": "integer"}, Description: "Page number (default: 1)"},
			{Name: "limit", In: "query", Required: false, Schema: map[string]interface{}{"type": "integer"}, Description: "Items per page (default: 50, max: 500)"},
			{Name: "severity", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Filter by severity: critical, warning, info"},
			{Name: "actor", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Filter by actor (username)"},
			{Name: "action", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Filter by action type (e.g. delete, scale, exec)"},
			{Name: "q", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Full-text search across all fields"},
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Audit events", map[string]interface{}{"items": []interface{}{}, "total": 100}),
		},
	})
	add("/api/audit/export", "get", OpenAPIOperation{
		Summary: "Export audit events as CSV", OperationID: "exportAuditEvents", Tags: []string{"Audit", "Security"},
		Description: "Exports filtered audit events as CSV for SIEM/compliance. Columns: ID, Timestamp, Severity, Actor, Action, Target, Success, Detail.",
		Parameters: []OpenAPIParam{
			{Name: "severity", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Filter by severity"},
			{Name: "from", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Start date (RFC3339)"},
			{Name: "to", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "End date (RFC3339)"},
		},
		Responses: map[string]OpenAPIResponse{
			"200": {Description: "CSV file"},
		},
	})

	// --- PDB Status (v14.55+) ---
	add("/api/pdbs", "get", OpenAPIOperation{
		Summary: "List Pod Disruption Budgets", OperationID: "listPDBs", Tags: []string{"Reliability"},
		Description: "Lists all PDBs with disruption status, matched workloads, and health assessment (healthy/at-risk/blocked). Useful for pre-drain safety checks.",
		Parameters:  []OpenAPIParam{queryParam("namespace", "Filter by namespace")},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PDB list", map[string]interface{}{
				"summary": map[string]int{"total": 5, "healthy": 4, "atRisk": 1, "blocked": 0},
				"items":   []interface{}{},
			}),
		},
	})

	// --- Compatibility Detection (v14.55+) ---
	add("/api/compatibility", "get", OpenAPIOperation{
		Summary: "K8s distribution & compatibility detection", OperationID: "compatibility", Tags: []string{"System", "Compatibility"},
		Description: "Detects K8s distribution (vanilla, k3s, RKE2, EKS, GKE, AKS, OpenShift, Talos), version compatibility with k8ops, and feature availability (ARM, Windows, GPU nodes).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Compatibility info", map[string]interface{}{
				"distribution": "k3s", "kubernetesVersion": "v1.33.1+k3s1",
				"compatible": true,
				"features":   map[string]bool{"arm": true, "gpu": false, "windows": false},
			}),
		},
	})

	// --- Certificate Expiry Scanner (v14.56+) ---
	add("/api/certificates/expiry", "get", OpenAPIOperation{
		Summary: "TLS certificate expiry scanner", OperationID: "certExpiry", Tags: []string{"Security", "Operations"},
		Description: "Scans all kubernetes.io/tls and Opaque secrets for X.509 certificates. Categorizes by expiry: expired (<0d), critical (<7d), warning (<30d), ok (>30d). Links certificates to Ingress resources.",
		Parameters:  []OpenAPIParam{queryParam("namespace", "Filter by namespace")},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Certificate expiry report", map[string]interface{}{
				"total": 57, "expired": 2, "critical": 0, "warning": 1, "ok": 54,
				"certificates": []interface{}{},
			}),
		},
	})

	// --- Server Drain Status (v14.57+) ---
	add("/api/system/drain-status", "get", OpenAPIOperation{
		Summary: "Server drain status", OperationID: "drainStatus", Tags: []string{"System", "Operations"},
		Description: "Reports whether the server is in graceful-shutdown draining mode. During drain, /readyz returns 503 to remove the pod from Service endpoints.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Drain status", map[string]interface{}{
				"draining": false, "shutdownInitiated": false, "activeConnections": 3, "uptimeSeconds": 3600,
			}),
		},
	})

	// --- Add-on Health Detection (v14.58+) ---
	add("/api/addons/health", "get", OpenAPIOperation{
		Summary: "K8s add-on health detection", OperationID: "addonHealth", Tags: []string{"System", "Add-ons"},
		Description: "Non-intrusive detection and health check of 39 common K8s add-ons across 12 categories: CNI, DNS, Ingress, Cert Manager, Load Balancer, Service Mesh, Backup, Monitoring, Policy, Storage, GitOps, Virtual Machine.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Add-on health report", map[string]interface{}{
				"summary":    map[string]int{"totalDetected": 6, "healthy": 3, "degraded": 3, "notInstalled": 33},
				"categories": map[string]interface{}{},
			}),
		},
	})

	// --- Capacity Forecast (v14.59+) ---
	add("/api/capacity/forecast", "get", OpenAPIOperation{
		Summary: "Cluster capacity exhaustion forecast", OperationID: "capacityForecast", Tags: []string{"Capacity", "Scalability"},
		Description: "Predicts when cluster resources (CPU, memory, pods, storage) will be exhausted. Estimates growth from pod creation timestamps. Risk levels: safe (<60%), moderate (60-80%), high (80-95%), critical (>95%). Provides days-to-exhaustion and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Capacity forecast", map[string]interface{}{
				"overallRisk": "safe", "nodeCount": 3, "podCount": 63,
				"forecasts": []interface{}{},
			}),
		},
	})

	// --- Deployment Rollout Status (v14.63+) ---
	add("/api/deployments/rollout", "get", OpenAPIOperation{
		Summary: "Deployment rollout status tracker", OperationID: "rolloutStatus", Tags: []string{"Deployments", "Operations"},
		Description: "Scans all Deployments, StatefulSets, and DaemonSets for rollout health. Detects in-progress rollouts, stalled updates, degraded availability, failed deployments (ProgressDeadlineExceeded), paused deployments, and scaled-to-zero workloads. Provides conditions, images, template hash, and issue diagnostics per workload.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
			queryParam("status", "Filter by rollout status: complete, in-progress, stalled, degraded, paused, failed, scaled-to-zero"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollout status report", map[string]interface{}{
				"summary": map[string]int{
					"total": 45, "deployments": 30, "statefulSets": 10, "daemonSets": 5,
					"complete": 40, "inProgress": 2, "degraded": 1, "failed": 1, "paused": 1,
				},
				"workloads": []interface{}{},
			}),
		},
	})

	// --- Resource Waste Detection (v14.64+) ---
	add("/api/resources/waste", "get", OpenAPIOperation{
		Summary: "Resource waste detector", OperationID: "resourceWaste", Tags: []string{"Resources", "Cost Optimization"},
		Description: "Scans for wasted and orphaned resources: dead services (no endpoints, especially LoadBalancer), unused PVCs, unattached PVs, orphaned ConfigMaps/Secrets, and empty namespaces. Each item includes severity rating, age, and actionable cleanup suggestions. Provides estimated cost risk level.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Resource waste report", map[string]interface{}{
				"summary": map[string]interface{}{
					"total":       15,
					"byCategory":  map[string]int{"dead-service": 3, "unused-pvc": 2, "orphaned-configmap": 5},
					"bySeverity":  map[string]int{"critical": 2, "high": 3, "medium": 5, "low": 5},
					"estCostRisk": "moderate",
				},
				"items": []interface{}{},
			}),
		},
	})

	// --- Scaling Bottleneck Detector (v14.65+) ---
	add("/api/scaling/bottlenecks", "get", OpenAPIOperation{
		Summary: "Scaling bottleneck detector", OperationID: "scalingBottlenecks", Tags: []string{"Scaling", "Capacity", "Scalability"},
		Description: "Scans for factors that prevent or limit horizontal scaling: node scheduling constraints (cordoned, pressure conditions), cluster pod capacity limits, resource quota pressure, HPA stuck at max replicas, PDBs blocking voluntary disruptions, and storage exhaustion. Provides cluster-level capacity summary and per-item recommendations.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scaling bottleneck report", map[string]interface{}{
				"clusterSummary": map[string]interface{}{
					"totalNodes": 3, "schedulableNodes": 3, "podCapacity": 330,
					"podsAllocated": 60, "podCapacityUsedPct": 18.2,
					"scalingHeadroomPods": 270,
				},
				"summary": map[string]interface{}{
					"total": 2, "blocking": 1,
					"byType": map[string]int{"hpa-stuck": 1, "pdb-blocking": 1},
				},
				"bottlenecks": []interface{}{},
			}),
		},
	})

	// --- RBAC Permission Risk Analyzer (v14.67+) ---
	add("/api/security/rbac-risk", "get", OpenAPIOperation{
		Summary: "RBAC permission risk analyzer", OperationID: "rbacRiskScan", Tags: []string{"Security"},
		Description: "Comprehensive RBAC analysis: maps all subjects (users/groups/service accounts) to effective permissions, identifies over-privileged accounts, detects privilege escalation paths (can modify RBAC bindings), wildcard access, sensitive resource access (secrets, exec), and unused bindings to non-existent SAs. Risk scoring 0-100 per subject.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("RBAC risk report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalSubjects": 25, "clusterScoped": 8,
					"privilegeEscalation": 2, "wildcardAccess": 3,
					"byRiskLevel": map[string]int{"critical": 1, "high": 3, "medium": 8, "low": 13},
				},
				"subjects": []interface{}{},
			}),
		},
	})

	// --- CronJob Execution Health Monitor (v14.68+) ---
	add("/api/operations/cronjobs/health", "get", OpenAPIOperation{
		Summary: "CronJob execution health monitor", OperationID: "cronJobHealth", Tags: []string{"Operations", "Batch"},
		Description: "Monitors all CronJobs for execution health: tracks job success/failure rates, detects consecutive failures, suspended crons, stale schedules, and never-executed crons. Links each CronJob to its child Jobs via owner references. Provides 5 health levels: healthy/warning/failing/suspended/no-runs.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CronJob health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalCronJobs": 5, "suspended": 1, "failedJobs": 3,
					"byStatus": map[string]int{"healthy": 3, "failing": 1, "suspended": 1},
				},
				"cronJobs": []interface{}{},
			}),
		},
	})

	// --- Service & Endpoint Health Monitor (v14.69+) ---
	add("/api/networking/health", "get", OpenAPIOperation{
		Summary: "Service & Endpoint health monitor", OperationID: "networkingHealth", Tags: []string{"Networking", "Product"},
		Description: "Scans all Services and Ingresses for networking health. Detects: services with no endpoints (dangling), selector mismatches, all endpoints not ready, degraded services (partial endpoint loss), LoadBalancer pending IP, and ingress backends pointing to missing or endpoint-less services. Provides 5 health levels: healthy/degraded/no-endpoints/misconfigured/external.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Networking health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalServices":      25,
					"byStatus":           map[string]int{"healthy": 20, "no-endpoints": 3, "misconfigured": 1, "external": 1},
					"totalIngresses":     5,
					"unhealthyIngress":   1,
					"noEndpointServices": 3,
				},
				"services":  []interface{}{},
				"ingresses": []interface{}{},
			}),
		},
	})

	// --- PV/PVC Storage Health Monitor (v14.70+) ---
	add("/api/storage/health", "get", OpenAPIOperation{
		Summary: "PV/PVC storage health monitor", OperationID: "storageHealth", Tags: []string{"Storage", "Scalability"},
		Description: "Scans all PersistentVolumeClaims and PersistentVolumes for storage health issues. Detects: PVCs stuck in Pending (provisioning failures, missing storage class, WaitForFirstConsumer), orphaned PVCs (bound but not mounted by any pod), lost PVCs, released/failed PVs needing manual cleanup, stale Available PVs wasting storage capacity. Provides storage class distribution with default class detection, reclaim policy analysis, and volume expansion capability reporting.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Storage health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPVCs":    25,
					"pvcByStatus":  map[string]int{"bound": 20, "pending": 2, "orphaned": 3},
					"pendingPVCs":  2,
					"orphanedPVCs": 3,
					"totalPVs":     28,
					"releasedPVs":  1,
				},
				"pvcs":           []interface{}{},
				"orphanedPVs":    []interface{}{},
				"storageClasses": []interface{}{},
			}),
		},
	})

	return spec
}

// handleAPIDocs serves a lightweight HTML page listing all API endpoints.
// GET /api/docs
func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	spec := buildOpenAPISpec()

	// Collect all operations grouped by tag
	tagGroups := map[string][]map[string]interface{}{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			for _, tag := range op.Tags {
				tagGroups[tag] = append(tagGroups[tag], map[string]interface{}{
					"method":      method,
					"path":        path,
					"summary":     op.Summary,
					"operationId": op.OperationID,
					"description": op.Description,
				})
			}
		}
	}

	// Sort tags
	tags := make([]string, 0, len(tagGroups))
	for t := range tagGroups {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]interface{}{
		"spec":          spec,
		"tagGroups":     tagGroups,
		"tagOrder":      tags,
		"endpointCount": len(spec.Paths),
	})
}
