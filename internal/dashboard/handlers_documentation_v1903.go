package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.03 — Documentation Dimension (Round 3)
// 1. Cluster Runbook Generator
// 2. API Drift Detector
// 3. Resource Topology Doc
// ============================================================

// ---------------------------------------------------------------
// 1. Cluster Runbook Generator
// ---------------------------------------------------------------

type ClusterRunbookResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RunbookSummary1903 `json:"summary"`
	Runbook         string             `json:"runbook"`
	Sections        []RunbookSection   `json:"sections"`
	CriticalSOPs    []RunbookSOP       `json:"criticalSOPs"`
	Recommendations []string           `json:"recommendations"`
}

type RunbookSummary1903 struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	CriticalSOPs    int `json:"criticalSOPs"`
	EscalationPaths int `json:"escalationPaths"`
	Nodes           int `json:"nodes"`
	Namespaces      int `json:"namespaces"`
	RunbookSections int `json:"runbookSections"`
}

type RunbookSection struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Priority string `json:"priority"`
}

type RunbookSOP struct {
	Title      string `json:"title"`
	Trigger    string `json:"trigger"`
	Steps      string `json:"steps"`
	Escalation string `json:"escalation"`
	Priority   string `json:"priority"`
}

func (s *Server) handleClusterRunbookGen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ClusterRunbookResult{ScannedAt: time.Now()}

	// Gather cluster info
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}

	result.Summary.Nodes = len(nodes.Items)
	result.Summary.Namespaces = nsCount

	// Section: Cluster Overview
	var nodeInfo []string
	for _, node := range nodes.Items {
		ready := "NotReady"
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = "Ready"
			}
		}
		arch := node.Status.NodeInfo.Architecture
		kubelet := node.Status.NodeInfo.KubeletVersion
		nodeInfo = append(nodeInfo, fmt.Sprintf("  - %s (%s, %s, kubelet %s)", node.Name, ready, arch, kubelet))
	}

	result.Sections = append(result.Sections, RunbookSection{
		Title: "Cluster Overview", Priority: "info",
		Content: fmt.Sprintf("Nodes: %d\nNamespaces: %d\nDeployments: %d\nServices: %d\nPVCs: %d\n\nNode Details:\n%s",
			len(nodes.Items), nsCount, len(deps.Items), len(svcs.Items), len(pvcs.Items), strings.Join(nodeInfo, "\n")),
	})

	// Section: Critical Workloads
	criticalCount := 0
	var wlList []string
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas >= 3 || dep.Labels["app.kubernetes.io/name"] != "" {
			criticalCount++
			wlList = append(wlList, fmt.Sprintf("  - %s/%s (replicas: %d)", dep.Namespace, dep.Name, replicas))
		}
	}

	result.Sections = append(result.Sections, RunbookSection{
		Title: "Critical Workloads", Priority: "high",
		Content: fmt.Sprintf("Identified %d critical workloads (>= 3 replicas or labeled app):\n%s",
			criticalCount, strings.Join(wlList, "\n")),
	})

	// Section: Emergency SOPs
	result.CriticalSOPs = generateCriticalSOPs1903()
	result.Summary.CriticalSOPs = len(result.CriticalSOPs)
	result.Summary.EscalationPaths = nsCount // one per namespace

	// Section: Storage
	result.Sections = append(result.Sections, RunbookSection{
		Title: "Storage Overview", Priority: "medium",
		Content: fmt.Sprintf("Total PVCs: %d\nReview backup policies for stateful workloads", len(pvcs.Items)),
	})

	// Generate full markdown runbook
	var sb strings.Builder
	sb.WriteString("# Cluster Operations Runbook\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", result.ScannedAt.Format(time.RFC3339)))
	for _, sec := range result.Sections {
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", sec.Title, sec.Content))
	}
	sb.WriteString("## Standard Operating Procedures\n\n")
	for _, sop := range result.CriticalSOPs {
		sb.WriteString(fmt.Sprintf("### %s\n\n**Trigger:** %s\n\n**Steps:**\n%s\n\n**Escalation:** %s\n\n",
			sop.Title, sop.Trigger, sop.Steps, sop.Escalation))
	}
	result.Runbook = sb.String()
	result.Summary.RunbookSections = len(result.Sections)

	// Score based on coverage
	if result.Summary.TotalWorkloads > 0 {
		sopPct := result.Summary.CriticalSOPs * 100 / (result.Summary.CriticalSOPs + 1)
		result.HealthScore = sopPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildRunbookRecs1903(&result)
	writeJSON(w, result)
}

