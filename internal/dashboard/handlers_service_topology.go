package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ServiceTopologyResult is the cluster-wide service dependency topology and
// cascade failure risk analysis.
type ServiceTopologyResult struct {
	ScannedAt         time.Time              `json:"scannedAt"`
	Summary           ServiceTopologySummary `json:"summary"`
	Nodes             []TopologyNode         `json:"nodes"`
	Edges             []TopologyEdge         `json:"edges"`
	CriticalHubs      []CriticalHub          `json:"criticalHubs"`
	OrphanServices    []OrphanService        `json:"orphanServices,omitempty"`
	IsolatedWorkloads []IsolatedWorkload     `json:"isolatedWorkloads,omitempty"`
	Risks             []TopologyRisk         `json:"risks"`
	Recommendations   []string               `json:"recommendations"`
	HealthScore       int                    `json:"healthScore"`
}

// ServiceTopologySummary aggregates topology statistics.
type ServiceTopologySummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	TotalServices     int     `json:"totalServices"`
	TotalEdges        int     `json:"totalEdges"`
	CrossNamespace    int     `json:"crossNamespaceEdges"`
	MaxDepth          int     `json:"maxDepth"`
	CriticalNodes     int     `json:"criticalNodes"`
	OrphanServices    int     `json:"orphanServices"`
	IsolatedWorkloads int     `json:"isolatedWorkloads"`
	AvgFanOut         float64 `json:"avgFanOut"`
	AvgFanIn          float64 `json:"avgFanIn"`
}

// TopologyNode represents a workload or service in the dependency graph.
type TopologyNode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // Deployment, StatefulSet, DaemonSet, Service
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	FanOut      int    `json:"fanOut"`      // how many services this depends on
	FanIn       int    `json:"fanIn"`       // how many workloads depend on this
	Criticality string `json:"criticality"` // critical, high, medium, low
	Replicas    int    `json:"replicas"`
	HasHA       bool   `json:"hasHA"` // replicas > 1
}

// TopologyEdge represents a dependency relationship.
type TopologyEdge struct {
	From    string `json:"from"` // source workload ID
	To      string `json:"to"`   // target service ID
	Type    string `json:"type"` // service-ref, config-ref, shared-pvc, external
	CrossNS bool   `json:"crossNamespace"`
}

// CriticalHub identifies a service/workload that many others depend on.
type CriticalHub struct {
	NodeID     string   `json:"nodeId"`
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Kind       string   `json:"kind"`
	FanIn      int      `json:"fanIn"`
	Dependents []string `json:"dependents"`
	HasHA      bool     `json:"hasHA"`
	Replicas   int      `json:"replicas"`
	RiskLevel  string   `json:"riskLevel"`
}

// OrphanService is a service with no workload selecting it.
type OrphanService struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"` // ClusterIP, NodePort, LoadBalancer
}

// IsolatedWorkload is a workload with no detected dependencies.
type IsolatedWorkload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

// TopologyRisk describes a cascade failure risk.
type TopologyRisk struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Resource string `json:"resource"`
	Issue    string `json:"issue"`
	Affected int    `json:"affected"` // how many workloads affected
}

