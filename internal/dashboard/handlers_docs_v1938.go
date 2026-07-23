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
// v19.38 — Documentation Dimension (Round 9)
// 1. Workload Ownership Registry — owner/team/escalation metadata
// 2. API Resource Inventory — CRD & native resource usage map
// 3. Cluster Capacity Report — resource capacity & allocation doc
// ============================================================

// ---------------------------------------------------------------
// 1. Workload Ownership Registry
// ---------------------------------------------------------------

type OwnershipRegistryResult1938 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         OwnershipRegistrySummary1938 `json:"summary"`
	Workloads       []OwnershipEntry1938         `json:"workloads"`
	MissingOwner    []OwnershipMissingEntry1938  `json:"missingOwner"`
	ByTeam          []TeamStat1938               `json:"byTeam"`
	Recommendations []string                     `json:"recommendations"`
}

type OwnershipRegistrySummary1938 struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	WithOwner      int     `json:"withOwner"`
	WithoutOwner   int     `json:"withoutOwner"`
	WithTeam       int     `json:"withTeam"`
	WithoutTeam    int     `json:"withoutTeam"`
	WithEscalation int     `json:"withEscalation"`
	UniqueTeams    int     `json:"uniqueTeams"`
	ComplianceRate float64 `json:"complianceRate"`
}

type OwnershipEntry1938 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Team       string `json:"team"`
	Escalation string `json:"escalation"`
	Contact    string `json:"contact"`
}

type OwnershipMissingEntry1938 struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	Missing   []string `json:"missing"`
	Severity  string   `json:"severity"`
}

type TeamStat1938 struct {
	Team      string `json:"team"`
	Workloads int    `json:"workloadCount"`
}