func generateCriticalSOPs1903() []RunbookSOP {
	var sops []RunbookSOP

	// Always add core SOPs
	sops = append(sops, RunbookSOP{
		Title: "Node NotReady Response", Priority: "critical",
		Trigger:    "Node condition becomes NotReady",
		Steps:      "1. Check node events: kubectl describe node <name>\n2. Check kubelet logs: journalctl -u kubelet\n3. Cordon node: kubectl cordon <name>\n4. Drain if needed: kubectl drain <name> --ignore-daemonsets\n5. Investigate hardware/network issues",
		Escalation: "If node unrecoverable in 30min, remove and provision replacement",
	})

	sops = append(sops, RunbookSOP{
		Title: "CrashLoopBackOff Response", Priority: "high",
		Trigger:    "Pod in CrashLoopBackOff state",
		Steps:      "1. Check pod logs: kubectl logs <pod> --previous\n2. Check events: kubectl describe pod <pod>\n3. Verify config/secrets are correct\n4. Check resource limits vs actual usage\n5. Rollback if recent deployment",
		Escalation: "If unresolved in 15min, rollback to previous version",
	})

	sops = append(sops, RunbookSOP{
		Title: "PVC Stuck Pending", Priority: "high",
		Trigger:    "PVC remains Pending for >5min",
		Steps:      "1. Check PVC events: kubectl describe pvc <name>\n2. Verify StorageClass provisioner is running\n3. Check cloud provider quota\n4. Verify zone availability",
		Escalation: "Contact storage team if provisioner is healthy but PVC still pending",
	})

	sops = append(sops, RunbookSOP{
		Title: "DNS Resolution Failure", Priority: "critical",
		Trigger:    "Services unreachable via cluster DNS",
		Steps:      "1. Check CoreDNS pods: kubectl get pods -n kube-system -l k8s-app=kube-dns\n2. Check CoreDNS config: kubectl get configmap coredns -n kube-system\n3. Test resolution: kubectl exec <pod> -- nslookup <service>\n4. Restart CoreDNS if needed",
		Escalation: "If CoreDNS healthy but resolution fails, check node /etc/resolv.conf",
	})

	return sops
}

// (removed appsv1Deployment type alias - no longer needed)

func buildRunbookRecs1903(r *ClusterRunbookResult) []string {
	recs := []string{fmt.Sprintf("Runbook generated: %d sections, %d SOPs, covering %d workloads across %d namespaces",
		r.Summary.RunbookSections, r.Summary.CriticalSOPs, r.Summary.TotalWorkloads, r.Summary.Namespaces)}
	if r.Summary.Nodes <= 1 {
		recs = append(recs, "Single-node cluster - add HA node SOPs when scaling to multi-node")
	}
	return recs
}

// ---------------------------------------------------------------
// 2. API Drift Detector
// ---------------------------------------------------------------

type APIDriftResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         APIDriftSummary     `json:"summary"`
	DriftedAPIs     []APIDriftEntry     `json:"driftedAPIs"`
	DeprecatedAPIs  []APIDriftEntry     `json:"deprecatedAPIs"`
	ByGroupVersion  []GroupVersionEntry `json:"byGroupVersion"`
	Recommendations []string            `json:"recommendations"`
}

type APIDriftSummary struct {
	TotalAPIs         int `json:"totalAPIs"`
	CurrentAPIs       int `json:"currentAPIs"`
	DeprecatedAPIs    int `json:"deprecatedAPIs"`
	RemovedAPIs       int `json:"removedAPIs"`
	PreviewAPIs       int `json:"previewAPIs"`
	TotalWorkloads    int `json:"totalWorkloads"`
	AffectedWorkloads int `json:"affectedWorkloads"`
}