// handleServiceTopology builds a cluster-wide service dependency graph and
// analyzes cascade failure risks.
// GET /api/product/service-topology?namespace=xxx
func (s *Server) handleServiceTopology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	result := ServiceTopologyResult{
		ScannedAt: time.Now(),
	}

	// 1. Collect all workloads (Deployments, StatefulSets, DaemonSets)
	workloadMap := map[string]*TopologyNode{} // workloadID → node
	var workloadList []workloadInfo

	// Deployments
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range deployments.Items {
			d := &deployments.Items[i]
			id := fmt.Sprintf("Deployment/%s/%s", d.Namespace, d.Name)
			replicas := 1
			if d.Spec.Replicas != nil {
				replicas = int(*d.Spec.Replicas)
			}
			workloadMap[id] = &TopologyNode{
				ID:          id,
				Kind:        "Deployment",
				Name:        d.Name,
				Namespace:   d.Namespace,
				Replicas:    replicas,
				HasHA:       replicas > 1,
				Criticality: "low",
			}
			workloadList = append(workloadList, workloadInfo{
				id:        id,
				kind:      "Deployment",
				name:      d.Name,
				namespace: d.Namespace,
				podSpec:   &d.Spec.Template.Spec,
				selector:  d.Spec.Selector,
			})
		}
	}

	// StatefulSets
	stss, err := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range stss.Items {
			sts := &stss.Items[i]
			id := fmt.Sprintf("StatefulSet/%s/%s", sts.Namespace, sts.Name)
			replicas := 1
			if sts.Spec.Replicas != nil {
				replicas = int(*sts.Spec.Replicas)
			}
			workloadMap[id] = &TopologyNode{
				ID:          id,
				Kind:        "StatefulSet",
				Name:        sts.Name,
				Namespace:   sts.Namespace,
				Replicas:    replicas,
				HasHA:       replicas > 1,
				Criticality: "low",
			}
			workloadList = append(workloadList, workloadInfo{
				id:        id,
				kind:      "StatefulSet",
				name:      sts.Name,
				namespace: sts.Namespace,
				podSpec:   &sts.Spec.Template.Spec,
				selector:  sts.Spec.Selector,
			})
		}
	}

	// DaemonSets
	dss, err := rc.clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range dss.Items {
			ds := &dss.Items[i]
			id := fmt.Sprintf("DaemonSet/%s/%s", ds.Namespace, ds.Name)
			workloadMap[id] = &TopologyNode{
				ID:          id,
				Kind:        "DaemonSet",
				Name:        ds.Name,
				Namespace:   ds.Namespace,
				Replicas:    int(ds.Status.DesiredNumberScheduled),
				HasHA:       ds.Status.DesiredNumberScheduled > 1,
				Criticality: "low",
			}
			workloadList = append(workloadList, workloadInfo{
				id:        id,
				kind:      "DaemonSet",
				name:      ds.Name,
				namespace: ds.Namespace,
				podSpec:   &ds.Spec.Template.Spec,
				selector:  ds.Spec.Selector,
			})
		}
	}

	// 2. Collect all services
	services, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range services.Items {
			svc := &services.Items[i]
			id := fmt.Sprintf("Service/%s/%s", svc.Namespace, svc.Name)
			// Only track ClusterIP and NodePort services (not ExternalName - those are external deps)
			if svc.Spec.Type == corev1.ServiceTypeExternalName {
				continue
			}
			// Skip headless services with no clusterIP (they're still valid but we track differently)
			node := &TopologyNode{
				ID:          id,
				Kind:        "Service",
				Name:        svc.Name,
				Namespace:   svc.Namespace,
				Criticality: "low",
			}
			// Check if service has a backing workload
			if len(svc.Spec.Selector) == 0 {
				// External or manually managed service
				node.HasHA = true
			}
			workloadMap[id] = node
		}
	}

	// 3. Build dependency edges by scanning env vars for service references
	var allEdges []TopologyEdge
	edgeSet := map[string]bool{} // dedup

	for _, wl := range workloadList {
		depServices := extractServiceDependencies(wl, services.Items)

		for _, svcRef := range depServices {
			svcID := fmt.Sprintf("Service/%s/%s", svcRef.namespace, svcRef.name)
			edgeKey := fmt.Sprintf("%s→%s", wl.id, svcID)

			// Verify the target service exists
			if _, exists := workloadMap[svcID]; !exists {
				continue
			}

			if !edgeSet[edgeKey] {
				edgeSet[edgeKey] = true
				crossNS := svcRef.namespace != wl.namespace
				allEdges = append(allEdges, TopologyEdge{
					From:    wl.id,
					To:      svcID,
					Type:    svcRef.depType,
					CrossNS: crossNS,
				})
			}
		}
	}

	// 4. Calculate fan-out and fan-in
	fanOutMap := map[string]int{} // workload → number of deps
	fanInMap := map[string]int{}  // service → number of dependents

	for _, edge := range allEdges {
		fanOutMap[edge.From]++
		fanInMap[edge.To]++
	}

	for id, node := range workloadMap {
		node.FanOut = fanOutMap[id]
		node.FanIn = fanInMap[id]

		// Determine criticality based on fan-in
		switch {
		case node.FanIn >= 10:
			node.Criticality = "critical"
		case node.FanIn >= 5:
			node.Criticality = "high"
		case node.FanIn >= 2:
			node.Criticality = "medium"
		default:
			node.Criticality = "low"
		}
	}

	// 5. Build nodes list
	var nodes []TopologyNode
	for _, node := range workloadMap {
		nodes = append(nodes, *node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		// Sort by fan-in descending, then by name
		if nodes[i].FanIn != nodes[j].FanIn {
			return nodes[i].FanIn > nodes[j].FanIn
		}
		return nodes[i].ID < nodes[j].ID
	})
	result.Nodes = nodes

	// Limit edges to prevent huge payloads
	if len(allEdges) > 500 {
		// Keep edges to/from critical nodes
		var importantEdges []TopologyEdge
		for _, e := range allEdges {
			if n, ok := workloadMap[e.To]; ok && n.FanIn >= 2 {
				importantEdges = append(importantEdges, e)
			}
		}
		if len(importantEdges) > 500 {
			importantEdges = importantEdges[:500]
		}
		result.Edges = importantEdges
	} else {
		result.Edges = allEdges
	}

	// 6. Identify critical hubs (services with fan-in >= 1)
	var criticalHubs []CriticalHub
	for _, node := range nodes {
		if node.FanIn < 1 {
			continue
		}
		// Collect dependent IDs
		var dependents []string
		for _, edge := range allEdges {
			if edge.To == node.ID {
				dependents = append(dependents, edge.From)
			}
		}

		riskLevel := "low"
		if !node.HasHA && node.FanIn >= 3 {
			riskLevel = "critical"
		} else if !node.HasHA && node.FanIn >= 2 {
			riskLevel = "high"
		} else if node.FanIn >= 5 {
			riskLevel = "high"
		} else if node.FanIn >= 3 {
			riskLevel = "medium"
		}

		criticalHubs = append(criticalHubs, CriticalHub{
			NodeID:     node.ID,
			Name:       node.Name,
			Namespace:  node.Namespace,
			Kind:       node.Kind,
			FanIn:      node.FanIn,
			Dependents: dependents,
			HasHA:      node.HasHA,
			Replicas:   node.Replicas,
			RiskLevel:  riskLevel,
		})
	}
	sort.Slice(criticalHubs, func(i, j int) bool {
		return criticalHubs[i].FanIn > criticalHubs[j].FanIn
	})
	if len(criticalHubs) > 30 {
		criticalHubs = criticalHubs[:30]
	}
	result.CriticalHubs = criticalHubs

	// 7. Detect orphan services (no selector match and no workload backing)
	if services != nil {
		for i := range services.Items {
			svc := &services.Items[i]
			if svc.Spec.Type == corev1.ServiceTypeExternalName {
				continue
			}
			// Check if any workload's pod template matches the selector
			if len(svc.Spec.Selector) > 0 {
				hasBacking := false
				for _, wl := range workloadList {
					if wl.namespace != svc.Namespace {
						continue
					}
					if wl.selector != nil && labelSelectorMatches(*wl.selector, labels.Set(svc.Spec.Selector)) {
						hasBacking = true
						break
					}
				}
				if !hasBacking {
					result.OrphanServices = append(result.OrphanServices, OrphanService{
						Name:      svc.Name,
						Namespace: svc.Namespace,
						Type:      string(svc.Spec.Type),
					})
				}
			}
		}
	}

	// 8. Detect isolated workloads (no service dependencies and no service selecting them)
	for _, wl := range workloadList {
		if fanOutMap[wl.id] == 0 {
			// Check if any service selects this workload
			hasService := false
			if services != nil {
				for i := range services.Items {
					svc := &services.Items[i]
					if svc.Namespace != wl.namespace || len(svc.Spec.Selector) == 0 {
						continue
					}
					if wl.selector != nil && labelSelectorMatches(*wl.selector, labels.Set(svc.Spec.Selector)) {
						hasService = true
						break
					}
				}
			}
			if !hasService {
				result.IsolatedWorkloads = append(result.IsolatedWorkloads, IsolatedWorkload{
					Name:      wl.name,
					Namespace: wl.namespace,
					Kind:      wl.kind,
				})
			}
		}
	}
	if len(result.IsolatedWorkloads) > 50 {
		result.IsolatedWorkloads = result.IsolatedWorkloads[:50]
	}

	// 9. Calculate max depth (longest dependency chain)
	maxDepth := computeMaxDepth(allEdges)

	// 10. Build summary
	crossNSCount := 0
	for _, e := range allEdges {
		if e.CrossNS {
			crossNSCount++
		}
	}
	totalFanOut := 0
	totalFanIn := 0
	criticalCount := 0
	for _, node := range nodes {
		totalFanOut += node.FanOut
		totalFanIn += node.FanIn
		if node.Criticality == "critical" || node.Criticality == "high" {
			criticalCount++
		}
	}
	result.Summary = ServiceTopologySummary{
		TotalWorkloads:    len(workloadList),
		TotalServices:     len(workloadMap) - len(workloadList),
		TotalEdges:        len(allEdges),
		CrossNamespace:    crossNSCount,
		MaxDepth:          maxDepth,
		CriticalNodes:     criticalCount,
		OrphanServices:    len(result.OrphanServices),
		IsolatedWorkloads: len(result.IsolatedWorkloads),
	}
	if len(workloadList) > 0 {
		result.Summary.AvgFanOut = float64(totalFanOut) / float64(len(workloadList))
	}
	if len(nodes) > 0 {
		result.Summary.AvgFanIn = float64(totalFanIn) / float64(len(nodes))
	}

	// 11. Build risks
	result.Risks = generateServiceTopologyRisks(result)

	// 12. Calculate health score
	score := 100
	// Critical hubs without HA are high risk
	for _, hub := range criticalHubs {
		if !hub.HasHA && hub.FanIn >= 3 {
			score -= 10
		} else if !hub.HasHA && hub.FanIn >= 2 {
			score -= 5
		}
	}
	// Orphan services indicate dead endpoints
	score -= len(result.OrphanServices) * 2
	// Too many isolated workloads suggests missing observability
	if result.Summary.IsolatedWorkloads > result.Summary.TotalWorkloads/3 && result.Summary.TotalWorkloads > 3 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 13. Recommendations
	result.Recommendations = generateServiceTopologyRecommendations(result)

	writeJSON(w, result)
}