func (s *Server) handleOwnershipRegistryV2(w http.ResponseWriter, r *http.Request) {
	result := OwnershipRegistryResult1938{ScannedAt: time.Now()}
	score := 100
	teamMap := make(map[string]int)

	checkOwnership := func(name, ns, kind string, annots, labels map[string]string) {
		if isSystemNamespace(ns) {
			return
		}
		result.Summary.TotalWorkloads++

		owner := getMetaValue(annots, labels, "owner")
		team := getMetaValue(annots, labels, "team")
		escalation := getMetaValue(annots, labels, "escalation")
		contact := getMetaValue(annots, labels, "contact")

		entry := OwnershipEntry1938{
			Name: name, Namespace: ns, Kind: kind,
			Owner: owner, Team: team, Escalation: escalation, Contact: contact,
		}
		result.Workloads = append(result.Workloads, entry)

		missing := []string{}
		if owner == "" {
			missing = append(missing, "owner")
			result.Summary.WithoutOwner++
		} else {
			result.Summary.WithOwner++
		}
		if team == "" {
			missing = append(missing, "team")
			result.Summary.WithoutTeam++
		} else {
			result.Summary.WithTeam++
			teamMap[team]++
		}
		if escalation == "" {
			result.Summary.WithoutTeam++
		} else {
			result.Summary.WithEscalation++
		}

		if len(missing) > 0 {
			severity := "low"
			if len(missing) >= 2 {
				severity = "medium"
			}
			result.MissingOwner = append(result.MissingOwner, OwnershipMissingEntry1938{
				Name: name, Namespace: ns, Kind: kind,
				Missing: missing, Severity: severity,
			})
			if len(missing) >= 2 {
				score -= 2
			} else {
				score -= 1
			}
		}
	}

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, d := range depList.Items {
		checkOwnership(d.Name, d.Namespace, "Deployment", d.Annotations, d.Labels)
	}

	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, st := range stsList.Items {
		checkOwnership(st.Name, st.Namespace, "StatefulSet", st.Annotations, st.Labels)
	}

	result.Summary.UniqueTeams = len(teamMap)
	for team, count := range teamMap {
		result.ByTeam = append(result.ByTeam, TeamStat1938{Team: team, Workloads: count})
	}

	if result.Summary.TotalWorkloads > 0 {
		result.Summary.ComplianceRate = float64(result.Summary.WithOwner) * 100 / float64(result.Summary.TotalWorkloads)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutOwner > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads missing owner metadata — add owner annotation", result.Summary.WithoutOwner))
	}
	if result.Summary.WithoutTeam > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads missing team label — add team for escalation routing", result.Summary.WithoutTeam))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func getMetaValue(annots, labels map[string]string, key string) string {
	// Check common annotation patterns
	keys := []string{
		key,
		"app.kubernetes.io/" + key,
		"owner." + key,
		"meta.helm.sh/" + key,
	}
	for _, k := range keys {
		if v, ok := annots[k]; ok {
			return v
		}
		if v, ok := labels[k]; ok {
			return v
		}
	}
	// Fuzzy match
	for k, v := range annots {
		if strings.Contains(strings.ToLower(k), key) {
			return v
		}
	}
	for k, v := range labels {
		if strings.Contains(strings.ToLower(k), key) {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------
// 2. API Resource Inventory
// ---------------------------------------------------------------

type APIInventoryResult1938 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         APIInventorySummary1938  `json:"summary"`
	Resources       []APIResourceEntry1938   `json:"resources"`
	Deprecated      []APIDeprecatedEntry1938 `json:"deprecated"`
	ByGroup         []APIGroupStat1938       `json:"byGroup"`
	Recommendations []string                 `json:"recommendations"`
}

type APIInventorySummary1938 struct {
	TotalResources  int `json:"totalResources"`
	CoreResources   int `json:"coreResources"`
	CRDResources    int `json:"crdResources"`
	Namespaced      int `json:"namespaced"`
	ClusterScoped   int `json:"clusterScoped"`
	VerbsGet        int `json:"verbsWithGet"`
	VerbsList       int `json:"verbsWithList"`
	DeprecatedCount int `json:"deprecatedCount"`
}

type APIResourceEntry1938 struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Group      string `json:"group"`
	Version    string `json:"version"`
	Namespaced bool   `json:"namespaced"`
}

type APIDeprecatedEntry1938 struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

type APIGroupStat1938 struct {
	Group string `json:"group"`
	Count int    `json:"resourceCount"`
}

func (s *Server) handleAPIInventory(w http.ResponseWriter, r *http.Request) {
	result := APIInventoryResult1938{ScannedAt: time.Now()}
	score := 100
	groupStats := make(map[string]int)

	_, apiResList, err := s.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		writeJSON(w, result)
		return
	}

	deprecatedVersions := map[string]bool{
		"extensions/v1beta1": true, "apps/v1beta1": true, "apps/v1beta2": true,
		"policy/v1beta1": true, "networking.k8s.io/v1beta1": true,
		"storage.k8s.io/v1beta1": true, "batch/v1beta1": true,
	}

	for _, apiGroup := range apiResList {
		gv := apiGroup.GroupVersion
		isCore := !strings.Contains(gv, "/")
		groupName := "core"
		if parts := strings.SplitN(gv, "/", 2); len(parts) == 2 {
			groupName = parts[0]
		}

		for _, res := range apiGroup.APIResources {
			// Skip subresources
			if strings.Contains(res.Name, "/") {
				continue
			}

			result.Summary.TotalResources++
			if isCore {
				result.Summary.CoreResources++
			} else {
				result.Summary.CRDResources++
			}
			if res.Namespaced {
				result.Summary.Namespaced++
			} else {
				result.Summary.ClusterScoped++
			}
			groupStats[groupName]++

			// Check deprecated
			isDeprecated := deprecatedVersions[gv]
			if isDeprecated {
				result.Summary.DeprecatedCount++
				result.Deprecated = append(result.Deprecated, APIDeprecatedEntry1938{
					Name: res.Name, Group: groupName, Version: gv,
					Reason: fmt.Sprintf("API version %s is deprecated", gv),
				})
				score -= 1
			}

			entry := APIResourceEntry1938{
				Name: res.Name, Kind: res.Kind,
				Group: groupName, Version: gv,
				Namespaced: res.Namespaced,
			}
			for _, v := range res.Verbs {
				if v == "get" {
					result.Summary.VerbsGet++
				}
				if v == "list" {
					result.Summary.VerbsList++
				}
			}
			// Limit output to reasonable size
			if len(result.Resources) < 200 {
				result.Resources = append(result.Resources, entry)
			}
		}
	}

	for g, c := range groupStats {
		result.ByGroup = append(result.ByGroup, APIGroupStat1938{Group: g, Count: c})
	}
	sort.Slice(result.ByGroup, func(i, j int) bool { return result.ByGroup[i].Count > result.ByGroup[j].Count })

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DeprecatedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources on deprecated API versions — migrate to v1", result.Summary.DeprecatedCount))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total API resources across %d groups", result.Summary.TotalResources, len(groupStats)))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Capacity Report
// ---------------------------------------------------------------

type CapacityReportResult1938 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         CapacityReportSummary1938 `json:"summary"`
	Nodes           []CapacityNodeEntry1938   `json:"nodes"`
	Allocations     []CapacityAllocEntry1938  `json:"allocations"`
	Recommendations []string                  `json:"recommendations"`
}

