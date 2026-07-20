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
// v18.91 — Product Dimension
// 1. PriorityClass Audit
// 2. Service Exposure Map
// 3. Workload Anti-Affinity HA Readiness
// ============================================================

// ---------------------------------------------------------------
// 1. PriorityClass Audit — analyzes priority class usage, preemption risk
// ---------------------------------------------------------------

// PriorityClassAuditResult provides a comprehensive priority class audit.
type PriorityClassAuditResult struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         PriorityClassAuditSummary `json:"summary"`
	DefinedClasses  []PriorityClassInfo1891   `json:"definedClasses"`
	WorkloadClasses []WorkloadPriorityEntry   `json:"workloadClasses"`
	PreemptionRisk  []PreemptionRiskEntry     `json:"preemptionRisk"`
	Recommendations []string                  `json:"recommendations"`
}

type PriorityClassAuditSummary struct {
	TotalWorkloads        int `json:"totalWorkloads"`
	WithPriorityClass     int `json:"withPriorityClass"`
	WithoutPriority       int `json:"withoutPriorityClass"`
	DefinedClasses        int `json:"definedClasses"`
	HighPriorityWorkloads int `json:"highPriorityWorkloads"`
	LowPriorityWorkloads  int `json:"lowPriorityWorkloads"`
	SystemCriticals       int `json:"systemCriticals"`
	BestEffort            int `json:"bestEffort"`
	PreemptionPairs       int `json:"preemptionPairs"`
}

type PriorityClassInfo1891 struct {
	Name             string `json:"name"`
	Value            int32  `json:"value"`
	GlobalDefault    bool   `json:"globalDefault"`
	PreemptionPolicy string `json:"preemptionPolicy"`
	Description      string `json:"description"`
	WorkloadCount    int    `json:"workloadCount"`
}

type WorkloadPriorityEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	PriorityClass string `json:"priorityClass"`
	PriorityValue int32  `json:"priorityValue"`
	RiskLevel     string `json:"riskLevel"`
	Issue         string `json:"issue"`
}

type PreemptionRiskEntry struct {
	Victim            string `json:"victim"`
	VictimNs          string `json:"victimNamespace"`
	VictimPriority    int32  `json:"victimPriority"`
	Preemptor         string `json:"preemptor"`
	PreemptorNs       string `json:"preemptorNamespace"`
	PreemptorPriority int32  `json:"preemptorPriority"`
	RiskLevel         string `json:"riskLevel"`
}

func (s *Server) handlePriorityClassAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PriorityClassAuditResult{ScannedAt: time.Now()}

	// Get all PriorityClasses
	pcList, err := rc.clientset.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
	pcMap := map[string]int32{} // name -> value
	if err == nil {
		for _, pc := range pcList.Items {
			result.Summary.DefinedClasses++
			info := PriorityClassInfo1891{
				Name:             pc.Name,
				Value:            pc.Value,
				GlobalDefault:    pc.GlobalDefault,
				PreemptionPolicy: stringPtr1891(pc.PreemptionPolicy),
				Description:      pc.Description,
				WorkloadCount:    0,
			}
			pcMap[pc.Name] = pc.Value
			result.DefinedClasses = append(result.DefinedClasses, info)
		}
	}

	// Analyze deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		analyzeWorkloadPriority1891(&result, dep.Name, dep.Namespace, "Deployment", dep.Spec.Template.Spec.PriorityClassName, pcMap)
	}

	// Analyze StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		analyzeWorkloadPriority1891(&result, ss.Name, ss.Namespace, "StatefulSet", ss.Spec.Template.Spec.PriorityClassName, pcMap)
	}

	// Count workload usage per priority class
	for i := range result.DefinedClasses {
		for _, wl := range result.WorkloadClasses {
			if wl.PriorityClass == result.DefinedClasses[i].Name {
				result.DefinedClasses[i].WorkloadCount++
			}
		}
	}

	// Detect preemption risk pairs
	result.PreemptionRisk = detectPreemptionPairs1891(&result)
	result.Summary.PreemptionPairs = len(result.PreemptionRisk)

	// Score
	if result.Summary.TotalWorkloads > 0 {
		coverage := result.Summary.WithPriorityClass * 100 / result.Summary.TotalWorkloads
		result.HealthScore = coverage
	}
	// Penalty for high preemption risk
	if result.Summary.PreemptionPairs > 5 {
		result.HealthScore -= 10
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildPriorityClassRecs1891(&result)
	writeJSON(w, result)
}