// workloadInfo holds workload metadata for topology scanning.
type workloadInfo struct {
	id        string
	kind      string
	name      string
	namespace string
	podSpec   *corev1.PodSpec
	selector  *metav1.LabelSelector
}

// serviceDepRef represents a detected service dependency.
type serviceDepRef struct {
	name      string
	namespace string
	depType   string
}

// extractServiceDependencies scans pod env vars, envFrom, and externalName
// references for service DNS names.
func extractServiceDependencies(wl workloadInfo, services []corev1.Service) []serviceDepRef {
	if wl.podSpec == nil {
		return nil
	}

	var deps []serviceDepRef
	seen := map[string]bool{}

	// Build a lookup of all service names per namespace for quick matching
	svcByNS := map[string]map[string]bool{} // namespace → set of service names
	for _, svc := range services {
		if svcByNS[svc.Namespace] == nil {
			svcByNS[svc.Namespace] = map[string]bool{}
		}
		svcByNS[svc.Namespace][svc.Name] = true
	}

	// Scan all containers' env vars
	for _, container := range wl.podSpec.Containers {
		scanEnvVars(container.Env, wl, svcByNS, &deps, seen)
	}
	for _, container := range wl.podSpec.InitContainers {
		scanEnvVars(container.Env, wl, svcByNS, &deps, seen)
	}

	return deps
}