type APIDriftEntry struct {
	GroupVersion string `json:"groupVersion"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Replacement  string `json:"replacement"`
	RemovedIn    string `json:"removedIn"`
	RiskLevel    string `json:"riskLevel"`
}

type GroupVersionEntry struct {
	GroupVersion  string `json:"groupVersion"`
	ResourceCount int    `json:"resourceCount"`
	Status        string `json:"status"`
}

func (s *Server) handleAPIDriftDetector(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := APIDriftResult{ScannedAt: time.Now()}

	// Known deprecated/removed API mapping (Kubernetes 1.25+)
	deprecatedAPIs := map[string]APIDriftEntry{
		"extensions/v1beta1": {
			GroupVersion: "extensions/v1beta1", Status: "removed", RemovedIn: "1.16",
			Replacement: "apps/v1 (Deployments), networking.k8s.io/v1 (Ingress)",
			RiskLevel:   "critical",
		},
		"networking.k8s.io/v1beta1": {
			GroupVersion: "networking.k8s.io/v1beta1", Status: "removed", RemovedIn: "1.19",
			Replacement: "networking.k8s.io/v1", RiskLevel: "critical",
		},
		"policy/v1beta1": {
			GroupVersion: "policy/v1beta1", Status: "removed", RemovedIn: "1.25",
			Replacement: "policy/v1 (PodDisruptionBudget)", RiskLevel: "critical",
		},
		"autoscaling/v2beta1": {
			GroupVersion: "autoscaling/v2beta1", Status: "deprecated", RemovedIn: "1.25",
			Replacement: "autoscaling/v2 (HPA)", RiskLevel: "high",
		},
		"autoscaling/v2beta2": {
			GroupVersion: "autoscaling/v2beta2", Status: "deprecated", RemovedIn: "1.26",
			Replacement: "autoscaling/v2 (HPA)", RiskLevel: "high",
		},
		"batch/v1beta1": {
			GroupVersion: "batch/v1beta1", Status: "deprecated", RemovedIn: "1.25",
			Replacement: "batch/v1 (CronJob)", RiskLevel: "high",
		},
	}

	// Get server preferred API resources
	apiGroups, _ := rc.clientset.Discovery().ServerPreferredResources()
	gvCount := map[string]int{}
	gvStatus := map[string]string{}

	for _, list := range apiGroups {
		if list == nil {
			continue
		}
		gv := list.GroupVersion
		result.Summary.TotalAPIs++
		gvCount[gv] += len(list.APIResources)

		if dep, ok := deprecatedAPIs[gv]; ok {
			result.Summary.DeprecatedAPIs++
			for _, res := range list.APIResources {
				entry := APIDriftEntry{
					GroupVersion: gv, Kind: res.Kind,
					Status: dep.Status, Replacement: dep.Replacement,
					RemovedIn: dep.RemovedIn, RiskLevel: dep.RiskLevel,
				}
				result.DriftedAPIs = append(result.DriftedAPIs, entry)
				if dep.Status == "removed" {
					result.Summary.RemovedAPIs++
				}
			}
			gvStatus[gv] = dep.Status
		} else if strings.Contains(gv, "alpha") {
			result.Summary.PreviewAPIs++
			gvStatus[gv] = "preview"
		} else {
			result.Summary.CurrentAPIs++
			gvStatus[gv] = "current"
		}
	}

	// Build by-group-version summary
	for gv, count := range gvCount {
		result.ByGroupVersion = append(result.ByGroupVersion, GroupVersionEntry{
			GroupVersion: gv, ResourceCount: count,
			Status: gvStatus[gv],
		})
	}
	sort.Slice(result.ByGroupVersion, func(i, j int) bool {
		return result.ByGroupVersion[i].Status == "removed" || result.ByGroupVersion[i].Status == "deprecated"
	})

	// Count affected workloads (deployments using deprecated APIs via annotations)
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
	}

	if result.Summary.TotalAPIs > 0 {
		currentPct := result.Summary.CurrentAPIs * 100 / result.Summary.TotalAPIs
		result.HealthScore = currentPct
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildAPIDriftRecs1903(&result)
	writeJSON(w, result)
}

func buildAPIDriftRecs1903(r *APIDriftResult) []string {
	recs := []string{fmt.Sprintf("API drift: %d total APIs (%d current, %d deprecated, %d removed, %d preview)",
		r.Summary.TotalAPIs, r.Summary.CurrentAPIs, r.Summary.DeprecatedAPIs,
		r.Summary.RemovedAPIs, r.Summary.PreviewAPIs)}
	if r.Summary.RemovedAPIs > 0 {
		recs = append(recs, fmt.Sprintf("%d removed APIs still referenced - immediate migration required", r.Summary.RemovedAPIs))
	}
	if r.Summary.DeprecatedAPIs > 0 {
		recs = append(recs, fmt.Sprintf("%d deprecated APIs detected - plan migration before next cluster upgrade", r.Summary.DeprecatedAPIs))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Resource Topology Doc
// ---------------------------------------------------------------

type TopologyDocResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TopologySummary1903 `json:"summary"`
	TopologyMD      string              `json:"topologyMarkdown"`
	NamespaceMap    []TopologyNSEntry   `json:"namespaceMap"`
	CriticalPaths   []TopologyPath      `json:"criticalPaths"`
	Recommendations []string            `json:"recommendations"`
}

type TopologySummary1903 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	TotalWorkloads  int `json:"totalWorkloads"`
	TotalServices   int `json:"totalServices"`
	TotalPVCs       int `json:"totalPVCs"`
	TotalIngress    int `json:"totalIngress"`
	EdgeNodes       int `json:"edgeNodes"`
}

type TopologyNSEntry struct {
	Namespace   string `json:"namespace"`
	Workloads   int    `json:"workloads"`
	Services    int    `json:"services"`
	PVCs        int    `json:"pvcs"`
	HasIngress  bool   `json:"hasIngress"`
	HasExternal bool   `json:"hasExternalAccess"`
}

type TopologyPath struct {
	From     string `json:"from"`
	To       string `json:"to"`
	PathType string `json:"pathType"`
	Critical bool   `json:"critical"`
}

func (s *Server) handleResourceTopologyDoc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TopologyDocResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})

	nsMap := map[string]*TopologyNSEntry{}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &TopologyNSEntry{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
			result.Summary.TotalNamespaces++
		}
		nsE.Workloads++
	}

	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++
		nsE, ok := nsMap[svc.Namespace]
		if !ok {
			nsE = &TopologyNSEntry{Namespace: svc.Namespace}
			nsMap[svc.Namespace] = nsE
			result.Summary.TotalNamespaces++
		}
		nsE.Services++
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
			nsE.HasExternal = true
			result.Summary.EdgeNodes++
		}
	}

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		nsE, ok := nsMap[pvc.Namespace]
		if !ok {
			nsE = &TopologyNSEntry{Namespace: pvc.Namespace}
			nsMap[pvc.Namespace] = nsE
			result.Summary.TotalNamespaces++
		}
		nsE.PVCs++
	}

	for _, ing := range ingresses.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		result.Summary.TotalIngress++
		nsE, ok := nsMap[ing.Namespace]
		if !ok {
			nsE = &TopologyNSEntry{Namespace: ing.Namespace}
			nsMap[ing.Namespace] = nsE
			result.Summary.TotalNamespaces++
		}
		nsE.HasIngress = true
		// Critical paths: ingress -> service
		for _, rule := range ing.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					result.CriticalPaths = append(result.CriticalPaths, TopologyPath{
						From:     fmt.Sprintf("ingress:%s/%s", ing.Namespace, ing.Name),
						To:       fmt.Sprintf("service:%s/%s", ing.Namespace, path.Backend.Service.Name),
						PathType: "ingress-to-service", Critical: true,
					})
				}
			}
		}
	}

	// Build namespace entries
	for _, ns := range nsMap {
		result.NamespaceMap = append(result.NamespaceMap, *ns)
	}
	sort.Slice(result.NamespaceMap, func(i, j int) bool {
		return result.NamespaceMap[i].Workloads > result.NamespaceMap[j].Workloads
	})

	// Generate markdown topology
	var sb strings.Builder
	sb.WriteString("# Resource Topology Map\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", result.ScannedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("## Summary\n\n- Namespaces: %d\n- Workloads: %d\n- Services: %d\n- PVCs: %d\n- Ingresses: %d\n\n",
		result.Summary.TotalNamespaces, result.Summary.TotalWorkloads, result.Summary.TotalServices,
		result.Summary.TotalPVCs, result.Summary.TotalIngress))
	sb.WriteString("## Namespace Topology\n\n")
	sb.WriteString("| Namespace | Workloads | Services | PVCs | Ingress | External |\n")
	sb.WriteString("|-----------|-----------|----------|------|---------|----------|\n")
	for _, ns := range result.NamespaceMap {
		ingStr := "No"
		extStr := "No"
		if ns.HasIngress {
			ingStr = "Yes"
		}
		if ns.HasExternal {
			extStr = "Yes"
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s | %s |\n", ns.Namespace, ns.Workloads, ns.Services, ns.PVCs, ingStr, extStr))
	}
	if len(result.CriticalPaths) > 0 {
		sb.WriteString("\n## Critical Traffic Paths\n\n")
		for _, p := range result.CriticalPaths {
			sb.WriteString(fmt.Sprintf("- %s -> %s (%s)\n", p.From, p.To, p.PathType))
		}
	}
	result.TopologyMD = sb.String()

	// Score: more documentation coverage = better
	coverage := 0
	if result.Summary.TotalNamespaces > 0 {
		coverage += 25
	}
	if result.Summary.TotalWorkloads > 0 {
		coverage += 25
	}
	if result.Summary.TotalServices > 0 {
		coverage += 25
	}
	if len(result.CriticalPaths) > 0 || result.Summary.TotalIngress == 0 {
		coverage += 25
	}
	result.HealthScore = coverage
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildTopologyRecs1903(&result)
	writeJSON(w, result)
}

func buildTopologyRecs1903(r *TopologyDocResult) []string {
	recs := []string{fmt.Sprintf("Topology doc: %d namespaces, %d workloads, %d services, %d critical paths mapped",
		r.Summary.TotalNamespaces, r.Summary.TotalWorkloads, r.Summary.TotalServices, len(r.CriticalPaths))}
	if r.Summary.EdgeNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d services with external exposure - document firewall rules and access controls", r.Summary.EdgeNodes))
	}
	return recs
}
