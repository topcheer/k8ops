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

	// --- ServiceAccount Security Audit (v14.72+) ---
	add("/api/security/service-accounts", "get", OpenAPIOperation{
		Summary: "ServiceAccount security audit", OperationID: "serviceAccountAudit", Tags: []string{"Security"},
		Description: "Comprehensive ServiceAccount security audit. Detects: unused SAs (>7 days, attack surface reduction), default SA used by pods (least-privilege violation), unnecessary token automounting, SAs bound to cluster-admin (critical), SAs with cluster-wide permissions, stale SAs with active permissions but no pod usage (>30 days), legacy long-lived token secrets (K8s <1.24). Provides risk score (0-100) and 5 severity levels per SA.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("ServiceAccount audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalServiceAccounts":    25,
					"unusedServiceAccounts":   5,
					"defaultSAUsedByPods":     3,
					"tokenAutoMountEnabled":   20,
					"highRiskServiceAccounts": 2,
					"bySeverity":              map[string]int{"critical": 1, "high": 1, "medium": 3, "low": 5, "info": 15},
				},
				"serviceAccounts": []interface{}{},
				"issues":          []interface{}{},
			}),
		},
	})

	// --- SLO/SLA Error Budget Tracker (v14.73+) ---
	add("/api/operations/slo", "get", OpenAPIOperation{
		Summary: "SLO/SLA error budget tracker", OperationID: "sloReport", Tags: []string{"Operations", "SRE"},
		Description: "Computes SLO/SLA compliance from API metrics. Tracks availability against configurable targets (99.9%/99.5%/99.0%/95.0%), error budget consumption, multi-window analysis (5m/1h/6h/24h), burn rate (SRE 14.4x alert threshold), per-endpoint error rates, and latency SLO (p99 < 500ms). Provides overall verdict: healthy/warning/at-risk/violated.",
		Parameters: []OpenAPIParam{
			queryParam("target", "SLO target: 99.9, 99.5, 99.0, or 95.0 (default: 99.9)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("SLO compliance report", map[string]interface{}{
				"target":        "99.9%",
				"availability":  99.95,
				"totalRequests": 10000,
				"errorRequests": 5,
				"errorRate":     0.05,
				"verdict":       "warning",
				"windows":       []interface{}{},
				"byEndpoint":    []interface{}{},
				"latencySLO":    map[string]interface{}{"target": "p99 < 500ms", "p99Ms": 45.3},
				"burnRate":      map[string]interface{}{"budgetMinutesPerMonth": 43.2, "consumedPercent": 50.0},
			}),
		},
	})

	// --- Resource Quota & Limit Range Monitor (v14.74+) ---
	add("/api/resources/quota", "get", OpenAPIOperation{
		Summary: "ResourceQuota & LimitRange monitor", OperationID: "quotaMonitor", Tags: []string{"Product", "Resources"},
		Description: "Scans all namespaces for ResourceQuota utilization and LimitRange defaults. Tracks CPU/memory/pod/configmap/secret/storage quotas per namespace with 4 usage levels: ok (<70%), warning (70-85%), critical (85-100%), exceeded (>100%). Identifies namespaces without quota protection. Provides top offenders ranking and LimitRange default/min/max constraint analysis.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Quota utilization report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces":   25,
					"withQuota":         15,
					"withoutQuota":      10,
					"exceededResources": 3,
					"criticalResources": 5,
					"byStatus":          map[string]int{"ok": 40, "warning": 5, "critical": 5, "exceeded": 3},
				},
				"namespaces":   []interface{}{},
				"topOffenders": []interface{}{},
			}),
		},
	})

	// --- Deployment Configuration Audit (v14.75+) ---
	add("/api/deployments/audit", "get", OpenAPIOperation{
		Summary: "Deployment configuration audit", OperationID: "deploymentAudit", Tags: []string{"Deployment", "Security"},
		Description: "Audits all Deployments, StatefulSets, and DaemonSets for configuration best-practice violations affecting reliability and safety. Checks: revision history limits (rollback capability), image pull policy correctness (:latest vs pinned tags), missing resource limits/requests, missing liveness/readiness/startup probes, security context (privileged, runAsNonRoot, readOnlyRootFilesystem, privilege escalation), update strategy (Recreate downtime, OnDelete manual updates, partitioned rollouts), lifecycle (termination grace period, preStop hooks), and pod-level security context (seccomp profile). Each finding includes severity (critical/warning/info), category, message, and actionable suggestion. Provides health score per workload (0=perfect, 100=worst) and aggregated top findings across the cluster.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
			queryParam("severity", "Filter by severity: critical, warning, info"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Deployment audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"total":         30,
					"deployments":   20,
					"statefulSets":  7,
					"daemonSets":    3,
					"withFindings":  18,
					"criticalCount": 5,
					"warningCount":  22,
					"infoCount":     15,
					"avgScore":      35,
				},
				"workloads":   []interface{}{},
				"topFindings": []interface{}{},
			}),
		},
	})

	// --- Scheduling Health & Resource Fragmentation (v14.76+) ---
	add("/api/scheduling/health", "get", OpenAPIOperation{
		Summary: "Scheduling health & resource fragmentation analyzer", OperationID: "schedulingHealth", Tags: []string{"Scheduling", "Scalability"},
		Description: "Analyzes cluster scheduling health, node schedulability, resource fragmentation, and pending pod diagnostics. Detects: cordoned/tainted nodes reducing effective capacity, nodes under pressure (memory/disk/PID/network), pods stuck in Pending with parsed FailedScheduling failure reasons (insufficient CPU/memory, taint mismatch, node selector conflict, volume binding failure), recent evictions (24h), oversized pods requesting more than any node can provide, and resource fragmentation patterns. Computes largest schedulable pod size, effective vs theoretical capacity, and a scheduling health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scheduling health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":         5,
					"schedulableNodes":   4,
					"unschedulableNodes": 1,
					"pendingPods":        2,
					"failedScheduling":   2,
					"healthScore":        85,
				},
				"nodes":              []interface{}{},
				"pendingPods":        []interface{}{},
				"largestFittablePod": map[string]interface{}{"maxCpuM": 2000, "maxMemoryGB": 8},
				"effectiveCapacity":  map[string]interface{}{"cpuLostPct": 20, "memLostPct": 20},
				"recommendations":    []string{},
			}),
		},
	})

	// --- Pod Security Posture Scan (v14.79+) ---
	add("/api/security/pods", "get", OpenAPIOperation{
		Summary: "Pod security posture scan", OperationID: "podSecurityScan", Tags: []string{"Security", "Compliance"},
		Description: "Audits all running pods for real-time security posture: privileged containers, hostNetwork/hostPID/hostIPC, hostPath mounts, dangerous Linux capabilities (SYS_ADMIN, NET_ADMIN, etc.), running as root (UID 0), privilege escalation, writable root filesystem, missing security context, :latest/no-tag images, images not pinned by digest, secrets injected as env vars, no resource limits, host port bindings. Provides per-pod risk score (0-100), aggregated findings by type and namespace.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
			queryParam("severity", "Filter by severity: critical, warning, info"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod security scan report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":      50,
					"podsWithIssues": 12,
					"criticalCount":  3,
					"warningCount":   8,
					"avgRiskScore":   20,
				},
				"pods":        []interface{}{},
				"topFindings": []interface{}{},
				"byNamespace": []interface{}{},
			}),
		},
	})

	// --- Event Storm Detector (v14.80+) ---
	add("/api/operations/event-storm", "get", OpenAPIOperation{
		Summary: "Event storm & cascade failure detector", OperationID: "eventStorm", Tags: []string{"Operations", "Events", "Alerting"},
		Description: "Analyzes Kubernetes Warning events to detect event storms, cascading failures, and resource flapping. Counts warning events in time windows (15min/1h/24h), classifies storm severity (critical/high/medium/low), identifies flapping resources (same resource+reason repeated 3+ times), aggregates events by namespace and reason, and provides actionable recommendations for investigation.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter events by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Event storm analysis report", map[string]interface{}{
				"stormDetected": true,
				"summary": map[string]interface{}{
					"events15Min":       30,
					"events1Hour":       150,
					"stormSeverity":     "high",
					"topNamespace":      "kube-system",
					"affectedResources": 12,
				},
				"namespaces":        []interface{}{},
				"topReasons":        []interface{}{},
				"flappingResources": []interface{}{},
				"recentEvents":      []interface{}{},
				"recommendations":   []string{},
			}),
		},
	})

	// --- Resource Dependency Graph & Blast Radius (v14.81+) ---
	add("/api/dependencies", "get", OpenAPIOperation{
		Summary: "Resource dependency graph & blast radius analyzer", OperationID: "dependencyGraph", Tags: []string{"Product", "Dependencies", "Topology"},
		Description: "Traces the full dependency graph for any workload (Deployment, StatefulSet, DaemonSet, Pod). Forward dependencies: ConfigMaps, Secrets, PVCs, ServiceAccounts referenced by the workload. Reverse dependencies: Services selecting the pods, Ingresses routing traffic, NetworkPolicies applying rules, HPAs scaling the workload, and other pods sharing the same ConfigMaps/Secrets. Provides blast radius assessment with risk level for safe change planning.",
		Parameters: []OpenAPIParam{
			{Name: "kind", In: "query", Required: true, Description: "Resource kind: Deployment, StatefulSet, DaemonSet, or Pod"},
			{Name: "name", In: "query", Required: true, Description: "Resource name"},
			queryParam("namespace", "Namespace (default: default)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Dependency graph", map[string]interface{}{
				"root": map[string]interface{}{"kind": "Deployment", "name": "my-app", "namespace": "default"},
				"dependencies": []interface{}{
					map[string]interface{}{"kind": "ConfigMap", "name": "app-config", "relation": "depends-on"},
					map[string]interface{}{"kind": "Secret", "name": "db-pass", "relation": "depends-on"},
				},
				"dependents": []interface{}{
					map[string]interface{}{"kind": "Service", "name": "my-app-svc", "relation": "selects"},
					map[string]interface{}{"kind": "Ingress", "name": "my-app-ing", "relation": "routes-to"},
				},
				"summary": map[string]interface{}{
					"blastRadius": 8,
					"riskLevel":   "medium",
				},
			}),
		},
	})

	// --- Topology Spread Compliance (v14.82+) ---
	add("/api/topology/spread", "get", OpenAPIOperation{
		Summary: "Topology spread constraint compliance checker", OperationID: "topologySpread", Tags: []string{"Scalability", "Topology", "HA"},
		Description: "Analyzes pod distribution across topology domains (zones, regions, nodes) to verify topology spread constraint compliance. For each workload: checks if topologySpreadConstraints are configured, computes actual pod distribution per domain, calculates actual skew (max - min pod count), compares against declared maxSkew, and classifies as balanced/skewed/no-constraint/single-replica. Also checks for nodes missing topology labels.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
			queryParam("domain", "Topology domain key (default: kubernetes.io/hostname, try: topology.kubernetes.io/zone)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Topology spread report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalDomains":      3,
					"domainKey":         "topology.kubernetes.io/zone",
					"balancedWorkloads": 10,
					"skewedWorkloads":   2,
					"maxSkew":           3,
				},
				"workloads": []interface{}{
					map[string]interface{}{
						"name":         "my-app",
						"status":       "skewed",
						"actualSkew":   3,
						"maxSkew":      1,
						"distribution": []interface{}{},
					},
				},
				"nodes": []interface{}{},
			}),
		},
	})

	// --- Secret Rotation & Lifecycle Audit (v14.85+) ---
	add("/api/security/secrets/rotation", "get", OpenAPIOperation{
		Summary: "Secret rotation & lifecycle audit", OperationID: "secretRotationAudit", Tags: []string{"Security", "Secrets", "Compliance"},
		Description: "Audits all Kubernetes secrets for rotation compliance and lifecycle management. Checks: secret age (stale >90d, very stale >180d), unused secrets (not referenced by any pod), TLS certificate secrets with expiry dates (expired or expiring <30d), Docker registry secrets, legacy service-account-token secrets, sensitive name detection (password/key/token/credential). Provides per-secret risk level, cluster-wide rotation score (0-100), and per-namespace/type breakdown.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter secrets by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Secret rotation audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalSecrets":  50,
					"staleSecrets":  10,
					"unusedSecrets": 5,
					"expiredTLS":    1,
					"rotationScore": 72,
				},
				"secrets":         []interface{}{},
				"byNamespace":     []interface{}{},
				"byType":          []interface{}{},
				"recommendations": []string{},
			}),
		},
	})

	// --- Health Probe Effectiveness Audit (v14.86+) ---
	add("/api/operations/probes", "get", OpenAPIOperation{
		Summary: "Health probe effectiveness analyzer", OperationID: "probeAudit", Tags: []string{"Operations", "Health", "Reliability"},
		Description: "Audits liveness, readiness, and startup probe configurations across all workloads (Deployment, StatefulSet, DaemonSet). Detects: missing probes, aggressive probes (period <5s), short timeouts (<2s), low failure thresholds (<3), slow readiness checks (>60s), high liveness failure thresholds (>10), identical liveness+readiness probes, slow-starting apps without startup probes. Provides per-workload risk score and cluster-wide effectiveness score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter workloads by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Probe effectiveness report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers":  20,
					"missingReadiness": 5,
					"missingLiveness":  3,
					"score":            72,
				},
				"workloads":   []interface{}{},
				"topFindings": []interface{}{},
			}),
		},
	})

	// --- Workload Staleness & Release Cadence (v14.87+) ---
	add("/api/product/staleness", "get", OpenAPIOperation{
		Summary: "Workload staleness & release cadence tracker", OperationID: "stalenessCheck", Tags: []string{"Product", "Workloads", "Lifecycle"},
		Description: "Tracks deployment staleness across all workloads (Deployment, StatefulSet, DaemonSet). Detects workloads not updated in 30/90/180+ days, identifies :latest tag usage, unpinned images (no digest), and provides per-workload risk levels. Includes age distribution buckets, per-namespace breakdown, and cluster-wide freshness score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter workloads by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Staleness report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads": 30,
					"staleWorkloads": 8,
					"usingLatest":    3,
					"freshnessScore": 65,
				},
				"workloads":       []interface{}{},
				"byNamespace":     []interface{}{},
				"imageAgeBuckets": []interface{}{},
			}),
		},
	})

	// --- Resource Over-commit & Pressure Analyzer (v14.88+) ---
	add("/api/scalability/overcommit", "get", OpenAPIOperation{
		Summary: "Resource over-commit & pressure analyzer", OperationID: "overcommitAnalysis", Tags: []string{"Scalability", "Resources", "Capacity"},
		Description: "Analyzes CPU and memory over-commit ratios across all nodes. For each node: calculates request commit (sum of requests vs allocatable), limit commit (sum of limits vs allocatable), pressure score (0-100), and risk level (safe/moderate/high/critical). Detects pods without limits or requests that could starve neighbors. Tracks cluster-wide over-commit ratios and provides per-namespace resource consumption breakdown.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Over-commit analysis report", map[string]interface{}{
				"summary": map[string]interface{}{
					"nodesAtRisk":         2,
					"totalCPULimitCommit": 2.5,
					"totalMemLimitCommit": 3.1,
					"clusterScore":        65,
				},
				"nodes":       []interface{}{},
				"noLimitPods": []interface{}{},
				"byNamespace": []interface{}{},
			}),
		},
	})

	// --- Image Security & Supply Chain (v14.92+) ---
	add("/api/security/images", "get", OpenAPIOperation{
		Summary: "Image security & supply chain analyzer", OperationID: "imageSecurityAudit", Tags: []string{"Security", "Images", "Supply Chain"},
		Description: "Scans all running container images for supply chain security risks. Checks: digest pinning (@sha256), :latest tag usage, no-tag images, old version tags, public vs private registries, unknown registries. Provides per-image risk level (critical/high/medium/low), per-registry statistics, top risk images, and cluster-wide image security score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image security report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalImages":   30,
					"usingLatest":   5,
					"notPinned":     20,
					"securityScore": 62,
				},
				"images":     []interface{}{},
				"byRegistry": []interface{}{},
				"topRisks":   []interface{}{},
			}),
		},
	})

	// --- Cluster Health Score Aggregator (v14.93+) ---
	add("/api/operations/health-score", "get", OpenAPIOperation{
		Summary: "Cluster health score aggregator", OperationID: "healthScore", Tags: []string{"Operations", "Health", "Monitoring"},
		Description: "Aggregates all cluster health signals into one comprehensive score (0-100, grade A-F). Combines 5 weighted categories: Node Health (25%), Pod Health (25%), Workload Health (20%), Event Activity (15%), API Server Latency (15%). Provides per-category scores, status (healthy/warning/critical), cluster-wide summary (node/pod/workload counts), and top actionable issues.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cluster health score", map[string]interface{}{
				"overallScore": 78,
				"grade":        "C",
				"status":       "healthy",
				"categories":   []interface{}{},
				"topIssues":    []interface{}{},
				"summary":      map[string]interface{}{},
			}),
		},
	})

	// --- Autoscaling Right-Sizing Recommendations (v14.94+) ---
	add("/api/scalability/autoscale-recommendations", "get", OpenAPIOperation{
		Summary: "HPA/VPA right-sizing recommendations", OperationID: "autoscaleRecommendations", Tags: []string{"Scalability", "Autoscaling", "Cost"},
		Description: "Analyzes HPA coverage and resource right-sizing across all workloads. Detects: multi-replica workloads without HPA, over-provisioned resource requests (>1 core or >2GB per container), under-provisioned workloads, HPAs pegged at max/min replicas, idle HPAs. Provides per-workload recommended CPU/memory values, potential CPU core and memory savings, HPA efficiency analysis, and cluster-wide autoscaling score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter workloads by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Autoscaling recommendations", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":           30,
					"withHPA":                  5,
					"overProvisioned":          8,
					"potentialCPUSavingsCores": 3.5,
					"autoscaleScore":           62,
				},
				"recommendations":   []interface{}{},
				"unscaledWorkloads": []interface{}{},
				"hpaEfficiency":     []interface{}{},
				"topSavings":        []interface{}{},
			}),
		},
	})

	// --- Ingress & Traffic Routing Health (v14.96+) ---
	add("/api/product/ingress-health", "get", OpenAPIOperation{
		Summary: "Ingress & traffic routing health monitor", OperationID: "ingressHealth", Tags: []string{"Product", "Networking", "Health"},
		Description: "Analyzes all Ingress resources for traffic routing health. Checks: backend service existence and endpoint readiness, TLS configuration, IngressClass validity, host+path conflicts across ingresses, missing rules. Provides per-ingress status (healthy/warning/critical), per-namespace breakdown, cluster-wide health score (0-100), and actionable recommendations.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter ingresses by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Ingress health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalIngresses":   15,
					"healthyIngresses": 10,
					"noBackend":        2,
					"hostConflicts":    1,
					"healthScore":      72,
				},
				"ingresses":   []interface{}{},
				"issues":      []interface{}{},
				"byNamespace": []interface{}{},
			}),
		},
	})

	// --- Container Security Context Audit (v14.98+) ---
	add("/api/security/containers", "get", OpenAPIOperation{
		Summary: "Container security context audit", OperationID: "containerSecurityAudit", Tags: []string{"Security", "Containers", "Pod Security"},
		Description: "Scans all running pods for container security context risks. Checks: privileged containers, allowPrivilegeEscalation, runAsUser=0 (root), runAsNonRoot=false, readOnlyRootFilesystem=false, hostNetwork/hostPID/hostIPC, hostPath mounts (with sensitive path detection), dangerous Linux capabilities (SYS_ADMIN, NET_ADMIN, etc), missing securityContext. Provides per-pod risk level (critical/high/medium/low), per-namespace breakdown, top risks, cluster security score (0-100), and actionable recommendations.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Container security report", map[string]interface{}{
				"summary": map[string]interface{}{
					"privileged":     2,
					"runAsRoot":      15,
					"hasHostNetwork": 1,
					"securityScore":  68,
				},
				"pods":        []interface{}{},
				"topRisks":    []interface{}{},
				"byNamespace": []interface{}{},
			}),
		},
	})

	// --- Node Condition & Resource Pressure (v14.99+) ---
	add("/api/operations/node-pressure", "get", OpenAPIOperation{
		Summary: "Node condition & resource pressure analyzer", OperationID: "nodePressure", Tags: []string{"Operations", "Nodes", "Health"},
		Description: "Analyzes all node conditions (DiskPressure, MemoryPressure, PIDPressure, NetworkUnavailable) and resource saturation (CPU/memory/pod density vs allocatable). Provides per-node risk level (critical/high/medium/low), usage percentages, condition details with duration, cluster-wide pressure score (0-100), and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node pressure report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":        3,
					"nodesWithPressure": 1,
					"diskPressure":      0,
					"memoryPressure":    1,
					"pressureScore":     78,
				},
				"nodes":    []interface{}{},
				"topRisks": []interface{}{},
			}),
		},
	})

	// --- PVC Binding & Storage Performance (v15.00+) ---
	add("/api/scalability/pvc-analysis", "get", OpenAPIOperation{
		Summary: "PVC binding & storage performance analyzer", OperationID: "pvcAnalysis", Tags: []string{"Scalability", "Storage", "Performance"},
		Description: "Analyzes all PersistentVolumeClaims for binding health and storage performance. Checks: PVC phases (Bound/Pending/Lost), stuck PVCs (>5min pending), bind time measurement, slow binding detection (>30s), storage class distribution, missing default StorageClass, storage provisioner analysis. Provides per-PVC status, per-storage-class statistics with avg bind time, stuck PVC diagnostics with root cause, cluster storage health score (0-100), and actionable recommendations.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter PVCs by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PVC analysis report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPVCs":     20,
					"boundPVCs":     18,
					"stuckPVCs":     1,
					"avgBindTimeMs": 3200,
					"healthScore":   85,
				},
				"pvcs":           []interface{}{},
				"byStorageClass": []interface{}{},
				"stuckPVCs":      []interface{}{},
				"issues":         []interface{}{},
			}),
		},
	})

	// --- Namespace Lifecycle & Governance (v15.02+) ---
	add("/api/product/namespaces/lifecycle", "get", OpenAPIOperation{
		Summary: "Namespace governance & lifecycle audit", OperationID: "namespaceLifecycle", Tags: []string{"Product", "Namespaces", "Governance"},
		Description: "Audits all namespaces for governance compliance. Checks: ResourceQuota presence, LimitRange presence, NetworkPolicy coverage, dedicated ServiceAccount (beyond default), required labels (app, team, env, owner), stale namespaces (no running pods), system namespace detection. Provides per-namespace risk level (critical/high/medium/low), compliance flags, cluster-wide governance score (0-100), and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Namespace governance report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces":  25,
					"activeNamespaces": 18,
					"withoutQuota":     5,
					"withoutNetPolicy": 8,
					"governanceScore":  62,
				},
				"namespaces": []interface{}{},
				"issues":     []interface{}{},
			}),
		},
	})

	// --- RBAC Effective Permissions & Escalation (v15.04+) ---
	add("/api/security/rbac-effective", "get", OpenAPIOperation{
		Summary: "RBAC effective permissions & escalation analyzer", OperationID: "rbacEffective", Tags: []string{"Security", "RBAC", "Access Control"},
		Description: "Analyzes effective RBAC permissions across all subjects (Users, Groups, ServiceAccounts). Aggregates ClusterRoleBindings and RoleBindings to compute each subject's actual permissions. Detects: cluster-admin equivalent access, privilege escalation paths (can create/modify RBAC), wildcard (*) permissions, secret readers, pod exec access, node access. Provides per-subject risk level (critical/high/medium/low), escalation risk paths, cluster-wide RBAC security score (0-100), and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("RBAC effective permissions report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalSubjects":   15,
					"clusterAdmins":   2,
					"escalationPaths": 1,
					"securityScore":   68,
				},
				"subjects":        []interface{}{},
				"privilegedUsers": []interface{}{},
				"escalationRisks": []interface{}{},
				"issues":          []interface{}{},
			}),
		},
	})

	// --- Container OOM Kill Tracker (v15.05+) ---
	add("/api/operations/oom-tracker", "get", OpenAPIOperation{
		Summary: "Container OOM kill tracker & memory analysis", OperationID: "oomTracker", Tags: []string{"Operations", "Containers", "Memory"},
		Description: "Tracks container OOMKilled events and analyzes memory configuration across all running pods. Detects: containers with OOMKilled termination reason, high restart counts (>=5), missing memory limits, low memory limits (<256MB), memory limits 10x+ higher than requests. Provides per-pod OOM risk level, top OOM offenders ranked by restart count, per-namespace OOM statistics, cluster-wide OOM risk score (0-100), and actionable recommendations including top offender identification.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("OOM tracker report", map[string]interface{}{
				"summary": map[string]interface{}{
					"oomKilledCount":   3,
					"podsWithOOM":      2,
					"highRestartCount": 1,
					"noMemLimit":       8,
					"oomRiskScore":     72,
				},
				"affectedPods": []interface{}{},
				"topKillers":   []interface{}{},
				"byNamespace":  []interface{}{},
			}),
		},
	})

	// --- Storage Capacity Exhaustion Predictor (v15.06+) ---
	add("/api/scalability/storage-forecast", "get", OpenAPIOperation{
		Summary: "Storage capacity exhaustion predictor", OperationID: "storageForecast", Tags: []string{"Scalability", "Storage", "Forecasting"},
		Description: "Predicts when storage capacity will be exhausted based on PV usage trends and growth rate estimation. Analyzes all bound PVs for: capacity vs used space, estimated daily growth rate, days to exhaustion, Longhorn actual-size annotation support, risk level per PV. Provides per-PV forecast with predicted exhaustion date, per-storage-class statistics, at-risk namespace ranking, cluster-wide days-to-full estimate, and actionable recommendations including top critical PV identification.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Storage forecast report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPVs":        25,
					"totalCapacityGB": 500,
					"usedCapacityGB":  320,
					"pvsFull":         1,
					"pvsNearFull":     3,
					"forecastDays":    45,
					"healthScore":     72,
				},
				"pvForecasts":      []interface{}{},
				"byStorageClass":   []interface{}{},
				"atRiskNamespaces": []interface{}{},
			}),
		},
	})

	// --- DNS Resolution Health Checker (v15.08+) ---
	add("/api/product/dns-health", "get", OpenAPIOperation{
		Summary: "DNS resolution health checker", OperationID: "dnsHealth", Tags: []string{"Product", "DNS", "Networking"},
		Description: "Analyzes cluster DNS resolution health. Checks: CoreDNS pod health (running/ready/restarts/version), CoreDNS ConfigMap Corefile (forwarders, plugins), headless service endpoint resolution (NXDOMAIN risk), NodeLocal DNS cache presence, pod custom dnsConfig ndots overrides, external-dns managed services. Provides per-pod CoreDNS status, headless service endpoint coverage, DNS configuration analysis, cluster DNS health score (0-100), and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("DNS health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"corednsPods":         2,
					"corednsReady":        2,
					"headlessNoEndpoints": 1,
					"healthScore":         88,
				},
				"coreDNS":          map[string]interface{}{},
				"dnsConfig":        map[string]interface{}{},
				"headlessServices": []interface{}{},
				"issues":           []interface{}{},
			}),
		},
	})

	// --- Admission Webhook Configuration Audit (v15.10+) ---
	add("/api/security/admission-audit", "get", OpenAPIOperation{
		Summary: "Admission webhook configuration auditor", OperationID: "admissionAudit", Tags: []string{"Security", "Admission Control", "Webhooks"},
		Description: "Audits all ValidatingWebhookConfigurations and MutatingWebhookConfigurations for security and reliability issues. Detects: missing CA bundles (TLS verification failure), failurePolicy=Ignore (silent failures), no namespaceSelector (catches all namespaces including system), broad scope (wildcard * resource matching), short timeouts (<3s), all operations matched without filtering. Provides per-webhook risk level (critical/high/medium/low), detailed rules analysis, cluster-wide admission security score (0-100), and actionable recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Admission webhook audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalValidating":     5,
					"totalMutating":       3,
					"healthyHooks":        6,
					"withIssues":          2,
					"noCABundle":          1,
					"failurePolicyIgnore": 2,
					"securityScore":       72,
				},
				"validatingWebhooks": []interface{}{},
				"mutatingWebhooks":   []interface{}{},
				"issues":             []interface{}{},
			}),
		},
	})

	// --- CrashLoopBackOff Detector (v15.11+) ---
	add("/api/operations/crashloop", "get", OpenAPIOperation{
		Summary: "CrashLoopBackOff detector & crash pattern analyzer", OperationID: "crashLoop", Tags: []string{"Operations", "Pods", "CrashLoop"},
		Description: "Detects CrashLoopBackOff state and analyzes crash patterns across all pods. Classifies each crashing container by pattern: OOM (memory exhaustion), config-error (exit code 1, missing deps), permission-denied (securityContext or volume issues), image-issue (pull failures), rolling-crash (rapid startup failure in new pods), or unknown. Estimates crash interval from pod age and restart count, detects rapid restarts (within last hour), identifies owner deployment, and provides root cause hypothesis per container. Includes per-namespace crash statistics, pattern grouping, top crashers ranking, cluster crash health score (0-100), and actionable recommendations with kubectl commands.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CrashLoop analysis report", map[string]interface{}{
				"summary": map[string]interface{}{
					"crashLoopPods":   3,
					"highRestartPods": 2,
					"rapidRestarts":   1,
					"patternOOM":      1,
					"healthScore":     72,
				},
				"affectedPods": []interface{}{},
				"patterns":     []interface{}{},
				"topCrashers":  []interface{}{},
				"byNamespace":  []interface{}{},
			}),
		},
	})

	// --- Pod Density & Scheduling Capacity (v15.12+) ---
	add("/api/scalability/pod-density", "get", OpenAPIOperation{
		Summary: "Pod density & scheduling capacity analyzer", OperationID: "podDensity", Tags: []string{"Scalability", "Scheduling", "Capacity"},
		Description: "Analyzes pod density and scheduling capacity across all nodes. Per-node: pod count vs max-pods limit, CPU/memory request vs allocatable, pod capacity percentage, pod headroom, risk level. Cluster-wide: total scheduling headroom (pod slots, CPU cores, memory GB), nodes at/near capacity, cordoned nodes, resource fragmentation detection (pod slots available but blocked by CPU/memory exhaustion). Bin-packing analysis: standard deviation of CPU/memory/pod distribution, imbalance score, distribution strategy classification (spread/moderate/uneven). Actionable recommendations for node expansion, fragmentation resolution, and workload rebalancing.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod density analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":        3,
					"schedulableNodes":  3,
					"avgPodsPerNode":    35.2,
					"totalHeadroomPods": 225,
					"cpuHeadroomCores":  12.5,
					"nodesNearFull":     1,
					"healthScore":       85,
				},
				"nodeAnalysis": []interface{}{},
				"binPacking":   map[string]interface{}{},
				"fragments":    []interface{}{},
			}),
		},
	})

	// --- Container Image Deployment Hygiene (v15.13+) ---
	add("/api/deployment/image-hygiene", "get", OpenAPIOperation{
		Summary: "Container image deployment hygiene analyzer", OperationID: "imageHygiene", Tags: []string{"Deployment", "Images", "CI/CD"},
		Description: "Analyzes all running container images for deployment hygiene. Checks: :latest tag usage (mutable, non-reproducible), missing tags (defaults to latest), digest pinning (@sha256), version tag classification, duplicate detection (same base image with multiple tags), registry trust level, per-registry distribution. Provides per-image risk level, replica count, namespace coverage, and pod list. Cluster-wide image hygiene score (0-100) with actionable recommendations for tag pinning, digest usage, duplicate consolidation, and private registry migration.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter pods by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image hygiene report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers": 25,
					"uniqueImages":    12,
					"latestTagCount":  3,
					"digestPinned":    5,
					"duplicateImages": 2,
					"hygieneScore":    72,
				},
				"images":     []interface{}{},
				"byRegistry": []interface{}{},
				"duplicates": []interface{}{},
			}),
		},
	})

	// --- ConfigMap & Secret Configuration Audit (v15.14+) ---
	add("/api/product/config-audit", "get", OpenAPIOperation{
		Summary: "ConfigMap & Secret configuration audit", OperationID: "configAudit", Tags: []string{"Product", "ConfigMaps", "Secrets"},
		Description: "Audits all ConfigMaps and Secrets for best practices. ConfigMaps: large size detection (>1MB slows etcd), unreferenced detection (not used by any pod via volume/env/envFrom), empty data keys, immutability flag. Secrets: stale credential detection (>180 days), unreferenced detection, plaintext credential key detection (password/token/key in Opaque secrets), immutability flag, rotation recommendation. Cross-references all pods to build accurate usage maps. Provides per-resource risk level, cluster-wide config audit health score (0-100), and actionable recommendations for cleanup, rotation policy, and etcd optimization.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Config audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalConfigMaps": 15,
					"totalSecrets":    8,
					"unreferencedCMs": 3,
					"largeCMs":        1,
					"oldSecrets":      2,
					"healthScore":     78,
				},
				"configMaps":   []interface{}{},
				"secrets":      []interface{}{},
				"unreferenced": []interface{}{},
				"largeConfigs": []interface{}{},
			}),
		},
	})

	// --- Certificate & TLS Expiry Monitor (v15.16+) ---
	add("/api/security/cert-expiry", "get", OpenAPIOperation{
		Summary: "Certificate & TLS expiry monitor", OperationID: "certExpiry", Tags: []string{"Security", "Certificates", "TLS"},
		Description: "Monitors all TLS certificates (kubernetes.io/tls type Secrets) for expiry. Parses each certificate's PEM data to extract: Common Name (CN), Subject Alternative Names (SANs), Issuer, validity period (NotBefore/NotAfter), key size, and self-signed status. Classifies risk: critical (expired or <30d), high (<60d), medium (<90d), low (>90d). Tracks which certificates are referenced by running pods via volume mounts. Provides cluster-wide certificate health score (0-100), per-namespace statistics, sorted expiry timeline, and actionable recommendations for renewal via cert-manager or manual rotation.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Certificate expiry report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalCerts":  15,
					"expired":     1,
					"expiring30d": 2,
					"expiring60d": 3,
					"expiring90d": 4,
					"healthScore": 72,
				},
				"expired":      []interface{}{},
				"expiringSoon": []interface{}{},
				"allCerts":     []interface{}{},
				"byNamespace":  []interface{}{},
			}),
		},
	})

	// --- PDB Compliance & Voluntary Disruption Risk (v15.17+) ---
	add("/api/operations/pdb-audit", "get", OpenAPIOperation{
		Summary: "PDB compliance & voluntary disruption risk analyzer", OperationID: "pdbAudit", Tags: []string{"Operations", "Disruption", "PDB"},
		Description: "Audits PodDisruptionBudget compliance and voluntary disruption risk. Matches PDBs to their target deployments via label selectors. Classifies PDB status: healthy (allowed disruptions > 0), blocked (allowed = 0, drain will stall), impossible (minAvailable > current pods, can never satisfy). Identifies multi-replica deployments without PDB coverage, ranked by replica count risk. Simulates node drain impact: per-node analysis of which PDBs would block eviction. Cluster-wide PDB coverage score (0-100) with actionable recommendations for PDB creation, impossible PDB fixes, and drain planning.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PDB audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalDeployments":        20,
					"totalPDBs":               8,
					"protectedCount":          8,
					"unprotectedCount":        7,
					"blockedCount":            1,
					"totalAllowedDisruptions": 12,
					"healthScore":             65,
				},
				"protectedWorkloads": []interface{}{},
				"unprotected":        []interface{}{},
				"blockers":           []interface{}{},
				"drainSimulation":    []interface{}{},
			}),
		},
	})

	// --- Namespace Resource Consumption & Cost Attribution (v15.18+) ---
	add("/api/scalability/ns-consumption", "get", OpenAPIOperation{
		Summary: "Namespace resource consumption & cost attribution", OperationID: "nsConsumption", Tags: []string{"Scalability", "FinOps", "Cost"},
		Description: "Analyzes per-namespace resource consumption and estimates cost attribution. Aggregates CPU/memory requests and limits across all pods, plus PVC storage capacity. Calculates estimated monthly cost per namespace using configurable pricing ($28/core CPU, $3.8/GB memory, $0.10/GB storage defaults). Identifies waste: over-provisioned namespaces (limit >> request, >5x over-commit ratio), idle namespaces (no running pods, wasted budget), and total wasted capacity in limit-request gap. Provides resource efficiency metrics (request/limit ratio), per-namespace cost share percentage, top 10 consumers ranked by cost, and actionable FinOps recommendations for right-sizing and cleanup.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Namespace consumption report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces":  12,
					"activeNamespaces": 10,
					"idleNamespaces":   2,
					"estMonthlyCost":   285.50,
					"avgEfficiency":    62.5,
				},
				"byNamespace":  []interface{}{},
				"topConsumers": []interface{}{},
				"wasteAnalysis": map[string]interface{}{
					"overProvisionedNS": 3,
					"idleCost":          45.20,
					"wasteScore":        38,
				},
				"costConfig": map[string]interface{}{},
			}),
		},
	})

	// --- Deployment Rollout Strategy & Health (v15.19+) ---
	add("/api/deployment/rollout-health", "get", OpenAPIOperation{
		Summary: "Deployment rollout strategy & health analyzer", OperationID: "rolloutHealth", Tags: []string{"Deployment", "Rollout", "Strategy"},
		Description: "Analyzes deployment rollout strategies and health status. Per-deployment: strategy type (RollingUpdate/Recreate), maxSurge/maxUnavailable config, revisionHistoryLimit (rollback readiness), progressDeadlineSeconds, minReadySeconds, replica status (desired/updated/ready/available/unavailable), conditions (Progressing, Available, ReplicaFailure). Classifies status: healthy (all replicas ready), progressing (rolling update in progress), stuck (Progressing=False or ReplicaFailure=True or deadline exceeded), paused. Detects: Recreate strategy with multiple replicas (causes downtime), revisionHistoryLimit=0 (rollback impossible), aggressive progressDeadline (<300s), missing minReadySeconds. Cluster-wide rollout health score (0-100) with actionable recommendations.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollout health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalDeployments": 15,
					"healthy":          12,
					"stuck":            1,
					"paused":           1,
					"defaultStrategy":  8,
					"recreateStrategy": 2,
					"healthScore":      78,
				},
				"deployments":   []interface{}{},
				"stuckRollouts": []interface{}{},
				"poorStrategy":  []interface{}{},
			}),
		},
	})

	// --- Network Policy Compliance & Traffic Isolation (v15.20+) ---
	add("/api/product/network-policy", "get", OpenAPIOperation{
		Summary: "Network policy compliance & traffic isolation auditor", OperationID: "networkPolicy", Tags: []string{"Product", "Security", "NetworkPolicy"},
		Description: "Audits NetworkPolicy coverage and traffic isolation across the cluster. Matches policies to pods via label selectors to determine which pods have traffic restrictions. Per-namespace: pod count, policy count, protected pod count, default-deny status, isolation score (0-100). Identifies: namespaces with pods but zero NetworkPolicies (all traffic unrestricted), unprotected pods (no policy selects them), permissive egress policies (0.0.0.0/0 = data exfiltration risk), missing default-deny baseline. Cluster-wide isolation score with actionable recommendations for policy creation.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Network policy audit", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces": 8,
					"totalPods":       45,
					"protectedPods":   15,
					"unprotectedPods": 30,
					"totalPolicies":   5,
					"defaultDenyNS":   1,
					"isolationScore":  33,
				},
				"byNamespace":     []interface{}{},
				"unprotectedPods": []interface{}{},
				"allPolicies":     []interface{}{},
			}),
		},
	})

	// --- Volume Security & Mount Risk (v15.22+) ---
	add("/api/security/volume-mounts", "get", OpenAPIOperation{
		Summary: "Volume & mount risk security auditor", OperationID: "volumeSecurity", Tags: []string{"Security", "Volumes", "Container Escape"},
		Description: "Audits all pod volume mounts for container escape risks. Scans every container's volumeMounts against 14 known dangerous paths (docker.sock, /proc, /sys, /, kubelet data, etcd, etc.). HostPath analysis: risk level per mount (critical/high/medium/low), read-write vs read-only, path sensitivity. Privileged container detection. Host namespace sharing (hostNetwork/hostPID/hostIPC). ServiceAccount token projection tracking. Per-namespace risk aggregation with critical mount counts. Cluster-wide volume security score (0-100, higher = safer).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Volume security audit", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":          45,
					"podsWithHostPath":   5,
					"podsWithPrivileged": 2,
					"criticalMounts":     3,
					"securityScore":      72,
				},
				"dangerousMounts": []interface{}{},
				"hostPathVolumes": []interface{}{},
				"saTokenVolumes":  []interface{}{},
				"byNamespace":     []interface{}{},
			}),
		},
	})

	// --- Topology Spread & Pod Distribution (v15.23+) ---
	add("/api/operations/topology-distribution", "get", OpenAPIOperation{
		Summary: "Topology spread & pod distribution auditor", OperationID: "topologySpread", Tags: []string{"Operations", "Scheduling", "Availability"},
		Description: "Audits pod distribution across nodes and topology spread constraint compliance. Per-workload: node distribution map, max pods per node, unique node count, spread ratio, topologySpreadConstraints status, podAntiAffinity status. Risk classification: critical (>70% on one node), high (>50%), medium (>34%), low (<34%). Identifies: concentrated workloads (single-node failure risk), missing constraints (multi-replica without TSC/anti-affinity), node load imbalance. Cluster-wide distribution score (0-100) with recommendations for topologySpreadConstraints adoption.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Topology spread report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":    15,
					"withConstraints":   5,
					"concentrated":      3,
					"wellSpread":        8,
					"distributionScore": 65,
				},
				"byController":      []interface{}{},
				"concentrated":      []interface{}{},
				"nodeLoadImbalance": []interface{}{},
			}),
		},
	})

	// --- Cluster Capacity Headroom & Scale-Out (v15.24+) ---
	add("/api/scalability/capacity-headroom", "get", OpenAPIOperation{
		Summary: "Cluster capacity headroom & scale-out readiness", OperationID: "capacityHeadroom", Tags: []string{"Scalability", "Capacity", "Planning"},
		Description: "Analyzes cluster capacity headroom and scale-out readiness. Per-node: allocatable vs used CPU/memory, headroom percentage, pod slot usage, bottleneck resource identification, full-node detection (<10% headroom). Cluster-wide: total/used/free CPU/memory, utilization %, bottleneck resource, headroom score (0-100, min of free CPU/memory/pod-slots). Pod scheduling profiles: how many small/medium/large/xlarge pods can fit before cluster is full, with limiting factor. Scale-out readiness: Cluster Autoscaler/Karpenter detection, urgency level (immediate/soon/no). Recommendations for node addition, workload right-sizing, and autoscaler configuration.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Capacity headroom report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":       5,
					"schedulableNodes": 5,
					"fullNodes":        1,
					"cpuUtilization":   72.5,
					"memUtilization":   68.3,
					"bottleneck":       "cpu",
					"headroomScore":    27,
				},
				"byNode":          []interface{}{},
				"bottleneckNodes": []interface{}{},
				"podProfiles":     []interface{}{},
				"scaleOutReady":   map[string]interface{}{},
			}),
		},
	})

	// --- Health Probe Compliance Auditor (v15.25+) ---
	add("/api/deployment/probe-compliance", "get", OpenAPIOperation{
		Summary: "Health probe compliance auditor", OperationID: "probeCompliance", Tags: []string{"Deployment", "Probes", "Reliability"},
		Description: "Audits liveness, readiness, and startup probe configuration across all deployments. Per-container: probe type (httpGet/tcpSocket/exec), path, port, timing thresholds (initialDelay, period, timeout, successThreshold, failureThreshold). Identifies: containers with zero probes (no health monitoring), missing readiness (traffic to unhealthy pods), missing liveness (stale containers won't restart), tcpSocket probes (less reliable than HTTP), missing startup probes (slow apps at risk of false liveness failures). Misconfiguration detection: excessive initialDelay (>120s/180s), slow period (>60s/30s), high failureThreshold (>10), long timeout (>10s), wrong successThreshold (>1 for liveness). Cluster-wide probe compliance health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Probe compliance report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers":  25,
					"hasLiveness":      18,
					"hasReadiness":     15,
					"hasStartup":       3,
					"missingLiveness":  7,
					"missingReadiness": 10,
					"noProbeAtAll":     4,
					"healthScore":      52,
				},
				"byWorkload":       []interface{}{},
				"missingReadiness": []interface{}{},
				"missingLiveness":  []interface{}{},
				"misconfigured":    []interface{}{},
			}),
		},
	})

	// --- Label & Annotation Hygiene (v15.26+) ---
	add("/api/product/label-hygiene", "get", OpenAPIOperation{
		Summary: "Label & annotation hygiene auditor", OperationID: "labelHygiene", Tags: []string{"Product", "Labels", "Governance"},
		Description: "Audits label and annotation hygiene across all workloads. Checks for: zero-label workloads (breaks Service selectors, monitoring, NetworkPolicy matching), missing standard labels (app.kubernetes.io/name for kubectl/Helm discovery), missing team/owner labels (breaks ownership tracking and FinOps cost attribution), missing version labels, malformed label keys (non-DNS-1123 format), and excessive labels (>20). Per-namespace hygiene scoring. Cluster-wide label compliance health score (0-100). Recommendations for label standardization.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Label hygiene report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":   15,
					"hasStandardLabel": 10,
					"hasTeamLabel":     7,
					"noLabels":         2,
					"malformedKeys":    1,
					"healthScore":      68,
				},
				"byWorkload":      []interface{}{},
				"noLabels":        []interface{}{},
				"missingStandard": []interface{}{},
				"byNamespace":     []interface{}{},
			}),
		},
	})

	// --- Endpoint Exposure & Attack Surface (v15.28+) ---
	add("/api/security/endpoint-exposure", "get", OpenAPIOperation{
		Summary: "Service endpoint exposure & attack surface auditor", OperationID: "endpointExposure", Tags: []string{"Security", "Network", "Attack Surface"},
		Description: "Maps all externally-accessible services and ingress routes to identify the cluster's attack surface. Per-service: type (ClusterIP/NodePort/LoadBalancer), exposure level (public/node/internal), external IPs, port analysis (HTTP/HTTPS), NetworkPolicy coverage status. Per-ingress: host list, TLS status, backend service, HTTP vs TLS route counts. Identifies: exposed services without NetworkPolicy (unrestricted access), ingress without TLS (plaintext traffic), NodePorts (exposed on all nodes), external IPs (manual firewall bypass). Per-namespace exposure aggregation. Cluster-wide attack surface score (0-100, higher = safer).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Endpoint exposure report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalServices":      25,
					"exposedExternal":    5,
					"loadBalancers":      2,
					"nodePorts":          3,
					"totalIngress":       4,
					"ingressNoTLS":       1,
					"noNetworkPolicy":    3,
					"attackSurfaceScore": 62,
				},
				"exposedServices": []interface{}{},
				"ingressRoutes":   []interface{}{},
				"byNamespace":     []interface{}{},
			}),
		},
	})

	// --- Image Pull & Container Start Failure Tracker (v15.29+) ---
	add("/api/operations/image-pull-failures", "get", OpenAPIOperation{
		Summary: "Image pull & container start failure tracker", OperationID: "imagePullFailures", Tags: []string{"Operations", "Troubleshooting", "Images"},
		Description: "Tracks image pull failures (ImagePullBackOff, ErrImagePull, ErrImageNeverPull) and container start failures (CreateContainerError, CreateContainerConfigError) across all pods. Per-container: image, reason, error message, restart count, age, risk level. Aggregates failures by unique image (failure count, pods affected, registry, reasons). Classifies root causes: registry authentication failures (unauthorized), Docker Hub rate limiting (toomanyrequests), invalid image names, config errors. Per-namespace failure tracking with health scoring. Cluster-wide image pull health score (0-100). Recommendations for imagePullSecrets, registry mirrors, and image verification.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image pull failure report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":          50,
					"failedPods":         3,
					"imagePullBackOff":   2,
					"registryAuthFail":   1,
					"uniqueFailedImages": 2,
					"healthScore":        88,
				},
				"failedContainers": []interface{}{},
				"byImage":          []interface{}{},
				"byNamespace":      []interface{}{},
			}),
		},
	})

	// --- Quota Utilization & Limit Compliance (v15.30+) ---
	add("/api/scalability/quota-utilization", "get", OpenAPIOperation{
		Summary: "Resource quota utilization & limit compliance auditor", OperationID: "quotaUtilization", Tags: []string{"Scalability", "Quota", "Governance"},
		Description: "Audits ResourceQuota utilization, LimitRange compliance, and container resource governance across the cluster. Per-quota: hard limits, used amounts, utilization percentage per resource, max utilization, risk level (critical >90%, high >80%). Per-LimitRange: default request/limit presence, max limit enforcement. Container analysis: containers without requests (scheduler blind spots), containers without limits (noisy neighbor risk). Per-namespace: quota presence, limit range presence, max utilization, risk level. Identifies: namespaces without quotas (unbounded consumption), critical quotas (>80%), unbounded containers, missing LimitRanges. Cluster-wide quota compliance score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Quota utilization report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces": 8,
					"nsWithQuota":     5,
					"nsWithoutQuota":  3,
					"criticalQuotas":  2,
					"totalContainers": 50,
					"noRequests":      8,
					"noLimits":        12,
					"unboundedRatio":  20.0,
					"complianceScore": 65,
				},
				"quotas":         []interface{}{},
				"criticalQuotas": []interface{}{},
				"limitRanges":    []interface{}{},
				"unboundedPods":  []interface{}{},
				"byNamespace":    []interface{}{},
			}),
		},
	})

	// --- Resource Limit & Enforcement Gap (v15.32+) ---
	add("/api/deployment/resource-limits", "get", OpenAPIOperation{
		Summary: "Resource limit & enforcement gap auditor", OperationID: "resourceLimits", Tags: []string{"Deployment", "Resources", "Governance"},
		Description: "Audits resource limits and enforcement gaps across all containers. Per-container: CPU/memory requests and limits in both human-readable and machine-numeric forms, request-to-limit ratio, risk classification. Identifies: unbounded containers (no limits at all — critical), missing memory limits (OOM kill risk), missing CPU limits (no throttling protection), under-provisioned containers (limit/request < 1.2x — tight burst headroom), over-provisioned containers (limit/request > 4x — wasted capacity), excessive requests (>2000m CPU or >4Gi memory). Per-namespace aggregation with total CPU/memory requests. Cluster-wide resource compliance score (0-100). Recommendations for right-sizing, LimitRange defaults, and resource governance.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Resource limit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers":  30,
					"noLimits":         5,
					"noCPULimit":       8,
					"noMemLimit":       6,
					"overProvisioned":  4,
					"underProvisioned": 3,
					"complianceScore":  55,
				},
				"byWorkload":       []interface{}{},
				"unbounded":        []interface{}{},
				"overProvisioned":  []interface{}{},
				"underProvisioned": []interface{}{},
				"byNamespace":      []interface{}{},
			}),
		},
	})

	// --- Orphaned Resource Detector (v15.33+) ---
	add("/api/product/orphaned-resources", "get", OpenAPIOperation{
		Summary: "Orphaned resource detector", OperationID: "orphanedResources", Tags: []string{"Product", "Cleanup", "Hygiene"},
		Description: "Detects orphaned resources across all 5 resource types. Orphaned Services: selector returns zero pods (traffic goes nowhere). Orphaned ConfigMaps: not referenced by any pod's volumes, envFrom, or env ValueFrom. Orphaned Secrets: not referenced by any pod (stale credential risk). Orphaned PVCs: not mounted by any pod (wasted storage). Orphaned Ingresses: backend service does not exist (404/502 for users). Skips auto-created resources (kube-root-ca.crt, service account tokens, kube-system services). Per-namespace orphan breakdown. Cluster-wide resource hygiene score (0-100). Recommendations for cleanup and CI/CD integration.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Orphaned resource report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalServices":    20,
					"totalConfigMaps":  35,
					"totalSecrets":     15,
					"totalPVCs":        8,
					"orphanedServices": 3,
					"orphanedConfigs":  10,
					"orphanedSecrets":  4,
					"orphanedPVCs":     2,
					"totalOrphaned":    19,
					"hygieneScore":     65,
				},
				"orphanedServices": []interface{}{},
				"orphanedConfigs":  []interface{}{},
				"orphanedSecrets":  []interface{}{},
				"orphanedPVCs":     []interface{}{},
				"orphanedIngress":  []interface{}{},
			}),
		},
	})

	// --- Seccomp & PSS Restricted Compliance (v15.34+) ---
	add("/api/security/seccomp-audit", "get", OpenAPIOperation{
		Summary: "Seccomp profile & PSS restricted compliance auditor", OperationID: "seccompAudit", Tags: []string{"Security", "Hardening", "PodSecurityStandards"},
		Description: "Audits seccomp profiles and Pod Security Standards restricted-level compliance across all containers. Per-container: seccomp profile type (RuntimeDefault/Localhost/Unconfined/unset), capabilities drop/add list, droppedAll flag, allowPrivilegeEscalation status, runAsNonRoot/runAsUser check, readOnlyRootFilesystem, privileged flag. PSS level classification: restricted (fully compliant) / baseline (partial) / privileged (fails baseline). Dangerous capability detection: SYS_ADMIN, SYS_MODULE, NET_ADMIN, SYS_PTRACE, etc. Container hardening score (0-100). Recommendations for Pod Security Admission, seccomp defaults, and capability minimization.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Seccomp & PSS compliance report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers": 25,
					"hasSeccomp":      10,
					"noSeccomp":       15,
					"droppedAllCaps":  8,
					"canEscalate":     12,
					"runsAsRoot":      5,
					"pssRestrictedOK": 4,
					"hardeningScore":  38,
				},
				"byWorkload":   []interface{}{},
				"nonCompliant": []interface{}{},
				"noSeccomp":    []interface{}{},
				"canEscalate":  []interface{}{},
			}),
		},
	})

	// --- Pod Restart Reason Analyzer (v15.35+) ---
	add("/api/operations/restart-reasons", "get", OpenAPIOperation{
		Summary: "Pod restart reason analyzer", OperationID: "restartReasons", Tags: []string{"Operations", "Troubleshooting", "Reliability"},
		Description: "Comprehensively categorizes WHY pods are restarting across the cluster. Goes beyond CrashLoopBackOff/OOM tracker to give the full restart picture. Per-container: last termination reason, exit code, restart count, risk level. Reason categorization: OOMKilled (exit 137), application errors (exit != 0), config errors (CreateContainerError, ErrImagePull), DeadlineExceeded (Jobs), Completed (exit 0), Unknown. Top 20 restarters by restart count. Per-namespace restart breakdown with reason distribution. Cluster-wide stability score (0-100) based on restarted/total pod ratio. Recommendations for memory tuning, log investigation, and backoff limits.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Restart reason report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":      100,
					"restartedPods":  15,
					"totalRestarts":  85,
					"oomKills":       5,
					"appErrors":      3,
					"configErrors":   2,
					"maxRestarts":    42,
					"stabilityScore": 77,
				},
				"byReason":      map[string]int{},
				"topRestarters": []interface{}{},
				"oomKills":      []interface{}{},
				"byNamespace":   []interface{}{},
			}),
		},
	})

	// --- HA & Single-Point-of-Failure Detector (v15.36+) ---
	add("/api/scalability/ha-audit", "get", OpenAPIOperation{
		Summary: "High availability & single-point-of-failure detector", OperationID: "haAudit", Tags: []string{"Scalability", "HA", "Reliability"},
		Description: "Detects single points of failure across all deployments. SPOF detection: single-replica deployments (any restart causes downtime), multi-replica without PDB (voluntary disruptions kill all pods), no pod anti-affinity (pods may co-locate on one node), single-node spread (all pods on one node despite multiple replicas), missing readiness probes (slow failover). Per-workload: replica count, ready replicas, PDB status, anti-affinity/topologySpread status, node spread count, readiness probe presence, SPOF risk list. Risk classification: critical (single replica or single-node spread), high (no PDB), medium (no anti-affinity or no readiness), low (fully HA). Per-namespace HA scoring. Cluster-wide HA score (0-100). Recommendations for scaling, PDB, anti-affinity, and topology spread.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("HA & SPOF report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":   15,
					"singleReplicas":   4,
					"multiReplica":     11,
					"noPDB":            6,
					"noAntiAffinity":   8,
					"singleNodeSpread": 2,
					"noReadiness":      3,
					"haScore":          52,
				},
				"singleReplicas": []interface{}{},
				"noPDB":          []interface{}{},
				"noAntiAffinity": []interface{}{},
				"allEntries":     []interface{}{},
			}),
		},
	})

	// --- Graceful Shutdown & Termination Compliance (v15.38+) ---
	add("/api/deployment/graceful-shutdown", "get", OpenAPIOperation{
		Summary: "Graceful shutdown & termination compliance auditor", OperationID: "gracefulShutdown", Tags: []string{"Deployment", "Lifecycle", "ZeroDowntime"},
		Description: "Audits graceful shutdown configuration for zero-downtime deployments. Per-container: preStop hook presence and action (httpGet/exec), readiness probe (needed for endpoint draining), terminationGracePeriodSeconds classification (short <10s / default 30s / custom / long >60s). Identifies: containers that WILL drop in-flight requests during rolling updates (no preStop + no readiness = critical), missing preStop hooks (SIGTERM sent immediately), missing readiness probes (endpoints not removed before termination), short grace periods (insufficient for slow shutdown apps). Graceful shutdown score (0-100). Recommendations for preStop hooks, drain endpoints, and grace period tuning.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Graceful shutdown report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalContainers": 25,
					"hasPreStop":      5,
					"noPreStop":       20,
					"hasReadiness":    15,
					"noReadiness":     10,
					"likelyDropReqs":  8,
					"shutdownScore":   35,
				},
				"byWorkload":       []interface{}{},
				"noPreStop":        []interface{}{},
				"shortGracePeriod": []interface{}{},
			}),
		},
	})

	// --- PV/PVC Storage Health (v15.39+) ---
	add("/api/product/pvc-health", "get", OpenAPIOperation{
		Summary: "PV/PVC storage health & capacity auditor", OperationID: "pvcHealth", Tags: []string{"Product", "Storage", "Capacity"},
		Description: "Audits PersistentVolume and PersistentVolumeClaim health across the cluster. Per-PVC: phase (Bound/Pending/Lost), storage class, access modes, capacity, bound PV name, risk level. Per-PV: phase (Bound/Available/Released/Failed), reclaim policy (Retain/Delete), capacity, claim ref. StorageClass analysis: provisioner, volume binding mode, allowVolumeExpansion flag, default SC detection, PVC count per SC. Issue detection: Pending PVCs (provisioning stuck), Lost PVCs (PV in Lost/Failed state), Failed PVs (storage backend errors), Released PVs (orphaned storage wasting capacity), SCs without volume expansion, missing default StorageClass, Reclaim Retain PVs (orphan risk). Per-namespace PVC stats. Storage health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Storage health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPVCs":   15,
					"boundPVCs":   12,
					"pendingPVCs": 2,
					"releasedPVs": 1,
					"healthScore": 76,
				},
				"pvcs":           []interface{}{},
				"pendingPVCs":    []interface{}{},
				"pvs":            []interface{}{},
				"storageClasses": []interface{}{},
			}),
		},
	})

	// --- CronJob & Batch Job Security Audit (v15.40+) ---
	add("/api/security/batch-audit", "get", OpenAPIOperation{
		Summary: "CronJob & batch job security audit", OperationID: "batchSecurity", Tags: []string{"Security", "BatchWorkloads", "CronJobs"},
		Description: "Audits CronJobs and one-shot Jobs for security risks. Batch workloads are the most overlooked security attack surface: they run with elevated SAs, mount secrets for data processing, and can be used for attacker persistence. Per-workload: privileged flag, hostPath mounts, hostNetwork/hostPID, ServiceAccount usage (default vs dedicated), resource limits, secret mount count, concurrency limit, schedule analysis. Detection: privileged containers (critical), hostPath access (critical), hostNetwork/hostPID (high), default ServiceAccount (medium), no resource limits (medium), suspicious every-minute schedules (persistence risk), no concurrency limit (fork-bomb risk), excessive secret mounts. Batch security score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Batch security report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalCronJobs":      8,
					"totalJobs":          3,
					"privileged":         1,
					"hostPath":           2,
					"defaultSA":          4,
					"suspiciousSchedule": 1,
					"securityScore":      55,
				},
				"cronJobs":    []interface{}{},
				"oneShotJobs": []interface{}{},
				"highRisk":    []interface{}{},
				"suspicious":  []interface{}{},
			}),
		},
	})

	// --- Pod Scheduling Latency Analyzer (v15.41+) ---
	add("/api/operations/scheduling-latency", "get", OpenAPIOperation{
		Summary: "Pod scheduling latency analyzer", OperationID: "schedulingLatency", Tags: []string{"Operations", "Scheduling", "Capacity"},
		Description: "Tracks pod scheduling latency across the cluster. Per-pod: time from creation to PodScheduled condition (seconds), current phase, assigned node, pending reason. Identifies: slow pods (>60s to schedule), very slow pods (>300s), unschedulable pods (Pending with Unschedulable condition), resource shortage (Insufficient cpu/memory), exceeded quota. Per-node average scheduling time and slow count. Per-namespace pending count. Cluster-wide scheduling efficiency score (0-100). Recommendations for capacity planning, priority classes, and scheduling constraint optimization.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scheduling latency report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":       150,
					"runningPods":     140,
					"pendingPods":     3,
					"avgScheduleSec":  12.5,
					"maxScheduleSec":  340,
					"slowCount":       8,
					"unschedulable":   2,
					"efficiencyScore": 72,
				},
				"slowPods":    []interface{}{},
				"pendingPods": []interface{}{},
				"byNode":      []interface{}{},
			}),
		},
	})

	// --- Node Failure Impact Simulator (v15.42+) ---
	add("/api/scalability/node-failure-sim", "get", OpenAPIOperation{
		Summary: "Node failure impact simulator", OperationID: "nodeFailureSim", Tags: []string{"Scalability", "HA", "FailureSimulation"},
		Description: "Simulates the impact of each node failing. For every node: which pods would be affected (count), can they be rescheduled on other nodes (resource capacity, node selector, taints/tolerations check), how many are unschedulable, how many are single-replica workloads (permanent downtime). Identifies critical nodes (>10 affected pods), nodes hosting single-replica workloads, worst-case blast radius. Excludes DaemonSet pods (they're on every node), completed pods, and kube-system pods from rescheduling analysis. Per-node: CPU/memory requests, top 5 affected workloads. Cluster-wide resilience score (0-100). Recommendations for anti-affinity, scaling, and node spreading.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node failure simulation report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":         5,
					"affectedPerNodeAvg": 8,
					"maxAffected":        15,
					"criticalNodes":      2,
					"singleReplicaNodes": 1,
					"resilienceScore":    65,
				},
				"byNode":        []interface{}{},
				"criticalNodes": []interface{}{},
				"singleReplica": []interface{}{},
			}),
		},
	})

	// --- Deployment Update Strategy & Rollback Readiness (v15.44+) ---
	add("/api/deployment/update-strategy", "get", OpenAPIOperation{
		Summary: "Deployment update strategy & rollback readiness auditor", OperationID: "updateStrategy", Tags: []string{"Deployment", "Rollout", "Rollback"},
		Description: "Audits deployment update strategies for safe rollouts and rollback readiness. Per-deployment: strategy type (RollingUpdate/Recreate), maxSurge/maxUnavailable values, revisionHistoryLimit, progressDeadlineSeconds. Detection: Recreate strategy (causes downtime, critical), maxUnavailable=100% (all pods down during update), maxSurge=0 (no extra capacity, slow rollouts), low revisionHistoryLimit (<3, insufficient rollback history), missing progressDeadlineSeconds (failed deploys hang indefinitely). Readiness score (0-100). Recommendations for strategy tuning, rollback capability, and progress tracking.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Update strategy report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":     15,
					"rollingUpdate":      12,
					"recreate":           3,
					"revHistoryLow":      4,
					"noProgressDeadline": 8,
					"readinessScore":     58,
				},
				"byWorkload":        []interface{}{},
				"recreateStrategy":  []interface{}{},
				"noRevisionHistory": []interface{}{},
			}),
		},
	})

	// --- StatefulSet Health & Ordered Rollout Audit (v15.45+) ---
	add("/api/product/statefulset-audit", "get", OpenAPIOperation{
		Summary: "StatefulSet health & ordered rollout auditor", OperationID: "statefulSetAudit", Tags: []string{"Product", "StatefulSet", "Storage"},
		Description: "Audits StatefulSet health and ordered rollout status. StatefulSets are critical for databases and stateful apps with unique challenges: ordered rollout, PVC retention, partition canary updates, headless service requirement. Per-StatefulSet: replica/ready/updated counts, current vs update revision, pod management policy (OrderedReady/Parallel), PVC retention policy (Retain/Delete), headless service existence, volume claim templates, partition canary status. Detection: missing headless service (critical, pod DNS fails), stuck rollouts (ready < replicas), PVC Delete retention (data loss on STS deletion), paused canary (partition > 0), no volumeClaimTemplates (should be Deployment), OrderedReady with large replicas (slow scaling). Health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("StatefulSet health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalStatefulSets": 8,
					"healthy":           6,
					"stuckRollout":      1,
					"noHeadlessSvc":     1,
					"pvcDelete":         2,
					"healthScore":       72,
				},
				"byWorkload":    []interface{}{},
				"stuckRollouts": []interface{}{},
			}),
		},
	})

	// --- Resource Contention & Throttling Detector (v15.46+) ---
	add("/api/operations/resource-contention", "get", OpenAPIOperation{
		Summary: "Resource contention & throttling detector", OperationID: "resourceContention", Tags: []string{"Operations", "Performance", "Resources"},
		Description: "Detects CPU throttling patterns, memory pressure, and resource contention between pods. Per-pod: CPU/memory request/limit values, restart count, node pressure status. Detection: pods on nodes with MemoryPressure/DiskPressure (critical, eviction risk), high-restart pods likely CPU throttled (liveness probe timeouts), no CPU limit (can starve neighbors), no memory limit (OOM cascade), CPU limit <100m (throttled under load), memory limit <128Mi (OOMKilled). Per-namespace contention stats. Contention score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Resource contention report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":       120,
					"throttledPods":   8,
					"memoryPressure":  3,
					"noCpuLimits":     15,
					"noMemoryLimits":  10,
					"cpuLimitTooLow":  5,
					"contentionScore": 62,
				},
				"throttledPods":  []interface{}{},
				"memoryPressure": []interface{}{},
				"byNamespace":    []interface{}{},
			}),
		},
	})

	// --- API Object Count & CRD Explosion Risk (v15.48+) ---
	add("/api/scalability/crd-explosion", "get", OpenAPIOperation{
		Summary: "API object count & CRD explosion risk detector", OperationID: "crdExplosion", Tags: []string{"Scalability", "API", "Capacity"},
		Description: "Counts API objects per resource type and detects CRD explosion risk. As clusters grow, excessive object counts (ConfigMaps, Secrets, CRDs) slow API server list/watch operations and increase etcd size. Per-resource-type: object count, risk level (>1000 critical, >500 high, >200 medium). Per-namespace: ConfigMap/Secret/Service/Pod counts, total objects, top 15 namespaces. Detection: very high object counts (>1000), high secret count per namespace (>100, encryption overhead), high ConfigMap count (>200, cleanup needed), excessive CRDs (>30, API overhead), largest namespace objects (>500, split recommended). Scalability score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CRD explosion risk report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalCRDs":        25,
					"totalConfigMaps":  350,
					"totalSecrets":     180,
					"highCountCRDs":    2,
					"scalabilityScore": 78,
				},
				"byResourceType": []interface{}{},
				"byNamespace":    []interface{}{},
			}),
		},
	})

	// --- Secret/ConfigMap Reference Integrity Checker (v15.49+) ---
	add("/api/deployment/ref-integrity", "get", OpenAPIOperation{
		Summary: "Secret/ConfigMap reference integrity checker", OperationID: "refIntegrity", Tags: []string{"Deployment", "Validation", "CrashLoop"},
		Description: "Verifies that every Secret and ConfigMap reference in Deployments, StatefulSets, and DaemonSets actually exists. Missing references are the #1 cause of CrashLoopBackOff after deployment. Checks: volume mounts (configMap/secret), envFrom (configMapRef/secretRef), env valueFrom (configMapKeyRef/secretKeyRef). For each reference: type, name, source (volume/envFrom/env), optional flag, existence status. Detection: broken references (critical, pod won't start), optional missing references (may be intentional). Integrity score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Reference integrity report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads": 15,
					"totalRefs":      45,
					"brokenRefs":     2,
					"optionalRefs":   3,
					"integrityScore": 93,
				},
				"brokenRefs": []interface{}{},
			}),
		},
	})

	// --- Affinity & Anti-Affinity Conflict Detector (v15.50+) ---
	add("/api/product/affinity-conflict", "get", OpenAPIOperation{
		Summary: "Affinity & anti-affinity conflict detector", OperationID: "affinityConflict", Tags: []string{"Product", "Scheduling", "Affinity"},
		Description: "Detects pods stuck due to unsatisfiable affinity/anti-affinity rules. Per-pod: has affinity/anti-affinity, type (required/preferred), topologyKey, match labels, pending reason. Builds topology domain map from node labels (hostname/zone/region) and checks if required anti-affinity can be satisfied. Detection: unsatisfiable anti-affinity (critical — topology domain too small), pending due to affinity constraints (high), required hard anti-affinity (medium). Health score (0-100). Recommendations for topology spreading, preferred vs required, and node label configuration.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Affinity conflict report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":            120,
					"pendingPods":          5,
					"pendingDueToAffinity": 3,
					"conflicts":            1,
					"healthScore":          75,
				},
				"conflicts":       []interface{}{},
				"pendingPods":     []interface{}{},
				"hasAntiAffinity": []interface{}{},
			}),
		},
	})

	// --- Node Lease & Heartbeat Health Monitor (v15.52+) ---
	add("/api/operations/node-lease", "get", OpenAPIOperation{
		Summary: "Node lease & heartbeat health monitor", OperationID: "nodeLease", Tags: []string{"Operations", "NodeHealth", "Heartbeat"},
		Description: "Monitors kubelet heartbeat freshness via node Lease objects. Tracks heartbeat age (renewTime), identifies stale (>40s) and very stale (>2min) heartbeats, nodes with no Lease, and NotReady nodes. Per-node: lease existence, heartbeat age, holder identity, kubelet version, active negative conditions. Critical for detecting zombie nodes before they cause split-brain or scheduling failures. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node heartbeat report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":         5,
					"readyNodes":         4,
					"staleHeartbeat":     1,
					"noLease":            0,
					"avgHeartbeatAgeSec": 15.5,
					"healthScore":        82,
				},
				"byNode":         []interface{}{},
				"staleHeartbeat": []interface{}{},
			}),
		},
	})

	// --- K8s Scalability Bottleneck Predictor (v15.53+) ---
	add("/api/scalability/bottleneck-predictor", "get", OpenAPIOperation{
		Summary: "K8s scalability bottleneck predictor", OperationID: "scalabilityBottleneck", Tags: []string{"Scalability", "Capacity", "Limits"},
		Description: "Predicts which Kubernetes resource will become the cluster's scalability bottleneck first. Compares actual usage against K8s recommended limits: max pods per node (110), total pods (150k), total services (5k), services per node (20, kube-proxy limit), total nodes (5k), namespaces (10k). Per-resource: current count, K8s limit, ratio (0-100%), status (healthy/warning/critical/bottleneck). Per-node: pod count, pod ratio, risk level. Identifies primary bottleneck type and ratio. Risk score (0-100, higher = safer).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scalability bottleneck prediction", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":      5,
					"maxPodsPerNode":  45,
					"avgPodsPerNode":  32,
					"totalPods":       160,
					"totalServices":   25,
					"bottleneckType":  "max_pods_per_node",
					"bottleneckRatio": 40.9,
					"riskScore":       59,
				},
				"byResource":  []interface{}{},
				"bottlenecks": []interface{}{},
			}),
		},
	})

	// --- Deployment Image Drift & Version Consistency Detector (v15.54+) ---
	add("/api/deployment/image-drift", "get", OpenAPIOperation{
		Summary: "Deployment image drift & version consistency detector", OperationID: "imageDrift", Tags: []string{"Deployment", "Images", "Drift"},
		Description: "Detects image version drift within workloads — pods in the same Deployment/StatefulSet/DaemonSet running different image versions. This happens during stalled rollouts, manual pod edits, or image tag mutation. Per-workload: distinct image variants with pod counts, drift detection, latest tag usage, digest presence. Detection: image drift (high, pods running different versions), latest tag (medium, not reproducible), no digest (low, mutable). Consistency score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image drift report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":   15,
					"driftedWorkloads": 2,
					"usingLatestTag":   4,
					"noDigest":         8,
					"consistencyScore": 62,
				},
				"driftedWorkloads": []interface{}{},
			}),
		},
	})

	// --- Node Taint & Pod Toleration Impact Analyzer (v15.56+) ---
	add("/api/product/taint-toleration", "get", OpenAPIOperation{
		Summary: "Node taint & pod toleration impact analyzer", OperationID: "taintToleration", Tags: []string{"Product", "Scheduling", "Taints"},
		Description: "Analyzes node taints and pod tolerations for maintenance planning and node pool isolation. Per-node: taint list, NoSchedule/NoExecute flags, cordon status, risk level. Per-taint: cluster-wide summary with affected nodes. Pod analysis: broad tolerations (key=Exists, tolerates everything — dangerous, can run on master). Detection: NoExecute taints (critical, evicting pods), cordoned nodes (warning), NoSchedule blocking scheduling, broad tolerations (warning, may run on tainted nodes). Impact score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Taint & toleration report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":       5,
					"nodesWithTaints":  2,
					"noScheduleNodes":  1,
					"noExecuteNodes":   0,
					"cordonedNodes":    1,
					"podsWithBroadTol": 3,
					"impactScore":      72,
				},
				"byNode":           []interface{}{},
				"blockedNodes":     []interface{}{},
				"broadTolerations": []interface{}{},
			}),
		},
	})

	// --- Control Plane Health Checker (v15.57+) ---
	add("/api/operations/control-plane", "get", OpenAPIOperation{
		Summary: "Control plane health checker", OperationID: "controlPlaneHealth", Tags: []string{"Operations", "ControlPlane", "Health"},
		Description: "Verifies control plane component health by checking kube-system pods (kube-apiserver, kube-scheduler, kube-controller-manager, etcd). Per-component: pod name, ready status, restart count, uptime, node kubelet version, risk level. Detection: unhealthy components (critical), excessive restarts (warning), recent restarts <1h uptime (warning), missing critical components like etcd or apiserver (critical). Handles k3s/microk8s/kind which run components as host processes (reports info, not error). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Control plane health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalComponents":     4,
					"healthyComponents":   4,
					"unhealthyComponents": 0,
					"hasEtcd":             true,
					"hasAPIServer":        true,
					"healthScore":         100,
				},
				"components": []interface{}{},
			}),
		},
	})

	// --- Namespace Isolation & Multi-tenancy Audit (v15.59+) ---
	add("/api/scalability/namespace-isolation", "get", OpenAPIOperation{
		Summary: "Namespace isolation & multi-tenancy audit", OperationID: "namespaceIsolation", Tags: []string{"Scalability", "Multi-tenancy", "Isolation"},
		Description: "Audits namespace isolation controls for multi-tenant cluster safety. Per-namespace: NetworkPolicy presence, ResourceQuota presence, LimitRange presence, PSA enforce label (privileged/baseline/restricted). System namespaces (kube-*, default) are excluded from checks. Detection: missing NetworkPolicy (pods accessible from anywhere), missing ResourceQuota (unlimited resource consumption), missing LimitRange (no default requests/limits), no PSA label (privileged pods allowed). Fully isolated = all 3 controls + PSA. Isolation score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Namespace isolation report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces":   15,
					"userNamespaces":    8,
					"withNetworkPolicy": 5,
					"withResourceQuota": 6,
					"fullyIsolated":     3,
					"isolationScore":    72,
				},
				"byNamespace": []interface{}{},
				"unisolated":  []interface{}{},
			}),
		},
	})

	// --- Deployment Revision History & Rollback Readiness (v15.60+) ---
	add("/api/deployment/revision-history", "get", OpenAPIOperation{
		Summary: "Deployment revision history & rollback readiness", OperationID: "revisionHistory", Tags: []string{"Deployment", "Rollback", "RevisionHistory"},
		Description: "Analyzes deployment revision history depth and rollback readiness. Per-deployment: revisionHistoryLimit, ReplicaSet count, current/updated replicas, oldest ReplicaSet age. Detection: revisionHistoryLimit=0 (critical, cannot rollback), revisionHistoryLimit<5 (warning, limited rollback), high churn >10 ReplicaSets (info, frequent deploys), stale ReplicaSets >30 days (etcd waste). Rollback readiness score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Revision history report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalDeployments":  15,
					"lowHistoryLimit":   3,
					"noHistoryLimit":    1,
					"rollbackReadiness": 82,
				},
				"byWorkload": []interface{}{},
			}),
		},
	})

	// --- ConfigMap/Secret Size & Memory Pressure Auditor (v15.61+) ---
	add("/api/product/configmap-size", "get", OpenAPIOperation{
		Summary: "ConfigMap/Secret size & memory pressure auditor", OperationID: "configmapSize", Tags: []string{"Product", "ConfigMap", "Storage"},
		Description: "Audits ConfigMap and Secret sizes for etcd pressure and kubelet memory issues. etcd has a 1.5MB max value size limit. Large ConfigMaps mounted as volumes increase kubelet memory and API server traffic. Per-resource: size in KB, key count, largest key, mount status. Per-namespace: total ConfigMap/Secret sizes. Detection: oversized ConfigMaps >1MB (warning), oversized Secrets >1MB (warning, encryption overhead), large mounted ConfigMaps (kubelet memory). Health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("ConfigMap/Secret size report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalConfigMaps": 50,
					"totalSecrets":    30,
					"oversizedCMs":    2,
					"largestCMSizeKB": 1500.0,
					"totalCMSizeMB":   12.5,
					"healthScore":     85,
				},
				"oversizedCMs": []interface{}{},
			}),
		},
	})

	// --- Pod Eviction & Node Pressure History Tracker (v15.63+) ---
	add("/api/operations/pod-evictions", "get", OpenAPIOperation{
		Summary: "Pod eviction & node pressure history tracker", OperationID: "podEvictions", Tags: []string{"Operations", "Eviction", "Pressure"},
		Description: "Tracks pod evictions and correlates with node pressure conditions. Scans for Failed pods with Evicted status. Per-pod: node, cause (memory/disk/pid/unknown), eviction time, message. Per-node: eviction count by cause, risk level. Per-namespace: eviction count. Detection: high eviction nodes (>=5), recent eviction spikes (>=3 in 24h). Health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod eviction report", map[string]interface{}{
				"summary": map[string]interface{}{
					"evictedPods":     5,
					"recentEvictions": 2,
					"memoryEvictions": 3,
					"diskEvictions":   2,
					"healthScore":     75,
				},
				"recentEvictions": []interface{}{},
				"byNode":          []interface{}{},
			}),
		},
	})

	// --- API Server Audit Logging Configuration Checker (v15.65+) ---
	add("/api/security/audit-policy", "get", OpenAPIOperation{
		Summary: "API server audit logging configuration checker", OperationID: "auditPolicy", Tags: []string{"Security", "Compliance", "Audit"},
		Description: "Verifies Kubernetes audit logging configuration for compliance. Checks: audit enabled (file/webhook backend), audit policy file presence, log retention (maxAge, maxBackup, maxSize), sensitive resource coverage (Secrets/ConfigMaps/RBAC verb auditing). Detects k3s/microk8s environments. Findings categorized as policy/backend/retention/coverage with pass/warning/fail status. Compliance score (0-100). Required for PCI-DSS, SOC2, HIPAA.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Audit policy report", map[string]interface{}{
				"summary": map[string]interface{}{
					"auditEnabled":    true,
					"hasPolicy":       true,
					"logBackend":      "file",
					"maxAgeDays":      30,
					"complianceScore": 70,
				},
				"findings": []interface{}{},
			}),
		},
	})

	// --- CSI Driver & Storage Capability Auditor (v15.67+) ---
	add("/api/scalability/csi-audit", "get", OpenAPIOperation{
		Summary: "CSI driver & storage capability auditor", OperationID: "csiAudit", Tags: []string{"Scalability", "Storage", "CSI"},
		Description: "Audits CSI drivers and StorageClass capabilities. Per-StorageClass: provisioner, default flag, binding mode, volume expansion support, reclaim policy, risk level. Per-CSIDriver: attach required, pod info on mount, fsGroup policy, snapshot support. Detection: no default StorageClass (warning), multiple defaults (warning), missing CSI driver for provisioner (warning), no expansion support (info), Delete reclaim policy (info), no VolumeSnapshotClass (info). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CSI audit report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalStorageClasses": 3,
					"defaultSCCount":      1,
					"expandableSCs":       2,
					"totalCSIDrivers":     1,
					"healthScore":         85,
				},
				"byStorageClass": []interface{}{},
				"csiDrivers":     []interface{}{},
			}),
		},
	})

	// --- Deployment Disruption & Maintenance Impact Analyzer (v15.69+) ---
	add("/api/deployment/disruption-impact", "get", OpenAPIOperation{
		Summary: "Deployment PDB disruption & maintenance impact analyzer", OperationID: "disruptionImpact", Tags: []string{"Deployment", "PDB", "Maintenance"},
		Description: "Analyzes how Deployments/StatefulSets interact with PodDisruptionBudgets during voluntary disruptions (node drains, cluster upgrades). Per-workload: PDB presence, minAvailable/maxUnavailable, evictable pod count, will-block-drain flag. Detection: blocking PDBs (minAvailable=replicas, critical — blocks all evictions), no PDB (warning — unprotected during maintenance), risky PDBs (minAvailable >= replicas). Maintenance readiness score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Disruption impact report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":   15,
					"withPDB":          12,
					"noPDB":            3,
					"blockDrain":       2,
					"maintenanceScore": 72,
				},
				"blockingWorkloads": []interface{}{},
				"safeWorkloads":     []interface{}{},
			}),
		},
	})

	// --- Batch Job Execution Health & Completion Analyzer (v15.70+) ---
	add("/api/product/job-health", "get", OpenAPIOperation{
		Summary: "Batch job execution health & completion analyzer", OperationID: "jobHealth", Tags: []string{"Product", "Batch", "Jobs"},
		Description: "Analyzes batch Job execution health. Per-job: status (Running/Complete/Failed/Suspended/Pending), duration, completions, succeeded/failed counts, backoffLimit, parent CronJob. Detection: failed jobs (warning), long-running >24h (warning, may be stuck), suspended jobs (info), no backoffLimit (info). Health score (0-100).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Job health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalJobs":     8,
					"runningJobs":   2,
					"completedJobs": 5,
					"failedJobs":    1,
					"healthScore":   85,
				},
				"byJob":      []interface{}{},
				"failedJobs": []interface{}{},
			}),
		},
	})

	// --- API Server Responsiveness & Pod Start Latency Monitor (v15.72+) ---
	add("/api/operations/api-latency", "get", OpenAPIOperation{
		Summary: "API server responsiveness & pod start latency monitor", OperationID: "apiLatency", Tags: []string{"Operations", "Latency", "API"},
		Description: "Monitors API server responsiveness and pod start latency. Checks: API server responsiveness (can list pods), pending pods >2min (slow scheduling), not-ready running pods (probe failures), container start delay >1min (image pull slowness). Per-pod: pending minutes, container start delay, risk level. Responsiveness score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("API latency report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":           50,
					"longStartingPods":    2,
					"notReadyPods":        1,
					"apiResponsive":       true,
					"responsivenessScore": 85,
				},
				"recentSlowPods": []interface{}{},
			}),
		},
	})

	// --- Secret Encryption at Rest Configuration Checker (v15.74+) ---
	add("/api/security/encryption-at-rest", "get", OpenAPIOperation{
		Summary: "Secret encryption at rest configuration checker", OperationID: "encryptionAtRest", Tags: []string{"Security", "Encryption", "Compliance"},
		Description: "Verifies if Kubernetes Secrets are encrypted at rest in etcd. Checks kube-apiserver for --encryption-provider-config flag. Detects k3s environments. Without encryption, anyone with etcd access can read all passwords, tokens, and certificates in plaintext. Findings categorized as configuration/provider/coverage/access. Security score (0-100). Required for PCI-DSS, SOC2, HIPAA compliance.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Encryption at rest report", map[string]interface{}{
				"summary": map[string]interface{}{
					"encryptionEnabled": true,
					"encryptionType":    "aescbc",
					"providerCount":     1,
					"securityScore":     100,
				},
				"findings": []interface{}{},
			}),
		},
	})

	// --- Cluster Scale Limits & Threshold Monitor (v15.75+) ---
	add("/api/scalability/scale-limits", "get", OpenAPIOperation{
		Summary: "Cluster scalability limits & threshold monitor", OperationID: "scaleLimits", Tags: []string{"Scalability", "Limits", "Capacity"},
		Description: "Checks cluster proximity to official Kubernetes scalability limits. Per-limit: nodes (5000), pods (150000), pods-per-node, services (5000), namespaces (10000), ConfigMaps, Secrets, pod capacity utilization. Status: safe (<60%), warning (60-80%), critical (>=80%). Scale score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scale limits report", map[string]interface{}{
				"summary": map[string]interface{}{
					"nodeCount":      5,
					"podCount":       120,
					"totalCapacity":  550,
					"utilizationPct": 21,
					"scaleScore":     100,
				},
				"limits": []interface{}{},
			}),
		},
	})

	// --- HPA Health & Scaling Activity Analyzer (v15.77+) ---
	add("/api/product/hpa-health", "get", OpenAPIOperation{
		Summary: "HPA health & scaling activity analyzer", OperationID: "hpaHealth", Tags: []string{"Product", "HPA", "Autoscaling"},
		Description: "Analyzes HorizontalPodAutoscaler health and scaling activity. Per-HPA: target ref, min/max/current/desired replicas, scaling active status, metrics count, conditions. Detection: at maxReplicas (warning, may be under-provisioned), no metrics (warning, cannot auto-scale), scaling inactive (info, check metrics server). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("HPA health report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalHPAs":     5,
					"atMaxReplicas": 1,
					"scalingActive": 4,
					"healthScore":   85,
				},
				"byHPA": []interface{}{},
			}),
		},
	})

	// --- Workload Maturity & Best Practices Scorer (v15.79+) ---
	add("/api/deployment/workload-maturity", "get", OpenAPIOperation{
		Summary: "Workload maturity & best practices scorer", OperationID: "workloadMaturity", Tags: []string{"Deployment", "BestPractices", "Maturity"},
		Description: "Scores each Deployment against K8s best practices checklist (8 checks, weights sum to 100): resource requests (15), probes (15), multi-replica (15), PDB (10), anti-affinity (15), security context (10), revision history (10), labels (10). Per-workload: maturity score 0-100, risk level. Cluster avg maturity score.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Workload maturity report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalWorkloads":   15,
					"hasResources":     12,
					"hasProbes":        10,
					"avgMaturityScore": 72.5,
				},
				"byWorkload": []interface{}{},
			}),
		},
	})

	// --- Volume Mount & Attach Error Tracker (v15.81+) ---
	add("/api/operations/volume-mount-errors", "get", OpenAPIOperation{
		Summary: "Volume mount & attach error tracker", OperationID: "volumeMountErrors", Tags: []string{"Operations", "Storage", "Volumes"},
		Description: "Tracks pods stuck in Pending/ContainerCreating due to volume mount/attach failures. Per-pod: error type (mount_fail, attach_fail, provisioning, timeout), error message, pending duration, risk level. By-error-type: mount failures, attach/detach failures, provisioning errors, timeouts. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Volume mount error report", map[string]interface{}{
				"summary": map[string]interface{}{
					"stuckPods":          2,
					"mountFailErrors":    1,
					"attachFailErrors":   1,
					"provisioningErrors": 0,
					"healthScore":        85,
				},
				"errorPods": []interface{}{},
			}),
		},
	})

	// --- Container Host Namespace & Privilege Exposure Auditor (v15.83+) ---
	add("/api/security/host-namespace", "get", OpenAPIOperation{
		Summary: "Container host namespace & privilege exposure auditor", OperationID: "hostNamespace", Tags: []string{"Security", "Namespace", "Privilege"},
		Description: "Audits containers for host namespace exposure and privilege escalation. Per-pod: hostNetwork, hostPID, hostIPC, privileged containers, hostPath mounts, added capabilities, runAsRoot. Risk levels: critical (privileged+hostNS), high (privileged or hostNS), medium (minor exposures). Exposure safety score (0-100, higher=safer).",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter by namespace (empty = all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Host namespace exposure report", map[string]interface{}{
				"summary": map[string]interface{}{
					"hostNetworkPods":      2,
					"privilegedContainers": 1,
					"hostPathMounts":       3,
					"exposureScore":        78,
				},
				"exposedPods": []interface{}{},
			}),
		},
	})

	// --- Deprecated API Version & Upgrade Readiness Checker (v15.84+) ---
	add("/api/product/api-deprecation", "get", OpenAPIOperation{
		Summary: "Deprecated API version & upgrade readiness checker", OperationID: "apiDeprecation", Tags: []string{"Product", "Upgrade", "API"},
		Description: "Checks for deprecated/removed Kubernetes API versions via API discovery. Detects: extensions/v1beta1, apps/v1beta1/v1beta2, networking.k8s.io/v1beta1, batch/v1beta1, autoscaling/v2beta1/v2beta2, policy/v1beta1 (PSP). Per-API: resource, old/new version, removedIn version, status. Upgrade readiness score (0-100). Removed APIs block cluster upgrades.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("API deprecation report", map[string]interface{}{
				"summary": map[string]interface{}{
					"deprecatedCount": 0,
					"removedCount":    0,
					"readyForUpgrade": true,
					"readinessScore":  100,
				},
				"clusterVersion": "v1.28.3",
			}),
		},
	})

	// --- Disaster Recovery Readiness & Backup Compliance Auditor (v15.86+) ---
	add("/api/scalability/dr-readiness", "get", OpenAPIOperation{
		Summary: "Disaster recovery readiness & backup compliance auditor", OperationID: "drReadiness", Tags: []string{"Scalability", "DR", "Backup"},
		Description: "Audits cluster disaster recovery readiness. Checks: Velero/backup controller presence, namespace backup label coverage, CSI snapshot controller, multi-AZ topology, PVC data protection. Per-namespace: protected vs unprotected. Findings categorized as backup/snapshot/topology/recovery. DR readiness score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("DR readiness report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces": 8,
					"protectedNS":     5,
					"hasVelero":       true,
					"multiAZ":         true,
					"readinessScore":  85,
				},
				"protectedNamespaces":   []interface{}{},
				"unprotectedNamespaces": []interface{}{},
			}),
		},
	})

	// --- Container Ephemeral Storage & emptyDir Limit Compliance (v15.88+) ---
	add("/api/deployment/ephemeral-storage", "get", OpenAPIOperation{
		Summary: "Ephemeral storage & emptyDir limit compliance checker", OperationID: "ephemeralStorage", Tags: []string{"Deployment", "Storage", "Compliance"},
		Description: "Checks container ephemeral storage limit compliance. Per-pod: ephemeral-storage limit presence, emptyDir volume count and size limits, unbounded emptyDir detection. Without limits, pods can fill node disk and trigger DiskPressure evictions. Compliance score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Ephemeral storage compliance report", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":         50,
					"hasEphemeralLimit": 40,
					"noEphemeralLimit":  10,
					"unboundedTmpfs":    3,
					"complianceScore":   85,
				},
				"byWorkload": []interface{}{},
			}),
		},
	})

	// --- Pod Startup Lifecycle & Bottleneck Analyzer (v15.89+) ---
	add("/api/operations/pod-startup", "get", OpenAPIOperation{
		Summary: "Pod startup lifecycle & bottleneck analyzer", OperationID: "podStartup", Tags: []string{"Operations", "Performance", "PodLifecycle"},
		Description: "Analyzes the full pod startup lifecycle from creation to ready. Breaks down startup time into phases: scheduling delay, init container duration, image pull & container creation, and readiness probe delay. Identifies slow-starting pods (>120s), pods stuck in Pending/ContainerCreating, and categorizes bottlenecks (scheduling, image_pull, init_container, probe, volume). Provides per-workload-type statistics and a cluster startup health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod startup lifecycle analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":         120,
					"runningPods":       115,
					"pendingPods":       3,
					"avgStartupSeconds": 28.5,
					"maxStartupSeconds": 180.0,
					"slowStartupCount":  2,
					"stuckCount":        3,
					"healthScore":       82,
				},
				"slowPods":    []interface{}{},
				"stuckPods":   []interface{}{},
				"bottlenecks": []interface{}{},
				"byWorkload":  []interface{}{},
			}),
		},
	})

	// --- Pod Security Admission Enforcement Auditor (v15.91+) ---
	add("/api/security/psa-audit", "get", OpenAPIOperation{
		Summary: "Pod Security Admission (PSA) enforcement auditor", OperationID: "psaAudit", Tags: []string{"Security", "Compliance", "PodSecurity"},
		Description: "Audits namespace-level Pod Security Admission (PSA) enforcement configuration. Checks pod-security.kubernetes.io/enforce, audit, and warn labels. Per-namespace: enforcement level (privileged/baseline/restricted/none), audit mode, warn mode, version pinning. Detects pods violating their namespace PSA policy (privileged containers, host namespaces, dangerous capabilities, root user, missing seccomp). Enforcement score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PSA enforcement analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNamespaces":    15,
					"userNamespaces":     10,
					"enforced":           7,
					"notEnforced":        8,
					"restrictedEnforced": 5,
					"baselineEnforced":   2,
					"violationCount":     3,
					"enforcementScore":   72,
				},
				"namespaces": []interface{}{},
				"violations": []interface{}{},
			}),
		},
	})

	// --- Pod QoS & Priority Class Distribution Auditor (v15.92+) ---
	add("/api/product/qos-priority", "get", OpenAPIOperation{
		Summary: "Pod QoS & Priority Class distribution auditor", OperationID: "qosPriority", Tags: []string{"Product", "Scheduling", "ResourceManagement"},
		Description: "Analyzes Pod Quality of Service (QoS) class distribution (Guaranteed/Burstable/BestEffort) and PriorityClass usage across the cluster. Per-namespace and per-workload-type QoS breakdown. Detects misconfigurations: BestEffort in user namespaces, single-replica Deployments without PriorityClass, Guaranteed QoS with low priority, pods with no resource requests. Identifies pods at high eviction risk (BestEffort + low priority). Lists all PriorityClasses with pod counts. QoS health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("QoS distribution analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":         120,
					"guaranteedPods":    30,
					"burstablePods":     70,
					"bestEffortPods":    20,
					"withPriorityClass": 80,
					"qosScore":          72,
				},
				"byNamespace":  []interface{}{},
				"byWorkload":   []interface{}{},
				"misconfigs":   []interface{}{},
				"evictionRisk": []interface{}{},
			}),
		},
	})

	// --- Resource Fragmentation & Bin-Packing Efficiency Analyzer (v15.93+) ---
	add("/api/scalability/fragmentation", "get", OpenAPIOperation{
		Summary: "Resource fragmentation & bin-packing efficiency analyzer", OperationID: "fragmentation", Tags: []string{"Scalability", "Capacity", "Scheduling"},
		Description: "Analyzes resource fragmentation and bin-packing efficiency across nodes. Per-node: allocatable vs requested vs available CPU/memory/pod slots, efficiency ratios, fragmentation score. Identifies fragmented nodes (resources available but unusable due to pod limit or resource imbalance). Counts stranded resources (CPU/memory that can't be scheduled). Simulates whether pods of common sizes (small/medium/large/xlarge) can be scheduled. Bin-packing score (0-100) and fragmentation score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Fragmentation analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":         5,
					"schedulableNodes":   5,
					"avgCpuEfficiency":   65.5,
					"avgMemEfficiency":   58.2,
					"fragmentedNodes":    2,
					"strandedCPUMilli":   2000,
					"binPackingScore":    62,
					"fragmentationScore": 71,
				},
				"byNode":          []interface{}{},
				"fragmentedNodes": []interface{}{},
				"podSimulations":  []interface{}{},
			}),
		},
	})

	// --- ConfigMap/Secret Config Sync & Staleness Detector (v15.95+) ---
	add("/api/deployment/config-sync", "get", OpenAPIOperation{
		Summary: "ConfigMap/Secret config sync & staleness detector", OperationID: "configSync", Tags: []string{"Deployment", "Configuration", "Reliability"},
		Description: "Detects pods running with stale configuration after ConfigMap/Secret updates. Identifies env var refs (env/envFrom) that do NOT auto-update on config changes, subPath volume mounts that don't auto-update, and workloads missing Reloader annotations. Cross-references pod start time with ConfigMap/Secret creation timestamps to find stale consumers. Staleness score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Config sync analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":         120,
					"podsWithConfigRef": 80,
					"envVarRefs":        35,
					"volumeRefs":        55,
					"stalePodCount":     8,
					"stalenessScore":    72,
				},
				"stalePods":     []interface{}{},
				"subPathMounts": []interface{}{},
				"noReloader":    []interface{}{},
				"byConfigMap":   []interface{}{},
			}),
		},
	})

	// --- Kubelet & Container Runtime Health Monitor (v15.96+) ---
	add("/api/operations/kubelet-health", "get", OpenAPIOperation{
		Summary: "Kubelet & container runtime health monitor", OperationID: "kubeletHealth", Tags: []string{"Operations", "NodeHealth", "Runtime"},
		Description: "Monitors kubelet and container runtime health across all nodes. Per-node: kubelet version, container runtime version, OS image, kernel, architecture, last heartbeat time and age, active conditions (NotReady, DiskPressure, MemoryPressure, PIDPressure, NetworkUnavailable). Detects: version skew (different kubelet versions across nodes), runtime skew, stale heartbeats (>60s), nodes with active conditions. Runtime and OS distribution tracking. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Kubelet health analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":        5,
					"healthyNodes":      4,
					"unhealthyNodes":    1,
					"versionSkew":       1,
					"oldHeartbeatNodes": 0,
					"healthScore":       82,
				},
				"byNode":          []interface{}{},
				"unhealthyNodes":  []interface{}{},
				"runtimeVersions": map[string]int{},
				"issues":          []interface{}{},
			}),
		},
	})

	// --- AppArmor & SELinux MAC Compliance Auditor (v15.98+) ---
	add("/api/security/mac-audit", "get", OpenAPIOperation{
		Summary: "AppArmor & SELinux MAC compliance auditor", OperationID: "macAudit", Tags: []string{"Security", "Compliance", "MAC"},
		Description: "Audits AppArmor and SELinux mandatory access control configuration across pods. Detects pods with unconfined AppArmor, permissive SELinux types, and missing MAC profiles in user namespaces. Checks node AppArmor/SELinux capability. Per-namespace compliance rates. Compliance score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("MAC compliance analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":          120,
					"withAppArmor":       80,
					"withSELinux":        40,
					"unconfinedAppArmor": 2,
					"complianceScore":    65,
				},
				"byNamespace":      []interface{}{},
				"nonCompliantPods": []interface{}{},
			}),
		},
	})

	// --- Service Endpoint & Connectivity Health Auditor (v15.99+) ---
	add("/api/product/service-connectivity", "get", OpenAPIOperation{
		Summary: "Service endpoint & connectivity health auditor", OperationID: "serviceConnectivity", Tags: []string{"Product", "Networking", "ServiceHealth"},
		Description: "Audits Service endpoint health and connectivity. Per-service: endpoint count, ready endpoints, selector matching. Detects zero-endpoint services, services with no ready endpoints, and selector gaps (selectors matching no pods). Service type distribution (ClusterIP/NodePort/LoadBalancer/Headless/ExternalName). Per-namespace health. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Service connectivity analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalServices":     50,
					"healthyServices":   45,
					"zeroEndpoints":     3,
					"notReadyEndpoints": 2,
					"healthScore":       85,
				},
				"unhealthyServices": []interface{}{},
				"byType":            []interface{}{},
			}),
		},
	})

	// --- IP Address & Pod CIDR Utilization Monitor (v16.01+) ---
	add("/api/scalability/ip-cidr-utilization", "get", OpenAPIOperation{
		Summary: "IP address & Pod CIDR utilization monitor", OperationID: "ipCidrUtilization", Tags: []string{"Scalability", "Networking", "Capacity"},
		Description: "Monitors Pod CIDR utilization and IP address capacity across nodes. Per-node: Pod CIDR range, address capacity, pods on node, utilization percentage, remaining IPs, dual-stack detection. Identifies nodes at/near IP exhaustion. Estimates cluster-wide Pod IP utilization. Service IP range detection. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("IP CIDR utilization analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalNodes":            5,
					"totalPodIPsUsed":       120,
					"totalPodCIDRCap":       1280,
					"overallUtilizationPct": 9.4,
					"healthScore":           95,
				},
				"byNode": []interface{}{},
			}),
		},
	})

	// --- Sidecar Container Overhead & Injection Auditor (v16.02+) ---
	add("/api/deployment/sidecar-audit", "get", OpenAPIOperation{
		Summary: "Sidecar container overhead & injection auditor", OperationID: "sidecarAudit", Tags: []string{"Deployment", "Resources", "Efficiency"},
		Description: "Analyzes sidecar containers across pods. Detects known sidecar patterns (Istio, Linkerd, Vault, Fluentd, etc.), calculates CPU/memory overhead per pod and namespace. Identifies high-overhead pods (>30% sidecar resources), injected-only pods (no app container). Per-type and per-namespace statistics. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Sidecar analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":        120,
					"podsWithSidecars": 60,
					"totalSidecars":    75,
					"cpuOverheadPct":   18.5,
					"healthScore":      78,
				},
				"bySidecarType": []interface{}{},
				"byNamespace":   []interface{}{},
			}),
		},
	})

	// --- DNS Resolution Health & CoreDNS Monitor (v16.03+) ---
	add("/api/operations/dns-health", "get", OpenAPIOperation{
		Summary: "DNS resolution health & CoreDNS monitor", OperationID: "dnsHealth", Tags: []string{"Operations", "DNS", "Networking"},
		Description: "Monitors DNS resolution health and CoreDNS performance. Checks CoreDNS pod readiness, version, ConfigMap (Corefile) for missing plugins (cache, health, ready, prometheus). Detects pods with incorrect DNS policy (Default instead of ClusterFirst). Per-namespace DNS policy statistics. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("DNS health analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"coreDNSFound":   2,
					"coreDNSReady":   2,
					"configMapFound": true,
					"podsMissingDNS": 3,
					"healthScore":    85,
				},
				"coreDNSPods": []interface{}{},
				"issues":      []interface{}{},
			}),
		},
	})

	// --- Pod Security Forensics & Incident Evidence Collector (v16.05+) ---
	add("/api/security/forensics", "get", OpenAPIOperation{
		Summary: "Pod security forensics & incident evidence collector", OperationID: "forensics", Tags: []string{"Security", "Forensics", "IncidentResponse"},
		Description: "Collects pod security forensics and incident evidence. Analyzes container exit codes, OOMKills, SIGKILL terminations, privileged container escapes, and container/image hash mismatches. Recent termination records with reasons, signals, and durations. Exit code distribution analysis. Per-pod suspicious activity flagging. Forensics health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Forensics analysis", map[string]interface{}{
				"summary": map[string]interface{}{
					"totalPods":           120,
					"podsWithIssues":      5,
					"oomKills":            2,
					"sigkillTerminations": 1,
					"exitCodeErrors":      4,
					"hashMismatches":      0,
					"forensicsScore":      82,
				},
				"suspiciousPods":     []interface{}{},
				"exitCodeAnalysis":   []interface{}{},
				"recentTerminations": []interface{}{},
			}),
		},
	})

	// --- Pod Topology Spread Constraint Validator (v16.06+) ---
	add("/api/product/topology-spread", "get", OpenAPIOperation{
		Summary: "Pod topology spread constraint validator", OperationID: "topologySpread", Tags: []string{"Product", "Scheduling", "HA"},
		Description: "Validates topology spread constraints across Deployments, StatefulSets, and DaemonSets. Detects multi-replica workloads without spread constraints. Validates maxSkew, topologyKey, and whenUnsatisfiable settings. Analyzes actual pod distribution across zone and hostname domains. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Topology spread analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalWorkloads": 15, "withSpread": 8, "withoutSpread": 7, "healthScore": 65},
			}),
		},
	})

	// --- Container Restart Policy & Lifecycle Auditor (v16.08+) ---
	add("/api/deployment/restart-policy", "get", OpenAPIOperation{
		Summary: "Restart policy & lifecycle hook auditor", OperationID: "restartPolicy", Tags: []string{"Deployment", "Lifecycle", "Reliability"},
		Description: "Audits container restart policies and lifecycle hooks. Detects policy mismatches (e.g. Job with Always, Deployment with Never). Tracks postStart/preStop hook coverage. Per-namespace statistics. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Restart policy analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalWorkloads": 50, "policyMismatches": 2, "noLifecycleHook": 15, "healthScore": 80},
			}),
		},
	})

	// --- Certificate Signing Request (CSR) Monitor (v16.09+) ---
	add("/api/operations/csr-monitor", "get", OpenAPIOperation{
		Summary:     "Certificate signing request & node bootstrap cert monitor",
		OperationID: "csrMonitor",
		Tags:        []string{"Operations", "Certificates", "NodeBootstrap"},
		Description: "Monitors Certificate Signing Requests (CSRs). Tracks pending, approved, denied, expired, and stale CSRs. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CSR analysis", map[string]interface{}{
				"summary": map[string]interface{}{"total": 15, "pending": 2, "healthScore": 75},
			}),
		},
	})

	// --- Node Topology Distribution & Multi-AZ Analyzer (v16.11+) ---
	add("/api/scalability/node-topology", "get", OpenAPIOperation{
		Summary:     "Node topology distribution & multi-AZ fault tolerance analyzer",
		OperationID: "nodeTopology",
		Tags:        []string{"Scalability", "HA", "Topology"},
		Description: "Analyzes node distribution across availability zones and regions. Per-zone: node count, CPU/memory allocation, pod count. Detects single-zone clusters, zone imbalance, and missing zone labels. Multi-AZ fault tolerance assessment. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node topology analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNodes": 5, "totalZones": 2, "healthScore": 75},
			}),
		},
	})

	// --- RBAC Overprivilege & Wildcard Permission Auditor (v16.12+) ---
	add("/api/security/rbac-audit", "get", OpenAPIOperation{
		Summary:     "RBAC overprivilege & wildcard permission auditor",
		OperationID: "rbacAudit",
		Tags:        []string{"Security", "RBAC", "Compliance"},
		Description: "Audits RBAC roles for overprivilege. Detects wildcard verbs/resources, excessive cluster-admin bindings, and least-privilege violations. Per-role severity classification. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("RBAC audit analysis", map[string]interface{}{
				"summary": map[string]interface{}{"overprivilegedCount": 3, "clusterAdminBindings": 1, "healthScore": 65},
			}),
		},
	})

	// --- Volume Snapshot & PVC Backup Compliance Auditor (v16.13+) ---
	add("/api/product/backup-compliance", "get", OpenAPIOperation{
		Summary:     "Volume snapshot & PVC backup compliance auditor",
		OperationID: "backupCompliance",
		Tags:        []string{"Product", "Backup", "DisasterRecovery"},
		Description: "Audits PVC backup and snapshot compliance. Detects unprotected PVCs in use, critical large PVCs without backup, Velero installation status. Per-namespace and per-storage-class compliance. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Backup compliance analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPVCs": 20, "unprotectedPVCs": 5, "healthScore": 70},
			}),
		},
	})

	// --- Deployment Scale Readiness & Autoscaling Gap Detector (v16.15+) ---
	add("/api/deployment/scale-readiness", "get", OpenAPIOperation{
		Summary:     "Deployment scale readiness & autoscaling gap detector",
		OperationID: "scaleReadiness",
		Tags:        []string{"Deployment", "Autoscaling", "HA"},
		Description: "Analyzes deployment and StatefulSet scale readiness. Detects: missing HPA, missing PDB, missing resource requests, single-replica workloads. Identifies workloads fully ready to scale (HPA + PDB + resources). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scale readiness analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalWorkloads": 20, "canScale": 12, "healthScore": 70},
			}),
		},
	})

	// --- etcd Health & Database Pressure Monitor (v16.16+) ---
	add("/api/operations/etcd-health", "get", OpenAPIOperation{
		Summary:     "etcd health & database pressure monitor",
		OperationID: "etcdHealth",
		Tags:        []string{"Operations", "etcd", "Database"},
		Description: "Monitors etcd pod health and database pressure. Tracks etcd pod readiness, version, restarts. Detects large ConfigMaps/Secrets (>100KB) that pressure etcd. Identifies single etcd instances (no HA quorum). etcd pressure and health scores (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("etcd health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"etcdFound": 3, "etcdReady": 3, "largeObjects": 2, "healthScore": 85},
			}),
		},
	})

	// --- Secret Data Exposure & Credential Leak Scanner (v16.18+) ---
	add("/api/security/secret-scan", "get", OpenAPIOperation{
		Summary:     "Secret data exposure & credential leak scanner",
		OperationID: "secretScan",
		Tags:        []string{"Security", "Secrets", "DataProtection"},
		Description: "Scans for secret data exposure and environment variable credential leaks. Detects inline credential values in env vars, sensitive secrets exposed as env vars, stale secrets (>90 days), and unreferenced secrets. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Secret exposure analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalSecrets": 30, "exposedPlainSecrets": 5, "healthScore": 75},
			}),
		},
	})

	// --- Init Container Reliability & Startup Dependency Auditor (v16.19+) ---
	add("/api/product/init-container-audit", "get", OpenAPIOperation{
		Summary:     "Init container reliability & startup dependency auditor",
		OperationID: "initContainerAudit",
		Tags:        []string{"Product", "InitContainers", "Reliability"},
		Description: "Audits init container reliability and startup dependencies. Detects missing resource requests/limits, excessive init containers (>5), RestartPolicy=Always sidecar behavior. Per-namespace and per-workload analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Init container audit analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPods": 50, "podsWithInit": 15, "totalInitContainers": 20, "healthScore": 80},
			}),
		},
	})

	// --- Deployment Replica Availability & Ready Pod Ratio Monitor (v16.21) ---
	add("/api/deployment/replica-availability", "get", OpenAPIOperation{
		Summary:     "Deployment replica availability & ready pod ratio monitor",
		OperationID: "replicaAvailability",
		Tags:        []string{"Deployment", "Availability", "Replicas"},
		Description: "Monitors replica availability across Deployments, StatefulSets, and DaemonSets. Detects ready/desired gaps, zero-ready workloads, stale replicas during rollouts. Per-namespace analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Replica availability analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalWorkloads": 30, "healthyWorkloads": 25, "gapWorkloads": 5, "healthScore": 85},
			}),
		},
	})

	// --- Multi-Tenant Resource Pressure & Quota Competition Auditor (v16.22) ---
	add("/api/scalability/tenant-pressure", "get", OpenAPIOperation{
		Summary:     "Multi-tenant resource pressure & quota competition auditor",
		OperationID: "tenantPressure",
		Tags:        []string{"Scalability", "MultiTenancy", "Quota"},
		Description: "Audits multi-tenant resource pressure and quota competition. Detects saturated quotas (>80%), critical quotas (>95%), unbounded namespaces (no quota + no LimitRange), resource hotspots. Per-namespace analysis with health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Tenant pressure analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNamespaces": 20, "saturatedQuotas": 3, "healthScore": 82},
			}),
		},
	})

	// --- API Server Request Throughput & Load Pressure Monitor (v16.23) ---
	add("/api/operations/api-load", "get", OpenAPIOperation{
		Summary:     "API server request throughput & load pressure monitor",
		OperationID: "apiLoad",
		Tags:        []string{"Operations", "APIServer", "Performance"},
		Description: "Monitors API server load by analyzing pod density, controller count, event volume, and warning ratio per namespace. Identifies dense namespaces, high-activity namespaces, and empty namespaces wasting watch resources. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("API load analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNamespaces": 20, "totalPods": 300, "healthScore": 85},
			}),
		},
	})

	// --- Security Context Drift & Runtime Policy Compliance Auditor (v16.25) ---
	add("/api/security/sec-drift", "get", OpenAPIOperation{
		Summary:     "Security context drift & runtime policy compliance auditor",
		OperationID: "secDrift",
		Tags:        []string{"Security", "Compliance", "Runtime"},
		Description: "Audits security context drift and runtime policy compliance. Detects missing runAsNonRoot, readOnlyRootFilesystem, allowPrivilegeEscalation, capability drops, privileged containers, dangerous capabilities (SYS_ADMIN, NET_ADMIN, etc.). Per-namespace and per-workload analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Security context drift analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPods": 50, "privileged": 2, "healthScore": 78},
			}),
		},
	})

	// --- HPA Target Utilization Gap & Scaling Behavior Auditor (v16.26) ---
	add("/api/product/hpa-gap", "get", OpenAPIOperation{
		Summary:     "HPA target utilization gap & scaling behavior auditor",
		OperationID: "hpaGap",
		Tags:        []string{"Product", "HPA", "Autoscaling"},
		Description: "Audits HPA target utilization gaps, scaling behavior, and cooldown configuration. Detects targets too high (>90%), too low (<30%), missing metrics, minReplicas==maxReplicas, missing scaleDown behavior, and high utilization gaps. Per-namespace analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("HPA gap analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalHPAs": 10, "highGapHPAs": 2, "healthScore": 82},
			}),
		},
	})

	// --- Node Pool & Cluster Autoscaler Health Monitor (v16.27) ---
	add("/api/scalability/node-pool-health", "get", OpenAPIOperation{
		Summary:     "Node pool & cluster autoscaler health monitor",
		OperationID: "nodePoolHealth",
		Tags:        []string{"Scalability", "NodePool", "Autoscaler"},
		Description: "Monitors node pool health, node readiness distribution, stale heartbeats, cordoned nodes, and cluster autoscaler presence. Detects unbalanced pools (>30% NotReady) and stale nodes. Per-pool and per-zone analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node pool health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNodes": 10, "readyNodes": 8, "healthScore": 85},
			}),
		},
	})

	// --- Helm Release Health & GitOps Drift Detector (v16.28) ---
	add("/api/deployment/helm-health-v2", "get", OpenAPIOperation{
		Summary:     "Helm release health & GitOps drift detector",
		OperationID: "helmHealth",
		Tags:        []string{"Deployment", "GitOps", "Helm"},
		Description: "Audits Helm release health by scanning Helm release secrets. Detects failed releases, pending/stuck installs, stale releases. Identifies releases in unusual states. Per-namespace analysis. Health score (0-100). Blind spot: GitOps/CD coverage.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Helm release health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalReleases": 10, "healthyReleases": 8, "healthScore": 85},
			}),
		},
	})

	// --- Prometheus Rule Health & Alert Coverage Auditor (v16.29) ---
	add("/api/operations/prom-health", "get", OpenAPIOperation{
		Summary:     "Prometheus rule health & alert coverage auditor",
		OperationID: "promHealth",
		Tags:        []string{"Operations", "Observability", "Prometheus"},
		Description: "Audits observability stack: detects Prometheus, Alertmanager, Grafana, metrics-server, kube-state-metrics. Scans PrometheusRule ConfigMaps for alert/recording rules. Identifies namespaces with no alerting coverage. Health score (0-100). Blind spot: Observability Stack.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Observability stack health", map[string]interface{}{
				"summary": map[string]interface{}{"hasPrometheus": true, "totalRules": 50, "healthScore": 85},
			}),
		},
	})

	// --- OPA/Gatekeeper Policy Compliance & Constraint Violation Auditor (v16.30) ---
	add("/api/security/opa-compliance", "get", OpenAPIOperation{
		Summary:     "OPA/Gatekeeper policy compliance & constraint violation auditor",
		OperationID: "opaCompliance",
		Tags:        []string{"Security", "Compliance", "OPA", "Gatekeeper"},
		Description: "Audits OPA Gatekeeper and Kyverno policy engine compliance. Detects Gatekeeper/Kyverno installation, scans Constraint CRDs for enforce/audit mode, counts violations per constraint and namespace. Blind spot: Compliance/Governance coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("OPA compliance analysis", map[string]interface{}{
				"summary": map[string]interface{}{"hasGatekeeper": true, "totalConstraints": 10, "violationCount": 3, "healthScore": 85},
			}),
		},
	})

	// --- Service Mesh Sidecar Health & mTLS Coverage Auditor (v16.31) ---
	add("/api/product/mesh-health", "get", OpenAPIOperation{
		Summary:     "Service mesh sidecar health & mTLS coverage auditor",
		OperationID: "meshHealth",
		Tags:        []string{"Product", "ServiceMesh", "mTLS"},
		Description: "Audits service mesh (Istio/Linkerd/Consul) sidecar health and mTLS coverage. Detects mesh control plane, sidecar injection rate, mTLS status per pod, sidecar restart patterns. Blind spot: Network/Service Mesh coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Mesh health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"hasIstio": true, "podsWithSidecar": 50, "mtlsEnabled": 45, "healthScore": 88},
			}),
		},
	})

	// --- CronJob Schedule Conflict & Resource Configuration Auditor (v16.38) ---
	add("/api/product/cronjob-schedule", "get", OpenAPIOperation{
		Summary:     "CronJob schedule conflict & resource configuration auditor",
		OperationID: "cronJobSchedule",
		Tags:        []string{"Product", "CronJob", "BatchWorkloads"},
		Description: "Audits CronJob schedule conflicts and resource configuration. Detects schedule clustering (3+ jobs at same time slot), suspended cron jobs, missing concurrency limits, missing resource limits, job history configuration, and timezone usage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CronJob health analysis", map[string]interface{}{
				"summary":        map[string]interface{}{"totalCronJobs": 10, "suspendedJobs": 1, "failedJobs": 2, "healthScore": 75},
				"scheduleIssues": []map[string]interface{}{{"timeSlot": "2:0", "conflictCount": 4}},
			}),
		},
	})

	// --- External Secrets & Secret Store CSI Health Auditor (v16.44) ---
	add("/api/product/external-secret-health", "get", OpenAPIOperation{
		Summary:     "External secrets & secret store CSI health auditor",
		OperationID: "externalSecretHealth",
		Tags:        []string{"Product", "Secrets", "Security"},
		Description: "Audits External Secrets Operator and Secret Store CSI Driver health. Detects ESO/CSI installation via pod image scan, lists ExternalSecret CRDs with sync status, SecretProviderClass CRDs, pod health (ready/restarts). Identifies failed syncs, unknown status secrets, and missing configurations. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("External secret health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"esoDetected": true, "totalSecrets": 15, "syncedSecrets": 13, "failedSecrets": 2, "healthScore": 82},
			}),
		},
	})

	// --- Idle Resource Cost Waste & Namespace Cost Attribution Auditor (v16.32) ---
	add("/api/scalability/cost-waste", "get", OpenAPIOperation{
		Summary:     "Idle resource cost waste & namespace cost attribution auditor",
		OperationID: "costWaste",
		Tags:        []string{"Scalability", "Cost", "FinOps"},
		Description: "Audits idle resource cost waste and namespace cost attribution. Detects idle pods (very low resource requests), over-provisioned pods (>4 CPU or >8Gi memory), idle namespaces. Calculates waste percentage and per-namespace cost distribution. Blind spot: Cost/FinOps coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cost waste analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPods": 100, "idlePods": 15, "wastePercent": 15.0, "healthScore": 82},
			}),
		},
	})

	// --- Node OS Patch & Kernel Version Drift Auditor (v16.34) ---
	add("/api/scalability/node-lifecycle", "get", OpenAPIOperation{
		Summary:     "Node OS patch, kernel drift, GPU & node rotation auditor",
		OperationID: "nodeLifecycle",
		Tags:        []string{"Scalability", "NodeLifecycle", "Infrastructure"},
		Description: "Audits node lifecycle: kernel version drift, OS image drift, architecture diversity, GPU resource availability, and node age/rotation needs. Identifies nodes older than 90/180 days. Blind spot: Node Lifecycle coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node lifecycle analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNodes": 10, "kernelVersions": 2, "gpuNodes": 1, "healthScore": 85},
			}),
		},
	})

	// --- Resource Request vs Limit Allocation Efficiency Auditor (v16.42) ---
	add("/api/scalability/alloc-efficiency", "get", OpenAPIOperation{
		Summary:     "Resource request vs limit allocation efficiency auditor",
		OperationID: "allocEfficiency",
		Tags:        []string{"Scalability", "Resources", "FinOps"},
		Description: "Audits resource request vs limit allocation efficiency across all containers. Detects overallocated containers (request ≈ limit, wasted scheduling), underallocated containers (request << limit, throttling risk), missing requests/limits, and computes overall CPU allocation efficiency ratio. Per-namespace and per-workload breakdown. Health score (0-100). Blind spot: Cost/FinOps deepening.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Allocation efficiency analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalContainers": 80, "noRequests": 5, "noLimits": 10, "allocEfficiency": 0.65, "healthScore": 78},
			}),
		},
	})

	// --- Rolling Update Risk & Surge Configuration Analyzer (v16.35) ---
	add("/api/deployment/surge-risk", "get", OpenAPIOperation{
		Summary:     "Rolling update risk & surge configuration analyzer",
		OperationID: "surgeRisk",
		Tags:        []string{"Deployment", "RollingUpdate", "Risk"},
		Description: "Analyzes rolling update strategy configuration risks. Detects maxUnavailable=100% (downtime), Recreate strategy (guaranteed downtime), maxSurge too high (>50%), and default surge configs. Per-workload risk analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Surge risk analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalWorkloads": 20, "highRisk": 3, "healthScore": 82},
			}),
		},
	})

	// --- Progressive Delivery & Canary Rollout Health Auditor (v16.45) ---
	add("/api/deployment/progressive-delivery", "get", OpenAPIOperation{
		Summary:     "Progressive delivery & canary rollout health auditor",
		OperationID: "progressiveDelivery",
		Tags:        []string{"Deployment", "Rollout", "Canary"},
		Description: "Audits progressive delivery posture: detects Argo Rollouts/Flagger installation, identifies Recreate vs RollingUpdate strategies, stalled rollouts, missing progressDeadlineSeconds, high-replica deployments without canary, and ProgressDeadlineExceeded conditions. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Progressive delivery analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalDeployments": 25, "stalledRollouts": 2, "recreateStrategy": 1, "healthScore": 82},
			}),
		},
	})

	// --- Pod Startup Latency & Readiness Performance Auditor (v16.39) ---
	add("/api/deployment/startup-latency", "get", OpenAPIOperation{
		Summary:     "Pod startup latency & readiness performance auditor",
		OperationID: "startupLatency",
		Tags:        []string{"Deployment", "Startup", "Performance"},
		Description: "Audits pod startup latency and readiness probe performance. Measures time from pod creation to ready state, computes p50/p90/p99 percentiles, identifies slow-starting pods (>60s), detects missing readiness/liveness probes, tracks CrashLoopBackOff pods, and correlates init container impact. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Startup latency analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPods": 50, "avgStartupMs": 15000, "p99StartupMs": 90000, "slowPods": 5, "healthScore": 78},
			}),
		},
	})

	// --- Alertmanager Config & Alert Routing Health Auditor (v16.36) ---
	add("/api/operations/alertmanager-health", "get", OpenAPIOperation{
		Summary:     "Alertmanager config & alert routing health auditor",
		OperationID: "alertmanagerHealth",
		Tags:        []string{"Operations", "Observability", "Alertmanager"},
		Description: "Audits Alertmanager configuration: detects Alertmanager installation, scans ConfigMaps for receiver/route config, checks for missing notification channels (slack/pagerduty/email), missing group_by grouping. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Alertmanager health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"hasAlertmanager": true, "totalReceivers": 5, "healthScore": 88},
			}),
		},
	})

	// --- Grafana Dashboard Availability & Datasource Health Auditor (v16.40) ---
	add("/api/operations/grafana-health", "get", OpenAPIOperation{
		Summary:     "Grafana dashboard availability & datasource health auditor",
		OperationID: "grafanaHealth",
		Tags:        []string{"Operations", "Observability", "Grafana"},
		Description: "Audits Grafana dashboard availability and datasource health. Detects Grafana installation via pod image scan, analyzes dashboard ConfigMaps for title/refresh/panel count/datasource references, identifies stale dashboards (no refresh or very long refresh), broken dashboards (panels but no datasource), and missing time ranges. Checks Grafana pod health (ready/restarts/probes). Health score (0-100). Blind spot: Observability Stack.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Grafana health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"grafanaDetected": true, "totalDashboards": 15, "staleDashboards": 3, "healthScore": 82},
			}),
		},
	})

	// --- Container Image Vulnerability & Patch Lag Auditor (v16.37) ---
	add("/api/security/image-vuln", "get", OpenAPIOperation{
		Summary:     "Container image vulnerability & patch lag auditor",
		OperationID: "imageVuln",
		Tags:        []string{"Security", "SupplyChain", "Images"},
		Description: "Audits container image supply chain: detects :latest tag usage, unpinned images (no @sha256 digest), duplicate tags, and image freshness. Identifies stale images for patch lag. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image vulnerability analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalImages": 50, "latestTag": 5, "noDigest": 30, "healthScore": 80},
			}),
		},
	})

	// --- Metrics Pipeline & kube-state-metrics Health Auditor (v16.46) ---
	add("/api/operations/metrics-pipeline", "get", OpenAPIOperation{
		Summary:     "Metrics pipeline & kube-state-metrics health auditor",
		OperationID: "metricsPipeline",
		Tags:        []string{"Operations", "Observability", "Metrics"},
		Description: "Audits metrics pipeline completeness: detects metrics-server, kube-state-metrics, node-exporter, and Prometheus installation via pod image scan. Checks component pod health (ready/restarts). Identifies missing critical components for HPA, alerting, and capacity planning. Health score (0-100). Blind spot: Observability Stack deepening.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Metrics pipeline analysis", map[string]interface{}{
				"summary": map[string]interface{}{"metricsServerDetected": true, "kubeStateMetricsDetected": true, "nodeExporterDetected": false, "healthScore": 75},
			}),
		},
	})

	// --- Kyverno Policy Compliance & Cluster Policy Audit (v16.41) ---
	add("/api/security/kyverno-compliance", "get", OpenAPIOperation{
		Summary:     "Kyverno policy compliance & cluster policy audit",
		OperationID: "kyvernoCompliance",
		Tags:        []string{"Security", "Compliance", "Policy"},
		Description: "Audits Kyverno policy compliance: detects Kyverno installation via pod image scan, lists ClusterPolicy and Policy CRDs, classifies rules by type (validate/mutate/generate), checks enforcement mode (Enforce vs Audit), background scan status, and policy violations. Identifies audit-only policies ready for enforcement. Health score (0-100). Blind spot: Compliance/Governance.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Kyverno compliance analysis", map[string]interface{}{
				"summary": map[string]interface{}{"kyvernoDetected": true, "totalPolicies": 12, "enforcePolicies": 8, "violationCount": 3, "healthScore": 82},
			}),
		},
	})

	// --- Pod Security Standards Compliance Scorecard (v16.47) ---
	add("/api/security/pss-scorecard", "get", OpenAPIOperation{
		Summary:     "Pod Security Standards compliance scorecard",
		OperationID: "pssScorecard",
		Tags:        []string{"Security", "PodSecurity", "Compliance"},
		Description: "Audits all containers against Pod Security Standards restricted profile: runAsNonRoot, seccompProfile, allowPrivilegeEscalation=false, capabilities.drop ALL, readOnlyRootFilesystem, privileged flag, hostNetwork/PID/IPC. Per-namespace compliance rate, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PSS compliance scorecard", map[string]interface{}{
				"summary": map[string]interface{}{"totalContainers": 80, "restrictedCompliant": 45, "privileged": 3, "healthScore": 72},
			}),
		},
	})

	// --- HPA Autoscaling Performance & Scaling Event Auditor (v16.48) ---
	add("/api/scalability/hpa-performance", "get", OpenAPIOperation{
		Summary:     "HPA autoscaling performance & scaling event auditor",
		OperationID: "hpaPerformance",
		Tags:        []string{"Scalability", "HPA", "Autoscaling"},
		Description: "Audits HPA autoscaling performance: current/desired replicas, utilization ratio, scaling active/limited conditions, missing metrics, over/underutilization, no scaling room (max=min), stale HPAs. Per-namespace stats, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("HPA performance analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalHPAs": 15, "scalingActive": 12, "scalingLimited": 2, "noMetrics": 1, "healthScore": 78},
			}),
		},
	})

	// --- Service Endpoint & DNS Resolution Health Auditor (v16.49) ---
	add("/api/product/endpoint-dns-health", "get", OpenAPIOperation{
		Summary:     "Service endpoint & DNS resolution health auditor",
		OperationID: "endpointDNSHealth",
		Tags:        []string{"Product", "Service", "DNS"},
		Description: "Audits service endpoint and DNS resolution health: detects services with no ready endpoints, headless services, external-name services, no-selector services, unnamed multi-port services. Cross-references Endpoints resources. Per-namespace stats, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Endpoint DNS health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalServices": 30, "noEndpoints": 3, "headlessServices": 5, "healthScore": 82},
			}),
		},
	})

	// --- ReplicaSet Staleness & Rollout History Auditor (v16.50) ---
	add("/api/deployment/rs-staleness", "get", OpenAPIOperation{
		Summary:     "ReplicaSet staleness & rollout history auditor",
		OperationID: "rsStaleness",
		Tags:        []string{"Deployment", "ReplicaSet", "Rollout"},
		Description: "Audits ReplicaSet staleness and rollout history: identifies stale ReplicaSets (replicas=0), revisionHistoryLimit configuration, excess ReplicaSets beyond limit, and old stale ReplicaSets consuming etcd storage. Per-namespace stats, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("RS staleness analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalDeployments": 25, "staleReplicaSets": 40, "healthScore": 85},
			}),
		},
	})

	// --- Audit Log Pipeline & Event Export Health Auditor (v16.51) ---
	add("/api/operations/audit-log-health", "get", OpenAPIOperation{
		Summary:     "Audit log pipeline & event export health auditor",
		OperationID: "auditLogHealth",
		Tags:        []string{"Operations", "Logging", "Events"},
		Description: "Audits audit log pipeline and event export health: detects fluent-bit/fluentd/vector/loki installation, checks exporter pod health (ready/restarts), scans namespace warning event rates, identifies high-event-rate namespaces. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Audit log health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"fluentBitDetected": true, "exporterPodCount": 3, "readyExporters": 3, "healthScore": 90},
			}),
		},
	})

	// --- SA Token Rotation & Access Risk Auditor (v16.52) ---
	add("/api/security/sa-token-audit", "get", OpenAPIOperation{
		Summary:     "ServiceAccount token rotation & access risk auditor",
		OperationID: "saTokenAudit",
		Tags:        []string{"Security", "ServiceAccount", "Token"},
		Description: "Audits ServiceAccount token configuration: detects auto-mount enabled SAs, long-lived tokens (>90 days), default SA used by pods, unused SAs with automount, and missing secret references. Per-namespace stats, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("SA token audit", map[string]interface{}{
				"summary": map[string]interface{}{"totalServiceAccounts": 30, "autoMountEnabled": 25, "defaultSAUsed": 5, "longLivedTokens": 3, "healthScore": 75},
			}),
		},
	})

	// --- PV Reclaim Policy & Storage Class Waste Auditor (v16.53) ---
	add("/api/scalability/pv-reclaim", "get", OpenAPIOperation{
		Summary:     "PV reclaim policy & storage class waste auditor",
		OperationID: "pvReclaim",
		Tags:        []string{"Scalability", "Storage", "PV"},
		Description: "Audits PV reclaim policy and storage class waste: detects Released PVs with Retain policy (orphaned storage), Failed PVs, Pending PVCs, Delete vs Retain reclaim policy distribution, and storage class statistics. Per-namespace and per-storage-class breakdown. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PV reclaim analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPVs": 50, "orphanedPVs": 3, "failedPVs": 1, "pendingPVCs": 2, "healthScore": 85},
			}),
		},
	})

	// --- ConfigMap & Secret Mount Injection Risk Auditor (v16.55) ---
	add("/api/product/config-mount-risk", "get", OpenAPIOperation{
		Summary:     "ConfigMap & Secret mount injection risk auditor",
		OperationID: "configMountRisk",
		Tags:        []string{"Product", "ConfigMap", "Secret"},
		Description: "Audits ConfigMap and Secret mount injection risks: detects missing ConfigMap references, large ConfigMaps (>500KB), non-optional mounts, subPath mounts (prevent hot-reload), envFrom Secret injection. Per-namespace stats, health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Config mount risk analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPods": 30, "configMapMounts": 15, "largeConfigMaps": 2, "healthScore": 85},
			}),
		},
	})

	// --- ArgoCD & Flux GitOps Sync Status Auditor (v16.57) ---
	add("/api/deployment/gitops-sync-status", "get", OpenAPIOperation{
		Summary:     "ArgoCD & Flux GitOps sync status & drift auditor",
		OperationID: "gitopsSync",
		Tags:        []string{"Deployment", "GitOps", "ArgoCD", "Flux"},
		Description: "Audits ArgoCD Application and Flux CRD (GitRepository, Kustomization, HelmRelease) sync status. Detects out-of-sync apps, sync failures, stale apps (>24h), configuration drift, missing auto-sync. Blind spot: GitOps/CD deepening. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("GitOps sync analysis", map[string]interface{}{
				"summary": map[string]interface{}{"argoCDDetected": true, "fluxDetected": false, "totalApps": 5, "healthyApps": 3, "outOfSyncApps": 1, "syncFailedApps": 1},
			}),
		},
	})

	// --- Alert Noise & Fatigue Detection Auditor (v16.58) ---
	add("/api/operations/alert-noise", "get", OpenAPIOperation{
		Summary:     "Alert noise & fatigue detection auditor",
		OperationID: "alertNoise",
		Tags:        []string{"Operations", "Observability", "Alertmanager"},
		Description: "Detects alert noise patterns: noisy alerts (>10 events/24h), flapping alerts (repeated fire/resolve cycles), alert storms (>20 events in 5min), stale silences (>7d), noise ratio. Blind spot: Observability deepening. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Alert noise analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalAlertEvents": 100, "noisyAlerts": 3, "flappingAlerts": 2, "alertStorms": 1},
			}),
		},
	})

	// --- Supply Chain & SBOM Coverage Auditor (v16.59) ---
	add("/api/security/supply-chain", "get", OpenAPIOperation{
		Summary:     "Supply chain & SBOM coverage security auditor",
		OperationID: "supplyChain",
		Tags:        []string{"Security", "SupplyChain", "Compliance"},
		Description: "Audits container image supply chain security: digest pinning, trusted registries, image signing, SBOM/provenance annotations, latest tag usage, stale images. Blind spot: Compliance/Governance deepening. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Supply chain analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalImages": 50, "usingDigest": 15, "usingLatestTag": 5, "unsignedImages": 30},
			}),
		},
	})

	// --- Resource Quota & Limit Range Security Audit (v16.66) ---
	add("/api/security/quota-security", "get", OpenAPIOperation{
		Summary:     "Resource quota & limit range security auditor",
		OperationID: "quotaSecurity",
		Tags:        []string{"Security", "ResourceQuota", "DoS-Prevention"},
		Description: "Audits resource quota and limit range security posture: namespaces without ResourceQuotas (DoS risk), namespaces without LimitRanges (unbounded pod requests), quota pressure (>80% CPU/memory/pod usage). Prevents resource exhaustion attacks by identifying unprotected namespaces. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Quota security analysis", map[string]interface{}{
				"summary":               map[string]interface{}{"totalNamespaces": 15, "withResourceQuota": 10, "unprotectedNamespaces": 5},
				"unprotectedNamespaces": []map[string]interface{}{{"namespace": "dev", "podCount": 8, "severity": "high"}},
			}),
		},
	})

	// --- PV Access Mode & Multi-Attach Risk Auditor (v16.67) ---
	add("/api/product/pv-access", "get", OpenAPIOperation{
		Summary:     "Persistent volume access mode & multi-attach risk auditor",
		OperationID: "pvAccess",
		Tags:        []string{"Product", "Storage", "VolumeSecurity"},
		Description: "Audits persistent volume access modes and multi-attach risks: RWO vs RWX distribution, unbound PVCs, RWX PVCs used by multiple pods (multi-attach data corruption risk), Delete vs Retain reclaim policy, missing storage class, per-storage-class stats. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PV access analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalPVs": 20, "totalPVCs": 35, "unboundPVCs": 3, "multiAttachPVCs": 2},
				"risks":   []map[string]interface{}{{"pvcName": "shared-data", "riskType": "multi-attach-rwx", "severity": "medium"}},
			}),
		},
	})

	// --- DORA Metrics Analyzer (v16.68) ---
	add("/api/deployment/dora-metrics", "get", OpenAPIOperation{
		Summary:     "DORA metrics: deployment frequency & delivery performance",
		OperationID: "doraMetrics",
		Tags:        []string{"Deployment", "DORA", "Delivery"},
		Description: "Analyzes DORA metrics: deployment frequency (deploys/day), lead time for changes, mean time to recovery (MTTR), change failure rate. Classifies delivery maturity as elite/high/medium/low. Per-namespace success rate, recent deployment events with strategy and status. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("DORA metrics analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalDeployments": 25, "deploymentFrequency": "5/day", "changeFailureRate": 0.12},
				"level":   "elite",
			}),
		},
	})

	// --- API Priority & Fairness Configuration Auditor (v16.69) ---
	add("/api/operations/apf-audit", "get", OpenAPIOperation{
		Summary:     "API Priority & Fairness configuration auditor",
		OperationID: "apfAudit",
		Tags:        []string{"Operations", "APF", "API Server"},
		Description: "Audits Kubernetes API Priority & Fairness (APF) configuration: FlowSchema resources, PriorityLevelConfiguration resources, missing priority level references, essential priority levels (global-default, leader-election, node-high), exempt flow count, catch-all flow configuration. Uses dynamic client to access flowcontrol.apiserver.k8s.io/v1 CRDs. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("APF configuration analysis", map[string]interface{}{
				"summary": map[string]interface{}{"flowSchemaCount": 10, "priorityLevelCount": 5, "missingPL": 0},
				"issues":  []map[string]interface{}{},
			}),
		},
	})

	// --- Capacity Planning & Growth Trend Predictor (v16.65) ---
	add("/api/scalability/capacity-plan", "get", OpenAPIOperation{
		Summary:     "Capacity planning & growth trend predictor",
		OperationID: "capacityPlan",
		Tags:        []string{"Scalability", "Capacity", "Forecast"},
		Description: "Predicts capacity exhaustion timelines based on current utilization and estimated growth. Per-node CPU/memory/pod utilization, daily growth rate, days-to-exhaust forecast, first bottleneck identification, recommended scale-out actions. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Capacity plan analysis", map[string]interface{}{
				"summary":  map[string]interface{}{"totalNodes": 5, "cpuUtilization": 0.65, "headroomDays": 45},
				"forecast": map[string]interface{}{"firstBottleneck": "Memory", "cpuExhaustDays": 60},
			}),
		},
	})

	// --- Spot/Preemptible Instance Readiness Auditor (v16.71) ---
	add("/api/scalability/spot-readiness", "get", OpenAPIOperation{
		Summary:     "Spot/preemptible instance readiness & cost optimization auditor",
		OperationID: "spotReadiness",
		Tags:        []string{"Scalability", "FinOps", "CostOptimization"},
		Description: "Audits spot/preemptible node usage and workload readiness: spot node detection (Karpenter, GCE, Azure), spot percentage, estimated cost savings, workloads on spot without tolerations (eviction risk), StatefulSet on spot (data loss risk), interruption handler detection (Node Termination Handler, Karpenter), spot anti-affinity coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Spot readiness analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalNodes": 10, "spotNodes": 3, "spotPercentage": 30, "estimatedSavings": 151.2},
				"atRiskWorkloads": []map[string]interface{}{{"name": "critical-db", "severity": "high", "reason": "StatefulSet on spot without toleration"}},
			}),
		},
	})

	// --- Service Traffic Policy & Routing Configuration Auditor (v16.72) ---
	add("/api/product/traffic-policy", "get", OpenAPIOperation{
		Summary:     "Service traffic policy & routing configuration auditor",
		OperationID: "trafficPolicy",
		Tags:        []string{"Product", "Networking", "TrafficRouting"},
		Description: "Audits service traffic policies and routing configuration: externalTrafficPolicy (Cluster vs Local), session affinity, service type distribution, over-exposed LoadBalancer services, external IPs, publishNotReadyAddresses, ExternalName services, missing selectors. Per-namespace issue breakdown. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Traffic policy analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalServices": 50, "loadBalancer": 5, "extTrafficCluster": 2},
				"issues":  []map[string]interface{}{{"name": "api-svc", "issueType": "ext-traffic-cluster", "severity": "medium"}},
			}),
		},
	})

	// --- Pod Priority Preemption & Scheduling Starvation Risk (v17.19) ---
	add("/api/product/priority-preemption", "get", OpenAPIOperation{
		Summary:     "Pod priority preemption & scheduling starvation risk analyzer",
		OperationID: "priorityPreemption",
		Tags:        []string{"Product", "Scheduling", "PriorityClass"},
		Description: "Analyzes pod priority classes, preemption vulnerability, and scheduling starvation: PriorityClass distribution & usage, preemption risks (negative/low priority pods), starvation risks (pending pods with low priority), priority heatmap, pending pod queue, recommendations for improving scheduling fairness. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Priority preemption analysis", map[string]interface{}{
				"score":           85,
				"status":          "healthy",
				"summary":         map[string]interface{}{"totalPriorityClasses": 3, "podsWithPriority": 20, "pendingPods": 1},
				"priorityHeatmap": []map[string]interface{}{{"range": "1K-99K (Normal)", "podCount": 15, "riskLevel": "low"}},
			}),
		},
	})

	// --- DaemonSet Rollout & Node Coverage Auditor (v16.73) ---
	add("/api/deployment/daemonset-audit", "get", OpenAPIOperation{
		Summary:     "DaemonSet rollout & node coverage auditor",
		OperationID: "daemonsetAudit",
		Tags:        []string{"Deployment", "DaemonSet", "NodeCoverage"},
		Description: "Audits DaemonSet rollout status and node coverage: desired vs scheduled vs updated vs ready pod counts, missing nodes (schedulable nodes without DS pods), stale revisions (pods running old revision), OnDelete vs RollingUpdate strategy, toleration coverage for tainted nodes, per-DS status (healthy/updating/degraded/critical), node gap analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("DaemonSet audit", map[string]interface{}{
				"summary":  map[string]interface{}{"totalDaemonSets": 5, "totalNodes": 10, "missingNodes": 2},
				"nodeGaps": []map[string]interface{}{{"daemonSet": "node-exporter", "nodeName": "node-5", "severity": "medium"}},
			}),
		},
	})

	// --- Deployment Concurrency Guard & Rolling Update Collision Detector (v17.20) ---
	add("/api/deployment/concurrency-guard", "get", OpenAPIOperation{
		Summary:     "Deployment concurrency & rolling update collision detector",
		OperationID: "concurrencyGuard",
		Tags:        []string{"Deployment", "RollingUpdate", "Concurrency"},
		Description: "Detects concurrent rolling update collisions: active rollouts, namespace-level concurrency (multiple workloads updating simultaneously), surge budget exhaustion risk, node saturation during rollouts, deployment window safety assessment. Provides safeToDeploy flag, blockers, and staggered deployment recommendations. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Concurrency guard analysis", map[string]interface{}{
				"safeToDeploy": true,
				"score":        95,
				"summary":      map[string]interface{}{"activeRollouts": 0, "collisionRisks": 0, "totalSurgePods": 12},
			}),
		},
	})

	// --- Security Policy Drift & Baseline Configuration Auditor (v16.74) ---
	add("/api/security/policy-drift", "get", OpenAPIOperation{
		Summary:     "Security policy drift & baseline configuration auditor",
		OperationID: "policyDrift",
		Tags:        []string{"Security", "PolicyDrift", "Compliance"},
		Description: "Audits security policy drift and baseline configuration: PSA enforce label gaps on namespaces, inconsistent PSA levels (privileged vs baseline), risky default role bindings (cluster-admin to default SAs), network policy baseline (default deny missing), API server security flags, system namespace PSA consistency. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Policy drift analysis", map[string]interface{}{
				"summary":      map[string]interface{}{"totalNamespaces": 15, "missingPSALabels": 3, "defaultRoleBindings": 2, "driftDetected": 8},
				"psaLabelGaps": []map[string]interface{}{{"namespace": "app-prod", "currentLevel": "", "expectedLevel": "baseline", "severity": "high"}},
			}),
		},
	})

	// --- Log Aggregation & Forwarding Pipeline Health Auditor (v16.75) ---
	add("/api/operations/log-pipeline", "get", OpenAPIOperation{
		Summary:     "Log aggregation & forwarding pipeline health auditor",
		OperationID: "logPipeline",
		Tags:        []string{"Operations", "Logging", "Observability"},
		Description: "Audits log aggregation pipeline health: log collectors (Fluent Bit, Fluentd, Vector, Promtail, Filebeat) as DaemonSets/Deployments, collector readiness, log forwarding ConfigMaps with output/filter configs, storage backends (Elasticsearch, Loki, Kafka, etc.), namespace coverage gaps. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Log pipeline analysis", map[string]interface{}{
				"summary":    map[string]interface{}{"totalCollectors": 2, "readyCollectors": 2, "hasFluentBit": true},
				"collectors": []map[string]interface{}{{"name": "fluent-bit", "kind": "DaemonSet", "status": "healthy"}},
			}),
		},
	})

	// --- Container Runtime Class & OCI Image Compliance Auditor (v16.76) ---
	add("/api/product/runtime-class", "get", OpenAPIOperation{
		Summary:     "Container runtime class & OCI image compliance auditor",
		OperationID: "runtimeClass",
		Tags:        []string{"Product", "RuntimeClass", "ImageCompliance"},
		Description: "Audits container runtime class usage and OCI image compliance: RuntimeClass definitions (kata, gVisor), node container runtime (containerd, cri-o), pod runtimeClassName assignment, :latest image tags, missing digest references, untrusted registry images. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Runtime class & image compliance analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalRuntimeClasses": 1, "podsUsingRuntime": 5, "imagesWithLatest": 3},
				"imageCompliance": []map[string]interface{}{{"podName": "app-1", "container": "web", "image": "nginx:latest", "issue": "Using :latest tag", "severity": "medium"}},
			}),
		},
	})

	// --- Image Pull Policy & Secret Management Auditor (v16.77) ---
	add("/api/deployment/image-pull-audit", "get", OpenAPIOperation{
		Summary:     "Image pull policy & secret management auditor",
		OperationID: "imagePullAudit",
		Tags:        []string{"Deployment", "ImagePull", "Security"},
		Description: "Audits image pull policy configuration and secret management: imagePullPolicy distribution (Always/IfNotPresent/Never), missing policies, private images without imagePullSecrets, stale dockerconfigjson secrets, duplicate secrets, wasteful Always pull on pinned images. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image pull audit", map[string]interface{}{
				"summary":      map[string]interface{}{"totalPods": 15, "alwaysPull": 3, "ifNotPresent": 10, "neverPull": 2},
				"policyIssues": []map[string]interface{}{{"podName": "app-1", "container": "web", "policy": "Never", "severity": "high"}},
			}),
		},
	})

	// --- VPA Configuration & Resource Recommendation Quality Auditor (v16.79) ---
	add("/api/scalability/vpa-audit", "get", OpenAPIOperation{
		Summary:     "VPA configuration & resource recommendation quality auditor",
		OperationID: "vpaAudit",
		Tags:        []string{"Scalability", "VPA", "Autoscaling"},
		Description: "Audits Vertical Pod Autoscaler configuration and resource recommendation quality: VPA installation status, VPA objects and update modes (Auto/Off/Initial/Recreate), workloads with OOM kills that could benefit from VPA, target workload coverage gaps, resource recommendation availability. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("VPA audit analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalVPAs": 3, "vpasWithRecommendations": 2, "vpaNotInstalled": false},
				"targetWorkloads": []map[string]interface{}{{"namespace": "app-prod", "kind": "Deployment", "name": "app-api", "hasOOMKill": true, "severity": "high"}},
			}),
		},
	})

	// --- Service Mesh Traffic Management & Circuit Breaker Health Auditor (v16.80) ---
	add("/api/product/mesh-traffic", "get", OpenAPIOperation{
		Summary:     "Service mesh traffic management & circuit breaker health auditor",
		OperationID: "meshTraffic",
		Tags:        []string{"Product", "ServiceMesh", "Network"},
		Description: "Audits service mesh traffic management and circuit breaker health: Istio/Linkerd installation detection, sidecar injection coverage per namespace, VirtualService retry/timeout configurations, DestinationRule circuit breaker/TLS settings, services without mesh protection. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Mesh traffic analysis", map[string]interface{}{
				"summary": map[string]interface{}{"hasIstio": true, "namespacesWithMesh": 5, "namespacesNoMesh": 2},
				"gaps":    []map[string]interface{}{{"namespace": "app-prod", "service": "api-svc", "issue": "No mesh sidecar injection", "severity": "medium"}},
			}),
		},
	})

	// --- Deployment Rollout Blocker & Pod Condition Auditor (v16.81) ---
	add("/api/deployment/rollout-blocker", "get", OpenAPIOperation{
		Summary:     "Deployment rollout blocker & pod condition auditor",
		OperationID: "rolloutBlocker",
		Tags:        []string{"Deployment", "Rollout", "PodHealth"},
		Description: "Audits deployment rollout blockers and pod conditions: ProgressDeadlineExceeded, no updated replicas, no ready replicas, CrashLoopBackOff, ImagePullBackOff, OOMKilled, Pending pods. Identifies blocked rollouts, degraded deployments, and pod-level issues. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollout blocker analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalDeployments": 10, "blockedRollouts": 1, "podsCrashLooping": 2},
				"blockedRollouts": []map[string]interface{}{{"namespace": "app-prod", "name": "api-deploy", "blocker": "ProgressDeadlineExceeded", "severity": "critical"}},
			}),
		},
	})

	// --- PSS Enforcement Gap & Workload Hardening Auditor (v16.82) ---
	add("/api/security/pss-hardening", "get", OpenAPIOperation{
		Summary:     "PSS enforcement gap & workload hardening auditor",
		OperationID: "pssHardening",
		Tags:        []string{"Security", "Hardening", "PSS"},
		Description: "Audits pod security standards enforcement gaps and workload hardening: privileged containers, allowPrivilegeEscalation, hostPID/Network/IPC, seccomp profile coverage, AppArmor profile, readOnlyRootFilesystem, added/dropped capabilities. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("PSS hardening analysis", map[string]interface{}{
				"summary":        map[string]interface{}{"totalPods": 15, "privilegedContainers": 1, "podsNoSeccomp": 5},
				"privilegedPods": []map[string]interface{}{{"podName": "app-1", "container": "c1", "issue": "Container runs in privileged mode", "severity": "critical"}},
			}),
		},
	})

	// --- Node Condition Trend & Hardware Failure Prediction (v16.83) ---
	add("/api/operations/node-trend", "get", OpenAPIOperation{
		Summary:     "Node condition trend & hardware failure prediction",
		OperationID: "nodeTrend",
		Tags:        []string{"Operations", "NodeHealth", "Predictive"},
		Description: "Audits node condition trends and predicts hardware failure risk: MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable, NotReady, stale heartbeat, kernel/runtime version drift, risk classification (low/medium/high/critical). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node trend analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalNodes": 5, "healthyNodes": 3, "nodesAtRisk": 2},
				"atRiskNodes": []map[string]interface{}{{"nodeName": "node-3", "severity": "high"}},
			}),
		},
	})

	// --- Endpoint Slice Health & Topology-Aware Routing Auditor (v16.84) ---
	add("/api/product/endpoint-slice", "get", OpenAPIOperation{
		Summary:     "Endpoint slice health & topology-aware routing auditor",
		OperationID: "endpointSlice",
		Tags:        []string{"Product", "EndpointSlice", "Network"},
		Description: "Audits endpoint slice health and topology-aware routing: endpoint readiness, topology hints, zone distribution, services without endpoints, not-ready endpoints. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Endpoint slice analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalServices": 10, "servicesWithEndpoints": 9, "readyEndpoints": 25},
				"gaps":    []map[string]interface{}{{"service": "app-svc", "issue": "No topology hints", "severity": "low"}},
			}),
		},
	})

	// --- Rolling Update Risk & Surge Configuration Analyzer (v16.85) ---
	add("/api/deployment/surge-risk", "get", OpenAPIOperation{
		Summary:     "Rolling update risk & surge configuration analyzer",
		OperationID: "surgeRisk",
		Tags:        []string{"Deployment", "Rollout", "Risk"},
		Description: "Audits rolling update surge and maxUnavailable configuration: high surge (>50%), high maxUnavailable (>50%), Recreate strategy risk, zero surge+zero unavailable stall, 100% unavailable risk. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Surge risk analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalDeployments": 10, "highSurge": 1, "recreateStrategy": 2},
				"deployments": []map[string]interface{}{{"name": "app-1", "strategy": "RollingUpdate", "maxSurge": "25%", "riskLevel": "low"}},
			}),
		},
	})

	// --- Resource Saturation & CPU/Memory Throttling Risk Predictor (v16.87) ---
	add("/api/scalability/saturation", "get", OpenAPIOperation{
		Summary:     "Resource saturation & CPU/memory throttling risk predictor",
		OperationID: "saturation",
		Tags:        []string{"Scalability", "Resources", "Throttling"},
		Description: "Audits resource saturation and CPU/memory throttling risk: unbounded pods (no limits), high CPU limit/request ratio (>5x), CPU limit < request (guaranteed throttling), OOM risk (no memory limit), per-namespace saturation. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Saturation analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalPods": 15, "unboundedPods": 3, "oomRiskPods": 5},
				"throttlingRisks": []map[string]interface{}{{"podName": "app-1", "issue": "No resource limits set", "severity": "medium"}},
			}),
		},
	})

	// --- Container Image Registry Rate Limit & Pull Reliability Auditor (v16.88) ---
	add("/api/operations/registry-rate-limit", "get", OpenAPIOperation{
		Summary:     "Container image registry rate limit & pull reliability auditor",
		OperationID: "registryRateLimit",
		Tags:        []string{"Operations", "Registry", "Reliability"},
		Description: "Audits container image registry rate limit risk and pull reliability: Docker Hub anonymous pull rate limiting, private registry authentication coverage, public vs private registry distribution, duplicate image detection, pods without imagePullSecrets. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Registry rate limit analysis", map[string]interface{}{
				"summary":    map[string]interface{}{"totalImages": 20, "usingDockerHub": 5, "rateLimitRisk": 3},
				"registries": []map[string]interface{}{{"registry": "docker.io", "imageCount": 5, "rateLimited": true, "riskLevel": "high"}},
			}),
		},
	})

	// --- Cert-Manager Health & Certificate Renewal Pipeline Auditor (v16.90) ---
	add("/api/product/cert-manager", "get", OpenAPIOperation{
		Summary:     "Cert-manager health & certificate renewal pipeline auditor",
		OperationID: "certManager",
		Tags:        []string{"Product", "CertManager", "TLS"},
		Description: "Audits cert-manager installation and certificate renewal pipeline: cert-manager detection, TLS secret scanning, certificate expiry tracking (<30d expiring, expired), cert-manager-managed vs manual certificates, issuer readiness. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cert-manager analysis", map[string]interface{}{
				"summary":      map[string]interface{}{"certManagerInstalled": true, "totalCertificates": 5, "expiringSoon": 1, "expired": 0},
				"certificates": []map[string]interface{}{{"name": "tls-cert", "namespace": "app-prod", "status": "valid", "daysUntilExpiry": 90}},
			}),
		},
	})

	// --- Deployment Resource Quota Impact & Namespace Deployment Capacity Auditor (v16.91) ---
	add("/api/deployment/quota-impact", "get", OpenAPIOperation{
		Summary:     "Deployment resource quota impact & namespace deployment capacity auditor",
		OperationID: "deployQuotaImpact",
		Tags:        []string{"Deployment", "Quota", "Capacity"},
		Description: "Audits deployment resource quota impact and namespace deployment capacity: per-namespace quota usage, over-quota namespaces, near-limit (>80%) namespaces, deployments that would be blocked or push >90% of quota, headroom analysis. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Quota impact analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNamespaces": 10, "nsOverQuota": 1, "deploysBlocked": 2},
				"impacts": []map[string]interface{}{{"namespace": "app-prod", "deployName": "api", "issue": "Would exceed CPU quota", "severity": "critical"}},
			}),
		},
	})

	// --- Runtime Threat Detection & Container Anomaly Auditor (v16.92) ---
	add("/api/security/runtime-threat", "get", OpenAPIOperation{
		Summary:     "Runtime threat detection & container anomaly auditor",
		OperationID: "runtimeThreat",
		Tags:        []string{"Security", "RuntimeSecurity", "ThreatDetection"},
		Description: "Audits runtime threat detection and container anomalies: Falco/Tracee/Tetragon/Cilium detection, detector health, privileged containers (runtime risk), high restart pods, OOMKilled containers, namespace coverage gaps. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Runtime threat analysis", map[string]interface{}{
				"summary":       map[string]interface{}{"hasFalco": true, "totalDetectors": 1, "privilegedPods": 2},
				"anomalousPods": []map[string]interface{}{{"podName": "app-1", "severity": "high"}},
			}),
		},
	})

	// --- Secret Management Posture & External Secret Integration Auditor (v16.99) ---
	add("/api/security/secret-posture", "get", OpenAPIOperation{
		Summary:     "Secret management posture & external secret integration auditor",
		OperationID: "secretPosture",
		Tags:        []string{"Security", "Secrets", "Compliance"},
		Description: "Audits secret management posture: External Secrets Operator, Sealed Secrets, SOPS, HashiCorp Vault detection. Per-secret managed/unmanaged classification, plaintext secret detection, empty/large secrets, SOPS encryption annotations, namespace risk levels. Integration status (integrated/partial/missing). Health score (0-100). Blind spot: Compliance/Governance.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Secret posture analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalSecrets": 50, "managedSecrets": 30, "plaintextSecrets": 5},
				"integration": map[string]interface{}{"externalSecretsOperator": true, "status": "partial"},
			}),
		},
	})

	// --- Namespace Security Posture & Trust Boundary Auditor (v17.05) ---
	add("/api/security/namespace-posture", "get", OpenAPIOperation{
		Summary:     "Namespace security posture & trust boundary auditor",
		OperationID: "namespacePosture",
		Tags:        []string{"Security", "Namespace", "PSA"},
		Description: "Audits per-namespace security posture: Pod Security Admission (enforce/warn/audit), default SA token auto-mount, network policy coverage, RBAC role bindings, resource quota, limit range. Trust level classification (high/medium/low/untrusted). Risk score (0-100). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Namespace security posture analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalNamespaces": 10, "withPSAEnforce": 5, "withoutNetworkPolicy": 3},
				"byNamespace": []map[string]interface{}{{"namespace": "default", "trustLevel": "low", "riskScore": 35}},
			}),
		},
	})

	// --- Container Image Provenance & Registry Trust Auditor (v17.11) ---
	add("/api/security/image-provenance-v3", "get", OpenAPIOperation{
		Summary:     "Container image provenance & registry trust auditor",
		OperationID: "imageProvenance",
		Tags:        []string{"Security", "Images", "SupplyChain"},
		Description: "Audits container image provenance: trusted vs untrusted registries, digest pinning (@sha256), mutable tag detection (:latest), per-registry and per-namespace stats. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Image provenance analysis", map[string]interface{}{
				"summary":    map[string]interface{}{"totalImages": 15, "withDigest": 5, "latestTag": 3, "untrustedRegistries": 2},
				"byRegistry": []map[string]interface{}{{"registry": "docker.io", "imageCount": 5, "trusted": false}},
			}),
		},
	})

	// --- Security Event Timeline & Threat Detection Pattern Auditor (v17.17) ---
	add("/api/security/threat-timeline", "get", OpenAPIOperation{
		Summary:     "Security event timeline & threat detection pattern auditor",
		OperationID: "threatTimeline",
		Tags:        []string{"Security", "Threat Detection", "Events"},
		Description: "Audits security-related events: RBAC changes (Role/ClusterRole/Binding), admission denials, forbidden/unauthorized access (403), secret access patterns, ConfigMap changes. Per-namespace risk levels, threat pattern detection. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Threat timeline analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalEvents": 15, "rbacChanges": 3, "admissionDenied": 2, "forbidden": 1},
				"byNamespace": []map[string]interface{}{{"namespace": "kube-system", "totalEvents": 8, "riskLevel": "critical"}},
			}),
		},
	})

	// --- Secret Age & Stale Credential Tracker (v17.23) ---
	add("/api/security/secret-age", "get", OpenAPIOperation{
		Summary:     "Secret age & stale credential tracker",
		OperationID: "secretAge",
		Tags:        []string{"Security", "Secrets", "Rotation"},
		Description: "Audits secret age and staleness: creation age analysis (90d/180d/365d thresholds), orphaned secret detection (not referenced by any pod), type distribution (TLS/Docker/Opaque), age bracket heatmap, per-namespace stale credential stats, TLS certificate secret tracking. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Secret age analysis", map[string]interface{}{
				"score":   72,
				"summary": map[string]interface{}{"totalSecrets": 45, "olderThan365d": 3, "olderThan180d": 8, "orphanedCount": 5},
				"byAge":   []map[string]interface{}{{"range": "90-180d", "count": 4, "risk": "medium"}},
			}),
		},
	})

	// --- CNI Plugin Health & Network Stack Configuration Auditor (v16.93) ---
	add("/api/operations/cni-health", "get", OpenAPIOperation{
		Summary:     "CNI plugin health & network stack configuration auditor",
		OperationID: "cniHealth",
		Tags:        []string{"Operations", "CNI", "Network"},
		Description: "Audits CNI plugin health and network stack: CNI type detection (Calico/Cilium/Flannel/Weave), per-node PodCIDR assignment, network unavailable conditions, CNI agent pod readiness, namespace coverage. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CNI health analysis", map[string]interface{}{
				"summary": map[string]interface{}{"cniType": "calico", "totalNodes": 5, "healthyNodes": 4, "nodesWithoutCNI": 1},
			}),
		},
	})

	// --- Observability Stack Integration Health Auditor (v16.98) ---
	add("/api/operations/observability-stack", "get", OpenAPIOperation{
		Summary:     "Observability stack integration health auditor",
		OperationID: "observabilityStack",
		Tags:        []string{"Operations", "Observability", "Monitoring"},
		Description: "Audits the full observability stack across three pillars (metrics, logging, tracing): detects backends (Prometheus, Loki, Jaeger, Tempo, OpenTelemetry), per-pillar agent DaemonSet coverage, backend pod readiness, namespace coverage. Health score (0-100). Blind spot: Observability Stack.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Observability stack analysis", map[string]interface{}{
				"summary": map[string]interface{}{"healthyPillars": 3, "missingPillars": 0, "totalBackends": 5, "agentCoverage": 100},
				"pillars": []map[string]interface{}{{"name": "metrics", "status": "healthy", "coverage": 100}},
			}),
		},
	})

	// --- Multi-Signal Incident Correlation & Root Cause Engine (v17.32) ---
	add("/api/operations/incident-correlation", "get", OpenAPIOperation{
		Summary:     "Multi-signal incident correlation & root cause suggestion engine",
		OperationID: "incidentCorrelation",
		Tags:        []string{"Operations", "AIOps", "Incident Management"},
		Description: "Collects signals from cluster warning events, pod lifecycle data (CrashLoopBackOff, OOMKilled, high restarts), and node pressure conditions. Correlates related signals into incident clusters using union-find with time-proximity (5min window), namespace, and node-based grouping. For each incident: determines severity, identifies probable root cause with confidence score, calculates blast radius (affected pods/namespaces/nodes/workloads), reconstructs timeline, and generates category-specific recommendations. AIOps core feature.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace (default: all)"),
			queryParam("window", "Time window in minutes (default: 60, max: 360)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Incident correlation analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalSignals": 25, "totalIncidents": 2, "criticalCount": 1, "affectedNamespaces": 3},
				"incidents": []map[string]interface{}{
					{"id": "INC-001", "title": "Resource pressure in node node-1 (MemoryPressure)", "severity": "critical", "category": "resource-pressure", "signalCount": 5, "rootCause": map[string]interface{}{"description": "MemoryPressure: kubelet running out of memory", "confidence": 80}},
				},
			}),
		},
	})

	// --- Cluster Operator & OLM Health Auditor (v17.04) ---
	add("/api/operations/operator-health", "get", OpenAPIOperation{
		Summary:     "Cluster operator & OLM health auditor",
		OperationID: "operatorHealth",
		Tags:        []string{"Operations", "Operators", "OLM"},
		Description: "Audits cluster operator health: detects operator deployments, OLM (Operator Lifecycle Manager) installation, per-operator pod readiness, crash loops, high restarts, namespace isolation, failing/degraded/healthy classification. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Operator health analysis", map[string]interface{}{
				"summary":   map[string]interface{}{"totalOperators": 3, "healthyOperators": 2, "failedOperators": 1, "olmDetected": true},
				"operators": []map[string]interface{}{{"name": "my-operator", "status": "healthy", "podsReady": 1, "podsTotal": 1}},
			}),
		},
	})

	// --- Pod Restart Pattern & CrashLoop Clustering Auditor (v17.10) ---
	add("/api/operations/restart-storm", "get", OpenAPIOperation{
		Summary:     "Pod restart pattern & crashloop clustering auditor",
		OperationID: "restartStorm",
		Tags:        []string{"Operations", "Reliability", "Diagnostics"},
		Description: "Audits pod restart patterns: high restart count detection (>5/>20), namespace clustering (multiple pods restarting in same namespace), same-image cascade detection, per-namespace restart stats, hotspot pods. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Restart storm analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalRestarts": 25, "highRestartPods": 3, "clusteringDetected": true},
				"hotspotPods": []map[string]interface{}{{"name": "app-1", "restarts": 10}},
			}),
		},
	})

	// --- Admission Webhook Configuration Health & Performance Risk Auditor (v17.16) ---
	add("/api/operations/webhook-health", "get", OpenAPIOperation{
		Summary:     "Admission webhook configuration health & performance risk auditor",
		OperationID: "webhookHealth",
		Tags:        []string{"Operations", "Admission", "Webhook"},
		Description: "Audits admission webhook configurations: mutating/validating classification, fail-open vs fail-closed, timeout analysis (none/short/long), namespace selector coverage, match-all-resources detection, service vs URL reference. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Webhook health analysis", map[string]interface{}{
				"summary":  map[string]interface{}{"totalWebhooks": 5, "failOpenCount": 2, "longTimeout": 1, "matchAllResources": 1},
				"webhooks": []map[string]interface{}{{"name": "validator", "failurePolicy": "Ignore", "riskLevel": "medium"}},
			}),
		},
	})

	// --- Kube-Proxy Health & Network Routing Stability Auditor (v17.22) ---
	add("/api/operations/kube-proxy-health", "get", OpenAPIOperation{
		Summary:     "Kube-proxy & network routing stability auditor",
		OperationID: "kubeProxyHealth",
		Tags:        []string{"Operations", "Networking", "KubeProxy"},
		Description: "Audits kube-proxy DaemonSet health, proxy mode (iptables/ipvs/ebpf), node coverage, pod restart patterns, service routing type distribution (ClusterIP/NodePort/LoadBalancer/ExternalName/headless), missing proxy nodes, iptables scale warnings. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Kube-proxy health analysis", map[string]interface{}{
				"score":          95,
				"proxyMode":      "ipvs",
				"summary":        map[string]interface{}{"kubeProxyFound": true, "desiredNodes": 5, "readyNodes": 5},
				"serviceRouting": map[string]interface{}{"totalServices": 30, "clusterIPServices": 25},
			}),
		},
	})

	// --- Extended Resource & Device Plugin Health Auditor (v17.24) ---
	add("/api/scalability/ext-resource-health", "get", OpenAPIOperation{
		Summary:     "Extended resource & device plugin health auditor",
		OperationID: "extResourceHealth",
		Tags:        []string{"Scalability", "GPU", "DevicePlugin"},
		Description: "Audits extended resources (GPU, FPGA, custom devices): device plugin pod health, GPU node tracking with model/driver version, resource utilization per type, per-node allocation, fully allocated GPU warnings, crash loop detection. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Extended resource health", map[string]interface{}{
				"score":     90,
				"summary":   map[string]interface{}{"totalExtendedResources": 1, "gpuNodes": 2, "totalDevicePlugins": 2},
				"gpuHealth": []map[string]interface{}{{"node": "gpu-node1", "gpuCount": 4, "allocated": 2, "model": "A100-SXM4"}},
			}),
		},
	})

	// --- Service Mesh Injection Coverage & Namespace Adoption (v17.25) ---
	add("/api/product/mesh-injection", "get", OpenAPIOperation{
		Summary:     "Service mesh injection coverage & namespace adoption analyzer",
		OperationID: "meshInjection",
		Tags:        []string{"Product", "ServiceMesh", "Injection"},
		Description: "Analyzes mesh injection adoption: namespace-level injection status (Istio/Linkerd/Consul), injection gap detection (enabled but no sidecar), opt-out tracking, per-namespace injection rate, mesh type detection, fully-meshed/partial-mesh/unmeshed classification. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Mesh injection analysis", map[string]interface{}{
				"meshType": "istio",
				"score":    72,
				"summary":  map[string]interface{}{"totalPods": 50, "injectedPods": 35, "injectionRate": 70.0, "meshEnabledNamespaces": 5},
			}),
		},
	})

	// --- Deployment Revision Diff & Pod Template Change Impact (v17.26) ---
	add("/api/deployment/revision-diff", "get", OpenAPIOperation{
		Summary:     "Deployment revision diff & pod template change impact analyzer",
		OperationID: "revisionDiff",
		Tags:        []string{"Deployment", "Revision", "TemplateChange"},
		Description: "Analyzes pod template changes across deployment revisions: missing probes, resource requests, security context gaps, privileged container detection, Recreate strategy downtime risk, revision history limit, breaking change identification, risk scoring per workload. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Revision diff analysis", map[string]interface{}{
				"score":           78,
				"summary":         map[string]interface{}{"totalWorkloads": 30, "withProbeChange": 5, "breakingChangeCount": 2, "riskyWorkloadCount": 3},
				"breakingChanges": []map[string]interface{}{{"namespace": "prod", "name": "api", "change": "Recreate strategy"}},
			}),
		},
	})

	// --- CoreDNS Configuration & Resolution Health Auditor (v17.28) ---
	add("/api/operations/coredns-health", "get", OpenAPIOperation{
		Summary:     "CoreDNS configuration & resolution health auditor",
		OperationID: "corednsHealth",
		Tags:        []string{"Operations", "DNS", "CoreDNS"},
		Description: "Audits CoreDNS Deployment/DaemonSet health, Corefile plugin analysis (errors/health/ready/forward/cache/loop/reload), NodeLocal DNS Cache detection, upstream server extraction, stub domain tracking, node coverage, pod restart monitoring. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("CoreDNS health analysis", map[string]interface{}{
				"score":          92,
				"summary":        map[string]interface{}{"coreDNSFound": true, "readyReplicas": 2, "hasNodeLocalDNS": false},
				"configAnalysis": map[string]interface{}{"forwardPlugin": true, "loopPlugin": true, "cachePlugin": true},
			}),
		},
	})

	// --- Workload Attack Surface & Blast Radius Analyzer (v17.29) ---
	add("/api/security/blast-radius", "get", OpenAPIOperation{
		Summary:     "Workload attack surface & blast radius analyzer",
		OperationID: "blastRadius",
		Tags:        []string{"Security", "AttackSurface", "BlastRadius"},
		Description: "Scores each pod's blast radius: privileged containers, hostNetwork/PID/IPC, hostPath mounts (including container runtime sockets), dangerous capabilities (SYS_ADMIN/NET_ADMIN), privilege escalation, mounted secret count. Per-namespace stats, attack vector mitigation, risk heatmap. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Blast radius analysis", map[string]interface{}{
				"score":         72,
				"summary":       map[string]interface{}{"totalPods": 50, "criticalRiskPods": 2, "privilegedPods": 3, "hostNetworkPods": 1},
				"attackVectors": []map[string]interface{}{{"vector": "privileged", "count": 3, "severity": "critical"}},
			}),
		},
	})

	// --- Node Resource Reservation & Allocatable Gap Analyzer (v17.30) ---
	add("/api/scalability/reservation-audit", "get", OpenAPIOperation{
		Summary:     "Node resource reservation & allocatable gap analyzer",
		OperationID: "reservationAudit",
		Tags:        []string{"Scalability", "Node", "Reservation"},
		Description: "Analyzes node resource reservation gap (capacity vs allocatable) for CPU/memory/pods: over-reserved detection (>25%%), under-reserved detection (<3%%), per-node-type grouping, cluster-wide reservation rate. Identifies misconfigured kube-reserved/system-reserved/eviction-threshold. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Reservation audit", map[string]interface{}{
				"score":   85,
				"summary": map[string]interface{}{"totalNodes": 5, "avgReservationPctCPU": 8.5, "overReservedNodes": 0},
			}),
		},
	})

	// --- Workload Replica Distribution & Anti-Affinity Coverage (v17.31) ---
	add("/api/product/replica-distribution", "get", OpenAPIOperation{
		Summary:     "Workload replica distribution & anti-affinity coverage analyzer",
		OperationID: "replicaDistribution",
		Tags:        []string{"Product", "HA", "Distribution"},
		Description: "Analyzes multi-replica workload spread across nodes: single-node concentration risk, insufficient spread detection, missing pod anti-affinity, per-node and per-zone pod distribution, spread scoring. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Replica distribution analysis", map[string]interface{}{
				"score":   82,
				"summary": map[string]interface{}{"totalWorkloads": 25, "goodSpread": 18, "poorSpread": 3, "atRiskCount": 2},
			}),
		},
	})

	// --- Cluster-Wide Service Dependency Topology & Cascade Risk Analyzer (v17.33) ---
	add("/api/product/service-topology", "get", OpenAPIOperation{
		Summary:     "Cluster-wide service dependency topology & cascade failure risk analyzer",
		OperationID: "serviceTopology",
		Tags:        []string{"Product", "AIOps", "Topology", "Dependencies"},
		Description: "Builds a cluster-wide service dependency graph by scanning all workloads (Deployments, StatefulSets, DaemonSets) for service DNS references in env vars. Calculates fan-in/fan-out per node, identifies critical hub services (high fan-in), detects single points of failure (non-HA services with multiple dependents), orphan services (no backing workload), isolated workloads (no dependencies), cross-namespace dependencies, and maximum dependency chain depth. Generates cascade failure risk assessment. AIOps core feature.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace (default: all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Service topology analysis", map[string]interface{}{
				"summary":      map[string]interface{}{"totalWorkloads": 30, "totalEdges": 45, "criticalNodes": 3, "crossNamespace": 5, "maxDepth": 4},
				"nodes":        []map[string]interface{}{{"id": "Service/prod/db", "fanIn": 8, "criticality": "critical"}},
				"edges":        []map[string]interface{}{{"from": "Deployment/prod/web", "to": "Service/prod/db", "type": "service-ref"}},
				"criticalHubs": []map[string]interface{}{{"name": "db", "fanIn": 8, "hasHA": false, "riskLevel": "critical"}},
			}),
		},
	})

	// --- Chaos Engineering Readiness Assessment (v17.34) ---
	add("/api/deployment/chaos-readiness", "get", OpenAPIOperation{
		Summary:     "Chaos engineering readiness assessment & experiment recommender",
		OperationID: "chaosReadiness",
		Tags:        []string{"Deployment", "AIOps", "Resilience", "Chaos Engineering"},
		Description: "Assesses every workload's resilience to chaos engineering experiments. Evaluates 6 readiness criteria: multi-replica HA, PDB coverage, health probes (liveness+readiness), graceful shutdown (PreStop hook + grace period), anti-affinity/topology spread, and multi-zone distribution. Assigns readiness level (ready/partial/fragile) with 0-100 score. Generates safe chaos experiment recommendations (pod-kill, network-latency, cpu-stress) for ready workloads. Calculates blast radius and max tolerable failures. AIOps core resilience feature.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace (default: all)"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Chaos readiness analysis", map[string]interface{}{
				"summary":          map[string]interface{}{"totalWorkloads": 30, "readyForChaos": 12, "fragileCount": 5, "readinessScore": 68},
				"workloads":        []map[string]interface{}{{"name": "api", "readinessLevel": "ready", "score": 85, "maxTolerableFailure": 1}},
				"experiments":      []map[string]interface{}{{"name": "pod-kill-api", "type": "pod-kill", "safe": true, "blastRadius": "small"}},
				"fragileWorkloads": []map[string]interface{}{{"name": "singleton", "score": 15, "readinessLevel": "fragile"}},
			}),
		},
	})

	// --- Cluster Carbon Footprint & Sustainability Analyzer (v17.35) ---
	add("/api/scalability/carbon-footprint", "get", OpenAPIOperation{
		Summary:     "Cluster carbon footprint & sustainability metrics analyzer",
		OperationID: "carbonFootprint",
		Tags:        []string{"Scalability", "FinOps", "Sustainability", "ESG"},
		Description: "Estimates cluster-wide energy consumption and carbon emissions. Detects cloud region from node metadata, maps to regional grid carbon intensity (gCO2/kWh). Calculates per-namespace and per-workload carbon attribution based on CPU/memory resource requests. Identifies carbon reduction opportunities: resource consolidation, workload right-sizing, green-hours scheduling, region relocation. Energy breakdown by component (CPU, memory, storage, network, PUE overhead). Green score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Carbon footprint analysis", map[string]interface{}{
				"summary":       map[string]interface{}{"totalPowerKW": 2.5, "monthlyCO2Kg": 612, "carbonIntensity": 340, "region": "AWS us-east-1", "wastedCO2KgMonth": 150},
				"byNamespace":   []map[string]interface{}{{"namespace": "prod", "monthlyCO2Kg": 250, "pctClusterTotal": 40}},
				"opportunities": []map[string]interface{}{{"type": "consolidate", "potentialSavingCO2KgMonth": 150, "severity": "high"}},
			}),
		},
	})

	// --- Admission Control Policy Gap & CEL Expression Auditor (v17.36) ---
	add("/api/security/admission-policy-audit", "get", OpenAPIOperation{
		Summary:     "Admission control policy gap & CEL expression auditor",
		OperationID: "admissionPolicyAudit",
		Tags:        []string{"Security", "Admission Control", "Policy"},
		Description: "Audits cluster admission control: validates webhook health (failurePolicy, sideEffects, timeout), detects OPA/Gatekeeper and Kyverno engines, calculates per-resource-type admission coverage, finds unprotected workloads, recommends CEL ValidatingAdmissionPolicies (K8s 1.30+) for lightweight enforcement without webhook servers.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Admission policy audit", map[string]interface{}{
				"summary":  map[string]interface{}{"totalValidatingWebhooks": 3, "coveragePercent": 65, "unprotectedWorkloads": 12},
				"webhooks": []map[string]interface{}{{"name": "pod-security", "failurePolicy": "Fail"}},
			}),
		},
	})

	// --- Pod Performance Anomaly & Noisy Neighbor Detector (v17.38) ---
	add("/api/operations/pod-anomaly", "get", OpenAPIOperation{
		Summary:     "Pod performance anomaly & noisy neighbor detector",
		OperationID: "podAnomaly",
		Tags:        []string{"Operations", "AIOps", "Anomaly Detection"},
		Description: "Detects pod performance anomalies by comparing pods against workload peers. Identifies outlier pods with significantly higher restart counts, noisy neighbors interfering with co-located workloads, and node hotspots with concentrated failures. Uses statistical peer comparison (variance analysis) to detect inconsistent replica behavior.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Pod anomaly analysis", map[string]interface{}{
				"summary":       map[string]interface{}{"analyzedPods": 120, "anomalousPods": 8, "noisyNodes": 2},
				"anomalousPods": []map[string]interface{}{{"name": "api-pod-3", "type": "outlier", "restartCount": 12}},
			}),
		},
	})

	// --- Cluster External Exposure Surface Risk Map (v17.39) ---
	add("/api/product/exposure-map", "get", OpenAPIOperation{
		Summary:     "Cluster external exposure surface risk map",
		OperationID: "exposureMap",
		Tags:        []string{"Product", "Security", "Network", "Attack Surface"},
		Description: "Maps the entire cluster's external attack surface by tracing all network entry points (Ingress, LoadBalancer, NodePort, ExternalIP) to their backing workloads. Identifies insecure endpoints (no TLS, no auth), sensitive paths (admin/debug/metrics), orphan exposure (no backing pods), and per-namespace exposure risk. Calculates cluster-wide exposure risk score.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Exposure surface analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"totalIngresses": 15, "withoutTLS": 3, "totalLoadBalancers": 2},
				"entryPoints": []map[string]interface{}{{"type": "ingress", "riskLevel": "high", "hasTLS": false}},
			}),
		},
	})

	// --- Rollback Risk & Revision Integrity Assessor (v17.41) ---
	add("/api/deployment/rollback-risk", "get", OpenAPIOperation{
		Summary:     "Rollback risk & revision integrity assessor",
		OperationID: "rollbackRisk",
		Tags:        []string{"Deployment", "AIOps", "Rollback"},
		Description: "Assesses rollback readiness for every workload. Checks revision history availability, image tag stability (:latest is risky), config dependency drift, replica count for zero-downtime rollback, and workload maturity. Identifies workloads where rollback would fail or cause downtime.",
		Parameters: []OpenAPIParam{
			queryParam("namespace", "Filter to specific namespace"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollback risk analysis", map[string]interface{}{
				"summary":   map[string]interface{}{"totalWorkloads": 30, "safeRollback": 15, "highRollbackRisk": 3},
				"workloads": []map[string]interface{}{{"name": "api", "riskLevel": "safe", "rollbackReady": true}},
			}),
		},
	})

	// --- Pod Lifecycle Stage Analyzer (v17.42) ---
	add("/api/operations/pod-lifecycle", "get", OpenAPIOperation{
		Summary:     "Pod lifecycle stage analyzer & dwell-time tracker",
		OperationID: "podLifecycle",
		Tags:        []string{"Operations", "AIOps", "Lifecycle"},
		Description: "Tracks pod lifecycle stages and dwell times. Identifies stuck pods (Pending >5min, Failed not cleaned), calculates P50/P90/P99 pending/creating/terminating durations, shows per-workload and per-node lifecycle distribution.",
		Parameters:  []OpenAPIParam{queryParam("namespace", "Filter")},
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Lifecycle analysis", map[string]interface{}{"summary": map[string]interface{}{"totalPods": 80, "running": 72}})},
	})

	// --- Workload Scaling Impact Simulator (v17.40) ---
	add("/api/scalability/scale-simulator", "get", OpenAPIOperation{
		Summary:     "Workload scaling impact simulator (what-if analysis)",
		OperationID: "scaleSimulator",
		Tags:        []string{"Scalability", "AIOps", "Capacity Planning"},
		Description: "Simulates the impact of scaling a workload to N replicas. Checks node capacity (CPU/memory), namespace ResourceQuota limits, pod count quotas, HPA alignment, and PDB constraints. Returns verdict (can-scale, risky, cannot-scale) with detailed checks and blockers.",
		Parameters: []OpenAPIParam{
			queryParam("workload", "Workload name (Deployment or StatefulSet)"),
			queryParam("namespace", "Namespace (default: default)"),
			queryParam("replicas", "Target replica count"),
		},
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scale simulation result", map[string]interface{}{
				"verdict": "can-scale",
				"checks":  []map[string]interface{}{{"name": "Node CPU Capacity", "status": "pass"}},
			}),
		},
	})

	// --- Cost Budget Alert & Namespace Spending Limit Auditor (v16.94) ---
	add("/api/scalability/budget-alert", "get", OpenAPIOperation{
		Summary:     "Cost budget alert & namespace spending limit auditor",
		OperationID: "budgetAlert",
		Tags:        []string{"Scalability", "FinOps", "Cost"},
		Description: "Audits cost budget alerts and namespace spending limits: per-namespace estimated monthly cost, budget annotation tracking (k8ops.io/monthly-budget), over-budget alerts, near-budget warnings (>80%), namespaces without budgets. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Budget alert analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNamespaces": 10, "nsOverBudget": 1, "totalEstCost": 500.0},
				"alerts":  []map[string]interface{}{{"namespace": "app-prod", "estCost": 120.0, "budget": 100.0, "severity": "critical"}},
			}),
		},
	})

	// --- Node Drain & Rotation Readiness Auditor (v17.00) ---
	add("/api/scalability/node-drain-readiness", "get", OpenAPIOperation{
		Summary:     "Node drain & rotation readiness auditor",
		OperationID: "nodeDrainReadiness",
		Tags:        []string{"Scalability", "NodeLifecycle", "Maintenance"},
		Description: "Audits per-node drain readiness for safe node rotation: identifies StatefulSet pods (PVC sticky), bare pods (will be lost), pods with local storage (data loss risk), PDB-protected pods, DaemonSet pods (won't move), cordoned nodes, single-replica workloads. Per-node drain safety classification (safe/risky/dangerous/cordoned). Health score (0-100). Blind spot: Node Lifecycle.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Node drain readiness analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalNodes": 5, "safeToDrain": 3, "riskyToDrain": 1, "dangerousToDrain": 1},
				"byNode":  []map[string]interface{}{{"nodeName": "node-1", "status": "safe", "drainable": true, "podCount": 12}},
			}),
		},
	})

	// --- Cluster Scaling History & Autoscaler Event Timeline Auditor (v17.06) ---
	add("/api/scalability/scaling-history", "get", OpenAPIOperation{
		Summary:     "Cluster scaling history & autoscaler event timeline auditor",
		OperationID: "scalingHistory",
		Tags:        []string{"Scalability", "Autoscaling", "Events"},
		Description: "Audits cluster scaling history from Kubernetes events: HPA scale-up/down events, cluster autoscaler node events, failed scaling operations, throttled scaling, hourly timeline. Per-action and per-namespace stats. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scaling history analysis", map[string]interface{}{
				"summary":  map[string]interface{}{"totalEvents": 15, "scaleUpEvents": 10, "scaleDownEvents": 5, "failedScales": 1},
				"timeline": []map[string]interface{}{{"hour": "14:00", "scaleUp": 3, "scaleDown": 1}},
			}),
		},
	})

	// --- Pod Resource Request Density & Scheduling Fit Auditor (v17.12) ---
	add("/api/scalability/scheduling-fit", "get", OpenAPIOperation{
		Summary:     "Pod resource request density & scheduling fit auditor",
		OperationID: "schedulingFit",
		Tags:        []string{"Scalability", "Scheduling", "Resources"},
		Description: "Audits pod resource request density vs node capacity: per-node packing %, over/under-provisioned pods, no-request pods, bin-packing efficiency. Fit category (underpacked/optimal/overpacked). Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Scheduling fit analysis", map[string]interface{}{
				"summary": map[string]interface{}{"avgNodePacking": 65.5, "overpackedNodes": 1, "noRequest": 3},
				"byNode":  []map[string]interface{}{{"nodeName": "node-1", "packingPct": 85.2, "fitCategory": "overpacked"}},
			}),
		},
	})

	// --- Namespace Resource Quota Saturation & Limit Exhaustion Predictor (v17.18) ---
	add("/api/scalability/quota-saturation", "get", OpenAPIOperation{
		Summary:     "Namespace resource quota saturation & limit exhaustion predictor",
		OperationID: "quotaSaturation",
		Tags:        []string{"Scalability", "Quota", "Capacity"},
		Description: "Predicts namespace quota exhaustion: per-resource saturation %, exhausted quotas (100%), critical saturation (>90%), high (>70%), namespaces without quota. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Quota saturation analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"nsWithQuota": 5, "exhaustedQuotas": 1, "criticalSaturation": 2},
				"byNamespace": []map[string]interface{}{{"namespace": "stressed", "maxSaturation": 100.0, "riskLevel": "critical"}},
			}),
		},
	})

	// --- Ingress TLS Certificate & HTTPS Enforcement Auditor (v16.95) ---
	add("/api/product/ingress-tls", "get", OpenAPIOperation{
		Summary:     "Ingress TLS certificate & HTTPS enforcement auditor",
		OperationID: "ingressTLS",
		Tags:        []string{"Product", "Ingress", "TLS"},
		Description: "Audits ingress TLS configuration and HTTPS enforcement: TLS coverage, cert-manager annotation tracking, HTTP->HTTPS redirect, TLS host mismatch detection, ingresses without TLS. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Ingress TLS analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalIngresses": 5, "withTLS": 3, "withoutTLS": 2},
			}),
		},
	})

	// --- East-West Traffic & Service-to-Service Connectivity Auditor (v17.01) ---
	add("/api/product/east-west-traffic", "get", OpenAPIOperation{
		Summary:     "East-west traffic & service-to-service connectivity auditor",
		OperationID: "eastWestTraffic",
		Tags:        []string{"Product", "Network", "ServiceMesh"},
		Description: "Audits east-west traffic: service exposure classification (ClusterIP/NodePort/LB/ExternalName/headless), network policy coverage, mesh sidecar coverage, cross-namespace access, publicly exposed services, per-namespace risk levels. Health score (0-100). Blind spot: Network/Service Mesh.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("East-west traffic analysis", map[string]interface{}{
				"summary":         map[string]interface{}{"totalServices": 20, "publiclyExposed": 2, "withoutNetworkPolicy": 5},
				"exposedServices": []map[string]interface{}{{"name": "web-svc", "type": "LoadBalancer", "riskLevel": "critical"}},
			}),
		},
	})

	// --- Container Port Exposure & Named Port Consistency Auditor (v17.07) ---
	add("/api/product/port-exposure", "get", OpenAPIOperation{
		Summary:     "Container port exposure & named port consistency auditor",
		OperationID: "portExposure",
		Tags:        []string{"Product", "Network", "Ports"},
		Description: "Audits container port configuration: hostPort conflicts, unnamed ports, hostPort usage risk, port naming consistency. Per-workload port mapping. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Port exposure analysis", map[string]interface{}{
				"summary":   map[string]interface{}{"totalContainers": 10, "withHostPort": 2, "hostPortConflicts": 1},
				"conflicts": []map[string]interface{}{{"port": 8080, "type": "host-port-conflict", "severity": "critical"}},
			}),
		},
	})

	// --- Service Endpoint vs Pod Readiness Mismatch Auditor (v17.13) ---
	add("/api/product/endpoint-mismatch", "get", OpenAPIOperation{
		Summary:     "Service endpoint vs pod readiness mismatch auditor",
		OperationID: "endpointMismatch",
		Tags:        []string{"Product", "Service", "Endpoints"},
		Description: "Audits service endpoint vs pod readiness: dead services (no ready endpoints), stale endpoints (endpoint/pod mismatch), selector matching, per-namespace stats. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Endpoint mismatch analysis", map[string]interface{}{
				"summary":        map[string]interface{}{"totalServices": 20, "deadServices": 2, "mismatchedServices": 1},
				"mismatchedSvcs": []map[string]interface{}{{"name": "api", "readyEndpoints": 1, "readyPods": 2}},
			}),
		},
	})

	// --- Deployment Env Config Drift & ConfigMap/Secret Reference Auditor (v16.96) ---
	add("/api/deployment/env-config-drift", "get", OpenAPIOperation{
		Summary:     "Deployment env config drift & ConfigMap/Secret reference auditor",
		OperationID: "envConfigDrift",
		Tags:        []string{"Deployment", "Config", "Drift"},
		Description: "Audits deployment environment configuration drift: missing ConfigMap/Secret references, hardcoded secrets in env vars, ConfigMap/Secret ref validation, env var count per deployment. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Env config drift analysis", map[string]interface{}{
				"summary": map[string]interface{}{"totalDeployments": 10, "missingRefs": 2, "hardcodedSecrets": 1},
			}),
		},
	})

	// --- Deployment Reproducibility & CI/CD Traceability Auditor (v17.02) ---
	add("/api/deployment/traceability", "get", OpenAPIOperation{
		Summary:     "Deployment reproducibility & CI/CD traceability auditor",
		OperationID: "deployTraceability",
		Tags:        []string{"Deployment", "GitOps", "CI/CD"},
		Description: "Audits deployment CI/CD traceability: version labels (app.kubernetes.io/version), git-commit annotations, build-timestamp, image digest pinning (@sha256), managed-by/part-of/created-by labels. Per-workload traceability score (0-100), missing field detection, full-trace vs no-trace classification. Health score (0-100). Blind spot: GitOps/CD.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Deployment traceability analysis", map[string]interface{}{
				"summary":    map[string]interface{}{"totalWorkloads": 15, "withFullTrace": 5, "withNoTrace": 3},
				"byWorkload": []map[string]interface{}{{"name": "api", "score": 85, "missingFields": []string{"build-time"}}},
			}),
		},
	})

	// --- Pod Termination Message & Exit Code Pattern Auditor (v17.08) ---
	add("/api/deployment/termination-audit", "get", OpenAPIOperation{
		Summary:     "Pod termination message & exit code pattern auditor",
		OperationID: "terminationAudit",
		Tags:        []string{"Deployment", "Reliability", "Diagnostics"},
		Description: "Audits pod termination patterns: OOMKilled detection, signal terminations (SIGKILL/SIGTERM), non-zero exit codes, termination message coverage, high restart count, exit code distribution, recurring termination patterns. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Termination audit analysis", map[string]interface{}{
				"summary":   map[string]interface{}{"terminatedPods": 5, "oomKilledCount": 2, "nonZeroExitCount": 3},
				"exitCodes": []map[string]interface{}{{"exitCode": 137, "count": 2, "reason": "OOMKilled"}},
			}),
		},
	})

	// --- Pod Readiness Gate Compliance & Custom Condition Auditor (v17.14) ---
	add("/api/deployment/deploy-readiness-gate", "get", OpenAPIOperation{
		Summary:     "Pod readiness gate compliance & custom condition auditor",
		OperationID: "readinessGate",
		Tags:        []string{"Deployment", "Reliability", "Readiness"},
		Description: "Audits pod readiness gates: detects workloads using custom readiness gates, blocked pods (gate condition False/Unknown), unknown gate conditions (no controller), per-namespace stats. Health score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Readiness gate analysis", map[string]interface{}{
				"summary":     map[string]interface{}{"withReadinessGates": 2, "gateBlockedPods": 1, "gateConditions": 3},
				"blockedPods": []map[string]interface{}{{"name": "app-1", "condition": "myapp/ready", "status": "Unknown"}},
			}),
		},
	})

	// --- Cluster Predictive Health & Risk Forecast Engine (v17.52) ---
	add("/api/operations/predictive-health", "get", OpenAPIOperation{
		Summary:     "Cluster predictive health & risk forecast engine",
		OperationID: "predictiveHealth",
		Tags:        []string{"Operations", "AIOps", "Predictive"},
		Description: "Predicts cluster risks before they impact. Analyzes node conditions (MemoryPressure, DiskPressure, PIDPressure), pod restart patterns, resource consumption trends, certificate expiry timelines, and capacity utilization to forecast issues across a 30-day horizon. Per-node risk scoring (0-100) with failure risk prediction. Per-pod risk classification (restart-loop, resource-starvation, eviction-risk). Resource trends with projected exhaustion dates. Risk timeline bucketed by ETA (<24h, 1-7d, 7-30d, >30d). Confidence score based on data completeness.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Predictive health forecast", map[string]interface{}{
				"summary":          map[string]interface{}{"totalNodes": 3, "criticalPredictions": 1, "highPredictions": 2, "nodesAtRisk": 2},
				"overallRiskLevel": "high",
				"confidenceScore":  80,
				"predictions":      []map[string]interface{}{{"category": "node-failure", "severity": "critical", "resource": "worker-3", "eta": "~5 days"}},
				"nodeRisks":        []map[string]interface{}{{"nodeName": "worker-3", "riskScore": 65, "memoryRisk": "critical"}},
				"riskTimeline":     []map[string]interface{}{{"when": "< 24h", "count": 1, "severity": "critical"}},
			}),
		},
	})

	// --- Deployment Change Readiness Pre-Flight Gate (v17.53) ---
	add("/api/deployment/change-readiness", "get", OpenAPIOperation{
		Summary:     "Deployment change readiness pre-flight gate",
		OperationID: "changeReadiness",
		Tags:        []string{"Deployment", "CI/CD", "Gate"},
		Description: "Pre-flight gate that evaluates whether the cluster is safe for new deployments. Runs 8 checks: node stability (no pressure conditions), active rollouts (<3 concurrent), failed pods (<10 crash-looping), PDB coverage (>50%), capacity headroom (<85% utilized), rollback path (RevisionHistoryLimit > 0), resource limits (containers have CPU/memory limits), health probes (readiness probes configured). Returns gate decision (proceed / proceed-with-caution / blocked), readiness score (0-100), detailed check results, blockers, warnings, recent failures, and capacity metrics. Designed for CI/CD pipeline integration as a deployment gate.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Change readiness assessment", map[string]interface{}{
				"gateDecision":     "proceed",
				"readinessScore":   95,
				"summary":          map[string]interface{}{"totalChecks": 8, "passed": 8, "failed": 0, "warnings": 0},
				"checks":           []map[string]interface{}{{"name": "node-stability", "category": "stability", "status": "pass"}},
				"capacityHeadroom": map[string]interface{}{"totalPodSlots": 110, "usedPodSlots": 45, "available": 65, "utilization": 40.9},
			}),
		},
	})

	// --- Resource Request Intelligence & Right-Sizing Engine (v17.54) ---
	add("/api/scalability/request-intelligence", "get", OpenAPIOperation{
		Summary:     "Resource request intelligence & right-sizing engine",
		OperationID: "requestIntelligence",
		Tags:        []string{"Scalability", "FinOps", "Right-Sizing"},
		Description: "Analyzes resource request right-sizing using multi-signal proxy analysis. Detects over-provisioned workloads (round-number requests on stable pods → potential 30% waste), under-provisioned workloads (OOMKill/restart-loop signals → failure risk), and missing-request workloads. Per-workload verdict (over/under-provisioned/optimal/no-requests) with specific CPU/memory recommendations and confidence levels. Quantifies monthly cost savings ($30/core, $4/GB cloud pricing), estimated node reduction, and risk assessment (OOM/throttle predictions). Cross-cutting insights and posture score (0-100).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Request intelligence report", map[string]interface{}{
				"postureScore":    75,
				"summary":         map[string]interface{}{"totalWorkloads": 30, "overProvisioned": 8, "underProvisioned": 3, "noRequests": 5},
				"savingsEstimate": map[string]interface{}{"monthlyTotal": 240.50, "nodesReduction": 1},
				"overProvisioned": []map[string]interface{}{{"name": "webapp", "verdict": "over-provisioned", "cpuRequestMillicores": 2000, "cpuRecommendMillicores": 1400}},
			}),
		},
	})

	// --- Per-Workload Reliability Scorecard (v17.55) ---
	add("/api/product/reliability-scorecard", "get", OpenAPIOperation{
		Summary:     "Per-workload reliability posture scorecard",
		OperationID: "reliabilityScorecard",
		Tags:        []string{"Product", "Reliability", "Scorecard"},
		Description: "Scores every workload (Deployment, StatefulSet, DaemonSet) across 7 reliability dimensions: replication (HA), probes (readiness/liveness/startup), resources (requests/limits), PDB coverage, security context (non-root/read-only), update strategy (rolling vs recreate), and affinity/topology spread. Each workload receives an A-F grade and 0-100 score. Cluster-wide grade and weakest signal analysis. Excludes kube-system namespaces.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Reliability scorecard", map[string]interface{}{
				"clusterGrade": "B",
				"clusterScore": 82,
				"workloads":    []map[string]interface{}{{"name": "api-server", "grade": "A", "score": 92}},
			}),
		},
	})

	// --- Cluster Security Posture Scorecard (v17.56) ---
	add("/api/security/posture-scorecard", "get", OpenAPIOperation{
		Summary: "Cluster-wide security posture scorecard", OperationID: "securityPosture", Tags: []string{"Security", "Posture", "Scorecard"},
		Description: "Comprehensive security posture across 5 dimensions: pod-security, host-access, network-isolation, resource-boundaries, attack-surface. Per-workload risk scoring with violation tracking. Attack surface quantification (host paths, cap escalation, SA token exposure, unrestricted egress). A-F cluster grade.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Security posture scorecard", map[string]interface{}{"clusterGrade": "C", "clusterScore": 72})},
	})

	// --- AIOps Incident Triage & Remediation Action Plan (v17.58) ---
	add("/api/operations/triage", "get", OpenAPIOperation{
		Summary: "AIOps incident triage & remediation action plan", OperationID: "triageReport", Tags: []string{"Operations", "AIOps", "Triage"},
		Description: "Correlates signals across dimensions (crash loops, node pressure, image failures, stuck rollouts, event storms) into prioritized incidents (P0-P3). Generates action plan with kubectl commands, effort estimates, and impact ratings. Separates quick wins from long-term fixes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Triage report", map[string]interface{}{"priority": "P1-urgent", "healthScore": 65})},
	})

	// --- Deployment Impact Simulator (v17.59) ---
	add("/api/deployment/impact-simulator", "get", OpenAPIOperation{
		Summary: "Deployment impact simulator & blast radius predictor", OperationID: "deployImpact", Tags: []string{"Deployment", "Simulation", "Risk"},
		Description: "Simulates deployment impact: single-replica risk, PDB coverage, dependent service blast radius, node co-location, cascade chain analysis. Ranks workloads by deployment risk (1=most risky). Per-workload impact level, estimated downtime, blockers, and mitigations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Impact simulation", map[string]interface{}{"clusterRiskLevel": "medium"})},
	})

	// --- Cost Intelligence & Spend Forecast Engine (v17.60) ---
	add("/api/scalability/cost-intelligence", "get", OpenAPIOperation{
		Summary:     "Cost intelligence & spend forecast engine",
		OperationID: "costIntelligence",
		Tags:        []string{"Scalability", "FinOps", "Cost", "Forecasting"},
		Description: "Advanced FinOps intelligence layer beyond static cost snapshots. Provides: (1) per-namespace cost trend analysis with spend velocity tracking (increasing/stable), (2) cost anomaly detection (concentration spikes, over-request ratios, idle waste, runaway growth), (3) monthly spend forecasting with growth rate estimation and budget recommendations, (4) ranked optimization opportunities by estimated annual savings (right-size, remove-idle, consolidate, spot-migrate), (5) FinOps maturity scorecard grading (A-F) across visibility, optimization, budget enforcement, efficiency, and allocation dimensions. Synthesizes data from all cost-related signals into actionable intelligence.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Cost intelligence report", map[string]interface{}{
				"summary":          map[string]interface{}{"monthlySpend": 1200.50, "annualProjection": 14406, "topNSPctOfSpend": 65.2},
				"forecast":         map[string]interface{}{"projectedMonthly": 1380, "growthRate": 15.0, "confidence": "high"},
				"finOpsScore":      map[string]interface{}{"grade": "C", "score": 72, "visibilityScore": 85, "budgetScore": 20},
				"topOpportunities": []map[string]interface{}{{"rank": 1, "type": "spot-migrate", "estimatedSavingsAnnual": 5760}},
			}),
		},
	})

	// --- SRE Four Golden Signals Unified Health Engine (v17.61) ---
	add("/api/product/golden-signals", "get", OpenAPIOperation{
		Summary:     "SRE four golden signals unified health engine",
		OperationID: "goldenSignals",
		Tags:        []string{"Product", "SRE", "Health", "Monitoring"},
		Description: "Synthesizes the four SRE golden signals (Latency, Traffic, Errors, Saturation) into a unified health view. Each signal is scored 0-100 with supporting metrics. Overall score follows the weakest-link principle (minimum of all signals). Detects cross-signal compound failure patterns (silent failures, cascading failures, low serving capacity, namespace hotspots). Per-namespace signal scores for targeted investigation. Actionable recommendations prioritized by weakest signal.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Golden signals health report", map[string]interface{}{
				"overallScore": 72,
				"overallGrade": "C",
				"signals":      []map[string]interface{}{{"name": "latency", "score": 85, "status": "healthy"}, {"name": "errors", "score": 72, "status": "warning"}},
				"topIssues":    []map[string]interface{}{{"severity": "critical", "title": "Silent failure pattern detected"}},
			}),
		},
	})

	// --- Security Remediation Priority Matrix (v17.62) ---
	add("/api/security/remediation-matrix", "get", OpenAPIOperation{
		Summary:     "Security remediation priority & risk-effort matrix",
		OperationID: "remediationMatrix",
		Tags:        []string{"Security", "Remediation", "Risk Management"},
		Description: "Collects security findings from the live cluster, scores them using CVSS-like methodology (0-100), and prioritizes remediation by risk × effort. Categories: privileged containers (critical, 95), root containers (high, 70), dangerous capabilities (high, 75), host namespaces (high, 72), missing NetworkPolicy (high, 65), mutable image tags (medium, 42), missing resource limits (medium, 40), unused SA tokens (medium, 45), missing PSA labels (medium, 38). Separates findings into quick wins (high risk, fixable in <1 hour) and strategic fixes. Provides ordered remediation plan (top 15 actions) and category risk aggregation.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Remediation matrix", map[string]interface{}{
				"summary":         map[string]interface{}{"totalFindings": 25, "criticalCount": 2, "quickWinCount": 8},
				"quickWins":       []map[string]interface{}{{"id": "F001", "severity": "critical", "riskScore": 95}},
				"remediationPlan": []map[string]interface{}{{"priority": 1, "action": "Set privileged: false"}},
			}),
		},
	})

	// --- MTTR & Incident Lifecycle Analytics (v17.63) ---
	add("/api/operations/mttr", "get", OpenAPIOperation{
		Summary:     "Mean time to recovery & incident lifecycle analytics",
		OperationID: "mttrAnalytics",
		Tags:        []string{"Operations", "SRE", "Incident Management"},
		Description: "Estimates MTTD, MTTR, incident frequency, and recovery effectiveness from pod restart patterns, container state transitions, and event history. Tracks OOMKill and CrashLoopBackOff recovery times. Detects restart bursts, hourly incident patterns (peak hours), and per-namespace stability scores. Provides trend analysis (improving/stable/degrading) and actionable recovery recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("MTTR report", map[string]interface{}{
				"summary":           map[string]interface{}{"totalRestarts": 45, "stabilityScore": 72},
				"mttrEstimate":      map[string]interface{}{"estMTTR": "5.2m", "confidence": "medium"},
				"incidentFrequency": map[string]interface{}{"incidentsPerDay": 12.5, "burstDetected": true},
			}),
		},
	})

	// --- Rollout Failure Forensics (v17.64) ---
	add("/api/deployment/rollout-forensics", "get", OpenAPIOperation{
		Summary:     "Rollout failure forensics & deployment pattern detector",
		OperationID: "rolloutForensics",
		Tags:        []string{"Deployment", "Forensics", "Reliability"},
		Description: "Correlates deployment state, pod conditions, and restart patterns to identify systematic rollout risks and deployment anti-patterns. Per-workload rollout reliability scoring (A-F). Detects: missing probes, Recreate strategy, single-replica, missing resources, no revision history, CrashLoopBackOff, stalled rollouts. Cluster-level risk factors and prioritized recommendations.",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Rollout forensics", map[string]interface{}{
				"summary":          map[string]interface{}{"totalDeployments": 50, "failed": 2, "highRiskCount": 8},
				"antiPatterns":     []map[string]interface{}{{"type": "no-readiness-probe", "affectedCount": 15}},
				"reliabilityScore": []map[string]interface{}{{"name": "api", "score": 45, "grade": "D"}},
			}),
		},
	})

	// --- Resource Governance (v17.75) ---
	add("/api/deployment/resource-governance", "get", OpenAPIOperation{
		Summary:     "Resource governance & namespace quota effectiveness",
		OperationID: "resourceGovernance",
		Tags:        []string{"Deployment", "Governance", "ResourceQuota"},
		Description: "Analyzes namespace resource governance: ResourceQuota coverage, LimitRange defaults, quota utilization, and policy enforcement gaps. Identifies ungoverned namespaces where pods can consume unlimited resources. Governance score (0-100, A-F grading).",
		Responses: map[string]OpenAPIResponse{
			"200": okResponse("Resource governance", map[string]interface{}{
				"summary":              map[string]interface{}{"totalNamespaces": 28, "nsWithQuota": 2, "nsWithoutQuota": 26},
				"ungovernedNamespaces": []map[string]interface{}{{"namespace": "default", "podCount": 5, "severity": "high"}},
			}),
		},
	})

	// --- Autoscaling Intelligence (v17.65) ---
	add("/api/scalability/autoscaling-intel", "get", OpenAPIOperation{
		Summary:     "Autoscaling intelligence & scaling behavior profiler",
		OperationID: "autoscalingIntel",
		Tags:        []string{"Scalability", "HPA", "Autoscaling"},
		Description: "Analyzes HPA coverage, scaling gaps, misconfigured HPAs, and provides tuning advice. Per-workload scaling profiles with verdicts (optimal/misconfigured/no-autoscaling).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Autoscaling report", map[string]interface{}{"summary": map[string]interface{}{"hpaCoverage": 35.2, "intelScore": 52}})},
	})

	// --- Ownership Map (v17.66) ---
	add("/api/product/ownership-map", "get", OpenAPIOperation{
		Summary:     "Workload ownership & accountability governance engine",
		OperationID: "ownershipMap",
		Tags:        []string{"Product", "Governance", "Ownership"},
		Description: "Maps team ownership of workloads, detects orphaned resources lacking ownership metadata, scores accountability, and tracks label coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ownership map", map[string]interface{}{"summary": map[string]interface{}{"coveragePct": 45.2, "accountabilityScore": 38}})},
	})

	// --- Platform Maturity (v17.67) ---
	add("/api/docs/platform-maturity", "get", OpenAPIOperation{
		Summary:     "Platform maturity assessment & capability matrix",
		OperationID: "platformMaturity",
		Tags:        []string{"Documentation", "Maturity", "Meta"},
		Description: "CMMI-style platform maturity assessment across six dimensions. Scores each dimension (0-100), maps blind spot coverage, identifies capability gaps, and generates a prioritized evolution roadmap.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Maturity report", map[string]interface{}{"overallScore": 78, "overallLevel": "defined"})},
	})

	// --- Compliance Posture (v17.68) ---
	add("/api/security/compliance-posture", "get", OpenAPIOperation{
		Summary:     "Multi-framework compliance posture & control mapping",
		OperationID: "compliancePosture",
		Tags:        []string{"Security", "Compliance", "Governance"},
		Description: "Maps cluster security state against SOC2, PCI-DSS, HIPAA, NIST 800-53, and GDPR. Cross-references findings with framework control families, identifies cross-framework gaps, and generates prioritized remediation plan.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Compliance posture", map[string]interface{}{"overallScore": 72, "frameworks": []interface{}{map[string]interface{}{"framework": "SOC2", "score": 80}}})},
	})

	// --- Observability Coverage (v17.69) ---
	add("/api/operations/obs-coverage", "get", OpenAPIOperation{
		Summary:     "Observability coverage & blind spot detector",
		OperationID: "obsCoverage",
		Tags:        []string{"Operations", "Observability", "Monitoring"},
		Description: "Identifies workloads flying blind — no monitoring, tracing, dashboards, runbooks, or alerts. Scores signal coverage across 5 dimensions per workload and cluster-wide.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Coverage report", map[string]interface{}{"summary": map[string]interface{}{"blindCount": 12, "signalQuality": "poor"}})},
	})

	// --- Config Consistency (v17.70) ---
	add("/api/deployment/config-consistency", "get", OpenAPIOperation{
		Summary:     "Configuration consistency & standardization auditor",
		OperationID: "configConsistency",
		Tags:        []string{"Deployment", "Configuration", "Governance"},
		Description: "Detects configuration drift across workloads, identifies non-conformant patterns (missing probes, resources, labels, security contexts), analyzes image registry distribution and resource tier patterns, and scores standardization maturity.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Consistency report", map[string]interface{}{"consistencyScore": 62, "grade": "D"})},
	})

	// --- Scheduling Intelligence (v17.71) ---
	add("/api/scalability/scheduling-intel", "get", OpenAPIOperation{
		Summary:     "Scheduling intelligence & bin-packing efficiency analyzer",
		OperationID: "schedulingIntel",
		Tags:        []string{"Scalability", "Scheduling", "BinPacking"},
		Description: "Analyzes node bin-packing efficiency, resource fragmentation, scheduling bottlenecks, and stranded resources. Per-node packing analysis with standard pod fit assessment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scheduling report", map[string]interface{}{"schedulingScore": 72, "summary": map[string]interface{}{"binPackScore": 72, "fragileNodes": 1}})},
	})

	// --- Dependency Resilience (v17.72) ---
	add("/api/product/dependency-resilience", "get", OpenAPIOperation{
		Summary:     "Service dependency resilience & cascade failure risk analyzer",
		OperationID: "dependencyResilience",
		Tags:        []string{"Product", "Resilience", "Dependencies"},
		Description: "Analyzes service-to-service dependency chains, identifies cascade failure risks, orphaned services, single-pod bottlenecks, and missing resilience patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resilience report", map[string]interface{}{"resilienceScore": 72, "grade": "C"})},
	})

	// --- Change Intelligence (v17.73) ---
	add("/api/operations/change-intel", "get", OpenAPIOperation{
		Summary:     "Change intelligence & blast radius analyzer",
		OperationID: "changeIntel",
		Tags:        []string{"Operations", "Change Management", "Risk"},
		Description: "Correlates recent cluster changes with health signals to identify change-induced degradation. Provides blast radius analysis, change velocity tracking, and change freeze recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Change intelligence", map[string]interface{}{"summary": map[string]interface{}{"totalChanges": 12, "riskyChangeCount": 2}})},
	})

	// --- Net Policy Effectiveness (v17.74) ---
	add("/api/security/net-policy-effectiveness", "get", OpenAPIOperation{
		Summary:     "Network policy effectiveness & zero-trust isolation scorer",
		OperationID: "netPolicyEffectiveness",
		Tags:        []string{"Security", "NetworkPolicy", "ZeroTrust"},
		Description: "Analyzes network policy effectiveness, namespace isolation, default-deny coverage, egress control, and zero-trust posture across the cluster.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Policy report", map[string]interface{}{"isolationScore": 25, "zeroTrustLevel": "low"})},
	})

	// --- Service Mesh Readiness (v17.76) ---
	add("/api/product/mesh-readiness", "get", OpenAPIOperation{
		Summary:     "Service mesh readiness & mTLS coverage gap analyzer",
		OperationID: "meshReadiness",
		Tags:        []string{"Product", "ServiceMesh", "mTLS"},
		Description: "Analyzes service mesh readiness: sidecar injection status, mTLS coverage, traffic management policy gaps (circuit breaker, retry, timeout). Detects Istio/Linkerd mesh presence and identifies unmeshed services lacking resilience policies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Mesh readiness", map[string]interface{}{"readinessScore": 0, "meshDetected": false, "mtlsCoverage": map[string]interface{}{"score": 0, "status": "no-mesh"}})},
	})

	// --- Idle Waste Detection (v17.77) ---
	add("/api/scalability/idle-waste", "get", OpenAPIOperation{
		Summary:     "Idle resource waste quantification & cost recovery",
		OperationID: "idleWaste",
		Tags:        []string{"Scalability", "FinOps", "Cost"},
		Description: "Detects and quantifies idle resource waste: zero-replica workloads, unmounted PVCs, LoadBalancer services. Estimates monthly waste cost and provides resource efficiency scoring (0-100, A-F).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Waste report", map[string]interface{}{"wasteScore": 75, "estimatedWaste": map[string]interface{}{"totalMonthly": 45.5}})},
	})

	// --- Policy Governance (v17.78) ---
	add("/api/security/policy-governance", "get", OpenAPIOperation{
		Summary:     "Admission policy governance & enforcement auditor",
		OperationID: "policyGovernance",
		Tags:        []string{"Security", "Compliance", "OPA", "Gatekeeper"},
		Description: "Analyzes admission policy governance: OPA Gatekeeper/Kyverno installation status, Pod Security Admission (PSA) label coverage, and policy enforcement gaps across namespaces. Enforcement scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Policy governance", map[string]interface{}{"enforcementScore": 0, "gatekeeperStatus": "not-installed", "psaCoverage": map[string]interface{}{"coveragePct": 0}})},
	})

	// --- API Quality (v17.79) ---
	add("/api/docs/api-quality", "get", OpenAPIOperation{
		Summary:     "Platform API endpoint quality & coverage gap analyzer",
		OperationID: "apiQuality",
		Tags:        []string{"Documentation", "Meta", "Quality"},
		Description: "Meta-analysis of platform API coverage by dimension: endpoint counts, coverage percentages, gap detection, and quality scoring. Identifies weakest dimensions and suggests areas for improvement.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API quality report", map[string]interface{}{"qualityScore": 85, "avgCoverage": 82.5, "byDimension": []interface{}{}})},
	})

	// --- Observability Cardinality (v17.80) ---
	add("/api/operations/obs-cardinality", "get", OpenAPIOperation{
		Summary:     "Observability data cardinality & volume cost analyzer",
		OperationID: "obsCardinality",
		Tags:        []string{"Operations", "Observability", "FinOps"},
		Description: "Analyzes observability data cardinality risk: Prometheus metric label explosion, log volume per namespace, collector health, and data pipeline cost estimation. Identifies high-cardinality label risks and log collection blind spots.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cardinality report", map[string]interface{}{"riskScore": 50, "grade": "D", "estMonthlyCost": 15.5})},
	})

	// --- GitOps Drift (v17.81) ---
	add("/api/deployment/gitops-drift", "get", OpenAPIOperation{
		Summary:     "GitOps sync health & configuration drift analyzer",
		OperationID: "gitOpsDrift",
		Tags:        []string{"Deployment", "GitOps", "Drift"},
		Description: "Deeply analyzes GitOps sync health: ArgoCD/Flux controller detection, Helm release tracking, manual deployment detection, ConfigMap staleness, and drift scoring. Identifies workloads not managed by GitOps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("GitOps drift report", map[string]interface{}{"driftScore": 60, "grade": "D", "summary": map[string]interface{}{"hasArgoCD": false, "manualChanges": 10}})},
	})

	// --- API Version Governance (v17.82) ---
	add("/api/product/api-version-governance", "get", OpenAPIOperation{
		Summary:     "K8s API version governance & deprecation tracker",
		OperationID: "apiVersionGovernance",
		Tags:        []string{"Product", "API", "Upgrade"},
		Description: "Analyzes Kubernetes API version governance: deprecated/removed API usage, alpha/beta API detection, CRD version health, and upgrade readiness assessment. Governance scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API version report", map[string]interface{}{"governanceScore": 95, "upgradeReadiness": "ready"})},
	})

	// --- Secret Lifecycle (v17.83) ---
	add("/api/security/secret-lifecycle", "get", OpenAPIOperation{
		Summary:     "Secret management lifecycle & rotation tracker",
		OperationID: "secretLifecycle",
		Tags:        []string{"Security", "Secrets", "Rotation"},
		Description: "Analyzes secret management lifecycle: secret age, rotation compliance, plaintext detection, secret sprawl across namespaces, and unused secret identification. Lifecycle scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret lifecycle report", map[string]interface{}{"lifecycleScore": 55, "grade": "D"})},
	})

	// --- DR Backup Verify (v17.84) ---
	add("/api/scalability/dr-backup-verify", "get", OpenAPIOperation{
		Summary:     "Disaster recovery & backup verification assessor",
		OperationID: "drBackupVerify",
		Tags:        []string{"Scalability", "DR", "Backup"},
		Description: "Analyzes DR readiness: backup tool detection (Velero/K8up/Longhorn), namespace backup coverage, unprotected PVC identification, RPO/RTO estimation, and restore readiness assessment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DR report", map[string]interface{}{"readinessScore": 0, "drReadiness": "not-ready", "estRPO": "unknown"})},
	})

	// --- Training Readiness (v17.85) ---
	add("/api/docs/training-readiness", "get", OpenAPIOperation{
		Summary:     "Platform onboarding & documentation quality assessor",
		OperationID: "trainingReadiness",
		Tags:        []string{"Documentation", "Onboarding"},
		Description: "Assesses onboarding quality: owner/team/docs/runbook label coverage on workloads, documentation completeness, and team knowledge transfer readiness. Scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Training readiness", map[string]interface{}{"onboardingScore": 25})},
	})

	// --- Certificate Expiry (v17.86) ---
	add("/api/operations/cert-expiry", "get", OpenAPIOperation{
		Summary:     "TLS certificate expiry & lifecycle monitor",
		OperationID: "certExpiry",
		Tags:        []string{"Operations", "Certificate", "TLS"},
		Description: "Monitors TLS certificate lifecycle: parses all kubernetes.io/tls secrets, checks expiry dates, identifies expired/expiring certs, detects self-signed certificates. Health scoring (0-100, A-F).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Certificate report", map[string]interface{}{"healthScore": 85, "summary": map[string]interface{}{"totalCerts": 60}})},
	})

	// --- Supply Chain (v17.87) ---
	add("/api/security/image-supply-chain", "get", OpenAPIOperation{
		Summary:     "Container image supply chain security scanner",
		OperationID: "supplyChain",
		Tags:        []string{"Security", "SupplyChain", "Image"},
		Description: "Analyzes supply chain security: registry trust, image digest pinning, :latest tag usage, pull policy compliance, unknown registry detection. Security scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Supply chain report", map[string]interface{}{"securityScore": 40})},
	})

	// --- Node OS Drift (v17.88) ---
	add("/api/scalability/node-os-drift", "get", OpenAPIOperation{
		Summary:     "Node OS lifecycle & kernel drift deep analyzer",
		OperationID: "nodeOSDrift",
		Tags:        []string{"Scalability", "Node", "OS"},
		Description: "Deeply analyzes node OS lifecycle: kernel version drift, OS image consistency, container runtime versions, node age, GPU availability, and rotation readiness. Health scoring (0-100, A-F).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node OS report", map[string]interface{}{"healthScore": 70})},
	})

	// --- Traffic Flow (v17.89) ---
	add("/api/product/traffic-flow", "get", OpenAPIOperation{
		Summary:     "East-west traffic flow & service communication map",
		OperationID: "trafficFlow",
		Tags:        []string{"Product", "Traffic", "Networking"},
		Description: "Analyzes east-west service communication: service exposure levels, endpoint health, isolated/orphaned service detection, LoadBalancer/NodePort audit. Flow scoring (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Traffic flow report", map[string]interface{}{"flowScore": 75})},
	})

	// --- Pipeline Health (v17.90) ---
	add("/api/deployment/pipeline-health", "get", OpenAPIOperation{
		Summary:     "CI/CD pipeline health & DORA maturity analyzer",
		OperationID: "pipelineHealth",
		Tags:        []string{"Deployment", "CI/CD", "DORA"},
		Description: "Analyzes deployment pipeline health: deploy frequency, change failure rate, rollback patterns, CI/CD controller detection, and DORA maturity classification.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pipeline health", map[string]interface{}{"healthScore": 70, "doraLevel": "Medium"})},
	})

	// --- Alert Rule Quality (v17.91) ---
	add("/api/operations/alert-rule-quality", "get", OpenAPIOperation{
		Summary:     "Alerting rule quality & coverage gap analyzer",
		OperationID: "alertRuleQuality",
		Tags:        []string{"Operations", "Alerting", "Observability"},
		Description: "Analyzes alerting rule quality: Prometheus/Alertmanager detection, rule count, workload alerting coverage, noise risk detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Alert quality", map[string]interface{}{"qualityScore": 0, "totalRules": 0})},
	})

	// --- Chargeback (v17.92) ---
	add("/api/scalability/chargeback", "get", OpenAPIOperation{
		Summary:     "Cost chargeback & team budget allocation report",
		OperationID: "chargeback",
		Tags:        []string{"Scalability", "FinOps", "Cost"},
		Description: "Detailed cost chargeback: per-namespace cost breakdown, shared infrastructure cost, waste cost, team-level budget allocation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Chargeback", map[string]interface{}{"totalMonthlyCost": 150.0, "namespaceCount": 28})},
	})

	// --- Runtime Threat (v17.93) ---
	add("/api/security/runtime-scan", "get", OpenAPIOperation{
		Summary:     "Runtime threat detection & behavioral anomaly scanner",
		OperationID: "runtimeThreat",
		Tags:        []string{"Security", "Runtime", "Threat"},
		Description: "Analyzes runtime security threats: privileged pods, host namespace access, hostPath mounts, dangerous capabilities, and run-as-root detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Threat report", map[string]interface{}{"threatScore": 55})},
	})

	// --- Executive Dashboard (v17.94) ---
	add("/api/docs/exec-dashboard", "get", OpenAPIOperation{
		Summary:     "Executive platform health summary & scorecard",
		OperationID: "execDashboard",
		Tags:        []string{"Documentation", "Executive", "Summary"},
		Description: "Aggregates scores from all dimensions into a single executive view. Overall health score, per-dimension breakdown, top risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Exec dashboard", map[string]interface{}{"overallScore": 68})},
	})

	// --- SLO Compliance (v17.95) ---
	add("/api/product/slo-compliance", "get", OpenAPIOperation{
		Summary:     "Service SLO compliance & error budget burn rate",
		OperationID: "sloCompliance",
		Tags:        []string{"Product", "SLO", "Reliability"},
		Description: "Analyzes service SLO compliance: availability estimation, error budget burn rate, per-namespace SLO tracking.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SLO report", map[string]interface{}{"complianceScore": 92})},
	})

	// --- Probe Latency (v17.96) ---
	add("/api/operations/probe-latency", "get", OpenAPIOperation{
		Summary: "Health probe latency & readiness performance analyzer", OperationID: "probeLatency",
		Tags:        []string{"Operations", "Probe", "Performance"},
		Description: "Analyzes health probe latency: startup/readiness/liveness probe configs, slow detection, timeout risks, missing probe detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Probe report", map[string]interface{}{"healthScore": 65})},
	})

	// --- Helm Health Deep (v17.97) ---
	add("/api/deployment/helm-health-deep", "get", OpenAPIOperation{
		Summary: "Deep Helm release health & chart staleness analyzer", OperationID: "helmHealthDeep",
		Tags:        []string{"Deployment", "Helm", "GitOps"},
		Description: "Deep analysis of Helm releases: version staleness, failed releases, chart age, release integrity.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Helm report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Spot Readiness Deep (v17.98) ---
	add("/api/scalability/spot-readiness-deep", "get", OpenAPIOperation{
		Summary: "Spot/preemptible instance readiness deep analyzer", OperationID: "spotReadinessDeep",
		Tags:        []string{"Scalability", "Spot", "FinOps"},
		Description: "Analyzes spot instance readiness: node label detection, toleration coverage, PDB gaps, disruption resilience.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Spot report", map[string]interface{}{"readinessScore": 50})},
	})

	// --- RBAC Blast (v17.99) ---
	add("/api/security/rbac-blast", "get", OpenAPIOperation{
		Summary: "RBAC privilege escalation & blast radius analyzer", OperationID: "rbacBlast",
		Tags:        []string{"Security", "RBAC", "Privilege"},
		Description: "Analyzes RBAC privilege escalation paths: cluster-admin bindings, wildcard permissions, privilege escalation roles.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("RBAC report", map[string]interface{}{"riskScore": 70})},
	})

	// --- Gateway Health (v18.00) ---
	add("/api/product/api-gateway-health", "get", OpenAPIOperation{
		Summary: "API gateway & ingress controller health analyzer", OperationID: "gatewayHealth",
		Tags:        []string{"Product", "Gateway", "Ingress"},
		Description: "Analyzes ingress controller health, TLS coverage, backend service routing, and ingress configuration gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Gateway report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Throttle Risk (v18.01) ---
	add("/api/operations/throttle-risk", "get", OpenAPIOperation{
		Summary: "Pod resource throttling risk & CPU pressure detector", OperationID: "throttleRisk",
		Tags:        []string{"Operations", "Resource", "Throttling"},
		Description: "Analyzes pod resource throttling: CPU/memory limit gaps, node pressure, unbounded resource consumption.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Throttle report", map[string]interface{}{"riskScore": 55})},
	})

	// --- Audit Trail (v18.02) ---
	add("/api/security/audit-trail", "get", OpenAPIOperation{
		Summary: "Audit logging coverage & compliance trail analyzer", OperationID: "auditTrail",
		Tags:        []string{"Security", "Audit", "Compliance"},
		Description: "Analyzes K8s audit logging: collector detection, audit policy, sensitive access tracking, compliance trail completeness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Audit trail report", map[string]interface{}{"complianceScore": 30})},
	})

	// --- Image Freshness (v18.03) ---
	add("/api/deployment/image-freshness", "get", OpenAPIOperation{
		Summary: "Container image freshness & update tracking", OperationID: "imageFreshness",
		Tags:        []string{"Deployment", "Image", "Updates"},
		Description: "Analyzes image freshness: version pinning, stale image detection, update tracking, :latest tag audit.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image freshness", map[string]interface{}{"healthScore": 45})},
	})

	// --- Multi-Cluster Conn (v18.04) ---
	add("/api/scalability/multi-cluster-conn", "get", OpenAPIOperation{
		Summary: "Multi-cluster connectivity & federation health", OperationID: "multiClusterConn",
		Tags:        []string{"Scalability", "MultiCluster", "Federation"},
		Description: "Detects multi-cluster tools (ClusterAPI/Karmada/ArgoCD fleet), analyzes connectivity, federation health, remote cluster coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Multi-cluster report", map[string]interface{}{"healthScore": 50})},
	})

	// --- Admission Audit (v18.05) ---
	add("/api/security/admission-posture", "get", OpenAPIOperation{
		Summary: "Admission controller posture & policy engine audit", OperationID: "admissionAudit",
		Tags:        []string{"Security", "Admission", "Policy"},
		Description: "Detects OPA/Gatekeeper/Kyverno, counts validating/mutating webhooks, assesses enforcement coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Admission report", map[string]interface{}{"postureScore": 20})},
	})

	// --- Dashboard Availability (v18.06) ---
	add("/api/operations/dashboard-availability", "get", OpenAPIOperation{
		Summary: "Grafana dashboard availability & observability UI coverage", OperationID: "dashAvail",
		Tags:        []string{"Operations", "Observability", "Dashboard"},
		Description: "Detects Grafana, counts dashboards, checks per-namespace observability coverage, identifies blind spots.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Dashboard report", map[string]interface{}{"healthScore": 0})},
	})

	// --- Storage Orphan (v18.07) ---
	add("/api/scalability/storage-orphan", "get", OpenAPIOperation{
		Summary: "Orphaned PVC & storage waste analyzer", OperationID: "storageOrphan",
		Tags:        []string{"Scalability", "Storage", "Waste"},
		Description: "Identifies orphaned PVCs, pending PVCs, unused storage, and estimates monthly waste cost.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage report", map[string]interface{}{"healthScore": 80})},
	})

	// --- Workload Deps (v18.08) ---
	add("/api/deployment/workload-deps", "get", OpenAPIOperation{
		Summary: "Workload dependency graph analyzer", OperationID: "workloadDeps",
		Tags:        []string{"Deployment", "Dependency", "Startup"},
		Description: "Analyzes workload dependencies: init containers, config/secret refs, startup ordering risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Dependency report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Metrics Pipeline (v18.09) ---
	add("/api/operations/metrics-pipe", "get", OpenAPIOperation{
		Summary: "Metrics pipeline integrity & scraping coverage", OperationID: "metricsPipeline",
		Tags:        []string{"Operations", "Metrics", "Pipeline"},
		Description: "Analyzes metrics pipeline: Prometheus detection, exporter health, scraping targets, blind workloads.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Metrics pipeline", map[string]interface{}{"healthScore": 0})},
	})

	// --- Platform Changelog (v18.10) ---
	add("/api/docs/platform-changelog", "get", OpenAPIOperation{
		Summary: "Platform changelog from recent resource changes", OperationID: "platformChangelog",
		Tags:        []string{"Documentation", "Changelog", "Audit"},
		Description: "Generates a platform changelog: new/updated deployments, services, configmaps in last 24h.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Changelog", map[string]interface{}{"totalChanges24h": 10})},
	})

	// --- Capacity Forecast (v18.11) ---
	add("/api/scalability/capacity-forecast-deep", "get", OpenAPIOperation{
		Summary: "Cluster capacity exhaustion forecast", OperationID: "capacityForecast",
		Tags:        []string{"Scalability", "Forecast", "Capacity"},
		Description: "Forecasts when CPU, memory, and pod slots will be exhausted based on current allocation trends.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Forecast", map[string]interface{}{"healthScore": 70})},
	})

	// --- Compliance Map (v18.12) ---
	add("/api/security/compliance-framework", "get", OpenAPIOperation{
		Summary: "SOC2/PCI-DSS/CIS compliance framework mapping", OperationID: "complianceMap",
		Tags:        []string{"Security", "Compliance", "SOC2"},
		Description: "Maps cluster state to SOC2, PCI-DSS, and CIS compliance controls with pass/fail status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Compliance", map[string]interface{}{"complianceScore": 30})},
	})

	// --- MTTR Analysis (v18.13) ---
	add("/api/product/mttr-analysis", "get", OpenAPIOperation{
		Summary: "Mean time to recovery from restart patterns", OperationID: "mttrAnalysis",
		Tags:        []string{"Product", "Reliability", "MTTR"},
		Description: "Analyzes pod restart patterns to estimate MTTR and identify crash-prone workloads.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("MTTR", map[string]interface{}{"healthScore": 55})},
	})

	// --- GitOps Sync (v18.14) ---
	add("/api/deployment/gitops-sync-status", "get", OpenAPIOperation{
		Summary: "GitOps sync state & drift detection", OperationID: "gitopsSync",
		Tags:        []string{"Deployment", "GitOps", "Drift"},
		Description: "Detects ArgoCD/Flux controllers, application sync status, out-of-sync apps, configuration drift.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("GitOps report", map[string]interface{}{"healthScore": 30})},
	})

	// --- Endpoint Probe (v18.15) ---
	add("/api/operations/endpoint-probe", "get", OpenAPIOperation{
		Summary: "Service endpoint readiness probe", OperationID: "endpointProbe",
		Tags:        []string{"Operations", "Endpoint", "Health"},
		Description: "Probes service endpoint readiness: healthy/partial/no-backend services, endpoint count tracking.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Endpoint probe", map[string]interface{}{"healthScore": 75})},
	})

	// --- Node Decommission (v18.16) ---
	add("/api/scalability/node-decomm", "get", OpenAPIOperation{
		Summary: "Node decommissioning & lifecycle rotation", OperationID: "nodeDecomm",
		Tags:        []string{"Scalability", "Node", "Lifecycle"},
		Description: "Analyzes node rotation candidates: age, readiness, rotation urgency, decommission readiness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node decommission", map[string]interface{}{"healthScore": 40})},
	})

	// --- Backup Coverage (v18.17) ---
	add("/api/operations/backup-coverage", "get", OpenAPIOperation{
		Summary: "Backup & disaster recovery posture analyzer", OperationID: "backupCoverage",
		Tags:        []string{"Operations", "Backup", "DR"},
		Description: "Detects Velero/backup tools, PVC backup coverage, schedule presence, restore readiness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Backup report", map[string]interface{}{"healthScore": 20})},
	})

	// --- Idle Zombie (v18.18) ---
	add("/api/deployment/idle-zombie", "get", OpenAPIOperation{
		Summary: "Idle/zombie workload detector", OperationID: "idleZombie",
		Tags:        []string{"Deployment", "Waste", "Idle"},
		Description: "Detects idle workloads consuming resources without traffic, zombie deployments with 0 ready replicas.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Idle report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Service Mesh (v18.19) ---
	add("/api/product/service-mesh", "get", OpenAPIOperation{
		Summary: "Service mesh coverage & mTLS analyzer", OperationID: "serviceMesh",
		Tags:        []string{"Product", "Mesh", "MTLS"},
		Description: "Detects Istio/Linkerd/Consul, sidecar injection rate, mTLS status, mesh coverage gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Mesh report", map[string]interface{}{"healthScore": 20})},
	})

	// --- Cloud Portability (v18.20) ---
	add("/api/product/cloud-portability", "get", OpenAPIOperation{
		Summary: "Cloud vendor lock-in & workload portability assessor", OperationID: "cloudPortability",
		Tags:        []string{"Product", "FinOps", "MultiCloud"},
		Description: "Assesses cloud vendor lock-in by detecting cloud-specific StorageClasses, annotations, node selectors, and volume types. Detects cloud vendor from node providerIDs (AWS/GCP/Azure). Classifies workloads as portable or cloud-locked. Generates prioritized migration plan with effort estimates. Portability score (0-100, A-F grading). Per-namespace portability stats with risk levels.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Portability report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Storage Performance (v18.21) ---
	add("/api/scalability/storage-performance", "get", OpenAPIOperation{
		Summary: "Storage performance tier classification & mismatch detector", OperationID: "storagePerformance",
		Tags:        []string{"Scalability", "Storage", "Performance"},
		Description: "Classifies StorageClasses by performance tier (fast/standard/slow/unknown) based on provisioner and naming. Infers workload storage needs from pod names/labels (database, message-queue, logging, general). Detects performance mismatches (e.g., database on slow-tier storage). Identifies unbound PVCs, unknown-tier storage, and missing fast-tier options. Health score (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage performance report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Workload Lifecycle (v18.22) ---
	add("/api/deployment/workload-lifecycle", "get", OpenAPIOperation{
		Summary: "Workload lifecycle stage classifier & cleanup advisor", OperationID: "workloadLifecycle",
		Tags:        []string{"Deployment", "Lifecycle", "FinOps"},
		Description: "Auto-classifies workloads by lifecycle stage (production, staging, development, deprecated, legacy) using namespace patterns, labels, annotations, replica counts, and age. Assigns operational priority (P0-P3) and risk levels. Identifies cleanup candidates (deprecated/legacy workloads consuming resources) with actionable steps (delete, archive, scale-down). Stale workload detection (>90 days, non-production). Lifecycle governance score (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Lifecycle report", map[string]interface{}{"healthScore": 80})},
	})

	// --- Upgrade Impact Simulator (v18.23) ---
	add("/api/deployment/upgrade-impact", "get", OpenAPIOperation{
		Summary: "K8s version upgrade impact simulator & readiness assessor", OperationID: "upgradeImpact",
		Tags:        []string{"Deployment", "Upgrade", "Platform"},
		Description: "Simulates the impact of upgrading to the next Kubernetes minor version. Checks: deprecated API versions that will be removed, breaking changes per target version, node version skew risk (kubelet vs control plane), addon compatibility (CoreDNS, CNI, CSI, cert-manager), workload-specific risks (privileged containers, hostNetwork, default SA). Generates prioritized pre-upgrade action plan with phases. Readiness score (0-100), verdict (ready/caution/blocked).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Upgrade impact report", map[string]interface{}{"readinessScore": 85})},
	})

	// --- Resource Inventory (v18.24) ---
	add("/api/docs/resource-inventory", "get", OpenAPIOperation{
		Summary: "Comprehensive cluster resource catalog & inventory", OperationID: "resourceInventory",
		Tags:        []string{"Documentation", "Inventory", "Audit"},
		Description: "Full cluster resource inventory documenting every resource type with counts, health status, age distribution, and ownership tracking. Lists: Deployments, StatefulSets, DaemonSets, Pods, Services, ConfigMaps, Secrets, PVCs, Ingresses, NetworkPolicies, ServiceAccounts, Roles, ClusterRoles, HPAs, PDBs, StorageClasses, Nodes. Per-kind health stats (healthy/unhealthy ratio). Per-namespace resource distribution. Orphaned resource detection (services without backing pods). Label hygiene coverage (app/team/env labels). Age distribution (new/week/month/quarter/old). Health score (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource inventory", map[string]interface{}{"healthScore": 90})},
	})

	// --- Unit Economics (v18.25) ---
	add("/api/scalability/unit-economics", "get", OpenAPIOperation{
		Summary: "FinOps unit economics: cost per pod/service/namespace", OperationID: "unitEconomics",
		Tags:        []string{"Scalability", "FinOps", "Cost"},
		Description: "Translates infrastructure costs into business-relevant unit metrics: cost per pod, cost per service, cost per namespace. Computes CPU/memory cost shares, efficiency ratios (limit-to-request), cost per core, cost per GB. Identifies savings opportunities (right-size limits, consolidate pods). Top 20 most expensive pods ranked by monthly cost. Per-namespace efficiency rating (high/medium/low). Monthly spend estimation from resource requests using cloud pricing models. Efficiency score (0-100, A-F grading).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Unit economics report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Platform Scorecard (v18.26) ---
	add("/api/docs/platform-scorecard", "get", OpenAPIOperation{
		Summary: "Unified platform engineering scorecard", OperationID: "platformScorecard",
		Tags:        []string{"Documentation", "Platform", "Executive"},
		Description: "Aggregates signals from infrastructure health, workload reliability, security posture, cost efficiency, operational maturity, and service connectivity into a single executive-level platform score. Weighted scoring with 6 dimensions. Identifies strengths (80+), weaknesses (<60), and generates prioritized improvement roadmap with timeline (quick-win/short-term/long-term) and effort estimates. Maturity level classification (elite/advanced/intermediate/developing/initial). Executive summary with trend direction.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Platform scorecard", map[string]interface{}{"overallScore": 75})},
	})

	// --- Signal Correlation (v18.27) ---
	add("/api/operations/signal-correlation", "get", OpenAPIOperation{
		Summary: "Proactive multi-signal anomaly correlation engine", OperationID: "signalCorrelation",
		Tags:        []string{"Operations", "AIOps", "Proactive"},
		Description: "Correlates signals from pod restart patterns, crash loops, OOM kills, resource pressure, node conditions, event storms, and scheduling failures to detect emerging issues before they become incidents. Identifies signal hotspots (namespaces/nodes with high anomaly density). Detects emerging risks (disk exhaustion, memory pressure, signal saturation) with probability scores and mitigation steps. Signal matrix showing all monitored signal sources and their current status. Health score (0-100).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Signal correlation report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Green Computing (v18.28) ---
	add("/api/scalability/green-computing", "get", OpenAPIOperation{
		Summary: "Green computing & sustainability scorecard", OperationID: "greenComputing",
		Tags:        []string{"Scalability", "Sustainability", "FinOps"},
		Description: "Beyond carbon footprint: assesses energy efficiency, PUE estimation, resource waste from idle workloads, workload density (pods/core), and energy-per-pod metrics. Estimates power consumption (kW), annual energy cost, and CO2 emissions. Identifies waste sources (idle CPU, unbounded resources) with their energy and carbon impact. Per-namespace efficiency ratings. Generates sustainability recommendations with annual savings and CO2 reduction estimates. Green verdict (eco-friendly/moderate/wasteful/critical).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Green computing report", map[string]interface{}{"healthScore": 70})},
	})

	// --- Deploy Window (v18.29) ---
	add("/api/deployment/deploy-window", "get", OpenAPIOperation{
		Summary: "Optimal deployment window analyzer", OperationID: "deployWindow",
		Tags:        []string{"Deployment", "Operations", "Risk"},
		Description: "Analyzes cluster event patterns over 7 days to identify the safest deployment windows. Computes per-hour activity scores from events, warnings, and pod restarts. Recommends top 3 low-risk windows and flags high-risk peak hours. Current deployment risk assessment with verdict (safe-to-deploy/caution/wait). Accounts for crash-loop pods, pending pods, and critical workload density.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy window report", map[string]interface{}{"verdict": "safe-to-deploy"})},
	})

	// --- Workload Criticality (v18.30) ---
	add("/api/product/workload-criticality", "get", OpenAPIOperation{
		Summary: "Workload criticality scoring & tier classification", OperationID: "workloadCriticality",
		Tags:        []string{"Product", "SLA", "Operations"},
		Description: "Scores each workload's business criticality (0-100) based on: replica count, PDB presence, HPA coverage, ingress exposure, resource commitment, age stability, and namespace patterns. Classifies into tiers: Tier-0 (critical, 99.99% SLA), Tier-1 (important, 99.9%), Tier-2 (standard, 99.5%), Tier-3 (best-effort). Per-tier PDB/HPA coverage gap analysis. SLA matrix with RTO/MTTR targets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Criticality report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Commit Optimizer (v18.31) ---
	add("/api/scalability/commit-optimizer", "get", OpenAPIOperation{
		Summary: "Resource commitment & reserved instance optimizer", OperationID: "commitOptimizer",
		Tags:        []string{"Scalability", "FinOps", "Cost"},
		Description: "Analyzes resource commitment patterns to identify savings through reserved instances, sustained-use discounts, and spot migration. Separates stable (always-on) from volatile (batch/spot) workloads. Computes stability scores per workload. Generates commitment plan with monthly/annual savings estimates and confidence scores. Per-namespace cost breakdown. Savings breakdown by category (reserved, right-size, spot).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Commit optimizer report", map[string]interface{}{"healthScore": 80})},
	})

	// --- Change Freeze (v18.32) ---
	add("/api/deployment/change-freeze", "get", OpenAPIOperation{
		Summary: "Change freeze detector & deployment risk gate", OperationID: "changeFreeze",
		Tags:        []string{"Deployment", "Operations", "Risk"},
		Description: "Evaluates cluster stability to determine if changes should proceed. Checks crash loops, recent failed deployments, warning event volume (1h/24h), pod age stability, and active incidents. Detects seasonal freeze periods (holidays, Black Friday). Provides verdict (proceed/caution/freeze) with stability score. Lists recent changes with health status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Change freeze report", map[string]interface{}{"verdict": "proceed"})},
	})

	// --- Attack Surface (v18.33) ---
	add("/api/security/attack-surface", "get", OpenAPIOperation{
		Summary: "External attack surface mapper & TLS gap analyzer", OperationID: "attackSurface",
		Tags:        []string{"Security", "Network", "Audit"},
		Description: "Catalogs every externally-reachable endpoint (Ingress, LoadBalancer, NodePort). Classifies exposure levels (public/internal/cluster-only). Identifies TLS gaps on ingress resources. Maps complete external attack surface with port counts, unique hosts, and high-risk endpoint detection. Per-namespace exposure breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Attack surface map", map[string]interface{}{"healthScore": 85})},
	})

	// --- Density Balance (v18.34) ---
	add("/api/scalability/density-balance", "get", OpenAPIOperation{
		Summary: "Pod scheduling density & node balance analyzer", OperationID: "densityBalance",
		Tags:        []string{"Scalability", "Scheduling", "HA"},
		Description: "Analyzes pod distribution across nodes for optimal fault tolerance. Identifies over-packed (>80%) and under-used (<20%) nodes. Computes Gini coefficient and standard deviation for distribution inequality. Detects namespace pod spread (how many nodes each namespace spans). Generates rebalancing recommendations (spread/consolidate). Balance score (0-100).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Density balance report", map[string]interface{}{"healthScore": 75})},
	})

	// --- Secret Rotation (v18.35) ---
	add(" /api/security/secret-rotation-v2", "get", OpenAPIOperation{
		Summary: "Secret rotation compliance & staleness tracker", OperationID: "secretRotation",
		Tags:        []string{"Security", "Secrets", "Compliance"},
		Description: "Evaluates secret rotation freshness across the cluster. Checks each secret against type-specific max age policies (TLS: 90d, Opaque: 180d, DockerConfig: 90d). Identifies stale, never-rotated, and critically expired secrets. Tracks secret usage by pods. Per-type and per-namespace compliance breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret rotation report", map[string]interface{}{"healthScore": 75})},
	})

	// --- HPA Behavior (v18.36) ---
	add("/api/scalability/hpa-behavior", "get", OpenAPIOperation{
		Summary: "HPA scaling behavior & flapping risk analyzer", OperationID: "hpaBehavior",
		Tags:        []string{"Scalability", "Autoscaling", "AIOps"},
		Description: "Analyzes existing HPA configurations for behavioral issues: flapping risk (aggressive scale-up without stabilization), missing behavior configs, min=max constraints, and suboptimal CPU targets. Classifies scale-up/down policies (aggressive/moderate/conservative). Generates per-HPA scores and actionable tuning recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HPA behavior report", map[string]interface{}{"healthScore": 80})},
	})

	// --- API Access Pattern (v18.37) ---
	add("/api/operations/api-access-pattern", "get", OpenAPIOperation{
		Summary: "API server access pattern & anomaly detector", OperationID: "apiAccessPattern",
		Tags:        []string{"Operations", "Audit", "AIOps"},
		Description: "Analyzes API server access patterns from event data. Identifies top callers, hot resources, read/write ratios, and access anomalies (high failure rate, dominant callers, resource hotspots). Per-namespace access distribution. Useful for detecting controller misbehavior, excessive API calls, and potential security concerns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API access report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Volume Budget (v18.38) ---
	add("/api/scalability/volume-budget", "get", OpenAPIOperation{
		Summary: "PVC storage budget & orphan detector", OperationID: "volumeBudget",
		Tags:        []string{"Scalability", "Storage", "FinOps"},
		Description: "Analyzes PVC usage, storage quota consumption, and volume lifecycle. Tracks PVC request vs capacity, identifies orphaned PVCs, detects pending provisioning failures. Per-namespace and per-storage-class breakdown. Monthly cost estimation. Storage budget forecast.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Volume budget report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Restart Pattern (v18.39) ---
	add("/api/operations/restart-pattern", "get", OpenAPIOperation{
		Summary: "Pod restart pattern & chronic issue analyzer", OperationID: "restartPattern",
		Tags:        []string{"Operations", "Reliability", "AIOps"},
		Description: "Analyzes pod restart history to detect chronic issues: cyclical restart patterns, periodic OOM kills, configuration-triggered restarts. Classifies patterns (chronic/oom-cycle/periodic/sporadic). Time-correlation analysis for restart spikes. Root cause guessing for each problematic workload.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart pattern report", map[string]interface{}{"healthScore": 85})},
	})

	// --- Cert Inventory (v18.40) ---
	add("/api/security/cert-inventory", "get", OpenAPIOperation{
		Summary: "TLS certificate inventory & expiry tracker", OperationID: "certInventory",
		Tags:        []string{"Security", "Compliance", "Certificates"},
		Description: "Inventories all TLS certificates from K8s TLS secrets. Checks expiry dates, identifies soon-to-expire and expired certificates. Tracks issuers, self-signed ratio, wildcard coverage. Detects cert-manager presence. Per-namespace breakdown. Compliance-friendly certificate landscape view.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Certificate inventory", map[string]interface{}{"healthScore": 85})},
	})

	// --- Env Var Audit (v18.41) ---
	add("/api/product/env-var-audit", "get", OpenAPIOperation{
		Summary: "Environment variable security & sprawl auditor", OperationID: "envVarAudit",
		Tags:        []string{"Product", "Security", "Configuration"},
		Description: "Audits environment variables across workloads: detects plaintext secrets, hardcoded URLs, config sprawl, missing best practices. Per-namespace breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Env var audit", map[string]interface{}{"healthScore": 85})},
	})

	// --- Scaling Simulator (v18.42) ---
	add("/api/scalability/scaling-simulator", "get", OpenAPIOperation{
		Summary: "Cluster scaling scenario simulator", OperationID: "scalingSimulator",
		Tags:        []string{"Scalability", "Capacity", "Planning"},
		Description: "Simulates cluster behavior under 1.5x/2x/3x/5x load. Identifies bottlenecks, additional nodes needed, and cost projections. Capacity headroom analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scaling simulation", map[string]interface{}{"healthScaleScore": 85})},
	})

	// --- Placement Score (v18.43) ---
	add("/api/product/placement-score", "get", OpenAPIOperation{
		Summary: "Pod scheduling placement quality scorer", OperationID: "placementScore",
		Tags:        []string{"Product", "Scheduling", "HA"},
		Description: "Evaluates pod placement quality: anti-affinity coverage, topology spread, node diversity, SPOF detection. Per-workload placement scores.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Placement score", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/operations/chaos-readiness", "get", OpenAPIOperation{
		Summary: "Chaos engineering readiness & resilience auditor", OperationID: "chaosReadiness",
		Tags:        []string{"Operations", "Reliability", "Resilience"},
		Description: "Assesses workload resilience for chaos engineering experiments. Evaluates PDB coverage, anti-affinity, graceful shutdown, health probes, resource limits, and simulates failure scenarios (pod kill, node drain, network partition).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Chaos readiness report", map[string]interface{}{"healthScore": 65})},
	})

	add("/api/security/supply-chain", "get", OpenAPIOperation{
		Summary: "Container supply chain security auditor", OperationID: "supplyChain",
		Tags:        []string{"Security", "SupplyChain", "Images"},
		Description: "Audits container image supply chain security: digest vs tag references, :latest usage, non-root execution, read-only rootfs, privileged containers, pull policy, and scan readiness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Supply chain report", map[string]interface{}{"healthScore": 55})},
	})

	add("/api/scalability/capacity-forecast-deep", "get", OpenAPIOperation{
		Summary: "Cluster capacity exhaustion forecast", OperationID: "capacityForecastDeep",
		Tags:        []string{"Scalability", "Capacity", "Forecast"},
		Description: "Projects cluster resource consumption using deployment growth indicators. Calculates utilization rates, monthly growth rates, time-to-exhaustion for CPU/memory/pods, and 90/180-day projections.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity forecast", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/operations/drain-impact", "get", OpenAPIOperation{
		Summary: "Node drain impact simulator", OperationID: "drainImpact",
		Tags:        []string{"Operations", "Maintenance", "Planning"},
		Description: "Simulates the effect of draining a specific node. Identifies evictable pods, rescheduling feasibility, capacity fit on remaining nodes, and service disruption impact. Pass ?node=<name> query parameter.",
		Parameters: []OpenAPIParam{
			{Name: "node", In: "query", Required: true, Schema: map[string]interface{}{"type": "string"}, Description: "Node name to simulate drain"},
		},
		Responses: map[string]OpenAPIResponse{"200": okResponse("Drain impact", map[string]interface{}{"safeToDrain": false, "riskLevel": "high"})},
	})

	add("/api/scalability/request-accuracy", "get", OpenAPIOperation{
		Summary: "Resource request accuracy & right-sizing analyzer", OperationID: "requestAccuracy",
		Tags:        []string{"Scalability", "FinOps", "Optimization"},
		Description: "Analyzes how accurately workload resource requests match actual needs. Identifies over-provisioned (wasted cost) and under-provisioned (throttle/OOM risk) containers with right-sizing recommendations and cost savings estimates.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Request accuracy report", map[string]interface{}{"healthScore": 65})},
	})

	add("/api/security/hardening-score", "get", OpenAPIOperation{
		Summary: "Comprehensive security hardening posture score", OperationID: "hardeningScore",
		Tags:        []string{"Security", "Hardening", "Compliance"},
		Description: "Aggregates findings across Pod Security Standards, network policies, secrets management, RBAC, admission control, and image security into a single weighted score with prioritized remediation guidance and compliance framework mapping.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Hardening score", map[string]interface{}{"overallScore": 45, "grade": "D"})},
	})

	add("/api/security/fix-plan", "get", OpenAPIOperation{
		Summary: "Security remediation action plan generator", OperationID: "securityFixPlan",
		Tags:        []string{"Security", "Remediation", "Automation"},
		Description: "Generates actionable kubectl patch commands for security issues. Provides copy-paste-ready fix commands, batch scripts, and prioritized remediation plans ranked by impact and effort.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Security fix plan", map[string]interface{}{"healthScore": 25})},
	})

	add("/api/docs/api-coverage-map", "get", OpenAPIOperation{
		Summary: "API endpoint coverage map by dimension", OperationID: "apiCoverageMap",
		Tags:        []string{"Documentation", "Quality"},
		Description: "Maps all platform API endpoints grouped by dimension with documentation coverage statistics.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Coverage map", map[string]interface{}{"totalEndpoints": 387})},
	})

	add("/api/deployment/release-gate", "get", OpenAPIOperation{
		Summary: "Pre-deployment release gate evaluator", OperationID: "releaseGate",
		Tags:        []string{"Deployment", "Quality", "Gate"},
		Description: "Evaluates whether the cluster is ready for a new deployment release. Checks PDB coverage, health probes, resource limits, security contexts, multi-node HA, update strategy, and anti-affinity.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Release gate", map[string]interface{}{"overallVerdict": "fail", "gateScore": 35})},
	})

	add("/api/product/service-catalog", "get", OpenAPIOperation{
		Summary: "Cluster service catalog & discovery map", OperationID: "serviceCatalog",
		Tags:        []string{"Product", "Services", "Discovery"},
		Description: "Comprehensive catalog of all Services: type, ports, backends, endpoints, external exposure, health status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Service catalog", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/operations/resource-topology", "get", OpenAPIOperation{
		Summary: "Resource dependency graph & orphan detector", OperationID: "resourceTopology",
		Tags:        []string{"Operations", "Topology", "Inventory"},
		Description: "Maps workload connections to ConfigMaps, Secrets, PVCs, and Services. Identifies orphaned and shared resources.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource topology", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/docs/api-explorer", "get", OpenAPIOperation{
		Summary: "Interactive API endpoint browser with search", OperationID: "apiExplorer",
		Tags:        []string{"Documentation", "API", "Explorer"},
		Description: "Searchable, filterable endpoint browser. Pass ?q=<query> for text search or ?tag=<tag> for tag filtering.",
		Parameters: []OpenAPIParam{
			{Name: "q", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Search query"},
			{Name: "tag", In: "query", Required: false, Schema: map[string]interface{}{"type": "string"}, Description: "Filter by tag"},
		},
		Responses: map[string]OpenAPIResponse{"200": okResponse("API explorer", map[string]interface{}{"totalEndpoints": 390})},
	})

	add("/api/scalability/orphan-cleanup", "get", OpenAPIOperation{
		Summary: "Orphaned resource cleanup planner", OperationID: "orphanCleanup",
		Tags:        []string{"Scalability", "FinOps", "Cleanup"},
		Description: "Identifies orphaned ConfigMaps, Secrets, PVCs with safe-to-delete assessment and batch kubectl delete commands.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cleanup plan", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/scalability/cost-anomaly", "get", OpenAPIOperation{
		Summary: "Cost anomaly detector", OperationID: "costAnomaly",
		Tags:        []string{"Scalability", "FinOps", "Anomaly"},
		Description: "Detects oversized requests, idle workloads, and cost anomalies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cost anomalies", map[string]interface{}{"healthScore": 70})},
	})

	add("/api/deployment/config-snapshot", "get", OpenAPIOperation{
		Summary: "Cluster config snapshot for drift detection", OperationID: "configSnapshot",
		Tags:        []string{"Deployment", "Audit", "Drift"},
		Description: "Captures point-in-time workload spec hashes and resource counts for drift comparison.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Config snapshot", map[string]interface{}{"snapshotId": "snap-123"})},
	})

	add("/api/operations/pod-health-index", "get", OpenAPIOperation{
		Summary: "Per-pod health score & issue detector", OperationID: "podHealthIndex",
		Tags:        []string{"Operations", "Health", "Pod"},
		Description: "Per-pod health score aggregated by workload and namespace. Combines restart count, ready status, probe failures, and resource pressure.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod health index", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/product/namespace-quota-map", "get", OpenAPIOperation{
		Summary: "Namespace quota & limit range coverage map", OperationID: "namespaceQuotaMap",
		Tags:        []string{"Product", "Quota", "Governance"},
		Description: "Comprehensive map of resource quotas, limit ranges, and actual usage per namespace.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Quota map", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/security/secret-exposure", "get", OpenAPIOperation{
		Summary: "Secret exposure & plaintext scanner", OperationID: "secretExposure",
		Tags:        []string{"Security", "Secrets", "Exposure"},
		Description: "Scans for secrets exposed through env vars, volume mounts, and insecure handling. Identifies plaintext and orphaned secrets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret exposure", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/docs/cluster-maturity", "get", OpenAPIOperation{
		Summary: "Cluster maturity model assessment", OperationID: "clusterMaturity",
		Tags:        []string{"Documentation", "Maturity", "Assessment"},
		Description: "Evaluates cluster against a 5-level Kubernetes maturity model. Identifies capabilities achieved and gaps to reach next level.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Maturity assessment", map[string]interface{}{"currentLevel": 2, "scorePct": 45})},
	})

	add("/api/scalability/right-size-engine", "get", OpenAPIOperation{
		Summary: "Resource right-sizing engine", OperationID: "rightSizeEngine",
		Tags:        []string{"Scalability", "FinOps", "Optimization"},
		Description: "Analyzes resource requests and generates concrete right-sizing patches with kubectl commands. Identifies oversized and undersized containers.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Right-size recommendations", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/deployment/deploy-risk", "get", OpenAPIOperation{
		Summary: "Pre-deployment risk assessment", OperationID: "deployRisk",
		Tags:        []string{"Deployment", "Risk", "Assessment"},
		Description: "Weighted multi-factor risk assessment: node HA, crash rate, restart patterns, PDB, probes, limits, update strategy.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy risk", map[string]interface{}{"overallRisk": 55, "verdict": "cautious"})},
	})

	add("/api/operations/pdb-generator", "get", OpenAPIOperation{
		Summary: "PDB manifest generator", OperationID: "pdbGenerator",
		Tags:        []string{"Operations", "PDB", "Automation"},
		Description: "Generates PodDisruptionBudget YAML manifests for multi-replica workloads without PDB. Provides batch kubectl apply commands.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PDB manifests", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/security/netpol-generator", "get", OpenAPIOperation{
		Summary: "NetworkPolicy manifest generator", OperationID: "netpolGenerator",
		Tags:        []string{"Security", "Network", "Automation"},
		Description: "Generates default-deny NetworkPolicy manifests for namespaces lacking network isolation. Includes DNS allow policies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("NetworkPolicy manifests", map[string]interface{}{"healthScore": 40})},
	})

	add("/api/product/service-dependency-map", "get", OpenAPIOperation{
		Summary: "Service dependency graph", OperationID: "serviceDependencyMap",
		Tags:        []string{"Product", "Services", "Dependencies"},
		Description: "Maps service-to-service dependencies by analyzing env vars and service references. Identifies critical paths and single points of failure.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Dependency map", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/scalability/quota-generator", "get", OpenAPIOperation{
		Summary: "ResourceQuota & LimitRange manifest generator", OperationID: "quotaGenerator",
		Tags:        []string{"Scalability", "Governance", "Automation"},
		Description: "Generates ResourceQuota and LimitRange YAML for namespaces lacking resource governance. Provides batch kubectl apply commands.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Quota manifests", map[string]interface{}{"healthScore": 40})},
	})

	add("/api/deployment/probe-generator", "get", OpenAPIOperation{
		Summary: "Health probe patch generator", OperationID: "probeGenerator",
		Tags:        []string{"Deployment", "Reliability", "Automation"},
		Description: "Generates livenessProbe and readinessProbe kubectl patches for containers missing health checks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Probe patches", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/docs/platform-insights", "get", OpenAPIOperation{
		Summary: "Unified executive platform insights", OperationID: "platformInsights",
		Tags:        []string{"Documentation", "Executive", "Dashboard"},
		Description: "Aggregates key metrics from all audit endpoints into a single executive summary with category scores, alerts, and trends.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Platform insights", map[string]interface{}{"overallScore": 45, "grade": "D"})},
	})

	add("/api/docs/action-priority-matrix", "get", OpenAPIOperation{
		Summary: "Prioritized remediation action queue", OperationID: "actionPriorityMatrix",
		Tags:        []string{"Documentation", "Planning", "Remediation"},
		Description: "Aggregates all platform findings into a single prioritized action queue with impact, effort, and urgency scoring. Includes quick wins and batch plan.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Priority matrix", map[string]interface{}{"platformScore": 35})},
	})

	add("/api/operations/health-trend", "get", OpenAPIOperation{
		Summary: "Cluster health trend over time", OperationID: "healthTrend",
		Tags:        []string{"Operations", "Trend", "Stability"},
		Description: "Tracks cluster health metrics over 8 weeks: pod counts, restart patterns, crash rates, new workloads, per-namespace stability.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Health trend", map[string]interface{}{"stabilityScore": 75})},
	})

	add("/api/scalability/image-cleanup", "get", OpenAPIOperation{
		Summary: "Unused image cleanup advisor", OperationID: "imageCleanup",
		Tags:        []string{"Scalability", "Cleanup", "Storage"},
		Description: "Identifies unused and stale container images on nodes to free disk space. Cross-references running pods with node image caches.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image cleanup", map[string]interface{}{"healthScore": 70})},
	})

	add("/api/operations/restart-analyzer", "get", OpenAPIOperation{
		Summary: "Pod restart pattern analyzer & root cause", OperationID: "restartAnalyzer",
		Tags:        []string{"Operations", "Stability", "Diagnostics"},
		Description: "Deep analysis of pod restart patterns: OOM kills, crash loops, probe failures. Distinguishes systematic issues from one-off incidents.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart analysis", map[string]interface{}{"stabilityScore": 65})},
	})

	add("/api/security/env-leak-scanner", "get", OpenAPIOperation{
		Summary: "Plaintext env var leak scanner", OperationID: "envLeakScanner",
		Tags:        []string{"Security", "Secrets", "Compliance"},
		Description: "Scans container env vars for plaintext passwords, tokens, API keys. Provides fix commands to migrate to Secrets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Env leak scan", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/deployment/update-strategy-auditor", "get", OpenAPIOperation{
		Summary: "Update strategy risk auditor", OperationID: "updateStrategyAuditor",
		Tags:        []string{"Deployment", "Strategy", "Risk"},
		Description: "Evaluates deployment update strategies: Recreate risk, missing surge/unavailable controls, revision history limits.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Strategy audit", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/product/label-score", "get", OpenAPIOperation{
		Summary: "Label hygiene score", OperationID: "labelScore",
		Tags:        []string{"Product", "Labels", "Quality"},
		Description: "Evaluates label quality: standard labels coverage, app/team/owner labels, inconsistent naming, orphaned selectors.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Label score", map[string]interface{}{"healthScore": 65})},
	})

	add("/api/scalability/storage-tier", "get", OpenAPIOperation{
		Summary: "Storage tier analyzer", OperationID: "storageTier",
		Tags:        []string{"Scalability", "Storage", "Cost"},
		Description: "Analyzes storage classes, PVC performance tiers, volume binding modes. Identifies cost optimization and configuration issues.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage tier", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/security/trust-chain", "get", OpenAPIOperation{
		Summary: "Trust chain auditor", OperationID: "trustChain",
		Tags:        []string{"Security", "Certificates", "Trust"},
		Description: "Audits TLS certificates, CA certs, SA tokens, and admission webhooks. Identifies expired certs, old tokens, and missing CA bundles.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Trust chain", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/operations/alert-fatigue", "get", OpenAPIOperation{
		Summary: "Event noise & alert fatigue analyzer", OperationID: "alertFatigue",
		Tags:        []string{"Operations", "Events", "Noise"},
		Description: "Analyzes Warning events to find noisy namespaces, repeated warnings, and event storms. Helps tune alerting.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Alert fatigue", map[string]interface{}{"noiseScore": 65})},
	})

	add("/api/deployment/deploy-frequency", "get", OpenAPIOperation{
		Summary: "Deployment frequency tracker (DORA)", OperationID: "deployFrequency",
		Tags:        []string{"Deployment", "DORA", "Metrics"},
		Description: "Tracks deployment rollout history: frequency, success rate, rollback patterns. Key DORA metric.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy frequency", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/docs/platform-comparison", "get", OpenAPIOperation{
		Summary: "Platform comparison & trend snapshot", OperationID: "platformComparison",
		Tags:        []string{"Documentation", "Trend", "Comparison"},
		Description: "Generates cluster state snapshot with category scores for trend tracking and regression detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Platform comparison", map[string]interface{}{"overallScore": 45})},
	})

	add("/api/security/container-hardening", "get", OpenAPIOperation{
		Summary: "Container security hardening scanner", OperationID: "containerHardening",
		Tags:        []string{"Security", "Hardening", "Patches"},
		Description: "Scans containers for missing securityContext fields and generates strategic kubectl patch commands.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Hardening scan", map[string]interface{}{"healthScore": 20})},
	})

	add("/api/scalability/autoscale-readiness", "get", OpenAPIOperation{
		Summary: "HPA autoscale readiness & generator", OperationID: "autoscaleReadiness",
		Tags:        []string{"Scalability", "HPA", "Automation"},
		Description: "Evaluates workloads for HPA suitability and generates ready-to-apply HPA YAML manifests.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Autoscale readiness", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/product/workload-efficiency", "get", OpenAPIOperation{
		Summary: "Workload resource efficiency scorer", OperationID: "workloadEfficiency",
		Tags:        []string{"Product", "FinOps", "Efficiency"},
		Description: "Evaluates request-to-limit ratios, replica waste, and anti-patterns. Estimates waste cost.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Workload efficiency", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/operations/capacity-gap", "get", OpenAPIOperation{
		Summary: "Capacity gap & node loss survival analyzer", OperationID: "capacityGap",
		Tags:        []string{"Operations", "Capacity", "HA"},
		Description: "Calculates node headroom, worst-case pod eviction capacity, and node loss survival scenarios.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity gap", map[string]interface{}{"healthScore": 50})},
	})

	add("/api/deployment/revision-drift", "get", OpenAPIOperation{
		Summary: "ReplicaSet revision drift detector", OperationID: "revisionDrift",
		Tags:        []string{"Deployment", "Drift", "History"},
		Description: "Detects configuration drift between ReplicaSets. Identifies failed rollbacks and stale revision history.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Revision drift", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/docs/knowledge-base", "get", OpenAPIOperation{
		Summary: "Auto-generated cluster knowledge base", OperationID: "knowledgeBase",
		Tags:        []string{"Documentation", "Knowledge", "Runbook"},
		Description: "Generates human-readable KB articles, runbooks, and FAQ from live cluster state findings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Knowledge base", map[string]interface{}{"totalArticles": 8})},
	})

	add("/api/security/compliance-gap", "get", OpenAPIOperation{
		Summary: "Compliance framework gap analysis", OperationID: "complianceGap",
		Tags:        []string{"Security", "Compliance", "Audit"},
		Description: "Gap analysis against CIS, NIST, SOC2 frameworks. Maps cluster findings to compliance controls with remediation roadmap.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Compliance gap", map[string]interface{}{"healthScore": 40})},
	})

	add("/api/scalability/scheduler-fairness", "get", OpenAPIOperation{
		Summary: "Pod scheduling fairness analyzer", OperationID: "schedulerFairness",
		Tags:        []string{"Scalability", "Scheduling", "Balance"},
		Description: "Analyzes pod distribution across nodes, identifies hotspots and under-utilized nodes, calculates fairness score.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scheduler fairness", map[string]interface{}{"fairnessScore": 75})},
	})

	add("/api/product/workload-fingerprint", "get", OpenAPIOperation{
		Summary: "Workload fingerprint & duplicate detector", OperationID: "workloadFingerprint",
		Tags:        []string{"Product", "Classification", "Drift"},
		Description: "Creates unique fingerprints for workloads based on config hash, resource profile, and behavior. Finds duplicates.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Workload fingerprints", map[string]interface{}{"healthScore": 70})},
	})

	add("/api/deployment/deploy-heatmap", "get", OpenAPIOperation{
		Summary: "Deployment activity heatmap", OperationID: "deployHeatmap",
		Tags:        []string{"Deployment", "Analytics", "DORA"},
		Description: "Shows deployment activity by namespace, hour, and weekday. Identifies bottlenecks and change windows.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy heatmap", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/operations/log-volume", "get", OpenAPIOperation{
		Summary: "Log volume estimator & noisy logger finder", OperationID: "logVolume",
		Tags:        []string{"Operations", "Logging", "Storage"},
		Description: "Estimates per-workload log volume to identify noisy loggers and log storage pressure.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Log volume", map[string]interface{}{"healthScore": 70})},
	})

	add("/api/docs/cluster-narrative", "get", OpenAPIOperation{
		Summary: "Human-readable cluster narrative report", OperationID: "clusterNarrative",
		Tags:        []string{"Documentation", "Report", "Executive"},
		Description: "Translates raw metrics into natural language paragraphs for executive reporting and onboarding.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cluster narrative", map[string]interface{}{"title": "k8ops report"})},
	})

	add("/api/security/config-audit-trail", "get", OpenAPIOperation{
		Summary: "Configuration change audit trail", OperationID: "configAuditTrail",
		Tags:        []string{"Security", "Audit", "Changes"},
		Description: "Tracks configuration changes across deployments via ReplicaSet revision history. Builds complete change timeline.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Config audit", map[string]interface{}{"healthScore": 65})},
	})

	add("/api/scalability/node-utilization-deep", "get", OpenAPIOperation{
		Summary: "Deep node utilization & top consumer analysis", OperationID: "nodeUtilizationDeep",
		Tags:        []string{"Scalability", "Nodes", "Capacity"},
		Description: "Per-node CPU/memory/pod utilization with top consumer identification and imbalance scoring.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node utilization", map[string]interface{}{"imbalanceScore": 75})},
	})

	add("/api/security/secret-rotation-plan", "get", OpenAPIOperation{
		Summary: "Secret rotation plan generator", OperationID: "secretRotationPlan",
		Tags:        []string{"Security", "Secrets", "Rotation"},
		Description: "Prioritized secret rotation plan with commands. Identifies old secrets, TLS certs needing renewal.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rotation plan", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/operations/event-correlation-deep", "get", OpenAPIOperation{
		Summary: "Deep event correlation & root cause", OperationID: "eventCorrelationDeep",
		Tags:        []string{"Operations", "Events", "Correlation"},
		Description: "Deep correlation of K8s events to find causal chains, cascading failures, and root causes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Event correlation", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/deployment/rollback-simulator", "get", OpenAPIOperation{
		Summary: "Rollback risk simulator", OperationID: "rollbackSimulator",
		Tags:        []string{"Deployment", "Rollback", "Risk"},
		Description: "Simulates rollback impact per deployment: data loss, HPA mismatch, revision history gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollback sim", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/docs/upgrade-planner", "get", OpenAPIOperation{
		Summary: "K8s upgrade planner & readiness", OperationID: "upgradePlanner",
		Tags:        []string{"Documentation", "Upgrade", "Planning"},
		Description: "Analyzes cluster upgrade readiness: API deprecations, node HA, PVC drain, generates checklist.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Upgrade plan", map[string]interface{}{"readinessScore": 50})},
	})

	add("/api/security/rbac-drift", "get", OpenAPIOperation{
		Summary: "RBAC drift & over-permissive role detector", OperationID: "rbacDrift",
		Tags:        []string{"Security", "RBAC", "Audit"},
		Description: "Detects over-permissive roles, wildcard permissions, stale bindings, and cluster-admin overuse.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("RBAC drift", map[string]interface{}{"healthScore": 70})},
	})

	add("/api/scalability/resource-forecast", "get", OpenAPIOperation{
		Summary: "Resource capacity forecast", OperationID: "resourceForecast",
		Tags:        []string{"Scalability", "Forecast", "Capacity"},
		Description: "Projects when CPU, memory, and pod capacity will be exhausted. Recommends scaling actions.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource forecast", map[string]interface{}{"healthScore": 75})},
	})

	add("/api/product/config-warmstart", "get", OpenAPIOperation{
		Summary: "Startup optimization & warm-start analyzer", OperationID: "configWarmstart",
		Tags:        []string{"Product", "Startup", "Optimization"},
		Description: "Identifies slow starters, recommends startupProbe, init containers, and probe improvements.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Warm-start analysis", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/operations/pod-slo", "get", OpenAPIOperation{
		Summary: "Pod SLO compliance tracker", OperationID: "podSLO",
		Tags:        []string{"Operations", "SLO", "Availability"},
		Description: "Evaluates per-workload SLO compliance based on pod readiness and restart frequency.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod SLO", map[string]interface{}{"healthScore": 95})},
	})

	add("/api/deployment/deploy-readiness-gate", "get", OpenAPIOperation{
		Summary: "Deployment readiness gate evaluator", OperationID: "deployReadinessGate",
		Tags:        []string{"Deployment", "Readiness", "Gate"},
		Description: "Composite readiness check: probes, resources, PDB, HPA, rollback capability.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Readiness gate", map[string]interface{}{"healthScore": 60})},
	})

	add("/api/docs/api-governance-score", "get", OpenAPIOperation{
		Summary: "API version governance score", OperationID: "apiGovernanceScore",
		Tags:        []string{"Documentation", "API", "Governance"},
		Description: "Evaluates API version usage, deprecations, and migration readiness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API governance", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/security/disruption-budget-gap", "get", OpenAPIOperation{
		Summary: "Disruption budget gap analyzer", OperationID: "disruptionBudgetGap",
		Tags:        []string{"Security", "Reliability", "PDB"},
		Description: "Scans all workloads (Deployments, StatefulSets) for PodDisruptionBudget coverage. Identifies single-replica workloads exposed to voluntary disruptions, critical labels on unprotected workloads, and provides a risk score (0-100) with grade.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Disruption budget gap", map[string]interface{}{"riskScore": 45, "grade": "C"})},
	})
	add("/api/product/cost-topology", "get", OpenAPIOperation{
		Summary: "Cost topology per namespace", OperationID: "costTopology",
		Tags:        []string{"Product", "Cost", "FinOps"},
		Description: "Analyzes resource requests across all namespaces to compute per-namespace cost attribution. Uses on-demand pricing model ($0.034/vCPU-hr, $0.0046/GB-hr). Identifies cost concentration, top spenders, efficiency ratings, and provides optimization recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cost topology", map[string]interface{}{"costScore": 60, "grade": "B"})},
	})
	add("/api/scalability/binpack-efficiency", "get", OpenAPIOperation{
		Summary: "Node bin-packing efficiency", OperationID: "binpackEfficiency",
		Tags:        []string{"Scalability", "Node", "Consolidation"},
		Description: "Analyzes node bin-packing efficiency by comparing pod resource requests against node allocatable capacity. Classifies nodes as idle/underutilized/moderate/packed. Identifies consolidation opportunities (nodes that can be drained), pod density metrics, and potential savings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Binpack efficiency", map[string]interface{}{"efficiencyScore": 70, "grade": "B"})},
	})
	add("/api/operations/slo-burn-rate", "get", OpenAPIOperation{
		Summary: "SLO error budget burn rate", OperationID: "sloBurnRate",
		Tags:        []string{"Operations", "SLO", "SRE"},
		Description: "Calculates SLO error budget burn rates using SRE methodology. Fast burn rate detects acute incidents (1h window, >14.4x = critical). Slow burn rate detects chronic issues (72h window). Estimates time to budget exhaustion per workload.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SLO burn rate", map[string]interface{}{"burnScore": 75, "grade": "B"})},
	})
	add("/api/deployment/surge-capacity", "get", OpenAPIOperation{
		Summary: "Rolling update surge capacity", OperationID: "surgeCapacity",
		Tags:        []string{"Deployment", "Capacity", "RollingUpdate"},
		Description: "Checks whether the cluster has enough resources to absorb maxSurge replicas during rolling updates. Calculates per-pod resource requests, surge requirements (CPU/memory), and compares against cluster-wide available capacity. Identifies workloads that will be blocked during deployment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Surge capacity", map[string]interface{}{"surgeScore": 80, "grade": "B"})},
	})
	add("/api/docs/runbook-coverage", "get", OpenAPIOperation{
		Summary: "Runbook coverage scanner", OperationID: "runbookCoverage",
		Tags:        []string{"Documentation", "Runbook", "SRE"},
		Description: "Scans workloads for documentation annotations (runbook, docs, wiki, oncall, sop, playbook). Identifies undocumented critical services. Supports standard annotations and app.kubernetes.io prefixed variants.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Runbook coverage", map[string]interface{}{"coverageScore": 30, "grade": "F"})},
	})
	add("/api/security/privilege-map", "get", OpenAPIOperation{
		Summary: "Cluster privilege exposure map", OperationID: "privilegeMap",
		Tags:        []string{"Security", "Privilege", "Container"},
		Description: "Builds a cluster-wide privilege exposure map. Scans all containers for: privileged flag, runAsUser=0 (root), hostPID/hostIPC/hostNetwork, dangerous Linux capabilities (CAP_SYS_ADMIN, CAP_SYS_PTRACE, etc.), allowPrivilegeEscalation, and readOnlyRootFilesystem. Provides risk-level classification per workload.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Privilege map", map[string]interface{}{"exposureScore": 50, "grade": "C"})},
	})
	add("/api/product/api-slo-correlation", "get", OpenAPIOperation{
		Summary: "API SLO correlation", OperationID: "apiSloCorrelation",
		Tags:        []string{"Product", "SLO", "Service"},
		Description: "Correlates Kubernetes Services with SLO readiness indicators: readiness/liveness probes, resource limits, HPA, PDB. Calculates per-service SLO readiness score (0-100). Identifies services missing critical SLO components.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API SLO correlation", map[string]interface{}{"correlationScore": 55, "grade": "C"})},
	})
	add("/api/scalability/eviction-risk", "get", OpenAPIOperation{
		Summary: "Pod eviction risk predictor", OperationID: "evictionRisk",
		Tags:        []string{"Scalability", "Eviction", "Stability"},
		Description: "Predicts which pods are at imminent eviction risk based on: node conditions (memory/disk/PID pressure), QoS class, OOM history, restart frequency, priority class, and resource limits. Risk score 0-100 per pod with categorized risk factors.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Eviction risk", map[string]interface{}{"riskScore": 70, "grade": "B"})},
	})
	add("/api/operations/golden-signal-budget", "get", OpenAPIOperation{
		Summary: "Golden signal budget", OperationID: "goldenSignalBudget",
		Tags:        []string{"Operations", "SRE", "GoldenSignals"},
		Description: "Unifies the four SRE golden signals (latency, traffic, errors, saturation) into a composite health budget per workload. Weighted scoring: latency 30%, traffic 20%, errors 30%, saturation 20%. Identifies weakest signal per workload.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Golden signal budget", map[string]interface{}{"compositeScore": 70, "grade": "B"})},
	})
	add("/api/deployment/preflight-check-v2", "get", OpenAPIOperation{
		Summary: "Deployment preflight check", OperationID: "preflightCheck",
		Tags:        []string{"Deployment", "Validation", "Safety"},
		Description: "Validates prerequisites before a rolling update: resource requests, readiness probes, PDB coverage, rolling update strategy, node health, HPA, revision history, graceful shutdown. 8 checks total with blocking and warning severity levels.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Preflight check", map[string]interface{}{"passRate": 75, "grade": "B"})},
	})
	add("/api/docs/capacity-runbook", "get", OpenAPIOperation{
		Summary: "Capacity runbook generator", OperationID: "capacityRunbook",
		Tags:        []string{"Documentation", "Capacity", "Runbook"},
		Description: "Auto-generates capacity planning documentation: cluster overview, headroom analysis (CPU/memory/pod slots), growth projection (5% monthly, days to exhaustion), emergency runbook steps. Bottleneck resource identification.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity runbook", map[string]interface{}{"capacityScore": 40, "grade": "C"})},
	})
	add("/api/security/secret-spray", "get", OpenAPIOperation{
		Summary: "Secret spray exposure", OperationID: "secretSpray",
		Tags:        []string{"Security", "Secret", "Exposure"},
		Description: "Analyzes how widely each Secret is mounted across pods. Over-sprayed secrets (mounted on 10+ pods) increase credential compromise blast radius. Checks volumes, env vars, envFrom, imagePullSecrets, and projected volumes. Classifies by spray level and risk score.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret spray", map[string]interface{}{"exposureScore": 50, "grade": "C"})},
	})
	add("/api/product/traffic-cost-split", "get", OpenAPIOperation{
		Summary: "Traffic cost split", OperationID: "trafficCostSplit",
		Tags:        []string{"Product", "Cost", "FinOps"},
		Description: "Splits cluster traffic cost by Service and Ingress. Attributes compute costs (CPU/memory requests) to API endpoints via service selectors. Identifies high-cost paths, unattributed costs, and cost concentration.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Traffic cost split", map[string]interface{}{"costScore": 70, "grade": "B"})},
	})
	add("/api/scalability/node-failure-blast", "get", OpenAPIOperation{
		Summary: "Node failure blast radius", OperationID: "nodeFailureBlast",
		Tags:        []string{"Scalability", "HA", "Failure"},
		Description: "Simulates single-node failure to calculate blast radius: affected workloads, unavailable pod percentage, single-replica workloads at risk, anti-affinity gaps. Estimates recovery time and worst-case scenario.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node failure blast", map[string]interface{}{"blastScore": 60, "grade": "B"})},
	})
	add("/api/operations/incident-timeline", "get", OpenAPIOperation{
		Summary: "Incident timeline reconstructor", OperationID: "incidentTimeline",
		Tags:        []string{"Operations", "Incident", "Timeline"},
		Description: "Reconstructs incident timelines from Kubernetes events, pod state transitions, and crash/restart patterns. Groups related events into incidents with severity, root cause, duration, and status (active/resolved). Provides event rate analysis and MTTR insights.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Incident timeline", map[string]interface{}{"healthScore": 70, "grade": "B"})},
	})
	add("/api/deployment/rollback-safety", "get", OpenAPIOperation{
		Summary: "Rollback safety auditor", OperationID: "rollbackSafety",
		Tags:        []string{"Deployment", "Rollback", "Safety"},
		Description: "Evaluates rollback safety by checking revision history depth, PVC usage (data migration risk), ConfigMap drift, and breaking changes. Classifies workloads as safe/caution/unsafe with actionable recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollback safety", map[string]interface{}{"safetyScore": 75, "grade": "B"})},
	})
	add("/api/docs/api-semantic-version", "get", OpenAPIOperation{
		Summary: "API semantic version tracker", OperationID: "apiSemanticVersion",
		Tags:        []string{"Documentation", "API", "Versioning"},
		Description: "Tracks Kubernetes API version semantics across all resources. Identifies deprecated APIs (extensions/v1beta1, apps/v1beta1, etc.), breaking changes, and removal timelines. Provides migration recommendations and maturity distribution (GA/Beta/Alpha).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API semantic version", map[string]interface{}{"maturityScore": 90, "grade": "A"})},
	})
	add("/api/security/cert-chain-validator", "get", OpenAPIOperation{
		Summary: "TLS certificate chain validator", OperationID: "certChainValidator",
		Tags:        []string{"Security", "TLS", "Certificate"},
		Description: "Validates TLS certificate chains from Secrets and Ingress hosts. Checks certificate expiry, chain completeness (intermediate CA presence), self-signed detection, key pair validity, and key size. Links to Ingress hosts for coverage analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cert chain validation", map[string]interface{}{"validationScore": 75, "grade": "B"})},
	})
	add("/api/product/feature-flag-audit", "get", OpenAPIOperation{
		Summary: "Feature flag audit", OperationID: "featureFlagAudit",
		Tags:        []string{"Product", "FeatureFlags", "Config"},
		Description: "Scans ConfigMaps, annotations, and environment variables for feature flags. Identifies stale/debug/test flags, unmanaged toggles, and env-var-based flags that require redeployment to change. Classifies by risk level.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Feature flag audit", map[string]interface{}{"coverageScore": 65, "grade": "C"})},
	})
	add("/api/scalability/autoscaler-gap", "get", OpenAPIOperation{
		Summary: "Cluster autoscaler gap", OperationID: "autoscalerGap",
		Tags:        []string{"Scalability", "Autoscaler", "Gap"},
		Description: "Analyzes Cluster Autoscaler/Karpenter configuration and identifies scaling gaps. Detects pending/unschedulable pods, node pool sizing issues, HA gaps, and provides autoscaler deployment recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Autoscaler gap", map[string]interface{}{"gapScore": 50, "grade": "C"})},
	})
	add("/api/operations/resource-saturation-watch", "get", OpenAPIOperation{
		Summary: "Resource saturation watchdog", OperationID: "resourceSaturationWatch",
		Tags:        []string{"Operations", "Saturation", "Watch"},
		Description: "Monitors real-time resource saturation across CPU, memory, disk, and PID dimensions. Calculates per-node saturation percentages, identifies hotspots, tracks namespace resource consumption, and predicts time-to-exhaustion.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Saturation watch", map[string]interface{}{"watchScore": 75, "grade": "B"})},
	})
	add("/api/deployment/deploy-frequency-trend", "get", OpenAPIOperation{
		Summary: "Deploy frequency trend", OperationID: "deployFrequencyTrend",
		Tags:        []string{"Deployment", "DORA", "Trend"},
		Description: "Analyzes deployment frequency patterns using ReplicaSet creation timestamps. Computes DORA metrics (Elite/High/Medium/Low), per-day deploy counts, per-workload deploy frequency, and average intervals.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy frequency", map[string]interface{}{"frequencyScore": 75, "grade": "B"})},
	})
	add("/api/docs/oncall-readiness", "get", OpenAPIOperation{
		Summary: "On-call readiness evaluator", OperationID: "oncallReadiness",
		Tags:        []string{"Documentation", "Oncall", "Readiness"},
		Description: "Evaluates whether the cluster can safely operate unattended. Checks: multi-node HA, PDB coverage, HPA autoscaling, health probes, resource limits, crash-loop detection, runbook annotations. Determines if safe for unattended operation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Oncall readiness", map[string]interface{}{"readinessScore": 50, "grade": "C"})},
	})
	add("/api/security/mtls-trust-domain", "get", OpenAPIOperation{
		Summary: "mTLS trust domain auditor", OperationID: "mtlsTrustDomain",
		Tags:        []string{"Security", "mTLS", "Mesh"},
		Description: "Audits mTLS configuration across the cluster. Detects Service Mesh type (Istio/Linkerd/Consul), checks namespace injection labels, mTLS mode (STRICT/PERMISSIVE), sidecar presence per pod, and authorization policies. Provides trust domain analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("mTLS trust", map[string]interface{}{"trustScore": 50, "grade": "C"})},
	})
	add("/api/product/latency-budget", "get", OpenAPIOperation{
		Summary: "Latency budget allocator", OperationID: "latencyBudget",
		Tags:        []string{"Product", "Latency", "SLO"},
		Description: "Allocates latency budgets across service paths and identifies services exceeding their allocated latency SLO. Estimates per-service latency from probe config, replica count, restart history. Provides component-level latency breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Latency budget", map[string]interface{}{"budgetScore": 60, "grade": "C"})},
	})
	add("/api/scalability/pod-disruption-tolerance", "get", OpenAPIOperation{
		Summary: "Pod disruption tolerance", OperationID: "podDisruptionTolerance",
		Tags:        []string{"Scalability", "Disruption", "HA"},
		Description: "Analyzes cluster tolerance to both voluntary (drains, maintenance) and involuntary (node failure) disruptions. Computes per-workload voluntary and involuntary scores, recovery time estimates, data loss risk for StatefulSets, and node spread analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Disruption tolerance", map[string]interface{}{"toleranceScore": 50, "grade": "C"})},
	})
	add("/api/operations/event-noise-filter", "get", OpenAPIOperation{
		Summary: "Event noise filter", OperationID: "eventNoiseFilter",
		Tags:        []string{"Operations", "Events", "Noise"},
		Description: "Analyzes Kubernetes events to identify noise patterns, duplicate events, and actionable signal-to-noise ratio. Classifies events by reason, detects noise-dominant patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Event noise", map[string]interface{}{"signalScore": 50})},
	})
	add("/api/deployment/progressive-rollout", "get", OpenAPIOperation{
		Summary: "Progressive rollout readiness", OperationID: "progressiveRollout",
		Tags:        []string{"Deployment", "Canary", "Progressive"},
		Description: "Evaluates rolling update strategies for progressive delivery: canary readiness, blue-green capability, probe/PDB/replica prerequisites.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Progressive rollout", map[string]interface{}{"readinessScore": 40})},
	})
	add("/api/docs/cost-anomaly-deep", "get", OpenAPIOperation{
		Summary: "Deep cost anomaly", OperationID: "costAnomalyDeep",
		Tags:        []string{"Documentation", "Cost", "Anomaly"},
		Description: "Deep cost anomaly detection by comparing per-namespace resource consumption against baselines. Identifies cost spikes and expensive-per-pod anomalies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cost anomaly", map[string]interface{}{"anomalyScore": 70})},
	})

	add("/api/security/runtime-drift-detect", "get", OpenAPIOperation{
		Summary: "Runtime drift detector", OperationID: "runtimeDriftDetect",
		Tags:        []string{"Security", "Drift", "Compliance"},
		Description: "Detects configuration drift between deployed pods and controller templates. Checks image, env var, and resource request mismatches.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Runtime drift", map[string]interface{}{"driftScore": 90})},
	})
	add("/api/product/svc-mesh-readiness", "get", OpenAPIOperation{
		Summary: "Service mesh readiness gate", OperationID: "svcMeshReadiness",
		Tags:        []string{"Product", "ServiceMesh", "Readiness"},
		Description: "Evaluates service mesh adoption readiness: protocol compatibility, probes, replica count, selector presence.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Mesh readiness", map[string]interface{}{"readinessScore": 50})},
	})
	add("/api/scalability/node-pool-rightsize", "get", OpenAPIOperation{
		Summary: "Node pool right-sizer", OperationID: "nodePoolRightsize",
		Tags:        []string{"Scalability", "Node", "Cost"},
		Description: "Recommends optimal node sizing based on actual utilization. Identifies over-provisioned, under-provisioned, and right-sized nodes with potential savings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node rightsize", map[string]interface{}{"rightsizeScore": 60})},
	})
	add("/api/operations/pod-restart-forensics", "get", OpenAPIOperation{
		Summary: "Pod restart forensics", OperationID: "podRestartForensics",
		Tags:        []string{"Operations", "Forensics", "Restart"},
		Description: "Forensic analysis of pod restarts: root cause classification (OOM, app-error, signal), pattern detection (crashloop, frequent), exit code analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart forensics", map[string]interface{}{"forensicsScore": 85})},
	})
	add("/api/deployment/deploy-window-optimizer", "get", OpenAPIOperation{
		Summary: "Deploy window optimizer", OperationID: "deployWindowOptimizer",
		Tags:        []string{"Deployment", "Window", "Risk"},
		Description: "Analyzes deployment patterns to recommend optimal windows. Hourly heatmap, weekly patterns, change-freeze compliance, recommended deploy times.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy window", map[string]interface{}{"optimizerScore": 70})},
	})
	add("/api/docs/platform-maturity-deep", "get", OpenAPIOperation{
		Summary: "Deep platform maturity", OperationID: "platformMaturityDeep",
		Tags:        []string{"Documentation", "Maturity", "Assessment"},
		Description: "Deep CNCF maturity model assessment across 6 dimensions (Automation, Reliability, Security, Observability, Scalability, Governance). Gap analysis and 12-month roadmap.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Platform maturity", map[string]interface{}{"overallScore": 50, "currentLevel": 2})},
	})
	add("/api/security/admission-bypass-audit", "get", OpenAPIOperation{
		Summary: "Admission bypass audit", OperationID: "admissionBypassAudit",
		Tags:        []string{"Security", "Admission", "Bypass"},
		Description: "Audits workloads that may bypass admission control: privileged containers, hostPID/IPC/network, hostPath volumes, default service accounts, SA token secrets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Bypass audit", map[string]interface{}{"bypassScore": 70})},
	})
	add("/api/product/golden-path-validator", "get", OpenAPIOperation{
		Summary: "Golden path validator", OperationID: "goldenPathValidator",
		Tags:        []string{"Product", "Compliance", "BestPractice"},
		Description: "Validates workloads against golden path standards: readiness/liveness probes, resource limits, multi-replica, PDB, affinity, rolling strategy. 7 checks per workload.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Golden path", map[string]interface{}{"complianceScore": 40})},
	})
	add("/api/scalability/cluster-fault-tolerance", "get", OpenAPIOperation{
		Summary: "Cluster fault tolerance", OperationID: "clusterFaultTolerance",
		Tags:        []string{"Scalability", "FaultTolerance", "HA"},
		Description: "Evaluates cluster survival across failure scenarios: node loss, zone outage, control plane failure. Identifies weak points and estimates recovery time.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Fault tolerance", map[string]interface{}{"toleranceScore": 40})},
	})
	add("/api/operations/pod-restart-storm", "get", OpenAPIOperation{
		Summary: "Pod restart storm detector", OperationID: "podRestartStorm",
		Tags:        []string{"Operations", "Restart", "Storm"},
		Description: "Detects pod restart storms - cascading restarts across multiple workloads within a short window. Correlation analysis for likely root causes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart storm", map[string]interface{}{"stormScore": 80})},
	})
	add("/api/deployment/deploy-pipeline-audit", "get", OpenAPIOperation{
		Summary: "Deploy pipeline audit", OperationID: "deployPipelineAudit",
		Tags:        []string{"Deployment", "Pipeline", "Audit"},
		Description: "Audits deployment pipeline health: image freshness, probe/resource coverage, rolling strategy, multi-replica readiness. Gap analysis with fix recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pipeline audit", map[string]interface{}{"pipelineScore": 50})},
	})
	add("/api/docs/platform-scorecard-deep", "get", OpenAPIOperation{
		Summary: "Deep platform scorecard", OperationID: "platformScorecardDeep",
		Tags:        []string{"Documentation", "Scorecard", "Assessment"},
		Description: "Comprehensive platform scorecard with weighted scoring across 6 categories: Reliability, Automation, Security, Observability, Cost, Governance. Industry benchmark comparison.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scorecard", map[string]interface{}{"overallScore": 55})},
	})
	add("/api/security/seccomp-profile-gap", "get", OpenAPIOperation{
		Summary: "Seccomp profile gap", OperationID: "seccompProfileGap",
		Tags:        []string{"Security", "Seccomp", "Hardening"},
		Description: "Analyzes which workloads lack seccomp profiles, leaving them vulnerable to unnecessary kernel syscall access. Checks pod and container-level seccomp.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Seccomp gap", map[string]interface{}{"gapScore": 50})},
	})
	add("/api/product/traffic-spike-guard", "get", OpenAPIOperation{
		Summary: "Traffic spike guard", OperationID: "trafficSpikeGuard",
		Tags:        []string{"Product", "Traffic", "Guard"},
		Description: "Monitors anomalous traffic patterns by analyzing service endpoint counts, single-point services, high-fanout, and external exposure via NodePort/LB.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Traffic spike", map[string]interface{}{"guardScore": 70})},
	})
	add("/api/scalability/node-life-forecast", "get", OpenAPIOperation{
		Summary: "Node lifecycle forecaster", OperationID: "nodeLifeForecast",
		Tags:        []string{"Scalability", "Node", "Lifecycle"},
		Description: "Predicts node lifecycle events based on age, health, pressure conditions, kubelet version. Identifies nodes needing replacement, upgrade, or investigation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node forecast", map[string]interface{}{"forecastScore": 60})},
	})
	add("/api/operations/crash-budget-tracker", "get", OpenAPIOperation{
		Summary: "Crash budget tracker", OperationID: "crashBudgetTracker",
		Tags:        []string{"Operations", "Crash", "Budget"},
		Description: "Tracks crash budget consumption per workload. Monthly budget allocation, daily crash rate, budget utilization percentage, and action-needed classification.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Crash budget", map[string]interface{}{"budgetScore": 70})},
	})
	add("/api/deployment/helm-drift-monitor", "get", OpenAPIOperation{
		Summary: "Helm drift monitor", OperationID: "helmDriftMonitor",
		Tags:        []string{"Deployment", "Helm", "Drift"},
		Description: "Monitors Helm release drift by checking release secrets, status, orphaned resources, and chart version consistency.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Helm drift", map[string]interface{}{"monitorScore": 60})},
	})
	add("/api/security/sa-token-lifecycle", "get", OpenAPIOperation{
		Summary: "SA token lifecycle", OperationID: "saTokenLifecycle",
		Tags:        []string{"Security", "ServiceAccount", "Token"},
		Description: "Analyzes ServiceAccount token lifecycle risks including long-lived tokens, auto-mount settings, and unused SAs.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SA token lifecycle", map[string]interface{}{"riskScore": 70})},
	})
	add("/api/product/endpoint-health-deep", "get", OpenAPIOperation{
		Summary: "Endpoint health deep", OperationID: "endpointHealthDeep",
		Tags:        []string{"Product", "Endpoint", "Health"},
		Description: "Deep health analysis of service endpoints including backing pod readiness ratios and degraded service detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Endpoint health", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/overcommit-risk", "get", OpenAPIOperation{
		Summary: "Overcommit risk", OperationID: "overcommitRisk",
		Tags:        []string{"Scalability", "Resource", "Overcommit"},
		Description: "Evaluates cluster overcommit risk by comparing resource requests vs limits vs actual allocatable capacity.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Overcommit risk", map[string]interface{}{"riskScore": 65})},
	})
	add("/api/operations/cluster-version-skew", "get", OpenAPIOperation{
		Summary: "Cluster version skew", OperationID: "clusterVersionSkew",
		Tags:        []string{"Operations", "Version", "Upgrade"},
		Description: "Detects Kubernetes version skew between control plane and kubelet. Flags nodes outside the N-2 supported skew policy.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Version skew", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/node-taint-impact", "get", OpenAPIOperation{
		Summary: "Node taint impact", OperationID: "nodeTaintImpact",
		Tags:        []string{"Operations", "Node", "Taint", "Scheduling"},
		Description: "Analyzes how node taints affect workload scheduling. Identifies blocking taints and untolerated pending pods.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Taint impact", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/operations/api-server-slo", "get", OpenAPIOperation{
		Summary: "API server SLO", OperationID: "apiServerSLO",
		Tags:        []string{"Operations", "API", "SLO"},
		Description: "Measures API server SLO compliance using event success rate and error rate analysis by verb and resource.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API SLO", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/deployment/immutable-config-audit", "get", OpenAPIOperation{
		Summary: "Immutable config audit", OperationID: "immutableConfigAudit",
		Tags:        []string{"Deployment", "Config", "Security"},
		Description: "Audits whether ConfigMaps and Secrets are set to immutable to reduce API server load and prevent accidental changes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Immutable config", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/deployment/sidecar-injection-audit", "get", OpenAPIOperation{
		Summary: "Sidecar injection audit", OperationID: "sidecarInjectionAudit",
		Tags:        []string{"Deployment", "Sidecar", "Mesh"},
		Description: "Audits sidecar container injection compliance and health. Detects missing mesh, logging, and monitoring sidecars.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Sidecar audit", map[string]interface{}{"healthScore": 30})},
	})
	add("/api/deployment/resource-quota-drift", "get", OpenAPIOperation{
		Summary: "Resource quota drift", OperationID: "resourceQuotaDrift",
		Tags:        []string{"Deployment", "Quota", "Governance"},
		Description: "Detects namespace resource quota drift and saturation. Identifies exhausted and saturated quotas.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Quota drift", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/docs/platform-risk-heatmap", "get", OpenAPIOperation{
		Summary: "Platform risk heatmap", OperationID: "platformRiskHeatmap",
		Tags:        []string{"Documentation", "Risk", "Heatmap"},
		Description: "Generates a multi-dimensional risk heatmap across all namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Risk heatmap", map[string]interface{}{"overallScore": 45})},
	})
	add("/api/docs/workload-maturity-matrix", "get", OpenAPIOperation{
		Summary: "Workload maturity matrix", OperationID: "workloadMaturityMatrix",
		Tags:        []string{"Documentation", "Maturity", "Matrix"},
		Description: "Evaluates workload maturity across 6 dimensions.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Maturity matrix", map[string]interface{}{"overallScore": 30})},
	})
	add("/api/docs/incident-playbook", "get", OpenAPIOperation{
		Summary: "Incident playbook", OperationID: "incidentPlaybook",
		Tags:        []string{"Documentation", "Incident", "Playbook"},
		Description: "Generates incident response playbooks for 6 common scenarios.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Incident playbook", map[string]interface{}{"readinessScore": 40})},
	})
	add("/api/product/canary-health", "get", OpenAPIOperation{
		Summary: "Canary health", OperationID: "canaryHealth",
		Tags:        []string{"Product", "Deployment", "Canary"},
		Description: "Analyzes canary and progressive deployment health. Detects stalled rollouts, restart storms, and replica gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Canary health", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/product/pvc-io-health", "get", OpenAPIOperation{
		Summary: "PVC IO health", OperationID: "pvcIOHealth",
		Tags:        []string{"Product", "Storage", "PVC"},
		Description: "Monitors PVC health: orphaned volumes, missing backups, large volumes, storage class distribution.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PVC health", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/product/ingress-conflict", "get", OpenAPIOperation{
		Summary: "Ingress conflict", OperationID: "ingressConflict",
		Tags:        []string{"Product", "Networking", "Ingress"},
		Description: "Detects ingress path conflicts, missing backend services, stale rules, and TLS gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ingress conflict", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/security/privilege-escalation-path", "get", OpenAPIOperation{
		Summary: "Privilege escalation path", OperationID: "privilegeEscalationPath",
		Tags:        []string{"Security", "RBAC", "Privilege"},
		Description: "Detects potential privilege escalation paths via RBAC: cluster-admin, escalate, bind, impersonate permissions.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Privilege escalation", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/security/network-segment-gap", "get", OpenAPIOperation{
		Summary: "Network segment gap", OperationID: "networkSegmentGap",
		Tags:        []string{"Security", "Network", "Segmentation"},
		Description: "Analyzes network segmentation gaps between namespaces. Identifies unisolated namespace pairs.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Network segmentation", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/security/image-baseline-drift", "get", OpenAPIOperation{
		Summary: "Image baseline drift", OperationID: "imageBaselineDrift",
		Tags:        []string{"Security", "Supply Chain", "Image"},
		Description: "Detects image drift by checking digest pinning, latest tag usage, and version tag consistency.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image baseline", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/pod-affinity-spread", "get", OpenAPIOperation{
		Summary: "Pod affinity spread", OperationID: "podAffinitySpread",
		Tags:        []string{"Scalability", "Scheduling", "HA"},
		Description: "Analyzes pod anti-affinity effectiveness and topology spread. Detects co-located replicas and single points of failure.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Affinity spread", map[string]interface{}{"healthScore": 45})},
	})
	add("/api/scalability/namespace-budget-enforce", "get", OpenAPIOperation{
		Summary: "Namespace budget enforce", OperationID: "namespaceBudgetEnforce",
		Tags:        []string{"Scalability", "Cost", "Governance"},
		Description: "Audits namespace resource budgets. Identifies namespaces without quotas, high spenders, and missing limit ranges.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Budget enforcement", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/resource-waste-deep", "get", OpenAPIOperation{
		Summary: "Resource waste deep", OperationID: "resourceWasteDeep",
		Tags:        []string{"Scalability", "Cost", "Waste"},
		Description: "Deep resource waste analysis: idle workloads, over-provisioned containers, zombie PVCs and ConfigMaps, with cost estimates.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource waste", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/operations/cert-transparency-monitor", "get", OpenAPIOperation{
		Summary: "Cert transparency", OperationID: "certTransparency",
		Tags:        []string{"Operations", "Certificate", "TLS"},
		Description: "Monitors TLS certificates across ingress and services for expiry and transparency.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cert transparency", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/coredns-config-audit", "get", OpenAPIOperation{
		Summary: "CoreDNS config audit", OperationID: "corednsConfigAudit",
		Tags:        []string{"Operations", "DNS", "Config"},
		Description: "Audits CoreDNS configuration health including memory limits, caching, and NodeLocalDNS.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("CoreDNS config", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/operations/webhook-timeout-audit", "get", OpenAPIOperation{
		Summary: "Webhook timeout audit", OperationID: "webhookTimeoutAudit",
		Tags:        []string{"Operations", "Admission", "Webhook"},
		Description: "Analyzes admission webhook configurations for timeout and failure risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Webhook audit", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/deployment/revision-history-hygiene", "get", OpenAPIOperation{
		Summary: "Revision history hygiene", OperationID: "revisionHistoryHygiene",
		Tags:        []string{"Deployment", "Cleanup", "Revision"},
		Description: "Audits deployment revision history limits and identifies wasteful old ReplicaSets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Revision history", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/resource-limit-coverage", "get", OpenAPIOperation{
		Summary: "Resource limit coverage", OperationID: "resourceLimitCoverage",
		Tags:        []string{"Deployment", "Resources", "Coverage"},
		Description: "Audits what fraction of containers have CPU/memory requests and limits set.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource coverage", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/deployment/ephemeral-storage-quota", "get", OpenAPIOperation{
		Summary: "Ephemeral storage quota", OperationID: "ephemeralStorageQuota",
		Tags:        []string{"Deployment", "Storage", "Quota"},
		Description: "Audits ephemeral storage usage and limits. Identifies pods without storage limits.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ephemeral storage", map[string]interface{}{"healthScore": 10})},
	})
	add("/api/docs/tech-debt-radar", "get", OpenAPIOperation{
		Summary: "Tech debt radar", OperationID: "techDebtRadar",
		Tags:        []string{"Documentation", "Debt", "Radar"},
		Description: "Tracks technical debt across the cluster with severity scoring by category.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Tech debt", map[string]interface{}{"healthScore": 30})},
	})
	add("/api/docs/sre-scorecard", "get", OpenAPIOperation{
		Summary: "SRE scorecard", OperationID: "sreScorecard",
		Tags:        []string{"Documentation", "SRE", "Reliability"},
		Description: "Generates SRE scorecard using error budget, availability, and change failure rate.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SRE scorecard", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/docs/compliance-crosswalk", "get", OpenAPIOperation{
		Summary: "Compliance crosswalk", OperationID: "complianceCrosswalk",
		Tags:        []string{"Documentation", "Compliance", "Crosswalk"},
		Description: "Maps cluster findings to multiple compliance frameworks: CIS, NIST, PCI-DSS, SOC2.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Compliance crosswalk", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/product/secret-mount-audit", "get", OpenAPIOperation{
		Summary: "Secret mount audit", OperationID: "secretMountAudit",
		Tags:        []string{"Product", "Secret", "Security"},
		Description: "Audits how secrets are mounted and detects risky patterns like env var exposure.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret mount audit", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/product/label-propagation", "get", OpenAPIOperation{
		Summary: "Label propagation", OperationID: "labelPropagation",
		Tags:        []string{"Product", "Label", "Governance"},
		Description: "Audits label consistency and propagation across workloads and services.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Label propagation", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/cronjob-orphan-audit", "get", OpenAPIOperation{
		Summary: "CronJob orphan audit", OperationID: "cronJobOrphanAudit",
		Tags:        []string{"Product", "CronJob", "Batch"},
		Description: "Detects orphaned or misconfigured CronJobs.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("CronJob orphan", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/security/hostpath-audit", "get", OpenAPIOperation{
		Summary: "HostPath audit", OperationID: "hostpathAudit",
		Tags:        []string{"Security", "Volume", "Isolation"},
		Description: "Detects hostPath volume mounts that bypass container isolation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HostPath audit", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/security/container-capabilities", "get", OpenAPIOperation{
		Summary: "Container capabilities", OperationID: "containerCapabilities",
		Tags:        []string{"Security", "Capability", "Container"},
		Description: "Audits Linux capabilities granted to containers. Detects privileged and dangerous caps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Container caps", map[string]interface{}{"healthScore": 20})},
	})
	add("/api/security/readonly-rootfs-audit", "get", OpenAPIOperation{
		Summary: "Readonly rootfs audit", OperationID: "readonlyRootfsAudit",
		Tags:        []string{"Security", "Filesystem", "Container"},
		Description: "Audits whether containers use read-only root filesystem.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Readonly rootfs", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/hpa-cooldown-audit", "get", OpenAPIOperation{
		Summary: "HPA cooldown audit", OperationID: "hpaCooldownAudit",
		Tags:        []string{"Scalability", "HPA", "Autoscaling"},
		Description: "Analyzes HPA scaling behavior and cooldown configuration for flapping risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HPA cooldown", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/scalability/resource-request-saturation", "get", OpenAPIOperation{
		Summary: "Resource request saturation", OperationID: "resourceRequestSaturation",
		Tags:        []string{"Scalability", "Resource", "Saturation"},
		Description: "Analyzes resource request vs node capacity saturation per node and namespace.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Request saturation", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/scalability/cluster-pod-limit", "get", OpenAPIOperation{
		Summary: "Cluster pod limit", OperationID: "clusterPodLimit",
		Tags:        []string{"Scalability", "Capacity", "Pod"},
		Description: "Forecasts when cluster runs out of pod IP / allocation capacity.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod limit", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/operations/pod-restart-forensics-deep", "get", OpenAPIOperation{
		Summary: "Pod restart forensics deep", OperationID: "podRestartForensicsDeep",
		Tags:        []string{"Operations", "Pod", "Forensics"},
		Description: "Deep forensic analysis of pod restart patterns with root cause guessing and timeline.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart forensics", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/operations/deployment-health-trend", "get", OpenAPIOperation{
		Summary: "Deployment health trend", OperationID: "deploymentHealthTrend",
		Tags:        []string{"Operations", "Deployment", "Health"},
		Description: "Analyzes deployment health trends over recent replicas and restart data.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy health", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/event-correlation-matrix", "get", OpenAPIOperation{
		Summary: "Event correlation matrix", OperationID: "eventCorrelationMatrix",
		Tags:        []string{"Operations", "Events", "Correlation"},
		Description: "Correlates events across namespaces to find systemic patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Event correlation", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/deployment/image-pull-latency", "get", OpenAPIOperation{
		Summary: "Image pull latency", OperationID: "imagePullLatency",
		Tags:        []string{"Deployment", "Image", "Registry"},
		Description: "Analyzes image pull performance and registry health.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image pull", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/probe-timeout-audit", "get", OpenAPIOperation{
		Summary: "Probe timeout audit", OperationID: "probeTimeoutAudit",
		Tags:        []string{"Deployment", "Probe", "Health"},
		Description: "Audits liveness/readiness probe timeout and interval configurations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Probe timeout", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/deployment/init-container-health", "get", OpenAPIOperation{
		Summary: "Init container health", OperationID: "initContainerHealth",
		Tags:        []string{"Deployment", "Init", "Container"},
		Description: "Audits init container configurations and failure patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Init health", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/docs/cost-optimization-roadmap", "get", OpenAPIOperation{
		Summary: "Cost optimization roadmap", OperationID: "costOptimizationRoadmap",
		Tags:        []string{"Documentation", "Cost", "Roadmap"},
		Description: "Generates a prioritized cost optimization plan with quick wins, medium-term and long-term actions.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cost roadmap", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/docs/security-posture-trend", "get", OpenAPIOperation{
		Summary: "Security posture trend", OperationID: "securityPostureTrend",
		Tags:        []string{"Documentation", "Security", "Trend"},
		Description: "Tracks security posture changes and trends using container security context analysis.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Security trend", map[string]interface{}{"healthScore": 20})},
	})
	add("/api/docs/capacity-planning-report", "get", OpenAPIOperation{
		Summary: "Capacity planning report", OperationID: "capacityPlanningReport",
		Tags:        []string{"Documentation", "Capacity", "Planning"},
		Description: "Generates a comprehensive capacity planning report with 3-month and 6-month forecasts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity report", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/env-var-drift-detect", "get", OpenAPIOperation{
		Summary: "Env var drift detect", OperationID: "envVarDriftDetect",
		Tags:        []string{"Product", "Config", "Drift"},
		Description: "Detects environment variable inconsistencies across same-name workloads in different namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Env drift", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/product/dns-record-audit", "get", OpenAPIOperation{
		Summary: "DNS record audit", OperationID: "dnsRecordAudit",
		Tags:        []string{"Product", "DNS", "Service"},
		Description: "Checks DNS records for services and ingresses. Identifies orphaned services and stale ingresses.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DNS audit", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/product/workload-startup-profile", "get", OpenAPIOperation{
		Summary: "Workload startup profile", OperationID: "workloadStartupProfile",
		Tags:        []string{"Product", "Startup", "Profile"},
		Description: "Profiles workload startup time and initialization patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Startup profile", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/seccomp-profile-audit", "get", OpenAPIOperation{
		Summary: "Seccomp profile audit", OperationID: "seccompProfileAudit",
		Tags:        []string{"Security", "Seccomp", "Runtime"},
		Description: "Audits seccomp profile settings across all pods.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Seccomp audit", map[string]interface{}{"healthScore": 5})},
	})
	add("/api/security/sa-token-age", "get", OpenAPIOperation{
		Summary: "SA token age", OperationID: "saTokenAge",
		Tags:        []string{"Security", "ServiceAccount", "Token"},
		Description: "Audits service account token ages and rotation status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SA token age", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/security/runtime-class-audit", "get", OpenAPIOperation{
		Summary: "Runtime class audit", OperationID: "runtimeClassAudit",
		Tags:        []string{"Security", "Runtime", "Isolation"},
		Description: "Audits RuntimeClass adoption for container isolation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Runtime class", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/pdb-gap-analysis", "get", OpenAPIOperation{
		Summary: "PDB gap analysis", OperationID: "pdbGapAnalysis",
		Tags:        []string{"Scalability", "PDB", "HA"},
		Description: "Analyzes PodDisruptionBudget coverage gaps for multi-replica deployments.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PDB gap", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/topology-spread-violation", "get", OpenAPIOperation{
		Summary: "Topology spread violation", OperationID: "topologySpreadViolation",
		Tags:        []string{"Scalability", "Topology", "Spread"},
		Description: "Detects topology spread constraint violations and uneven pod distribution.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Topology spread", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/scalability/overcommit-deep", "get", OpenAPIOperation{
		Summary: "Overcommit deep", OperationID: "overcommitDeep",
		Tags:        []string{"Scalability", "Resource", "Overcommit"},
		Description: "Deep overcommit analysis with bin-packing efficiency scoring.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Overcommit deep", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/operations/node-condition-trend", "get", OpenAPIOperation{
		Summary: "Node condition trend", OperationID: "nodeConditionTrend",
		Tags:        []string{"Operations", "Node", "Condition"},
		Description: "Tracks node condition flapping and stability across the cluster.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node condition", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/operations/container-log-size", "get", OpenAPIOperation{
		Summary: "Container log size", OperationID: "containerLogSize",
		Tags:        []string{"Operations", "Log", "Storage"},
		Description: "Estimates container log disk usage and identifies pods without log policies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Log size", map[string]interface{}{"healthScore": 20})},
	})
	add("/api/operations/kubelet-config-drift", "get", OpenAPIOperation{
		Summary: "Kubelet config drift", OperationID: "kubeletConfigDrift",
		Tags:        []string{"Operations", "Kubelet", "Config"},
		Description: "Detects kubelet configuration inconsistencies across nodes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Kubelet drift", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/deployment/rollout-blocker-detect", "get", OpenAPIOperation{
		Summary: "Rollout blocker detect", OperationID: "rolloutBlockerDetect",
		Tags:        []string{"Deployment", "Rollout", "Blocker"},
		Description: "Detects deployments with conditions blocking successful rollouts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollout blocker", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/deployment/termination-grace-audit", "get", OpenAPIOperation{
		Summary: "Termination grace audit", OperationID: "terminationGraceAudit",
		Tags:        []string{"Deployment", "Shutdown", "Grace"},
		Description: "Audits terminationGracePeriodSeconds and preStop hooks for graceful shutdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Termination grace", map[string]interface{}{"healthScore": 30})},
	})
	add("/api/deployment/max-surge-audit", "get", OpenAPIOperation{
		Summary: "Max surge audit", OperationID: "maxSurgeAudit",
		Tags:        []string{"Deployment", "Strategy", "Surge"},
		Description: "Analyzes rolling update maxSurge/maxUnavailable configuration.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Max surge", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/api-coverage-gap", "get", OpenAPIOperation{
		Summary: "API coverage gap", OperationID: "apiCoverageGap",
		Tags:        []string{"Documentation", "Coverage", "Gap"},
		Description: "Analyzes which Kubernetes resource types are underrepresented in API coverage. Identifies blind spots in observability for critical resources.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API coverage", map[string]interface{}{"coverageScore": 55})},
	})
	add("/api/docs/backup-compliance-deep", "get", OpenAPIOperation{
		Summary: "Backup compliance deep", OperationID: "backupComplianceDeep",
		Tags:        []string{"Documentation", "Backup", "DR"},
		Description: "Deep backup compliance audit: namespace backup policy coverage, PVC snapshot status, secret backup risk, DR checklist with etcd/encryption/replication/restore-test verification.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Backup compliance", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/docs/label-taxonomy-standard", "get", OpenAPIOperation{
		Summary: "Label taxonomy standard", OperationID: "labelTaxonomyStandard",
		Tags:        []string{"Documentation", "Labels", "Governance"},
		Description: "Analyzes label usage across all resources, detects inconsistencies (case, naming variants), and recommends standardized K8s app.kubernetes.io/* label taxonomy.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Label taxonomy", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/docs/change-impact-brief", "get", OpenAPIOperation{
		Summary: "Change impact brief", OperationID: "changeImpactBrief",
		Tags:        []string{"Documentation", "Change", "Impact"},
		Description: "Structured change impact assessment: analyzes recent cluster changes (72h), blast radius per deployment, rollback readiness (revision history + PDB), and risk areas.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Change impact", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/product/priority-class-audit", "get", OpenAPIOperation{
		Summary: "Priority class audit", OperationID: "priorityClassAudit",
		Tags:        []string{"Product", "Scheduling", "Priority"},
		Description: "Audits priority class usage across workloads, detects preemption risk pairs, and identifies workloads without priority class assignments.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Priority class audit", map[string]interface{}{"healthScore": 30})},
	})
	add("/api/product/service-exposure-map", "get", OpenAPIOperation{
		Summary: "Service exposure map", OperationID: "serviceExposureMap",
		Tags:        []string{"Product", "Network", "Security"},
		Description: "Maps all service exposure paths (ClusterIP, NodePort, LoadBalancer, Ingress) and identifies over-exposed services that should be internal-only.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Service exposure", map[string]interface{}{"healthScore": 65})},
	})
	add("/api/product/antiaffinity-ha", "get", OpenAPIOperation{
		Summary: "Anti-affinity HA readiness", OperationID: "antiaffinityHA",
		Tags:        []string{"Product", "HA", "Scheduling"},
		Description: "Analyzes pod anti-affinity rules and topology spread constraints for HA readiness. Identifies single-replica workloads and multi-replica workloads without proper distribution.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Anti-affinity HA", map[string]interface{}{"healthScore": 25})},
	})
	add("/api/security/image-registry-allowlist", "get", OpenAPIOperation{
		Summary: "Image registry allowlist", OperationID: "imageRegistryAllowlist",
		Tags:        []string{"Security", "SupplyChain", "Images"},
		Description: "Audits container image registry trust posture: identifies untrusted registries, :latest tags, images without digest pinning.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image registry", map[string]interface{}{"healthScore": 40})},
	})
	add("/api/security/sa-mount-exposure", "get", OpenAPIOperation{
		Summary: "SA mount exposure", OperationID: "saMountExposure",
		Tags:        []string{"Security", "RBAC", "ServiceAccount"},
		Description: "Audits ServiceAccount token auto-mount exposure: identifies over-mounted tokens, high-privilege SAs, unused SAs.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SA mount exposure", map[string]interface{}{"healthScore": 20})},
	})
	add("/api/security/tls-version-audit", "get", OpenAPIOperation{
		Summary: "TLS version audit", OperationID: "tlsVersionAudit",
		Tags:        []string{"Security", "TLS", "Certificates"},
		Description: "Audits TLS configuration: Ingress TLS coverage, certificate expiry, weak key sizes, self-signed certs, deprecated X.509 versions.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("TLS audit", map[string]interface{}{"healthScore": 55})},
	})
	add("/api/scalability/mem-pressure-forecast", "get", OpenAPIOperation{
		Summary: "Memory pressure forecast", OperationID: "memPressureForecast",
		Tags:        []string{"Scalability", "Memory", "Forecast"},
		Description: "Predicts node memory exhaustion based on pod requests, overcommit ratio, and usage trends. Identifies critical nodes at risk of OOM kills.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Memory pressure", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/scale-concurrency", "get", OpenAPIOperation{
		Summary: "Scaling concurrency limit", OperationID: "scaleConcurrency",
		Tags:        []string{"Scalability", "Scaling", "Capacity"},
		Description: "Calculates how many workloads can scale simultaneously without resource exhaustion. Identifies CPU/memory/pod bottleneck and per-workload scale-up potential.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scale concurrency", map[string]interface{}{"healthScore": 65})},
	})
	add("/api/scalability/disruption-window", "get", OpenAPIOperation{
		Summary: "Disruption window", OperationID: "disruptionWindow",
		Tags:        []string{"Scalability", "HA", "Maintenance"},
		Description: "Analyzes safe pod disruption windows based on PDBs, replica counts, and single-replica risk. Calculates total disruption budget and recommends maintenance windows.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Disruption window", map[string]interface{}{"healthScore": 25})},
	})
	add("/api/deployment/deploy-reproducibility", "get", OpenAPIOperation{
		Summary: "Deploy reproducibility", OperationID: "deployReproducibility",
		Tags:        []string{"Deployment", "Reproducibility", "SupplyChain"},
		Description: "Audits deployment reproducibility: image tag pinning, digest usage, hardcoded env vars, ConfigMap references. Identifies non-reproducible workloads.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Reproducibility", map[string]interface{}{"healthScore": 30})},
	})
	add("/api/deployment/update-compliance-deep", "get", OpenAPIOperation{
		Summary: "Update compliance deep", OperationID: "updateComplianceDeep",
		Tags:        []string{"Deployment", "Strategy", "Compliance"},
		Description: "Deep audit of deployment update strategy compliance: RollingUpdate vs Recreate, maxSurge/maxUnavailable, progressDeadlineSeconds, revision history limits.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Update compliance", map[string]interface{}{"healthScore": 65})},
	})
	add("/api/deployment/restart-policy-deep", "get", OpenAPIOperation{
		Summary: "Restart policy deep", OperationID: "restartPolicyDeep",
		Tags:        []string{"Deployment", "Restart", "Health"},
		Description: "Deep analysis of container restart policies: liveness probe coverage, restart history patterns, CrashLoopBackOff detection, preStop hook configuration.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart policy", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/operations/pod-phase-timeline", "get", OpenAPIOperation{
		Summary: "Pod phase timeline", OperationID: "podPhaseTimeline",
		Tags:        []string{"Operations", "PodHealth", "Lifecycle"},
		Description: "Analyzes pod lifecycle phase distribution: Pending, Running, Failed, Succeeded. Identifies stale pods, long-pending pods, very old running pods, and namespace phase breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod phase timeline", map[string]interface{}{"healthScore": 96})},
	})
	add("/api/operations/image-gc-pressure", "get", OpenAPIOperation{
		Summary: "Image GC pressure", OperationID: "imageGCPressure",
		Tags:        []string{"Operations", "Images", "Disk"},
		Description: "Monitors image garbage collection pressure: duplicate image variants, unused images, node disk pressure conditions, and estimated cleanup savings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image GC", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/controller-reconcile", "get", OpenAPIOperation{
		Summary: "Controller reconcile", OperationID: "controllerReconcile",
		Tags:        []string{"Operations", "Controller", "Reconcile"},
		Description: "Analyzes controller reconcile health: replica mismatches, unhealthy deployment conditions, orphaned pods without owners, and owner chain tracking.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Controller reconcile", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/pod-escape-risk", "get", OpenAPIOperation{
		Summary: "Pod escape risk", OperationID: "podEscapeRisk",
		Tags:        []string{"Security", "PodSecurity", "Isolation"},
		Description: "Identifies containers that could escape isolation: privileged mode, hostPID/hostIPC/hostNetwork, dangerous capabilities, hostPath mounts, runAsRoot.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod escape risk", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/security/egress-policy-gap", "get", OpenAPIOperation{
		Summary: "Egress policy gap", OperationID: "egressPolicyGap",
		Tags:        []string{"Security", "Network", "Egress"},
		Description: "Analyzes missing egress network policies: namespaces without egress control, default-deny coverage, zero-trust posture assessment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Egress policy", map[string]interface{}{"healthScore": 10})},
	})
	add("/api/security/cis-benchmark-lite", "get", OpenAPIOperation{
		Summary: "CIS benchmark lite", OperationID: "cisBenchmarkLite",
		Tags:        []string{"Security", "Compliance", "CIS"},
		Description: "Basic CIS Kubernetes Benchmark checks: RBAC, pod security, network policies, wildcard permissions. 11 automated checks with remediation guidance.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("CIS benchmark", map[string]interface{}{"cisScore": 45})},
	})
	add("/api/docs/ownership-registry", "get", OpenAPIOperation{
		Summary: "Ownership registry", OperationID: "ownershipRegistry",
		Tags:        []string{"Documentation", "Ownership", "Governance"},
		Description: "Maps resource ownership and team accountability: identifies workloads without team labels, critical unowned resources, team distribution.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ownership registry", map[string]interface{}{"healthScore": 41})},
	})
	add("/api/docs/release-note-gen", "get", OpenAPIOperation{
		Summary: "Release note generator", OperationID: "releaseNoteGen",
		Tags:        []string{"Documentation", "Release", "Changelog"},
		Description: "Auto-generates release notes from recent cluster changes (24h window): image updates, config changes, scaling events with markdown output.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Release notes", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/docs/incident-postmortem", "get", OpenAPIOperation{
		Summary: "Incident postmortem", OperationID: "incidentPostmortem",
		Tags:        []string{"Documentation", "Incident", "Postmortem"},
		Description: "Generates structured incident postmortem template from detected incidents (72h): OOM kills, CrashLoopBackOff, node failures, with action items and timeline.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Postmortem", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/product/storage-class-audit", "get", OpenAPIOperation{
		Summary: "Storage class audit", OperationID: "storageClassAudit",
		Tags:        []string{"Product", "Storage", "Performance"},
		Description: "Audits storage class usage, PVC binding status, provisioner types, reclaim policies, and expansion support.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage class", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/workload-interdependency", "get", OpenAPIOperation{
		Summary: "Workload interdependency", OperationID: "workloadInterdependency",
		Tags:        []string{"Product", "Network", "Dependency"},
		Description: "Maps service-to-service dependencies via env var references, identifies hub services with high fan-in, orphaned services with no dependents.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Interdependency", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/product/dns-resolution-health", "get", OpenAPIOperation{
		Summary: "DNS resolution health", OperationID: "dnsResolutionHealth",
		Tags:        []string{"Product", "DNS", "Network"},
		Description: "Analyzes CoreDNS deployment health, config issues, service DNS naming, headless services, and external name mappings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DNS health", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/request-efficiency", "get", OpenAPIOperation{
		Summary: "Request efficiency", OperationID: "requestEfficiency",
		Tags:        []string{"Scalability", "Resources", "Efficiency"},
		Description: "Analyzes resource request vs actual usage: over-provisioned containers, missing requests/limits, per-namespace resource spend.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Request efficiency", map[string]interface{}{"healthScore": 62})},
	})
	add("/api/scalability/bin-packing-score", "get", OpenAPIOperation{
		Summary: "Bin-packing score", OperationID: "binPackingScore",
		Tags:        []string{"Scalability", "Scheduling", "Efficiency"},
		Description: "Calculates node bin-packing efficiency: CPU/memory usage per node, fragmentation report, overall packing score.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Bin-packing", map[string]interface{}{"healthScore": 18})},
	})
	add("/api/scalability/multi-zone-ha", "get", OpenAPIOperation{
		Summary: "Multi-zone HA", OperationID: "multiZoneHA",
		Tags:        []string{"Scalability", "HA", "FaultDomain"},
		Description: "Analyzes multi-zone fault domain distribution: zone spread, single-zone workloads at risk, topology spread constraint coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Multi-zone HA", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/deployment/graceful-shutdown-audit", "get", OpenAPIOperation{
		Summary: "Graceful shutdown audit", OperationID: "gracefulShutdownAudit",
		Tags:        []string{"Deployment", "Lifecycle", "Shutdown"},
		Description: "Audits graceful shutdown readiness: preStop hooks, terminationGracePeriodSeconds, readiness probes for connection draining.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Graceful shutdown", map[string]interface{}{"healthScore": 10})},
	})
	add("/api/deployment/rollout-speed", "get", OpenAPIOperation{
		Summary: "Rollout speed analyzer", OperationID: "rolloutSpeed",
		Tags:        []string{"Deployment", "Rollout", "Performance"},
		Description: "Analyzes rollout speed and progress: RollingUpdate vs Recreate, maxSurge/maxUnavailable, updated/ready replicas, revision history.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollout speed", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/deployment/deploy-conflict", "get", OpenAPIOperation{
		Summary: "Deploy conflict detector", OperationID: "deployConflict",
		Tags:        []string{"Deployment", "Conflict", "Scheduling"},
		Description: "Detects deployment conflicts: name collisions across namespaces, resource pressure zones, concurrent deployment windows.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy conflicts", map[string]interface{}{"healthScore": 95})},
	})
	add("/api/operations/node-maint-window", "get", OpenAPIOperation{
		Summary: "Node maintenance window", OperationID: "nodeMaintWindow",
		Tags:        []string{"Operations", "Maintenance", "NodeHealth"},
		Description: "Analyzes cordon/drain impact: pods affected per node, PDB coverage, safe maintenance windows.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Maintenance window", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/operations/resource-leak-detector", "get", OpenAPIOperation{
		Summary: "Resource leak detector", OperationID: "resourceLeakDetector",
		Tags:        []string{"Operations", "Cleanup", "Waste"},
		Description: "Detects orphaned ConfigMaps, Secrets, and PVCs not referenced by any pod. Identifies resource leaks and estimated waste.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource leaks", map[string]interface{}{"healthScore": 52})},
	})
	add("/api/operations/log-agg-health", "get", OpenAPIOperation{
		Summary: "Log aggregation health", OperationID: "logAggHealth",
		Tags:        []string{"Operations", "Logging", "Observability"},
		Description: "Analyzes container log health: noisy loggers with high restart counts, silent containers, per-namespace log patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Log agg health", map[string]interface{}{"healthScore": 97})},
	})
	add("/api/security/vol-encryption-audit", "get", OpenAPIOperation{
		Summary: "Volume encryption audit", OperationID: "volEncryptionAudit",
		Tags:        []string{"Security", "Storage", "Encryption"},
		Description: "Audits volume encryption posture: PVC encryption status via StorageClass parameters, identifies unencrypted volumes in namespaces with sensitive data.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Volume encryption", map[string]interface{}{"healthScore": 33})},
	})
	add("/api/security/webhook-posture", "get", OpenAPIOperation{
		Summary: "Webhook posture", OperationID: "webhookPosture",
		Tags:        []string{"Security", "Admission", "Webhook"},
		Description: "Audits admission webhook posture: TLS/CA bundle coverage, timeout configuration, failure policy, latency risk.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Webhook posture", map[string]interface{}{"healthScore": 83})},
	})
	add("/api/security/key-rotation-compliance", "get", OpenAPIOperation{
		Summary: "Key rotation compliance", OperationID: "keyRotationCompliance",
		Tags:        []string{"Security", "Secrets", "Rotation"},
		Description: "Tracks secret key rotation compliance: identifies overdue secrets (>90d, >180d, >365d), categorizes by freshness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Key rotation", map[string]interface{}{"healthScore": 10})},
	})
	add("/api/docs/cluster-runbook-gen", "get", OpenAPIOperation{
		Summary: "Cluster runbook generator", OperationID: "clusterRunbookGen",
		Tags:        []string{"Documentation", "Runbook", "Operations"},
		Description: "Auto-generates a cluster operations runbook with SOPs for NodeNotReady, CrashLoopBackOff, PVC stuck pending, and DNS failure scenarios.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cluster runbook", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/api-drift-detector", "get", OpenAPIOperation{
		Summary: "API drift detector", OperationID: "apiDriftDetector",
		Tags:        []string{"Documentation", "API", "Upgrade"},
		Description: "Detects API version drift: identifies deprecated, removed, and preview APIs from server preferred resources. Maps known deprecated versions to replacements.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API drift", map[string]interface{}{"healthScore": 93})},
	})
	add("/api/docs/resource-topology-doc", "get", OpenAPIOperation{
		Summary: "Resource topology doc", OperationID: "resourceTopologyDoc",
		Tags:        []string{"Documentation", "Topology", "Architecture"},
		Description: "Generates resource topology documentation: namespace-level workload/service/PVC/ingress mapping, critical traffic paths, markdown export.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Topology doc", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/product/cost-attribution", "get", OpenAPIOperation{
		Summary: "Cost attribution matrix", OperationID: "costAttribution",
		Tags:        []string{"Product", "Cost", "FinOps"},
		Description: "Analyzes resource cost attribution by namespace and team: CPU/memory/PVC costs, waste detection for over-provisioned workloads, estimated monthly spend.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cost attribution", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/product/quota-forecast", "get", OpenAPIOperation{
		Summary: "Quota utilization forecast", OperationID: "quotaForecast",
		Tags:        []string{"Product", "Quota", "Capacity"},
		Description: "Forecasts resource quota utilization: tracks ResourceQuota usage per namespace, identifies critical (>90%) and high (>75%) usage, lists namespaces without quotas.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Quota forecast", map[string]interface{}{"healthScore": 55})},
	})
	add("/api/product/mesh-readiness-deep", "get", OpenAPIOperation{
		Summary: "Mesh readiness deep", OperationID: "meshReadinessDeep",
		Tags:        []string{"Product", "ServiceMesh", "Readiness"},
		Description: "Deep service mesh adoption readiness scan: checks sidecar injection status, named ports, liveness/readiness probes, identifies blockers preventing mesh adoption.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Mesh readiness", map[string]interface{}{"healthScore": 50})},
	})
	add("/api/scalability/hpa-effectiveness", "get", OpenAPIOperation{
		Summary: "HPA effectiveness", OperationID: "hpaEffectiveness",
		Tags:        []string{"Scalability", "HPA", "Autoscaling"},
		Description: "Analyzes HPA effectiveness: scaling activity status, at-max/at-min replicas, misconfigured HPAs, deployments missing HPA.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HPA effectiveness", map[string]interface{}{"healthScore": 7})},
	})
	add("/api/scalability/scheduling-latency", "get", OpenAPIOperation{
		Summary: "Scheduling latency", OperationID: "schedulingLatency",
		Tags:        []string{"Scalability", "Scheduling", "Performance"},
		Description: "Measures pod scheduling latency: average/P95 scheduling time, pending/unschedulable pods, per-namespace breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scheduling latency", map[string]interface{}{"healthScore": 62})},
	})
	add("/api/scalability/capacity-headroom-v2", "get", OpenAPIOperation{
		Summary: "Capacity headroom v2", OperationID: "capacityHeadroomV2",
		Tags:        []string{"Scalability", "Capacity", "Planning"},
		Description: "Calculates cluster capacity headroom: available CPU/memory/pod slots per node, additional pods that can be scheduled, scaling capacity by resource dimension.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity headroom", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/product/env-secret-leak", "get", OpenAPIOperation{
		Summary: "Env secret leak detector", OperationID: "envSecretLeak",
		Tags:        []string{"Product", "Security", "Configuration"},
		Description: "Scans container environment variables for hardcoded secrets: passwords, API keys, tokens, private keys, connection strings. Identifies values that should use SecretKeyRef instead.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Env secret leak", map[string]interface{}{"healthScore": 86})},
	})
	add("/api/product/probe-coverage-gap", "get", OpenAPIOperation{
		Summary: "Probe coverage gap", OperationID: "probeCoverageGap",
		Tags:        []string{"Product", "Health", "Reliability"},
		Description: "Identifies containers missing liveness/readiness/startup probes. Critical for traffic routing and zero-downtime deployments.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Probe coverage", map[string]interface{}{"healthScore": 36})},
	})
	add("/api/product/gpu-audit", "get", OpenAPIOperation{
		Summary: "GPU/accelerator audit", OperationID: "gpuAudit",
		Tags:        []string{"Product", "GPU", "Resources"},
		Description: "Audits GPU resource requests vs node availability: identifies workloads requesting GPU, node GPU capacity, over-subscription risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("GPU audit", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/operations/backup-snapshot-audit", "get", OpenAPIOperation{
		Summary: "Backup snapshot audit", OperationID: "backupSnapshotAudit",
		Tags:        []string{"Operations", "Backup", "DR"},
		Description: "Audits backup snapshot coverage: identifies PVCs without VolumeSnapshot protection, tracks snapshot readiness and age, lists unprotected namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Backup audit", map[string]interface{}{"healthScore": 33})},
	})
	add("/api/operations/job-success-rate", "get", OpenAPIOperation{
		Summary: "Job success rate", OperationID: "jobSuccessRate",
		Tags:        []string{"Operations", "Jobs", "Batch"},
		Description: "Analyzes Job and CronJob success rates: tracks failed jobs, long-running jobs, per-namespace breakdown, average duration.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Job success", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/event-retention", "get", OpenAPIOperation{
		Summary: "Event retention & volume", OperationID: "eventRetention",
		Tags:        []string{"Operations", "Events", "Observability"},
		Description: "Analyzes Kubernetes event volume: total events, warning ratio, noisy components, per-namespace distribution, unique reasons breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Event retention", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/security/capability-audit", "get", OpenAPIOperation{
		Summary: "Linux capability audit", OperationID: "capabilityAudit",
		Tags:        []string{"Security", "Capabilities", "Hardening"},
		Description: "Audits Linux capabilities: privileged containers, high-risk cap-add (CAP_SYS_ADMIN, CAP_NET_ADMIN), best-practice cap-drop ALL, per-namespace risk.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capability audit", map[string]interface{}{"healthScore": 86})},
	})
	add("/api/security/host-namespace-audit", "get", OpenAPIOperation{
		Summary: "Host namespace audit", OperationID: "hostNamespaceAudit",
		Tags:        []string{"Security", "Namespace", "Isolation"},
		Description: "Audits host namespace access: hostPID, hostNetwork, hostIPC, hostPath volume mounts (restricted vs unrestricted).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Host namespace audit", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/security/pss-compliance", "get", OpenAPIOperation{
		Summary: "Pod Security Standard compliance", OperationID: "pssCompliance",
		Tags:        []string{"Security", "PSS", "Compliance"},
		Description: "Checks Pod Security Standards compliance: baseline level (no privileged, no host namespaces) and restricted level (runAsNonRoot, allowPrivilegeEscalation, readOnlyRootFS).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PSS compliance", map[string]interface{}{"healthScore": 37})},
	})
	add("/api/docs/compliance-report", "get", OpenAPIOperation{
		Summary: "Compliance report generator", OperationID: "complianceReport",
		Tags:        []string{"Documentation", "Compliance", "Audit"},
		Description: "Generates compliance report across CIS Benchmark, PCI-DSS, and SOC2 frameworks with pass/fail checks, severity levels, and remediation guidance.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Compliance report", map[string]interface{}{"healthScore": 55})},
	})
	add("/api/docs/slo-handbook", "get", OpenAPIOperation{
		Summary: "SLO handbook generator", OperationID: "sloHandbook",
		Tags:        []string{"Documentation", "SLO", "Reliability"},
		Description: "Auto-generates SLO handbook with per-service availability, error budgets, burn rates, and SLI definitions based on pod readiness.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SLO handbook", map[string]interface{}{"healthScore": 99})},
	})
	add("/api/docs/cluster-faq", "get", OpenAPIOperation{
		Summary: "Cluster FAQ generator", OperationID: "clusterFAQ",
		Tags:        []string{"Documentation", "FAQ", "Onboarding"},
		Description: "Generates cluster-specific FAQ with troubleshooting guides, operational commands, and common issue resolutions organized by category.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cluster FAQ", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/scalability/burst-capacity", "get", OpenAPIOperation{
		Summary: "Burst capacity calculator", OperationID: "burstCapacity",
		Tags:        []string{"Scalability", "Burst", "Capacity"},
		Description: "Calculates how many pods can be created instantly in a burst scenario: identifies CPU/memory/pod-limit bottlenecks, per-node breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Burst capacity", map[string]interface{}{"healthScore": 42})},
	})
	add("/api/scalability/elasticity-index", "get", OpenAPIOperation{
		Summary: "Resource elasticity index", OperationID: "elasticityIndex",
		Tags:        []string{"Scalability", "Elasticity", "Autoscaling"},
		Description: "Combined HPA+VPA+Cluster Autoscaler readiness score: identifies workloads with zero elasticity, partial coverage, and full elastic posture.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Elasticity index", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/scalability/scale-bottleneck", "get", OpenAPIOperation{
		Summary: "Scale bottleneck detector", OperationID: "scaleBottleneck",
		Tags:        []string{"Scalability", "Bottleneck", "Constraints"},
		Description: "Identifies scaling constraints: CPU request limits, hard affinity, large images, pod density, single-node risk, readiness probe delays.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Scale bottleneck", map[string]interface{}{"healthScore": 35})},
	})
	add("/api/deployment/image-consistency", "get", OpenAPIOperation{
		Summary: "Image consistency checker", OperationID: "imageConsistency",
		Tags:        []string{"Deployment", "Image", "Versioning"},
		Description: "Checks image version consistency across deployments: :latest tag usage, pinned images, multi-registry sprawl, inconsistent images within same deployment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image consistency", map[string]interface{}{"healthScore": 79})},
	})
	add("/api/deployment/config-reload-readiness", "get", OpenAPIOperation{
		Summary: "Config reload readiness", OperationID: "configReloadReadiness",
		Tags:        []string{"Deployment", "ConfigMap", "HotReload"},
		Description: "Checks ConfigMap mount types: volume mounts (hot reload ready) vs env-var mounts (need restart). Identifies workloads that need restart for config updates.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Config reload", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/deployment/deploy-freeze-status", "get", OpenAPIOperation{
		Summary: "Deploy freeze status", OperationID: "deployFreezeStatus",
		Tags:        []string{"Deployment", "Freeze", "Maintenance"},
		Description: "Checks for active deploy freeze windows: namespace freeze annotations, weekend freeze periods, recent change volume, safe-to-deploy assessment.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Deploy freeze", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/operations/control-plane-health", "get", OpenAPIOperation{
		Summary: "Control plane health", OperationID: "controlPlaneHealth",
		Tags:        []string{"Operations", "ControlPlane", "Health"},
		Description: "Checks health of control plane components: kube-apiserver, kube-controller-manager, kube-scheduler, etcd, coredns. Tracks replica readiness and restart counts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Control plane health", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/operations/csi-driver-health", "get", OpenAPIOperation{
		Summary: "CSI driver health", OperationID: "csiDriverHealth",
		Tags:        []string{"Operations", "Storage", "CSI"},
		Description: "Audits CSI storage driver health: checks plugin pod readiness, provisioner status, daemonset distribution, identifies degraded or missing drivers.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("CSI driver health", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/cert-renewal-timeline", "get", OpenAPIOperation{
		Summary: "Cert renewal timeline", OperationID: "certRenewalTimeline",
		Tags:        []string{"Operations", "Certificates", "TLS"},
		Description: "Tracks certificate renewal timeline: identifies TLS secrets expiring in 7d/30d/90d, expired certificates, generates renewal action timeline.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cert renewal", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/security/dns-exfil-risk-v2", "get", OpenAPIOperation{
		Summary: "DNS exfiltration risk", OperationID: "dnsExfilRisk",
		Tags:        []string{"Security", "DNS", "Exfiltration"},
		Description: "Assesses DNS exfiltration risk: checks for DNS egress NetworkPolicy coverage, suspicious env vars with external URLs, unrestricted namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DNS exfil risk", map[string]interface{}{"healthScore": 14})},
	})
	add("/api/security/port-forward-audit-v2", "get", OpenAPIOperation{
		Summary: "Port forward audit", OperationID: "portForwardAudit",
		Tags:        []string{"Security", "Exposure", "Ports"},
		Description: "Audits port exposure: hostPort container bindings, NodePort/LoadBalancer services, high-risk ports (SSH, Redis, DB), per-namespace breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Port forward", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/security/image-provenance-v3", "get", OpenAPIOperation{
		Summary: "Image supply chain provenance", OperationID: "imageProvenance",
		Tags:        []string{"Security", "SupplyChain", "Image"},
		Description: "Audits image supply chain: registry trust status, digest pinning, tag usage, image policy webhook presence (Cosign/Kyverno).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image provenance", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/dr-plan-gen", "get", OpenAPIOperation{
		Summary: "Disaster recovery plan", OperationID: "drPlanGen",
		Tags:        []string{"Documentation", "DR", "Recovery"},
		Description: "Auto-generates DR plan: RTO/RPO assessment, backup status, 8-step recovery procedure, markdown export.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DR plan", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/docs/adr-generator", "get", OpenAPIOperation{
		Summary: "Architecture decision records", OperationID: "adrGenerator",
		Tags:        []string{"Documentation", "ADR", "Architecture"},
		Description: "Auto-generates ADRs from cluster state: topology, storage, networking, monitoring, security decisions with context and consequences.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("ADR", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/docs/migration-checklist", "get", OpenAPIOperation{
		Summary: "Migration checklist", OperationID: "migrationChecklist",
		Tags:        []string{"Documentation", "Migration", "Checklist"},
		Description: "Generates 12-item cluster migration checklist: pre-migration, data backup, networking, migration execution, post-migration verification.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Migration checklist", map[string]interface{}{"healthScore": 8})},
	})
	add("/api/product/limit-range-audit", "get", OpenAPIOperation{
		Summary: "LimitRange audit", OperationID: "limitRangeAudit",
		Tags:        []string{"Product", "Resources", "LimitRange"},
		Description: "Audits LimitRange coverage: namespaces with/without default CPU/memory limits, max constraints, unprotected container count.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("LimitRange", map[string]interface{}{"healthScore": 10})},
	})
	add("/api/product/tenant-isolation", "get", OpenAPIOperation{
		Summary: "Tenant isolation score", OperationID: "tenantIsolation",
		Tags:        []string{"Product", "MultiTenancy", "Isolation"},
		Description: "Multi-tenant isolation assessment: NetworkPolicy, ResourceQuota, LimitRange, and RBAC coverage per namespace.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Tenant isolation", map[string]interface{}{"healthScore": 17})},
	})
	add("/api/product/resource-share", "get", OpenAPIOperation{
		Summary: "Resource share ratio", OperationID: "resourceShare",
		Tags:        []string{"Product", "Resources", "Fairness"},
		Description: "Namespace resource fairness analysis: CPU/memory share percentage, identifies imbalanced consumers, fairness score.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource share", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/scalability/api-throttle-risk", "get", OpenAPIOperation{
		Summary: "API throttle risk", OperationID: "apiThrottleRisk",
		Tags:        []string{"Scalability", "API", "Throttle"},
		Description: "Estimates API server QPS load from controller count, pod/resource counts, watch event rate, identifies high-consumer namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API throttle", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/pod-density-opt", "get", OpenAPIOperation{
		Summary: "Pod density optimizer", OperationID: "podDensityOpt",
		Tags:        []string{"Scalability", "Density", "BinPacking"},
		Description: "Analyzes pod density per node: identifies underutilized nodes (consolidation opportunity) and overutilized nodes (capacity risk).",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod density", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/scalability/overcommit-forecast", "get", OpenAPIOperation{
		Summary: "Overcommit forecast", OperationID: "overcommitForecast",
		Tags:        []string{"Scalability", "Overcommit", "Capacity"},
		Description: "Forecasts resource overcommit: CPU request vs limit ratios, unbounded workloads, per-namespace overcommit risk, cluster capacity saturation.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Overcommit", map[string]interface{}{"healthScore": 60})},
	})
	add("/api/deployment/manifest-drift", "get", OpenAPIOperation{
		Summary: "Manifest drift detector", OperationID: "manifestDrift",
		Tags:        []string{"Deployment", "Drift", "GitOps"},
		Description: "Detects manifest drift: replica count mismatch, image update lag, GitOps OutOfSync status, per-namespace breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Manifest drift", map[string]interface{}{"healthScore": 92})},
	})
	add("/api/deployment/preflight-check-v2", "get", OpenAPIOperation{
		Summary: "Pre-flight deploy check", OperationID: "preflightCheck",
		Tags:        []string{"Deployment", "PreFlight", "Gate"},
		Description: "Pre-deployment readiness gate: 7 checks covering node availability, cluster capacity, PVC health, DNS, service endpoints, resource limits.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pre-flight", map[string]interface{}{"healthScore": 51})},
	})
	add("/api/deployment/helm-health-v2", "get", OpenAPIOperation{
		Summary: "Helm release health", OperationID: "helmHealth",
		Tags:        []string{"Deployment", "Helm", "Release"},
		Description: "Audits Helm release health: deployed/failed/pending status, stale release detection (>90d), version tracking, Helm v2/v3 compatibility.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Helm health", map[string]interface{}{"healthScore": 74})},
	})
	add("/api/operations/storage-io-latency", "get", OpenAPIOperation{
		Summary: "Storage I/O latency risk", OperationID: "storageIoLatency",
		Tags:        []string{"Operations", "Storage", "Performance"},
		Description: "Estimates storage latency risk: local vs network storage PVC analysis, IOPS estimation, high-risk large PVCs on network storage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage latency", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/network-packet-loss", "get", OpenAPIOperation{
		Summary: "Network packet loss risk", OperationID: "networkPacketLoss",
		Tags:        []string{"Operations", "Network", "Connectivity"},
		Description: "Assesses network packet loss risk: node readiness, pod readiness, CNI health, services without endpoints.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Network loss", map[string]interface{}{"healthScore": 100})},
	})
	add("/api/operations/cgroup-pressure", "get", OpenAPIOperation{
		Summary: "Cgroup pressure monitor", OperationID: "cgroupPressure",
		Tags:        []string{"Operations", "Cgroup", "Pressure"},
		Description: "Monitors cgroup CPU/memory pressure: CFS throttle risk, OOM kill risk, per-namespace resource concentration.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Cgroup pressure", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/security/secret-exposure-graph", "get", OpenAPIOperation{
		Summary: "Secret exposure graph", OperationID: "secretExposureGraph",
		Tags:        []string{"Security", "Secrets", "Exposure"},
		Description: "Maps secret mount topology: identifies over-exposed secrets (>3 mounts), unused secrets, duplicate secrets, per-namespace breakdown.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret exposure", map[string]interface{}{"healthScore": 97})},
	})
	add("/api/security/admission-exception", "get", OpenAPIOperation{
		Summary: "Admission exception audit", OperationID: "admissionException",
		Tags:        []string{"Security", "Admission", "PSA"},
		Description: "Audits admission control: PSA enforcement labels, exception configurations, Gatekeeper/Kyverno/Cosign webhook presence.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Admission exception", map[string]interface{}{"healthScore": 0})},
	})
	add("/api/security/proc-mount-risk", "get", OpenAPIOperation{
		Summary: "Proc mount & tmpfs write risk", OperationID: "procMountRisk",
		Tags:        []string{"Security", "ProcMount", "Hardening"},
		Description: "Detects Unmasked proc mount, writable tmpfs (emptyDir with Memory medium), writable hostPath volumes across all containers.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Proc mount risk", map[string]interface{}{"healthScore": 84})},
	})
	add("/api/security/token-projection", "get", OpenAPIOperation{
		Summary:     "Token Projection Audit",
		OperationID: "token-projection", Tags: []string{"Security"},
		Description: "Audits projected vs legacy auto-mounted service account tokens. Identifies default SA usage and rotation gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Token projection", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/sysctl-risk", "get", OpenAPIOperation{
		Summary:     "Sysctl Risk Audit",
		OperationID: "sysctl-risk", Tags: []string{"Security"},
		Description: "Detects dangerous kernel sysctls in pod security contexts. Identifies unsafe sysctl names and their security impacts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Sysctl risk", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/security/hostport-exposure", "get", OpenAPIOperation{
		Summary:     "HostPort Exposure Map",
		OperationID: "hostport-exposure", Tags: []string{"Security"},
		Description: "Maps HostPort usage that bypasses NetworkPolicy isolation. Detects privileged ports, conflicts, and exposure risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HostPort exposure", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/naming-audit", "get", OpenAPIOperation{
		Summary:     "Naming Convention Audit",
		OperationID: "naming-audit", Tags: []string{"Documentation"},
		Description: "Audits resource names for DNS-1123 compliance, uppercase, underscores, length violations, and reserved prefixes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Naming audit", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/docs/env-var-catalog", "get", OpenAPIOperation{
		Summary:     "Environment Variable Catalog",
		OperationID: "env-var-catalog", Tags: []string{"Documentation"},
		Description: "Inventories all environment variables across workloads. Detects sensitive values, conflicts, and hardcoded configurations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Env var catalog", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/annotation-inventory", "get", OpenAPIOperation{
		Summary:     "Annotation Inventory",
		OperationID: "annotation-inventory", Tags: []string{"Documentation"},
		Description: "Catalogs all annotations across resources. Classifies as standard, custom, or deprecated for governance.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Annotation inventory", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/docs/policy-catalog", "get", OpenAPIOperation{
		Summary:     "Policy Catalog",
		OperationID: "policy-catalog", Tags: []string{"Documentation"},
		Description: "Comprehensive policy inventory documenting all NetworkPolicies, PodSecurityAdmission labels, RBAC bindings, LimitRanges, and ResourceQuotas across the cluster.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Policy catalog", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/docs/service-dependency-graph", "get", OpenAPIOperation{
		Summary:     "Service Dependency Graph",
		OperationID: "service-dependency-graph", Tags: []string{"Documentation"},
		Description: "Maps inter-service dependencies by scanning env vars, config references, and network policies. Identifies hub services, orphans, and cross-namespace dependencies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Dependency graph", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/docs/performance-baseline", "get", OpenAPIOperation{
		Summary:     "Performance Baseline Report",
		OperationID: "performance-baseline", Tags: []string{"Documentation"},
		Description: "Captures resource usage baselines per workload. Documents CPU/memory requests, identifies anomalies, and recommends thresholds for monitoring.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Performance baseline", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/image-lifecycle", "get", OpenAPIOperation{
		Summary:     "Image Lifecycle Tracker",
		OperationID: "image-lifecycle", Tags: []string{"Product"},
		Description: "Tracks image freshness, tag types (pinned/floating), reuse counts, and staleness. Identifies images with floating tags and high blast radius.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image lifecycle", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/product/volume-snapshot-readiness", "get", OpenAPIOperation{
		Summary:     "Volume Snapshot Readiness",
		OperationID: "volume-snapshot-readiness", Tags: []string{"Product"},
		Description: "Evaluates PVC CSI snapshot eligibility by checking storage class provisioner support and PVC bound status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Snapshot readiness", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/product/idle-resource", "get", OpenAPIOperation{
		Summary:     "Idle Resource Detector",
		OperationID: "idle-resource", Tags: []string{"Product"},
		Description: "Detects idle pods (zero service references, long-running) and unused services. Estimates wasted CPU, memory, and monthly cost.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Idle resources", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/product/secret-version-history", "get", OpenAPIOperation{
		Summary:     "Secret Version History",
		OperationID: "secret-version-history", Tags: []string{"Product"},
		Description: "Tracks secret age, rotation history, key count, mount usage, and identifies stale or unused secrets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret versions", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/crd-health", "get", OpenAPIOperation{
		Summary:     "CRD Health Monitor",
		OperationID: "crd-health", Tags: []string{"Product"},
		Description: "Monitors CustomResourceDefinitions for deprecated versions, operator health, and version consolidation opportunities.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("CRD health", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/product/autosize-recommender", "get", OpenAPIOperation{
		Summary:     "Workload Autosize Recommender",
		OperationID: "autosize-recommender", Tags: []string{"Product"},
		Description: "Analyzes workload resource requests and recommends right-sizing. Estimates cost savings from over-provisioned workloads.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Autosize recommendations", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/restart-rate", "get", OpenAPIOperation{
		Summary:     "Pod Restart Rate Limiter",
		OperationID: "restart-rate", Tags: []string{"Scalability"},
		Description: "Analyzes pod restart rates to detect crash loops and flapping. Identifies pods with excessive restart frequency.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart rate", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/node-affinity-compliance", "get", OpenAPIOperation{
		Summary:     "Node Affinity Compliance",
		OperationID: "node-affinity-compliance", Tags: []string{"Scalability"},
		Description: "Audits node selectors, affinity rules, and anti-affinity for scheduling health. Detects unschedulable risks and SPOF.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node affinity", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/scalability/quota-pressure-index", "get", OpenAPIOperation{
		Summary:     "Resource Quota Pressure Index",
		OperationID: "quota-pressure-index", Tags: []string{"Scalability"},
		Description: "Forecasts resource quota exhaustion. Identifies namespaces approaching quota limits and recommends capacity planning.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Quota pressure", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/sts-health", "get", OpenAPIOperation{
		Summary:     "StatefulSet Health Audit",
		OperationID: "sts-health", Tags: []string{"Deployment"},
		Description: "Audits StatefulSet ordinal readiness, update strategy, partition settings, and PVC binding health.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("StatefulSet health", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/image-pull-secret-gap", "get", OpenAPIOperation{
		Summary:     "Image Pull Secret Gap",
		OperationID: "image-pull-secret-gap", Tags: []string{"Deployment"},
		Description: "Detects pods using private registries without imagePullSecrets. Identifies ImagePullBackOff risk.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pull secret gap", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/topology-distribution", "get", OpenAPIOperation{
		Summary:     "Pod Topology Distribution",
		OperationID: "topology-distribution", Tags: []string{"Deployment"},
		Description: "Analyzes how pod replicas are distributed across nodes. Detects concentration risks and spread quality.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Topology distribution", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/rollback-window", "get", OpenAPIOperation{
		Summary:     "Rollback Window Analyzer",
		OperationID: "rollback-window", Tags: []string{"Scalability"},
		Description: "Analyzes deployment rollback readiness by checking revisionHistoryLimit, replica count, and rollout status.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollback window", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/scalability/dns-scalability", "get", OpenAPIOperation{
		Summary:     "DNS Resolution Scalability",
		OperationID: "dns-scalability", Tags: []string{"Scalability"},
		Description: "Analyzes CoreDNS capacity, replica-to-pod ratio, autoscaler presence, and DNS resolution bottlenecks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DNS scalability", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/conn-pool-exhaustion", "get", OpenAPIOperation{
		Summary:     "Connection Pool Exhaustion Risk",
		OperationID: "conn-pool-exhaustion", Tags: []string{"Scalability"},
		Description: "Detects endpoints with high connection fan-out, single-pod SPOFs, and connection pool exhaustion risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Connection pool risk", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/annotation-compliance", "get", OpenAPIOperation{
		Summary:     "Annotation Compliance",
		OperationID: "annotation-compliance", Tags: []string{"Deployment"},
		Description: "Audits workloads for required metadata annotations: owner, contact, managed-by, deployment revision.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Annotation compliance", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/multi-arch-audit", "get", OpenAPIOperation{
		Summary:     "Multi-Arch Image Audit",
		OperationID: "multi-arch-audit", Tags: []string{"Deployment"},
		Description: "Audits container images for multi-architecture support and portability risks across cluster node architectures.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Multi-arch audit", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/container-deps", "get", OpenAPIOperation{
		Summary:     "Container Dependency Mapper",
		OperationID: "container-deps", Tags: []string{"Deployment"},
		Description: "Maps inter-container dependencies within pods: init containers, shared volumes, sidecar patterns, and startup order risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Container dependencies", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/ingress-health-monitor", "get", OpenAPIOperation{
		Summary:     "Ingress Health Monitor",
		OperationID: "ingress-health", Tags: []string{"Operations"},
		Description: "Monitors Ingress rule conflicts, TLS coverage, backend service health, and orphan detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ingress health", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/job-lifecycle", "get", OpenAPIOperation{
		Summary:     "Job Lifecycle Tracker",
		OperationID: "job-lifecycle", Tags: []string{"Operations"},
		Description: "Tracks Job and CronJob completion, failure rates, staleness, and cleanup recommendations.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Job lifecycle", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/leader-election", "get", OpenAPIOperation{
		Summary:     "Leader Election Health",
		OperationID: "leader-election", Tags: []string{"Operations"},
		Description: "Monitors leader election leases, detects stale leases, and evaluates failover readiness for controllers.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Leader election", map[string]interface{}{"healthScore": 90})},
	})

	add("/api/operations/pvc-lifecycle", "get", OpenAPIOperation{
		Summary:     "PVC Lifecycle Monitor",
		OperationID: "pvc-lifecycle", Tags: []string{"Operations"},
		Description: "Monitors PVC binding health, pending claims, released PVs, and reclaimable storage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PVC lifecycle", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/endpoint-latency", "get", OpenAPIOperation{
		Summary:     "Service Endpoint Latency",
		OperationID: "endpoint-latency", Tags: []string{"Operations"},
		Description: "Analyzes endpoint readiness ratio, not-ready addresses, and service traffic health.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Endpoint latency", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/container-forensics", "get", OpenAPIOperation{
		Summary:     "Container State Forensics",
		OperationID: "container-forensics", Tags: []string{"Operations"},
		Description: "Analyzes container states, exit codes, OOM kills, and CrashLoopBackOff patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Container forensics", map[string]interface{}{"healthScore": 80})},
	})

	add("/api/security/volume-mount-audit", "get", OpenAPIOperation{
		Summary:     "Volume Mount Audit",
		OperationID: "volume-mount-audit", Tags: []string{"Security"},
		Description: "Audits volume mounts for sensitive host path exposure, hostNetwork, hostPID, and hostIPC risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Volume mount audit", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/priv-esc-risk", "get", OpenAPIOperation{
		Summary:     "Privilege Escalation Risk",
		OperationID: "priv-esc-risk", Tags: []string{"Security"},
		Description: "Analyzes security contexts for privileged mode, AllowPrivilegeEscalation, runAsUser=0, and missing constraints.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Privilege escalation", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/image-base-scan", "get", OpenAPIOperation{
		Summary:     "Image Base Layer Scan",
		OperationID: "image-base-scan", Tags: []string{"Security"},
		Description: "Identifies base image types (distroless, alpine, debian), latest tags, and attack surface indicators.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Image base scan", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/label-standardization", "get", OpenAPIOperation{
		Summary:     "Label Standardization Report",
		OperationID: "label-standardization", Tags: []string{"Documentation"},
		Description: "Audits label governance across resources. Identifies missing labels, custom vs standard keys, and compliance rate.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Label standardization", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/resource-age-distribution", "get", OpenAPIOperation{
		Summary:     "Resource Age Distribution",
		OperationID: "resource-age-distribution", Tags: []string{"Documentation"},
		Description: "Analyzes resource lifecycle age. Buckets by age range, identifies oldest resources for modernization review.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Age distribution", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/docs/ns-isolation-matrix", "get", OpenAPIOperation{
		Summary:     "Namespace Isolation Matrix",
		OperationID: "ns-isolation-matrix", Tags: []string{"Documentation"},
		Description: "Documents namespace boundary controls: NetworkPolicy, ResourceQuota, LimitRange, and PSA coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("NS isolation matrix", map[string]interface{}{"healthScore": 80})},
	})

	add("/api/product/mesh-ready-check", "get", OpenAPIOperation{
		Summary: "Mesh Readiness Checker", OperationID: "mesh-ready-check", Tags: []string{"Product"},
		Description: "Checks for mesh sidecar presence, injection annotations, and enrollment gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Mesh readiness", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/product/vol-access-audit", "get", OpenAPIOperation{
		Summary: "Volume Access Mode Audit", OperationID: "vol-access-audit", Tags: []string{"Product"},
		Description: "Audits PVC access modes for RWO/ROX/RWX compatibility and shared volume conflicts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Volume access", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/product/pdb-gap-analysis-v2", "get", OpenAPIOperation{
		Summary: "PDB Gap Analysis", OperationID: "pdb-gap-analysis-v2", Tags: []string{"Product"},
		Description: "Analyzes PDB coverage across workloads. Identifies unprotected deployments.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("PDB gap", map[string]interface{}{"healthScore": 80})},
	})

	add("/api/scalability/sched-queue-depth", "get", OpenAPIOperation{
		Summary: "Scheduler Queue Depth", OperationID: "sched-queue-depth", Tags: []string{"Scalability"},
		Description: "Analyzes scheduling pressure by tracking pending, unschedulable, and stuck pods.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Sched queue", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/pod-spread-violation", "get", OpenAPIOperation{
		Summary: "Pod Spread Violation", OperationID: "pod-spread-violation", Tags: []string{"Scalability"},
		Description: "Detects topology spread constraint violations and uneven pod distribution across nodes.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pod spread", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/scalability/ha-topo-score", "get", OpenAPIOperation{
		Summary: "HA Topology Score", OperationID: "ha-topo-score", Tags: []string{"Scalability"},
		Description: "Scores HA readiness by analyzing multi-zone distribution, replica diversity, and failure domain coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HA topology", map[string]interface{}{"healthScore": 80})},
	})

	add("/api/deployment/revision-timeline", "get", OpenAPIOperation{
		Summary: "Deployment Revision Timeline", OperationID: "revision-timeline", Tags: []string{"Deployment"},
		Description: "Tracks deployment revision history from ReplicaSets. Identifies stale revisions and cleanup opportunities.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Revision timeline", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/qos-distribution", "get", OpenAPIOperation{
		Summary: "Pod QoS Class Distribution", OperationID: "qos-distribution", Tags: []string{"Deployment"},
		Description: "Analyzes QoS class distribution (Guaranteed/Burstable/BestEffort) across pods and namespaces.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("QoS distribution", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/ds-health", "get", OpenAPIOperation{
		Summary: "DaemonSet Health Monitor", OperationID: "ds-health", Tags: []string{"Deployment"},
		Description: "Monitors DaemonSet rollout status, node coverage, misscheduled pods, and update progress.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DS health", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/operations/hpa-scaling-events", "get", OpenAPIOperation{
		Summary: "HPA Scaling Event Tracker", OperationID: "hpa-scaling-events", Tags: []string{"Operations"},
		Description: "Tracks recent HPA scale-up/scale-down events from Kubernetes event stream. Detects thrashing patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("HPA scaling events", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/node-cond-history", "get", OpenAPIOperation{
		Summary: "Node Condition History", OperationID: "node-cond-history", Tags: []string{"Operations"},
		Description: "Analyzes node conditions for pressure states, readiness transitions, and flapping patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Node conditions", map[string]interface{}{"healthScore": 90})},
	})
	add("/api/operations/config-change-tracker", "get", OpenAPIOperation{
		Summary: "ConfigMap Change Tracker", OperationID: "config-change-tracker", Tags: []string{"Operations"},
		Description: "Tracks ConfigMap changes, detects stale configs, and flags oversized ConfigMaps affecting etcd.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Config changes", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/security/rbac-overexpose", "get", OpenAPIOperation{
		Summary: "RBAC Overexposure Auditor", OperationID: "rbac-overexpose", Tags: []string{"Security"},
		Description: "Audits RBAC bindings for wildcard verbs, wildcard resources, cluster-admin overuse, and escalation risks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("RBAC overexposure", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/security/secret-enc-rest", "get", OpenAPIOperation{
		Summary: "Secret Encryption at Rest", OperationID: "secret-enc-rest", Tags: []string{"Security"},
		Description: "Checks etcd encryption status, audits secret types, and identifies stale SA tokens and large secrets.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Secret encryption", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/security/webhook-risk", "get", OpenAPIOperation{
		Summary: "Admission Webhook Risk", OperationID: "webhook-risk", Tags: []string{"Security"},
		Description: "Analyzes admission webhook failure policies, timeouts, catch-all rules, and namespace selector coverage.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Webhook risk", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/docs/ownership-registry-v2", "get", OpenAPIOperation{
		Summary: "Workload Ownership Registry", OperationID: "ownership-registry-v2", Tags: []string{"Documentation"},
		Description: "Documents workload ownership metadata including owner, team, escalation path, and contact info.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Ownership registry", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/docs/api-resource-inventory", "get", OpenAPIOperation{
		Summary: "API Resource Inventory", OperationID: "api-resource-inventory", Tags: []string{"Documentation"},
		Description: "Inventories all API resources (native + CRD), groups, versions, and deprecated API detection.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API inventory", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/docs/capacity-report", "get", OpenAPIOperation{
		Summary: "Cluster Capacity Report", OperationID: "capacity-report", Tags: []string{"Documentation"},
		Description: "Documents cluster resource capacity, allocation by namespace, and utilization percentages.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Capacity report", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/product/res-wastage", "get", OpenAPIOperation{
		Summary: "Container Resource Wastage", OperationID: "res-wastage", Tags: []string{"Product"},
		Description: "Analyzes overcommitted resource limits vs requests. Identifies high ratio containers and estimates waste.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource wastage", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/product/sa-usage-tracker", "get", OpenAPIOperation{
		Summary: "Service Account Usage Tracker", OperationID: "sa-usage-tracker", Tags: []string{"Product"},
		Description: "Tracks which ServiceAccounts are actually used by pods vs orphaned. Identifies default SA overuse.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("SA usage", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/product/ep-slice-health", "get", OpenAPIOperation{
		Summary: "Endpoint Slice Health", OperationID: "ep-slice-health", Tags: []string{"Product"},
		Description: "Analyzes endpoint slice distribution, readiness, and identifies services with no ready endpoints.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("EP slice health", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/scalability/res-pressure-score", "get", OpenAPIOperation{
		Summary: "Resource Pressure Score", OperationID: "res-pressure-score", Tags: []string{"Scalability"},
		Description: "Analyzes node resource contention by calculating weighted pressure scores from CPU, memory, and pod density.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Resource pressure", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/scalability/anti-affinity-coverage", "get", OpenAPIOperation{
		Summary: "Anti-Affinity Coverage", OperationID: "anti-affinity-coverage", Tags: []string{"Scalability"},
		Description: "Audits multi-replica workloads for anti-affinity and topology spread constraint coverage. Identifies HA gaps.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Anti-affinity coverage", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/scalability/startup-latency", "get", OpenAPIOperation{
		Summary: "Pod Startup Latency", OperationID: "startup-latency", Tags: []string{"Scalability"},
		Description: "Measures pod startup latency from scheduling to ready. Identifies slow starters and image pull bottlenecks.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Startup latency", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/deployment/pause-detect", "get", OpenAPIOperation{
		Summary: "Deployment Pause Detector", OperationID: "pause-detect", Tags: []string{"Deployment"},
		Description: "Detects paused deployments, incomplete rollouts, and stale deployments not updated in 90+ days.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Pause detection", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/deployment/tag-compliance", "get", OpenAPIOperation{
		Summary: "Image Tag Compliance", OperationID: "tag-compliance", Tags: []string{"Deployment"},
		Description: "Audits image tags for reproducibility: pinned digests, versioned tags vs :latest and floating tags.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Tag compliance", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/deployment/rollout-strategy", "get", OpenAPIOperation{
		Summary: "Rollout Strategy Analyzer", OperationID: "rollout-strategy", Tags: []string{"Deployment"},
		Description: "Analyzes deployment rollout strategies (RollingUpdate vs Recreate), maxSurge/maxUnavailable, and zero-downtime capability.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Rollout strategy", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/operations/restart-storm", "get", OpenAPIOperation{
		Summary: "Pod Restart Storm Detector", OperationID: "restart-storm", Tags: []string{"Operations"},
		Description: "Detects pods in restart storms based on restart count and restart rate per hour. Identifies CrashLoopBackOff patterns.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Restart storm", map[string]interface{}{"healthScore": 80})},
	})
	add("/api/operations/event-storm", "get", OpenAPIOperation{
		Summary: "Event Storm Analyzer", OperationID: "event-storm", Tags: []string{"Operations"},
		Description: "Analyzes Kubernetes event volume and rate for abnormal patterns. Detects event storms and high-frequency warnings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Event storm", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/operations/taint-impact", "get", OpenAPIOperation{
		Summary: "Node Taint Impact", OperationID: "taint-impact", Tags: []string{"Operations"},
		Description: "Analyzes node taints and their impact on pod scheduling. Identifies NoSchedule/NoExecute taints and blocked pods.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Taint impact", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/security/netpol-coverage-v2", "get", OpenAPIOperation{
		Summary: "Network Policy Coverage Auditor", OperationID: "netpol-coverage-v2", Tags: []string{"Security"},
		Description: "Audits NetworkPolicy coverage by namespace. Identifies uncovered namespaces, default-deny adoption, and isolated pod counts.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("NetPol coverage", map[string]interface{}{"healthScore": 70})},
	})
	add("/api/security/seccomp-exposure", "get", OpenAPIOperation{
		Summary: "Container Syscall Exposure", OperationID: "seccomp-exposure", Tags: []string{"Security"},
		Description: "Analyzes seccomp profiles and Linux capabilities. Identifies unconfined containers, missing profiles, and dangerous capAdd.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Seccomp exposure", map[string]interface{}{"healthScore": 75})},
	})
	add("/api/security/api-discovery-exposure", "get", OpenAPIOperation{
		Summary: "API Discovery Exposure", OperationID: "api-discovery-exposure", Tags: []string{"Security"},
		Description: "Inventories API resource exposure surface and checks for anonymous/unauthenticated RBAC bindings.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("API discovery", map[string]interface{}{"healthScore": 85})},
	})

	add("/api/docs/dependency-graph", "get", OpenAPIOperation{
		Summary: "Dependency Graph Mapper", OperationID: "dependency-graph", Tags: []string{"Documentation"},
		Description: "Maps service-to-service dependencies by scanning env vars, configmaps, and volume references. Detects unresolved dependencies.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Dependency graph", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/docs/storage-class-inventory", "get", OpenAPIOperation{
		Summary: "Storage Class Inventory", OperationID: "storage-class-inventory", Tags: []string{"Documentation"},
		Description: "Documents all StorageClasses with provisioner, reclaim policy, binding mode, and PVC usage statistics.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("Storage class inventory", map[string]interface{}{"healthScore": 85})},
	})
	add("/api/docs/dns-resolution-map", "get", OpenAPIOperation{
		Summary: "DNS Resolution Map", OperationID: "dns-resolution-map", Tags: []string{"Documentation"},
		Description: "Maps service DNS names (FQDN), detects name collisions across namespaces, and documents headless services.",
		Responses:   map[string]OpenAPIResponse{"200": okResponse("DNS map", map[string]interface{}{"healthScore": 90})},
	})

	return spec
}

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