func analyzeWorkloadPriority1891(result *PriorityClassAuditResult, name, ns, kind, pcName string, pcMap map[string]int32) {
	entry := WorkloadPriorityEntry{
		Name:          name,
		Namespace:     ns,
		Kind:          kind,
		PriorityClass: pcName,
	}

	if pcName == "" {
		result.Summary.WithoutPriority++
		entry.Issue = "no priority class set - may be preempted unexpectedly"
		entry.RiskLevel = "high"
		// Default priority is usually 0
		entry.PriorityValue = 0
		result.Summary.BestEffort++
	} else {
		result.Summary.WithPriorityClass++
		if val, ok := pcMap[pcName]; ok {
			entry.PriorityValue = val
			if val >= 1000000 {
				result.Summary.SystemCriticals++
				entry.RiskLevel = "info"
			} else if val >= 100000 {
				result.Summary.HighPriorityWorkloads++
				entry.RiskLevel = "medium"
			} else if val <= 0 {
				result.Summary.LowPriorityWorkloads++
				result.Summary.BestEffort++
				entry.RiskLevel = "medium"
				entry.Issue = "best-effort priority - can be preempted by any higher priority workload"
			} else {
				entry.RiskLevel = "low"
			}
		} else {
			entry.Issue = "priority class not found: " + pcName
			entry.RiskLevel = "high"
		}
	}

	result.WorkloadClasses = append(result.WorkloadClasses, entry)
}

func detectPreemptionPairs1891(result *PriorityClassAuditResult) []PreemptionRiskEntry {
	var pairs []PreemptionRiskEntry

	// Sort by priority value descending
	sorted := make([]WorkloadPriorityEntry, len(result.WorkloadClasses))
	copy(sorted, result.WorkloadClasses)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PriorityValue > sorted[j].PriorityValue
	})

	// Find high-priority workloads that could preempt low-priority ones in same namespace
	for i, high := range sorted {
		if high.PriorityValue < 100000 {
			break
		}
		for j := len(sorted) - 1; j > i; j-- {
			low := sorted[j]
			if low.PriorityValue >= high.PriorityValue {
				continue
			}
			// Only flag if in same namespace or cluster-wide
			if high.Namespace == low.Namespace {
				pairs = append(pairs, PreemptionRiskEntry{
					Victim:            low.Name,
					VictimNs:          low.Namespace,
					VictimPriority:    low.PriorityValue,
					Preemptor:         high.Name,
					PreemptorNs:       high.Namespace,
					PreemptorPriority: high.PriorityValue,
					RiskLevel:         "medium",
				})
				if len(pairs) >= 30 {
					return pairs
				}
			}
		}
	}

	return pairs
}

func buildPriorityClassRecs1891(result *PriorityClassAuditResult) []string {
	recs := []string{
		fmt.Sprintf("PriorityClass audit: %d workloads, %d with priority class (%d%%), %d defined classes",
			result.Summary.TotalWorkloads, result.Summary.WithPriorityClass,
			safePercent1891(result.Summary.WithPriorityClass, result.Summary.TotalWorkloads),
			result.Summary.DefinedClasses),
	}
	if result.Summary.WithoutPriority > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without priority class - assign appropriate priority to control preemption behavior", result.Summary.WithoutPriority))
	}
	if result.Summary.BestEffort > 0 {
		recs = append(recs, fmt.Sprintf("%d best-effort workloads (priority <= 0) at risk of preemption by any higher-priority workload", result.Summary.BestEffort))
	}
	if result.Summary.PreemptionPairs > 0 {
		recs = append(recs, fmt.Sprintf("%d potential preemption pairs detected - review priority class assignments to avoid cascading evictions", result.Summary.PreemptionPairs))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Service Exposure Map — maps all service exposure paths
// ---------------------------------------------------------------

// ServiceExposureResult maps all service exposure paths and identifies over-exposed services.
type ServiceExposureResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ServiceExposureSummary `json:"summary"`
	ExposurePaths   []ServiceExposureEntry `json:"exposurePaths"`
	OverExposed     []ServiceExposureEntry `json:"overExposed"`
	InternalOnly    []ServiceExposureEntry `json:"internalOnly"`
	IngressRoutes   []IngressExposureEntry `json:"ingressRoutes"`
	Recommendations []string               `json:"recommendations"`
}