// scanEnvVars checks env vars for service DNS references.
func scanEnvVars(envs []corev1.EnvVar, wl workloadInfo, svcByNS map[string]map[string]bool, deps *[]serviceDepRef, seen map[string]bool) {
	for _, env := range envs {
		value := env.Value
		if env.ValueFrom != nil && env.ValueFrom.FieldRef != nil {
			value = env.ValueFrom.FieldRef.FieldPath
		}
		if value == "" {
			continue
		}

		// Look for service DNS patterns: <svc-name>.<ns>.svc, <svc-name>.svc
		// Also check direct service name references
		findServiceRefs(value, wl, svcByNS, deps, seen)
	}
}

// findServiceRefs searches a string for Kubernetes service DNS names.
func findServiceRefs(value string, wl workloadInfo, svcByNS map[string]map[string]bool, deps *[]serviceDepRef, seen map[string]bool) {
	valueLower := strings.ToLower(value)

	// Pattern 1: explicit DNS reference (svc.namespace.svc, svc.namespace.svc.cluster.local)
	// Extract the part before ".svc" to get the service DNS name
	if idx := strings.Index(valueLower, ".svc"); idx > 0 {
		// Get the substring before ".svc"
		beforeSvc := valueLower[:idx]
		// The service DNS name is "name.namespace" — find the last separator
		// Handle URLs like "postgres://db-service.prod" or "http://api-service.ns-backend"
		svcDNSName := beforeSvc
		// Strip protocol prefix (e.g., "postgres://", "http://", "redis://")
		if protoIdx := strings.Index(svcDNSName, "://"); protoIdx >= 0 {
			svcDNSName = svcDNSName[protoIdx+3:]
		}
		// Strip user info (e.g., "user:pass@")
		if atIdx := strings.Index(svcDNSName, "@"); atIdx >= 0 {
			svcDNSName = svcDNSName[atIdx+1:]
		}

		// Now svcDNSName should be "name.namespace"
		parts := strings.Split(svcDNSName, ".")
		if len(parts) >= 2 {
			svcName := parts[len(parts)-2]
			svcNS := parts[len(parts)-1]
			if svcs, ok := svcByNS[svcNS]; ok && svcs[svcName] {
				key := fmt.Sprintf("%s/%s", svcNS, svcName)
				if !seen[key] {
					seen[key] = true
					*deps = append(*deps, serviceDepRef{
						name:      svcName,
						namespace: svcNS,
						depType:   "service-ref",
					})
				}
			}
			return
		}
	}

	// Pattern 2: bare service name that matches a service in the same namespace
	if svcs, ok := svcByNS[wl.namespace]; ok {
		// Check if the value contains or equals a known service name
		for svcName := range svcs {
			if svcName == wl.name {
				continue // don't self-reference
			}
			if valueLower == svcName || strings.Contains(valueLower, svcName) {
				key := fmt.Sprintf("%s/%s", wl.namespace, svcName)
				if !seen[key] {
					seen[key] = true
					*deps = append(*deps, serviceDepRef{
						name:      svcName,
						namespace: wl.namespace,
						depType:   "service-ref",
					})
				}
			}
		}
	}
}