type CapacityReportSummary1938 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalCPUCore   float64 `json:"totalCPUCores"`
	TotalMemGB     float64 `json:"totalMemoryGB"`
	AllocatedCPU   float64 `json:"allocatedCPUCores"`
	AllocatedMem   float64 `json:"allocatedMemoryGB"`
	CPUUtilization float64 `json:"cpuUtilizationPct"`
	MemUtilization float64 `json:"memUtilizationPct"`
	PodCapacity    int     `json:"podCapacity"`
	PodsRunning    int     `json:"podsRunning"`
	PodUtilization float64 `json:"podUtilizationPct"`
}

type CapacityNodeEntry1938 struct {
	Name         string  `json:"name"`
	CPUCapacity  string  `json:"cpuCapacity"`
	MemCapacity  string  `json:"memCapacity"`
	CPUAllocated float64 `json:"cpuAllocatedCores"`
	MemAllocated float64 `json:"memAllocatedGB"`
	CPUPct       float64 `json:"cpuAllocPct"`
	MemPct       float64 `json:"memAllocPct"`
	PodCount     int     `json:"podCount"`
	PodCapacity  int     `json:"podCapacity"`
}

type CapacityAllocEntry1938 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequested"`
	MemReq    float64 `json:"memRequestedGB"`
	PodCount  int     `json:"podCount"`
}

func (s *Server) handleCapacityReport(w http.ResponseWriter, r *http.Request) {
	result := CapacityReportResult1938{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Node capacity
	var totalCPU, totalMem float64
	var totalPodCap int
	for _, node := range nodeList.Items {
		cpu := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		podCap := 110
		if pcs := node.Status.Allocatable.Pods(); pcs != nil {
			podCap = int(pcs.Value())
		}
		totalCPU += cpu
		totalMem += memGB
		totalPodCap += podCap
		result.Summary.TotalNodes++
		result.Summary.PodCapacity += podCap

		result.Nodes = append(result.Nodes, CapacityNodeEntry1938{
			Name:        node.Name,
			CPUCapacity: node.Status.Allocatable.Cpu().String(),
			MemCapacity: fmt.Sprintf("%.1fGB", memGB),
			PodCapacity: podCap,
		})
	}

	// Pod allocations by namespace
	nsAlloc := make(map[string]*CapacityAllocEntry1938)
	var allocCPU, allocMem float64
	var runningPods int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		runningPods++

		// Node assignment
		for i := range result.Nodes {
			if result.Nodes[i].Name == pod.Spec.NodeName {
				result.Nodes[i].PodCount++
			}
		}

		podCPU := 0.0
		podMem := 0.0
		for _, c := range pod.Spec.Containers {
			podCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			podMem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
		allocCPU += podCPU
		allocMem += podMem

		ns := pod.Namespace
		if isSystemNamespace(ns) {
			continue
		}
		if nsAlloc[ns] == nil {
			nsAlloc[ns] = &CapacityAllocEntry1938{Namespace: ns}
		}
		nsAlloc[ns].CPUReq += podCPU
		nsAlloc[ns].MemReq += podMem
		nsAlloc[ns].PodCount++
	}

	for i := range result.Nodes {
		if totalCPU > 0 {
			result.Nodes[i].CPUPct = result.Nodes[i].CPUAllocated / totalCPU * 100
		}
	}

	for _, alloc := range nsAlloc {
		result.Allocations = append(result.Allocations, *alloc)
	}
	sort.Slice(result.Allocations, func(i, j int) bool { return result.Allocations[i].CPUReq > result.Allocations[j].CPUReq })

	result.Summary.TotalCPUCore = totalCPU
	result.Summary.TotalMemGB = totalMem
	result.Summary.AllocatedCPU = allocCPU
	result.Summary.AllocatedMem = allocMem
	result.Summary.PodsRunning = runningPods

	if totalCPU > 0 {
		result.Summary.CPUUtilization = allocCPU / totalCPU * 100
	}
	if totalMem > 0 {
		result.Summary.MemUtilization = allocMem / totalMem * 100
	}
	if totalPodCap > 0 {
		result.Summary.PodUtilization = float64(runningPods) / float64(totalPodCap) * 100
	}

	// Score based on utilization
	if result.Summary.CPUUtilization > 80 {
		score -= 10
	}
	if result.Summary.MemUtilization > 80 {
		score -= 10
	}
	if result.Summary.PodUtilization > 80 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.CPUUtilization > 70 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("CPU at %.0f%% — add nodes or optimize workloads", result.Summary.CPUUtilization))
	}
	if result.Summary.PodUtilization > 70 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Pods at %.0f%% of capacity — consider adding nodes", result.Summary.PodUtilization))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Total: %.1f CPU cores, %.1f GB memory across %d nodes", totalCPU, totalMem, result.Summary.TotalNodes))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