type ServiceExposureSummary struct {
	TotalServices   int `json:"totalServices"`
	ClusterIP       int `json:"clusterIP"`
	NodePort        int `json:"nodePort"`
	LoadBalancer    int `json:"loadBalancer"`
	ExternalName    int `json:"externalName"`
	Headless        int `json:"headless"`
	WithIngress     int `json:"withIngress"`
	PubliclyExposed int `json:"publiclyExposed"`
	OverExposed     int `json:"overExposed"`
	InternalSafe    int `json:"internalSafe"`
}

type ServiceExposureEntry struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Type          string   `json:"type"`
	ClusterIP     string   `json:"clusterIP"`
	ExternalIPs   []string `json:"externalIPs,omitempty"`
	Ports         []string `json:"ports"`
	HasIngress    bool     `json:"hasIngress"`
	ExposureLevel string   `json:"exposureLevel"`
	Selectors     []string `json:"selectors,omitempty"`
	RiskLevel     string   `json:"riskLevel"`
	Issue         string   `json:"issue"`
}

type IngressExposureEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Host           string `json:"host"`
	PathCount      int    `json:"pathCount"`
	TLSEnabled     bool   `json:"tlsEnabled"`
	BackendService string `json:"backendService"`
}

func (s *Server) handleServiceExposureMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ServiceExposureResult{ScannedAt: time.Now()}

	// Map ingresses to their backend services
	ingressMap := map[string][]IngressExposureEntry{} // ns/svc -> ingresses
	ingList, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	for _, ing := range ingList.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		tlsEnabled := len(ing.Spec.TLS) > 0
		hosts := []string{}
		if len(ing.Spec.Rules) > 0 {
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" {
					hosts = append(hosts, rule.Host)
				}
			}
		}
		hostStr := strings.Join(hosts, ", ")
		if hostStr == "" {
			hostStr = "*"
		}

		pathCount := 0
		for _, rule := range ing.Spec.Rules {
			if rule.IngressRuleValue.HTTP != nil {
				pathCount += len(rule.IngressRuleValue.HTTP.Paths)
				for _, path := range rule.IngressRuleValue.HTTP.Paths {
					backend := ""
					if path.Backend.Service != nil {
						backend = path.Backend.Service.Name
					}
					key := ing.Namespace + "/" + backend
					ingressMap[key] = append(ingressMap[key], IngressExposureEntry{
						Name:           ing.Name,
						Namespace:      ing.Namespace,
						Host:           hostStr,
						PathCount:      pathCount,
						TLSEnabled:     tlsEnabled,
						BackendService: backend,
					})
				}
			}
		}
		if pathCount == 0 && len(ingList.Items) > 0 {
			// Default backend
			ingressMap[ing.Namespace+"/"] = append(ingressMap[ing.Namespace+"/"], IngressExposureEntry{
				Name:       ing.Name,
				Namespace:  ing.Namespace,
				Host:       hostStr,
				PathCount:  0,
				TLSEnabled: tlsEnabled,
			})
		}

		result.IngressRoutes = append(result.IngressRoutes, IngressExposureEntry{
			Name:       ing.Name,
			Namespace:  ing.Namespace,
			Host:       hostStr,
			PathCount:  pathCount,
			TLSEnabled: tlsEnabled,
		})
		result.Summary.WithIngress++
	}

	// Analyze services
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		entry := ServiceExposureEntry{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			ClusterIP: svc.Spec.ClusterIP,
			Type:      string(svc.Spec.Type),
		}

		// Build port list
		for _, port := range svc.Spec.Ports {
			portStr := fmt.Sprintf("%d/%s", port.Port, string(port.Protocol))
			if port.NodePort > 0 {
				portStr += fmt.Sprintf(" (nodePort:%d)", port.NodePort)
			}
			entry.Ports = append(entry.Ports, portStr)
		}

		// External IPs
		if len(svc.Spec.ExternalIPs) > 0 {
			entry.ExternalIPs = svc.Spec.ExternalIPs
		}

		// Selector
		for k, v := range svc.Spec.Selector {
			entry.Selectors = append(entry.Selectors, k+"="+v)
		}

		// Check ingress association
		key := svc.Namespace + "/" + svc.Name
		if ings, ok := ingressMap[key]; ok && len(ings) > 0 {
			entry.HasIngress = true
		}

		// Classify exposure level
		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			if svc.Spec.ClusterIP == "None" {
				entry.ExposureLevel = "headless"
				result.Summary.Headless++
			} else {
				entry.ExposureLevel = "internal"
				result.Summary.ClusterIP++
			}
			entry.RiskLevel = "low"
		case corev1.ServiceTypeNodePort:
			entry.ExposureLevel = "cluster-nodeport"
			result.Summary.NodePort++
			result.Summary.PubliclyExposed++
			entry.RiskLevel = "medium"
			if entry.HasIngress {
				entry.Issue = "NodePort with Ingress - consider restricting to ClusterIP"
				result.Summary.OverExposed++
				result.OverExposed = append(result.OverExposed, entry)
			}
		case corev1.ServiceTypeLoadBalancer:
			entry.ExposureLevel = "public-lb"
			result.Summary.LoadBalancer++
			result.Summary.PubliclyExposed++
			entry.RiskLevel = "high"
			if entry.HasIngress {
				entry.Issue = "LoadBalancer with Ingress - double exposure, consider using ClusterIP + Ingress only"
				result.Summary.OverExposed++
				result.OverExposed = append(result.OverExposed, entry)
			}
		case corev1.ServiceTypeExternalName:
			entry.ExposureLevel = "external-name"
			result.Summary.ExternalName++
			entry.RiskLevel = "info"
		default:
			entry.ExposureLevel = "unknown"
		}

		// Check for no selector (endpoints manually managed)
		if len(svc.Spec.Selector) == 0 && svc.Spec.Type != corev1.ServiceTypeExternalName {
			entry.Issue = "no selector - endpoints must be managed manually"
			entry.RiskLevel = "medium"
		}

		result.ExposurePaths = append(result.ExposurePaths, entry)
		if entry.RiskLevel == "low" {
			result.Summary.InternalSafe++
			result.InternalOnly = append(result.InternalOnly, entry)
		}
	}

	// Score: more internal-safe services = better posture
	if result.Summary.TotalServices > 0 {
		internalPct := result.Summary.InternalSafe * 100 / result.Summary.TotalServices
		overExpPenalty := result.Summary.OverExposed * 5
		result.HealthScore = internalPct - overExpPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildServiceExposureRecs1891(&result)
	writeJSON(w, result)
}