// computeMaxDepth finds the longest path in the dependency DAG using BFS.
func computeMaxDepth(edges []TopologyEdge) int {
	// Build adjacency list
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}

	// Find all nodes
	allNodes := map[string]bool{}
	for _, e := range edges {
		allNodes[e.From] = true
		allNodes[e.To] = true
	}

	// Calculate in-degree for topological sort
	inDegree := map[string]int{}
	for n := range allNodes {
		inDegree[n] = 0
	}
	for _, e := range edges {
		inDegree[e.To]++
	}

	// BFS with depth tracking, starting from nodes with no incoming edges
	type bfsEntry struct {
		node  string
		depth int
	}
	var queue []bfsEntry
	for n, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, bfsEntry{node: n, depth: 1})
		}
	}

	maxDepth := 0
	processed := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		processed++

		if curr.depth > maxDepth {
			maxDepth = curr.depth
		}

		for _, next := range adj[curr.node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, bfsEntry{node: next, depth: curr.depth + 1})
			}
		}
	}

	// If we couldn't process all nodes, there's a cycle
	if processed < len(allNodes) {
		// Approximate with number of nodes as max depth
		if len(allNodes) > maxDepth {
			maxDepth = len(allNodes)
		}
	}

	return maxDepth
}

// generateServiceTopologyRisks identifies cascade failure risks from the topology.
func generateServiceTopologyRisks(result ServiceTopologyResult) []TopologyRisk {
	var risks []TopologyRisk

	for _, hub := range result.CriticalHubs {
		if !hub.HasHA && hub.FanIn >= 1 {
			risks = append(risks, TopologyRisk{
				Severity: "critical",
				Category: "single-point-of-failure",
				Resource: fmt.Sprintf("%s/%s", hub.Namespace, hub.Name),
				Issue: fmt.Sprintf("%s %s has %d dependent workloads but only %d replica(s) — failure will cascade",
					hub.Kind, hub.Name, hub.FanIn, hub.Replicas),
				Affected: hub.FanIn,
			})
		} else if hub.FanIn >= 5 {
			risks = append(risks, TopologyRisk{
				Severity: "warning",
				Category: "high-fan-in",
				Resource: fmt.Sprintf("%s/%s", hub.Namespace, hub.Name),
				Issue: fmt.Sprintf("%s %s is a critical hub with %d dependents — ensure HA and monitoring",
					hub.Kind, hub.Name, hub.FanIn),
				Affected: hub.FanIn,
			})
		}
	}

	if result.Summary.CrossNamespace > 0 {
		risks = append(risks, TopologyRisk{
			Severity: "info",
			Category: "cross-namespace-dependency",
			Resource: "cluster-wide",
			Issue:    fmt.Sprintf("%d cross-namespace service dependencies detected — ensure NetworkPolicy coverage", result.Summary.CrossNamespace),
			Affected: result.Summary.CrossNamespace,
		})
	}

	if len(result.OrphanServices) > 0 {
		risks = append(risks, TopologyRisk{
			Severity: "warning",
			Category: "orphan-service",
			Resource: fmt.Sprintf("%d services", len(result.OrphanServices)),
			Issue:    "Services with selectors but no matching workloads — dead endpoints that may cause silent failures",
			Affected: len(result.OrphanServices),
		})
	}

	if result.Summary.MaxDepth > 5 {
		risks = append(risks, TopologyRisk{
			Severity: "warning",
			Category: "deep-dependency-chain",
			Resource: "cluster-wide",
			Issue:    fmt.Sprintf("Dependency chain depth is %d — deep chains amplify cascade failures", result.Summary.MaxDepth),
			Affected: result.Summary.MaxDepth,
		})
	}

	return risks
}

