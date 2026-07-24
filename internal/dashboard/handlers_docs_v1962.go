package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.62 — Documentation Dimension (Round 13)
// 1. Port Catalog — all exposed ports & service mapping
// 2. RBAC Cheatsheet — who-can-do-what permission matrix
// 3. Cluster Blueprint — cluster architecture & topology overview
// ============================================================

// ---------------------------------------------------------------
// 1. Port Catalog
// ---------------------------------------------------------------

type PortCatalogResult1962 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PortCatalogSummary1962 `json:"summary"`
	PortMappings    []PortCatalogEntry1962 `json:"portMappings"`
	Conflicts       []PortConflict1962     `json:"conflicts"`
	Recommendations []string               `json:"recommendations"`
}

type PortCatalogSummary1962 struct {
	TotalServices    int `json:"totalServices"`
	TotalPorts       int `json:"totalPorts"`
	NodePortServices int `json:"nodePortServices"`
	LoadBalancerSvc  int `json:"loadBalancerServices"`
	ClusterIPSvc     int `json:"clusterIPServices"`
	ExposedPorts     int `json:"externallyExposedPorts"`
	UniqueNodePorts  int `json:"uniqueNodePorts"`
}

type PortCatalogEntry1962 struct {
	Service    string `json:"service"`
	Namespace  string `json:"namespace"`
	Type       string `json:"serviceType"`
	PortName   string `json:"portName"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
	External   bool   `json:"externallyExposed"`
}

type PortConflict1962 struct {
	NodePort int32    `json:"nodePort"`
	Services []string `json:"services"`
}

func (s *Server) handlePortCatalog(w http.ResponseWriter, r *http.Request) {
	result := PortCatalogResult1962{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	// Track node port usage for conflict detection
	nodePortMap := make(map[int32][]string)

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		svcType := string(svc.Spec.Type)

		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++

			entry := PortCatalogEntry1962{
				Service:    svc.Name,
				Namespace:  svc.Namespace,
				Type:       svcType,
				PortName:   p.Name,
				Port:       p.Port,
				TargetPort: p.TargetPort.String(),
				External:   svcType == "LoadBalancer" || svcType == "NodePort",
			}

			if p.NodePort > 0 {
				entry.NodePort = p.NodePort
				result.Summary.UniqueNodePorts++
				nodePortMap[p.NodePort] = append(nodePortMap[p.NodePort],
					fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))
			}

			if entry.External {
				result.Summary.ExposedPorts++
			}

			result.PortMappings = append(result.PortMappings, entry)
		}

		switch svcType {
		case "LoadBalancer":
			result.Summary.LoadBalancerSvc++
		case "NodePort":
			result.Summary.NodePortServices++
		case "ClusterIP":
			result.Summary.ClusterIPSvc++
		}
	}

	// Detect node port conflicts
	for np, svcs := range nodePortMap {
		if len(svcs) > 1 {
			result.Conflicts = append(result.Conflicts, PortConflict1962{
				NodePort: np, Services: svcs,
			})
			score -= 5
		}
	}

	sort.Slice(result.PortMappings, func(i, j int) bool {
		if result.PortMappings[i].Namespace == result.PortMappings[j].Namespace {
			return result.PortMappings[i].Port < result.PortMappings[j].Port
		}
		return result.PortMappings[i].Namespace < result.PortMappings[j].Namespace
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services, %d ports (%d externally exposed)", result.Summary.TotalServices, result.Summary.TotalPorts, result.Summary.ExposedPorts))
	if len(result.Conflicts) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d node port conflicts detected", len(result.Conflicts)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. RBAC Cheatsheet
// ---------------------------------------------------------------

type RBACCheatsheetResult1962 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         RBACCheatsheetSummary1962 `json:"summary"`
	RoleBindings    []RBACBindingEntry1962    `json:"roleBindings"`
	ClusterBindings []RBACBindingEntry1962    `json:"clusterRoleBindings"`
	HighRiskRoles   []RBACRoleEntry1962       `json:"highRiskRoles"`
	Recommendations []string                  `json:"recommendations"`
}

type RBACCheatsheetSummary1962 struct {
	TotalSubjects       int `json:"totalSubjects"`
	RoleBindings        int `json:"roleBindings"`
	ClusterRoleBindings int `json:"clusterRoleBindings"`
	ClusterAdmins       int `json:"clusterAdmins"`
	HighRiskVerbs       int `json:"highRiskVerbBindings"`
	WildcardBindings    int `json:"wildcardBindings"`
}

type RBACBindingEntry1962 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Subject     string `json:"subject"`
	SubjectKind string `json:"subjectKind"`
	RoleRef     string `json:"roleRef"`
	RoleKind    string `json:"roleKind"`
}

type RBACRoleEntry1962 struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	RiskLevel string   `json:"riskLevel"`
}

func (s *Server) handleRBACCheatsheet(w http.ResponseWriter, r *http.Request) {
	result := RBACCheatsheetResult1962{ScannedAt: time.Now()}
	score := 100

	// RoleBindings
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		result.Summary.RoleBindings++
		for _, sub := range rb.Subjects {
			result.Summary.TotalSubjects++
			entry := RBACBindingEntry1962{
				Name: rb.Name, Namespace: rb.Namespace,
				Subject: sub.Name, SubjectKind: string(sub.Kind),
				RoleRef: rb.RoleRef.Name, RoleKind: rb.RoleRef.Kind,
			}
			result.RoleBindings = append(result.RoleBindings, entry)
		}
	}

	// ClusterRoleBindings
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		result.Summary.ClusterRoleBindings++
		for _, sub := range crb.Subjects {
			result.Summary.TotalSubjects++
			entry := RBACBindingEntry1962{
				Name: crb.Name, Namespace: "",
				Subject: sub.Name, SubjectKind: string(sub.Kind),
				RoleRef: crb.RoleRef.Name, RoleKind: crb.RoleRef.Kind,
			}
			result.ClusterBindings = append(result.ClusterBindings, entry)

			if crb.RoleRef.Name == "cluster-admin" {
				result.Summary.ClusterAdmins++
				score -= 3
			}
		}
	}

	// Check for high-risk verbs and wildcards
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	for _, role := range roleList.Items {
		for _, rule := range role.Rules {
			riskLevel := "low"
			verbs := verbSliceToStr1962(rule.Verbs)
			resources := verbSliceToStr1962(rule.Resources)

			if strings.Contains(verbs, "*") {
				result.Summary.WildcardBindings++
				riskLevel = "high"
				score -= 3
			}
			if strings.Contains(verbs, "create") || strings.Contains(verbs, "delete") ||
				strings.Contains(verbs, "escalate") || strings.Contains(verbs, "impersonate") {
				result.Summary.HighRiskVerbs++
				if riskLevel != "high" {
					riskLevel = "medium"
				}
			}
			if strings.Contains(resources, "*") && riskLevel == "high" {
				riskLevel = "critical"
				score -= 5
			}

			if riskLevel == "medium" || riskLevel == "high" || riskLevel == "critical" {
				result.HighRiskRoles = append(result.HighRiskRoles, RBACRoleEntry1962{
					Name: role.Name, Namespace: role.Namespace,
					Verbs: rule.Verbs, Resources: rule.Resources,
					RiskLevel: riskLevel,
				})
			}
		}
	}

	// ClusterRoles
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, role := range crList.Items {
		// Skip system roles
		if strings.HasPrefix(role.Name, "system:") {
			continue
		}
		for _, rule := range role.Rules {
			verbs := verbSliceToStr1962(rule.Verbs)
			resources := verbSliceToStr1962(rule.Resources)
			riskLevel := "low"

			if strings.Contains(verbs, "*") {
				riskLevel = "high"
			}
			if strings.Contains(verbs, "escalate") || strings.Contains(verbs, "impersonate") {
				riskLevel = "critical"
				score -= 5
			}
			if strings.Contains(resources, "*") && riskLevel == "high" {
				riskLevel = "critical"
				score -= 3
			}

			if riskLevel == "critical" || riskLevel == "high" {
				result.HighRiskRoles = append(result.HighRiskRoles, RBACRoleEntry1962{
					Name: role.Name, Namespace: "cluster",
					Verbs: rule.Verbs, Resources: rule.Resources,
					RiskLevel: riskLevel,
				})
			}
		}
	}

	sort.Slice(result.HighRiskRoles, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.HighRiskRoles[i].RiskLevel] < riskOrder[result.HighRiskRoles[j].RiskLevel]
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d subjects, %d cluster-admins, %d high-risk roles", result.Summary.TotalSubjects, result.Summary.ClusterAdmins, len(result.HighRiskRoles)))
	if result.Summary.WildcardBindings > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d wildcard (*) bindings — replace with explicit permissions", result.Summary.WildcardBindings))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func verbSliceToStr1962(s []string) string {
	return strings.Join(s, ",")
}

// ---------------------------------------------------------------
// 3. Cluster Blueprint
// ---------------------------------------------------------------

type ClusterBlueprintResult1962 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         ClusterBlueprintSummary1962 `json:"summary"`
	Nodes           []BlueprintNodeEntry1962    `json:"nodes"`
	Namespaces      []BlueprintNSEntry1962      `json:"namespaces"`
	Addons          []BlueprintAddonEntry1962   `json:"addons"`
	Recommendations []string                    `json:"recommendations"`
}

type ClusterBlueprintSummary1962 struct {
	ClusterName      string  `json:"clusterName"`
	K8sVersion       string  `json:"kubernetesVersion"`
	ContainerRuntime string  `json:"containerRuntime"`
	TotalNodes       int     `json:"totalNodes"`
	TotalNamespaces  int     `json:"totalNamespaces"`
	TotalPods        int     `json:"totalPods"`
	TotalServices    int     `json:"totalServices"`
	TotalCPU         float64 `json:"totalCPUCores"`
	TotalMemory      float64 `json:"totalMemoryGB"`
	Provider         string  `json:"provider"`
}

type BlueprintNodeEntry1962 struct {
	Name    string  `json:"name"`
	Role    string  `json:"role"`
	Version string  `json:"version"`
	OS      string  `json:"osImage"`
	CPU     int64   `json:"cpuCores"`
	Memory  float64 `json:"memoryGB"`
}

type BlueprintNSEntry1962 struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	PodCount int    `json:"podCount"`
}

type BlueprintAddonEntry1962 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

func (s *Server) handleClusterBlueprint(w http.ResponseWriter, r *http.Request) {
	result := ClusterBlueprintResult1962{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	// Summary
	result.Summary.TotalNamespaces = len(nsList.Items)
	result.Summary.TotalPods = len(podList.Items)
	result.Summary.TotalServices = len(svcList.Items)

	// Detect provider from node labels
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if result.Summary.K8sVersion == "" {
			result.Summary.K8sVersion = node.Status.NodeInfo.KubeletVersion
		}
		if result.Summary.ContainerRuntime == "" {
			result.Summary.ContainerRuntime = node.Status.NodeInfo.ContainerRuntimeVersion
		}

		entry := BlueprintNodeEntry1962{
			Name:    node.Name,
			Version: node.Status.NodeInfo.KubeletVersion,
			OS:      node.Status.NodeInfo.OSImage,
		}

		// Node role
		role := "worker"
		for k := range node.Labels {
			if strings.Contains(k, "control-plane") || strings.Contains(k, "master") {
				role = "control-plane"
			}
		}
		entry.Role = role

		// Resources
		entry.CPU = node.Status.Capacity.Cpu().Value()
		if mem := node.Status.Capacity.Memory(); mem != nil {
			entry.Memory = float64(mem.Value()) / (1024 * 1024 * 1024)
		}
		result.Summary.TotalCPU += float64(entry.CPU)
		result.Summary.TotalMemory += entry.Memory

		result.Nodes = append(result.Nodes, entry)

		// Detect provider
		if result.Summary.Provider == "" {
			for _, providerKey := range []string{
				"cloud.google.com/gke-nodepool",
				"eks.amazonaws.com/nodegroup-name",
				"kops.k8s.io/instancegroup",
			} {
				if _, ok := node.Labels[providerKey]; ok {
					if strings.Contains(providerKey, "gke") {
						result.Summary.Provider = "GKE"
					} else if strings.Contains(providerKey, "eks") {
						result.Summary.Provider = "EKS"
					} else {
						result.Summary.Provider = "kops"
					}
					break
				}
			}
			if result.Summary.Provider == "" {
				if _, ok := node.Labels["k3s.io/hostname"]; ok {
					result.Summary.Provider = "k3s"
				}
			}
		}
	}
	result.Summary.ClusterName = "k8ops-cluster"

	// Namespaces
	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		podsPerNS[pod.Namespace]++
	}
	for _, ns := range nsList.Items {
		result.Namespaces = append(result.Namespaces, BlueprintNSEntry1962{
			Name: ns.Name, Status: string(ns.Status.Phase),
			PodCount: podsPerNS[ns.Name],
		})
	}

	// Detect addons from system namespaces
	addonNamespaces := map[string]bool{
		"kube-system": true, "k8ops-system": true, "cert-manager": true,
		"istio-system": true, "monitoring": true, "ingress-nginx": true,
	}
	for _, ns := range nsList.Items {
		if addonNamespaces[ns.Name] {
			result.Addons = append(result.Addons, BlueprintAddonEntry1962{
				Name: ns.Name, Namespace: ns.Name, Type: "system-addon",
			})
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Cluster: %s on %s, K8s %s, %d nodes", result.Summary.ClusterName, result.Summary.Provider, result.Summary.K8sVersion, result.Summary.TotalNodes))
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Workload: %d pods, %d services, %d namespaces", result.Summary.TotalPods, result.Summary.TotalServices, result.Summary.TotalNamespaces))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// suppress unused import
var _ rbacv1.Role
var _ corev1.Service