func buildServiceExposureRecs1891(result *ServiceExposureResult) []string {
	recs := []string{
		fmt.Sprintf("Service exposure: %d services, %d internal, %d public (%d LB, %d NodePort), %d over-exposed",
			result.Summary.TotalServices, result.Summary.InternalSafe,
			result.Summary.PubliclyExposed, result.Summary.LoadBalancer,
			result.Summary.NodePort, result.Summary.OverExposed),
	}
	if result.Summary.OverExposed > 0 {
		recs = append(recs, fmt.Sprintf("%d over-exposed services (LoadBalancer/NodePort with Ingress) - reduce attack surface", result.Summary.OverExposed))
	}
	if result.Summary.LoadBalancer > 0 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer services - verify NetworkPolicy coverage for all publicly exposed services", result.Summary.LoadBalancer))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Workload Anti-Affinity HA Readiness
// ---------------------------------------------------------------

// AntiaffinityHAResult analyzes pod anti-affinity rules for HA readiness.
type AntiaffinityHAResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         AntiaffinityHASummary `json:"summary"`
	ByWorkload      []AntiaffinityEntry   `json:"byWorkload"`
	HAGaps          []AntiaffinityEntry   `json:"haGaps"`
	NodeSpread      []NodeSpreadEntry1891 `json:"nodeSpread"`
	Recommendations []string              `json:"recommendations"`
}