// generateServiceTopologyRecommendations produces actionable recommendations.
func generateServiceTopologyRecommendations(result ServiceTopologyResult) []string {
	var recs []string

	// Single point of failure warnings
	spofCount := 0
	for _, hub := range result.CriticalHubs {
		if !hub.HasHA && hub.FanIn >= 2 {
			spofCount++
		}
	}
	if spofCount > 0 {
		recs = append(recs, fmt.Sprintf("%d single point(s) of failure detected — services with multiple dependents but no HA. Scale up replicas to at least 2", spofCount))
	}

	// Critical hub monitoring
	if len(result.CriticalHubs) > 0 {
		top := result.CriticalHubs[0]
		recs = append(recs, fmt.Sprintf("Service %q in %s is the most critical hub (%d dependents) — prioritize its monitoring and alerting",
			top.Name, top.Namespace, top.FanIn))
	}

	// Cross-namespace dependencies
	if result.Summary.CrossNamespace > 0 {
		recs = append(recs, fmt.Sprintf("%d cross-namespace dependencies — verify NetworkPolicy rules allow inter-namespace traffic", result.Summary.CrossNamespace))
	}

	// Orphan services
	if len(result.OrphanServices) > 0 {
		recs = append(recs, fmt.Sprintf("%d orphan service(s) with no backing workload — clean up stale Services or fix workload selectors", len(result.OrphanServices)))
	}

	// Deep dependency chain
	if result.Summary.MaxDepth > 5 {
		recs = append(recs, fmt.Sprintf("Maximum dependency depth is %d — consider decoupling long chains with events/queues to reduce cascade risk", result.Summary.MaxDepth))
	}

	// Isolated workloads
	if result.Summary.IsolatedWorkloads > 0 && result.Summary.TotalWorkloads > 0 {
		pct := result.Summary.IsolatedWorkloads * 100 / maxInt(result.Summary.TotalWorkloads, 1)
		if pct > 30 {
			recs = append(recs, fmt.Sprintf("%d%% of workloads (%d) are isolated with no service dependencies — verify this is intentional",
				pct, result.Summary.IsolatedWorkloads))
		}
	}

	if len(recs) == 0 && result.Summary.TotalWorkloads > 0 {
		recs = append(recs, "Service topology is healthy — no critical hubs, single points of failure, or orphaned services detected")
	}

	return recs
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
