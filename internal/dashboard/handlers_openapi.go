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