type AntiaffinityHASummary struct {
	TotalWorkloads       int `json:"totalWorkloads"`
	WithAntiAffinity     int `json:"withAntiAffinity"`
	WithPodAntiAffinity  int `json:"withPodAntiAffinity"`
	WithNodeAntiAffinity int `json:"withNodeAntiAffinity"`
	WithTopologySpread   int `json:"withTopologySpread"`
	NoAffinity           int `json:"noAffinity"`
	SingleReplica        int `json:"singleReplica"`
	MultiReplicaNoHA     int `json:"multiReplicaNoHA"`
	HAReady              int `json:"haReady"`
	TotalNodes           int `json:"totalNodes"`
}

type AntiaffinityEntry struct {
	Name                 string `json:"name"`
	Namespace            string `json:"namespace"`
	Kind                 string `json:"kind"`
	Replicas             int32  `json:"replicas"`
	HasPodAntiAffinity   bool   `json:"hasPodAntiAffinity"`
	HasNodeAntiAffinity  bool   `json:"hasNodeAntiAffinity"`
	HasTopologySpread    bool   `json:"hasTopologySpread"`
	PodSpreadAcrossNodes int    `json:"podSpreadAcrossNodes"`
	HAReady              bool   `json:"haReady"`
	RiskLevel            string `json:"riskLevel"`
	Issue                string `json:"issue"`
}

type NodeSpreadEntry1891 struct {
	Node           string `json:"node"`
	PodCount       int    `json:"podCount"`
	NamespaceCount int    `json:"namespaceCount"`
}

func (s *Server) handleAntiaffinityHA(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := AntiaffinityHAResult{ScannedAt: time.Now()}

	// Count nodes
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodes.Items)

	// Build pod -> node map and pod owner -> node spread
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	podOwnerNodeMap := map[string]map[string]int{} // "ns/ownerKind/ownerName" -> node -> count
	nodePodCount := map[string]int{}
	nodeNsCount := map[string]map[string]bool{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		ownerKind, ownerName := getOwnerKind(pod.OwnerReferences), getOwnerName(pod.OwnerReferences)
		if ownerName == "" {
			ownerKind, ownerName = "Pod", pod.Name
		}
		key := pod.Namespace + "/" + ownerKind + "/" + ownerName
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			nodeName = "<unassigned>"
		}
		if podOwnerNodeMap[key] == nil {
			podOwnerNodeMap[key] = map[string]int{}
		}
		podOwnerNodeMap[key][nodeName]++
		nodePodCount[nodeName]++
		if nodeNsCount[nodeName] == nil {
			nodeNsCount[nodeName] = map[string]bool{}
		}
		nodeNsCount[nodeName][pod.Namespace] = true
	}

	// Build node spread entries
	for node, count := range nodePodCount {
		result.NodeSpread = append(result.NodeSpread, NodeSpreadEntry1891{
			Node:           node,
			PodCount:       count,
			NamespaceCount: len(nodeNsCount[node]),
		})
	}
	sort.Slice(result.NodeSpread, func(i, j int) bool {
		return result.NodeSpread[i].PodCount > result.NodeSpread[j].PodCount
	})

	// Analyze deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		analyzeAntiaffinity1891(&result, dep.Name, dep.Namespace, "Deployment",
			dep.Spec.Replicas, dep.Spec.Template.Spec.Affinity,
			dep.Spec.Template.Spec.TopologySpreadConstraints,
			podOwnerNodeMap)
	}

	// Analyze StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		analyzeAntiaffinity1891(&result, ss.Name, ss.Namespace, "StatefulSet",
			ss.Spec.Replicas, ss.Spec.Template.Spec.Affinity,
			ss.Spec.Template.Spec.TopologySpreadConstraints,
			podOwnerNodeMap)
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		haPct := result.Summary.HAReady * 100 / result.Summary.TotalWorkloads
		result.HealthScore = haPct
	}
	// If only 1 node, cap score since HA is physically impossible
	if result.Summary.TotalNodes <= 1 {
		result.HealthScore = result.HealthScore / 2
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildAntiaffinityHARecs1891(&result)
	writeJSON(w, result)
}

func analyzeAntiaffinity1891(result *AntiaffinityHAResult, name, ns, kind string, replicas *int32,
	affinity *corev1.Affinity, topologySpread []corev1.TopologySpreadConstraint,
	podOwnerNodeMap map[string]map[string]int) {

	result.Summary.TotalWorkloads++

	repCount := int32(1)
	if replicas != nil {
		repCount = *replicas
	}

	entry := AntiaffinityEntry{
		Name:      name,
		Namespace: ns,
		Kind:      kind,
		Replicas:  repCount,
	}

	// Check affinity rules
	if affinity != nil {
		if affinity.PodAntiAffinity != nil {
			entry.HasPodAntiAffinity = true
			result.Summary.WithPodAntiAffinity++
			// Check if it's a hard or soft requirement
			if len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				entry.HasNodeAntiAffinity = true
				result.Summary.WithNodeAntiAffinity++
			}
		}
	}

	if len(topologySpread) > 0 {
		entry.HasTopologySpread = true
		result.Summary.WithTopologySpread++
	}

	if entry.HasPodAntiAffinity || entry.HasTopologySpread {
		result.Summary.WithAntiAffinity++
	}

	// Calculate actual pod spread across nodes
	ownerKey := ns + "/" + kind + "/" + name
	if nodeMap, ok := podOwnerNodeMap[ownerKey]; ok {
		entry.PodSpreadAcrossNodes = len(nodeMap)
	}

	// Classify HA readiness
	switch {
	case repCount <= 1:
		result.Summary.SingleReplica++
		entry.RiskLevel = "high"
		entry.Issue = "single replica - no HA possible, any node failure causes downtime"
		entry.HAReady = false
	case entry.HasPodAntiAffinity && entry.PodSpreadAcrossNodes > 1:
		entry.HAReady = true
		entry.RiskLevel = "low"
		result.Summary.HAReady++
	case entry.HasTopologySpread && entry.PodSpreadAcrossNodes > 1:
		entry.HAReady = true
		entry.RiskLevel = "low"
		result.Summary.HAReady++
	case entry.PodSpreadAcrossNodes > 1:
		entry.RiskLevel = "medium"
		entry.Issue = "pods spread across nodes but no anti-affinity rule - may coalesce on single node after restart"
		entry.HAReady = false
		result.Summary.MultiReplicaNoHA++
		result.HAGaps = append(result.HAGaps, entry)
	default:
		entry.RiskLevel = "high"
		if !entry.HasPodAntiAffinity && !entry.HasTopologySpread {
			entry.Issue = "multi-replica workload without anti-affinity - pods may land on same node"
			result.Summary.NoAffinity++
			result.Summary.MultiReplicaNoHA++
		} else {
			entry.Issue = "anti-affinity configured but pods not spread - check node availability"
		}
		entry.HAReady = false
		result.HAGaps = append(result.HAGaps, entry)
	}

	result.ByWorkload = append(result.ByWorkload, entry)
}

func buildAntiaffinityHARecs1891(result *AntiaffinityHAResult) []string {
	recs := []string{
		fmt.Sprintf("Anti-affinity HA readiness: %d workloads, %d HA-ready (%d%%), %d single-replica, %d multi-replica without HA",
			result.Summary.TotalWorkloads, result.Summary.HAReady,
			safePercent1891(result.Summary.HAReady, result.Summary.TotalWorkloads),
			result.Summary.SingleReplica, result.Summary.MultiReplicaNoHA),
	}
	if result.Summary.TotalNodes <= 1 {
		recs = append(recs, fmt.Sprintf("Only %d node(s) in cluster - HA requires at least 2 nodes for anti-affinity to be effective", result.Summary.TotalNodes))
	}
	if result.Summary.SingleReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d single-replica workloads - scale to 2+ replicas for HA", result.Summary.SingleReplica))
	}
	if result.Summary.MultiReplicaNoHA > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workloads without anti-affinity - add podAntiAffinity or topologySpreadConstraints", result.Summary.MultiReplicaNoHA))
	}
	return recs
}

// Helper functions
func stringPtr1891(s *corev1.PreemptionPolicy) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func safePercent1891(num, denom int) int {
	if denom == 0 {
		return 0
	}
	return num * 100 / denom
}
