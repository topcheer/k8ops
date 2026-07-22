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
// v19.20 — Documentation Dimension (Round 6)
// 1. Policy Catalog — comprehensive policy inventory & doc
// 2. Service Dependency Graph — inter-service dependency map
// 3. Performance Baseline Report — resource usage baseline doc
// ============================================================

// ---------------------------------------------------------------
// 1. Policy Catalog — documents all policies in the cluster
// ---------------------------------------------------------------

type PolicyCatalogResult1920 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PolicyCatalogSummary  `json:"summary"`
	NetworkPolicies []PolicyCatalogEntry  `json:"networkPolicies"`
	PodSecurity     []PolicyCatalogEntry  `json:"podSecurity"`
	RBACBindings    []RBACPolicyEntry1920 `json:"rbacBindings"`
	LimitRanges     []PolicyCatalogEntry  `json:"limitRanges"`
	Quotas          []PolicyCatalogEntry  `json:"quotas"`
	Gaps            []PolicyGapEntry1920  `json:"gaps"`
	Recommendations []string              `json:"recommendations"`
}

type PolicyCatalogSummary struct {
	TotalNetworkPolicies    int `json:"totalNetworkPolicies"`
	NamespacesWithNetPol    int `json:"namespacesWithNetPol"`
	NamespacesWithoutNetPol int `json:"namespacesWithoutNetPol"`
	NamespacesWithPSA       int `json:"namespacesWithPSA"`
	NamespacesWithoutPSA    int `json:"namespacesWithoutPSA"`
	TotalRBACBindings       int `json:"totalRBACBindings"`
	ClusterAdminBindings    int `json:"clusterAdminBindings"`
	NamespacesWithLimits    int `json:"namespacesWithLimits"`
	NamespacesWithQuota     int `json:"namespacesWithQuota"`
	TotalGaps               int `json:"totalGaps"`
}

type PolicyCatalogEntry struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Kind      string            `json:"kind"`
	Details   string            `json:"details"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type RBACPolicyEntry1920 struct {
	Subject        string `json:"subject"`
	SubjectKind    string `json:"subjectKind"`
	Namespace      string `json:"namespace"`
	Role           string `json:"role"`
	RoleKind       string `json:"roleKind"`
	IsClusterAdmin bool   `json:"isClusterAdmin"`
}

type PolicyGapEntry1920 struct {
	Namespace string `json:"namespace"`
	GapType   string `json:"gapType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handlePolicyCatalog(w http.ResponseWriter, r *http.Request) {
	result := PolicyCatalogResult1920{
		ScannedAt: time.Now(),
	}
	score := 100

	// Collect namespaces
	nsList, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	allNS := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		allNS = append(allNS, ns.Name)
	}

	// Network Policies
	netPolNS := make(map[string]bool)
	netPols, err := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, np := range netPols.Items {
			if isSystemNamespace(np.Namespace) {
				continue
			}
			netPolNS[np.Namespace] = true
			detail := fmt.Sprintf("Ingress: %d rules, Egress: %d rules", len(np.Spec.Ingress), len(np.Spec.Egress))
			if len(np.Spec.PodSelector.MatchLabels) > 0 {
				detail += fmt.Sprintf(", Selector: %v", np.Spec.PodSelector.MatchLabels)
			}
			result.NetworkPolicies = append(result.NetworkPolicies, PolicyCatalogEntry{
				Name:      np.Name,
				Namespace: np.Namespace,
				Kind:      "NetworkPolicy",
				Details:   detail,
			})
		}
	}
	result.Summary.TotalNetworkPolicies = len(result.NetworkPolicies)
	result.Summary.NamespacesWithNetPol = len(netPolNS)
	result.Summary.NamespacesWithoutNetPol = len(allNS) - len(netPolNS)
	if result.Summary.NamespacesWithoutNetPol > 0 {
		score -= result.Summary.NamespacesWithoutNetPol * 2
	}

	// Pod Security Standards (PSA labels on namespaces)
	psaNS := 0
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		enforceLevel := ns.Labels["pod-security.kubernetes.io/enforce"]
		if enforceLevel != "" {
			psaNS++
			result.PodSecurity = append(result.PodSecurity, PolicyCatalogEntry{
				Name:      ns.Name,
				Namespace: ns.Name,
				Kind:      "PodSecurityAdmission",
				Details:   fmt.Sprintf("Enforce: %s, Audit: %s", enforceLevel, ns.Labels["pod-security.kubernetes.io/audit"]),
			})
		}
	}
	result.Summary.NamespacesWithPSA = psaNS
	result.Summary.NamespacesWithoutPSA = len(allNS) - psaNS
	if result.Summary.NamespacesWithoutPSA > 0 {
		score -= result.Summary.NamespacesWithoutPSA * 3
	}

	// RBAC Bindings (ClusterRoleBinding + RoleBinding)
	crbList, err := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, crb := range crbList.Items {
			isAdmin := strings.Contains(crb.RoleRef.Name, "cluster-admin") || strings.Contains(crb.RoleRef.Name, "admin")
			for _, sub := range crb.Subjects {
				result.RBACBindings = append(result.RBACBindings, RBACPolicyEntry1920{
					Subject:        sub.Name,
					SubjectKind:    string(sub.Kind),
					Namespace:      sub.Namespace,
					Role:           crb.RoleRef.Name,
					RoleKind:       crb.RoleRef.Kind,
					IsClusterAdmin: isAdmin,
				})
				if isAdmin {
					result.Summary.ClusterAdminBindings++
				}
			}
		}
	}
	result.Summary.TotalRBACBindings = len(result.RBACBindings)
	if result.Summary.ClusterAdminBindings > 5 {
		score -= (result.Summary.ClusterAdminBindings - 5) * 2
	}

	// LimitRanges
	lrNS := make(map[string]bool)
	lrList, err := s.clientset.CoreV1().LimitRanges("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, lr := range lrList.Items {
			if isSystemNamespace(lr.Namespace) {
				continue
			}
			lrNS[lr.Namespace] = true
			details := make([]string, 0)
			for _, item := range lr.Spec.Limits {
				details = append(details, fmt.Sprintf("%s/%s", item.Type, item.DefaultRequest.Cpu().String()))
			}
			result.LimitRanges = append(result.LimitRanges, PolicyCatalogEntry{
				Name:      lr.Name,
				Namespace: lr.Namespace,
				Kind:      "LimitRange",
				Details:   strings.Join(details, ", "),
			})
		}
	}
	result.Summary.NamespacesWithLimits = len(lrNS)

	// Resource Quotas
	rqNS := make(map[string]bool)
	rqList, err := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, rq := range rqList.Items {
			if isSystemNamespace(rq.Namespace) {
				continue
			}
			rqNS[rq.Namespace] = true
			details := make([]string, 0)
			for k, v := range rq.Spec.Hard {
				details = append(details, fmt.Sprintf("%s=%s", k, v.String()))
			}
			result.Quotas = append(result.Quotas, PolicyCatalogEntry{
				Name:      rq.Name,
				Namespace: rq.Namespace,
				Kind:      "ResourceQuota",
				Details:   strings.Join(details, ", "),
			})
		}
	}
	result.Summary.NamespacesWithQuota = len(rqNS)

	// Gaps
	for _, ns := range allNS {
		if !netPolNS[ns] {
			result.Gaps = append(result.Gaps, PolicyGapEntry1920{
				Namespace: ns,
				GapType:   "NetworkPolicy",
				Severity:  "high",
				Detail:    "No NetworkPolicy — all ingress/egress is unrestricted",
			})
		}
		if !psaNS_exists(ns, nsList) {
			result.Gaps = append(result.Gaps, PolicyGapEntry1920{
				Namespace: ns,
				GapType:   "PodSecurityAdmission",
				Severity:  "medium",
				Detail:    "No PSA enforce label — unrestricted pod security",
			})
		}
		if !lrNS[ns] {
			result.Gaps = append(result.Gaps, PolicyGapEntry1920{
				Namespace: ns,
				GapType:   "LimitRange",
				Severity:  "low",
				Detail:    "No LimitRange — pods can be created without resource limits",
			})
		}
		if !rqNS[ns] {
			result.Gaps = append(result.Gaps, PolicyGapEntry1920{
				Namespace: ns,
				GapType:   "ResourceQuota",
				Severity:  "medium",
				Detail:    "No ResourceQuota — unbounded resource consumption",
			})
		}
	}
	result.Summary.TotalGaps = len(result.Gaps)

	// Score
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	// Recs
	if result.Summary.NamespacesWithoutNetPol > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Add default deny NetworkPolicy to %d unprotected namespaces", result.Summary.NamespacesWithoutNetPol))
	}
	if result.Summary.NamespacesWithoutPSA > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Enable PSA enforce=baseline on %d namespaces", result.Summary.NamespacesWithoutPSA))
	}
	if result.Summary.ClusterAdminBindings > 5 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Review %d cluster-admin bindings — least privilege recommended", result.Summary.ClusterAdminBindings))
	}
	if result.Summary.NamespacesWithLimits == 0 {
		result.Recommendations = append(result.Recommendations, "Create LimitRange in application namespaces to enforce resource defaults")
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// helper for PSA check
func psaNS_exists(name string, nsList *corev1.NamespaceList) bool {
	for _, ns := range nsList.Items {
		if ns.Name == name && ns.Labels["pod-security.kubernetes.io/enforce"] != "" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 2. Service Dependency Graph — maps inter-service dependencies
// ---------------------------------------------------------------

type ServiceDepGraphResult1920 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ServiceDepSummary1920 `json:"summary"`
	Services        []ServiceDepEntry1920 `json:"services"`
	Dependencies    []ServiceDepLink1920  `json:"dependencies"`
	Hubs            []ServiceDepHub1920   `json:"hubs"`
	Orphans         []string              `json:"orphans"`
	Recommendations []string              `json:"recommendations"`
}

type ServiceDepSummary1920 struct {
	TotalServices int `json:"totalServices"`
	TotalDeps     int `json:"totalDependencies"`
	CrossNSDeps   int `json:"crossNamespaceDeps"`
	MaxFanOut     int `json:"maxFanOut"`
	MaxFanIn      int `json:"maxFanIn"`
	OrphanCount   int `json:"orphanCount"`
	TotalSvcDNS   int `json:"totalSvcDNS"`
}

type ServiceDepEntry1920 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	FanOut    int    `json:"fanOut"`
	FanIn     int    `json:"fanIn"`
}

type ServiceDepLink1920 struct {
	Source   string `json:"source"`
	SourceNS string `json:"sourceNS"`
	Target   string `json:"target"`
	TargetNS string `json:"targetNS"`
	LinkType string `json:"linkType"`
}

type ServiceDepHub1920 struct {
	Service     string `json:"service"`
	Namespace   string `json:"namespace"`
	Connections int    `json:"connections"`
}

func (s *Server) handleServiceDepGraph(w http.ResponseWriter, r *http.Request) {
	result := ServiceDepGraphResult1920{
		ScannedAt: time.Now(),
	}
	score := 100

	// List services
	svcList, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	result.Summary.TotalServices = len(svcList.Items)

	// Build service registry: "ns/svc-name" -> exists
	svcSet := make(map[string]bool)
	svcByNS := make(map[string][]string)
	for _, svc := range svcList.Items {
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		svcSet[key] = true
		svcByNS[svc.Namespace] = append(svcByNS[svc.Namespace], svc.Name)
	}

	// Scan pods for env vars referencing services (DNS names)
	type depKey struct{ src, tgt string }
	depMap := make(map[depKey]string) // link type
	fanOutMap := make(map[string]int)
	fanInMap := make(map[string]int)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Labels["app"])
			if pod.Labels["app"] == "" {
				continue
			}
			for _, container := range pod.Spec.Containers {
				for _, env := range container.Env {
					val := env.Value
					if val == "" && env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
						val = env.ValueFrom.ConfigMapKeyRef.Name
					}
					if val == "" {
						continue
					}
					// Check if value references a known service
					for svcKey := range svcSet {
						parts := strings.Split(svcKey, "/")
						if len(parts) != 2 {
							continue
						}
						svcNS, svcName := parts[0], parts[1]
						// Match patterns: "svc-name", "svc-name.namespace", "svc-name.namespace.svc"
						if strings.Contains(val, svcName) && (strings.Contains(val, svcNS) || pod.Namespace == svcNS) {
							if svcKey == podKey {
								continue // self-reference
							}
							dk := depKey{src: podKey, tgt: svcKey}
							if _, exists := depMap[dk]; !exists {
								depMap[dk] = "env-var-ref"
								fanOutMap[podKey]++
								fanInMap[svcKey]++
								if pod.Namespace != svcNS {
									result.Summary.CrossNSDeps++
								}
							}
						}
					}
				}
			}
		}
	}

	// Network policies reveal allowed traffic (count namespace selectors)
	result.Summary.TotalSvcDNS = 0

	// Build entries
	for dk, lt := range depMap {
		srcParts := strings.Split(dk.src, "/")
		tgtParts := strings.Split(dk.tgt, "/")
		result.Dependencies = append(result.Dependencies, ServiceDepLink1920{
			Source:   dk.src,
			SourceNS: getPart(srcParts, 0),
			Target:   dk.tgt,
			TargetNS: getPart(tgtParts, 0),
			LinkType: lt,
		})
	}
	result.Summary.TotalDeps = len(result.Dependencies)

	// Fan out/in
	for key, count := range fanOutMap {
		if count > result.Summary.MaxFanOut {
			result.Summary.MaxFanOut = count
		}
		parts := strings.Split(key, "/")
		result.Services = append(result.Services, ServiceDepEntry1920{
			Name:      getPart(parts, 1),
			Namespace: getPart(parts, 0),
			FanOut:    count,
		})
	}
	for key, count := range fanInMap {
		if count > result.Summary.MaxFanIn {
			result.Summary.MaxFanIn = count
		}
		// Update fanIn on existing or create new
		found := false
		for i := range result.Services {
			if fmt.Sprintf("%s/%s", result.Services[i].Namespace, result.Services[i].Name) == key {
				result.Services[i].FanIn = count
				found = true
				break
			}
		}
		if !found {
			parts := strings.Split(key, "/")
			result.Services = append(result.Services, ServiceDepEntry1920{
				Name:      getPart(parts, 1),
				Namespace: getPart(parts, 0),
				FanIn:     count,
			})
		}
	}

	// Hubs (high fan-in services)
	for _, svc := range result.Services {
		if svc.FanIn >= 3 {
			result.Hubs = append(result.Hubs, ServiceDepHub1920{
				Service:     svc.Name,
				Namespace:   svc.Namespace,
				Connections: svc.FanIn,
			})
		}
	}

	// Orphans (services with no incoming deps)
	for _, svc := range svcList.Items {
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		if fanInMap[key] == 0 && svc.Spec.Type != corev1.ServiceTypeExternalName {
			result.Orphans = append(result.Orphans, key)
		}
	}
	result.Summary.OrphanCount = len(result.Orphans)

	// Score
	if result.Summary.CrossNSDeps > 10 {
		score -= 5
	}
	if result.Summary.OrphanCount > len(svcList.Items)/3 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OrphanCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Review %d orphan services with no detected dependencies", result.Summary.OrphanCount))
	}
	if result.Summary.MaxFanOut > 10 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Service with %d fan-out dependencies detected — consider circuit breakers", result.Summary.MaxFanOut))
	}
	if len(result.Hubs) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d hub services with high fan-in — ensure HA for these critical services", len(result.Hubs)))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Performance Baseline Report — captures resource usage baselines
// ---------------------------------------------------------------

type PerfBaselineResult1920 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PerfBaselineSummary1920 `json:"summary"`
	Workloads       []PerfBaselineEntry1920 `json:"workloads"`
	Nodes           []PerfNodeBaseline1920  `json:"nodes"`
	Thresholds      []PerfThreshold1920     `json:"thresholds"`
	Anomalies       []PerfAnomaly1920       `json:"anomalies"`
	Recommendations []string                `json:"recommendations"`
}

type PerfBaselineSummary1920 struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	AvgCPURequest    float64 `json:"avgCPURequestCores"`
	AvgMemRequestMB  int     `json:"avgMemRequestMB"`
	AvgCPULimit      float64 `json:"avgCPULimitCores"`
	AvgMemLimitMB    int     `json:"avgMemLimitMB"`
	TotalNodes       int     `json:"totalNodes"`
	TotalAnomalies   int     `json:"totalAnomalies"`
	HighCPUWorkloads int     `json:"highCPUWorkloads"`
	HighMemWorkloads int     `json:"highMemWorkloads"`
}

type PerfBaselineEntry1920 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Kind       string  `json:"kind"`
	CPURequest string  `json:"cpuRequest"`
	MemRequest string  `json:"memRequest"`
	CPULimit   string  `json:"cpuLimit"`
	MemLimit   string  `json:"memLimit"`
	CPUTotal   float64 `json:"cpuTotalCores"`
	MemTotalMB int     `json:"memTotalMB"`
	Replicas   int     `json:"replicas"`
	IsHighCPU  bool    `json:"isHighCPU"`
	IsHighMem  bool    `json:"isHighMem"`
}

type PerfNodeBaseline1920 struct {
	Name        string `json:"name"`
	CPUCapacity string `json:"cpuCapacity"`
	MemCapacity string `json:"memCapacity"`
	CPUAlloc    string `json:"cpuAllocated"`
	MemAlloc    string `json:"memAllocated"`
	AllocPct    int    `json:"allocPct"`
}

type PerfThreshold1920 struct {
	Metric     string `json:"metric"`
	Threshold  string `json:"threshold"`
	ActionType string `json:"actionType"`
}

type PerfAnomaly1920 struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
}

func (s *Server) handlePerfBaseline(w http.ResponseWriter, r *http.Request) {
	result := PerfBaselineResult1920{
		ScannedAt: time.Now(),
	}
	score := 100

	// Pod metrics for baselines
	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Aggregate by workload (app label)
	type wlKey struct{ ns, name, kind string }
	wlData := make(map[wlKey]*PerfBaselineEntry1920)
	var totalCPU float64
	var totalMemMB int
	var highCPU, highMem int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}
		kind := "Pod"
		if owner := getPodOwnerKind(pod); owner != "" {
			kind = owner
		}
		key := wlKey{ns: pod.Namespace, name: appName, kind: kind}
		wl, exists := wlData[key]
		if !exists {
			wl = &PerfBaselineEntry1920{
				Name:      appName,
				Namespace: pod.Namespace,
				Kind:      kind,
			}
			wlData[key] = wl
		}
		wl.Replicas++
		for _, container := range pod.Spec.Containers {
			cpuReq := container.Resources.Requests.Cpu()
			memReq := container.Resources.Requests.Memory()
			cpuLim := container.Resources.Limits.Cpu()
			memLim := container.Resources.Limits.Memory()
			if !cpuReq.IsZero() {
				wl.CPUTotal += cpuReq.AsApproximateFloat64()
			}
			if !memReq.IsZero() {
				wl.MemTotalMB += int(memReq.Value() / (1024 * 1024))
			}
			if wl.CPURequest == "" && !cpuReq.IsZero() {
				wl.CPURequest = cpuReq.String()
			}
			if wl.MemRequest == "" && !memReq.IsZero() {
				wl.MemRequest = memReq.String()
			}
			if wl.CPULimit == "" && !cpuLim.IsZero() {
				wl.CPULimit = cpuLim.String()
			}
			if wl.MemLimit == "" && !memLim.IsZero() {
				wl.MemLimit = memLim.String()
			}
		}
	}

	for _, wl := range wlData {
		totalCPU += wl.CPUTotal
		totalMemMB += wl.MemTotalMB
		if wl.CPUTotal > 2.0 {
			wl.IsHighCPU = true
			highCPU++
			result.Anomalies = append(result.Anomalies, PerfAnomaly1920{
				Workload:  wl.Name,
				Namespace: wl.Namespace,
				Type:      "high-cpu-request",
				Detail:    fmt.Sprintf("CPU request %.2f cores exceeds 2.0 baseline", wl.CPUTotal),
				Severity:  "warning",
			})
		}
		if wl.MemTotalMB > 4096 {
			wl.IsHighMem = true
			highMem++
			result.Anomalies = append(result.Anomalies, PerfAnomaly1920{
				Workload:  wl.Name,
				Namespace: wl.Namespace,
				Type:      "high-mem-request",
				Detail:    fmt.Sprintf("Memory request %dMB exceeds 4GB baseline", wl.MemTotalMB),
				Severity:  "warning",
			})
		}
		if wl.CPURequest == "" {
			result.Anomalies = append(result.Anomalies, PerfAnomaly1920{
				Workload:  wl.Name,
				Namespace: wl.Namespace,
				Type:      "missing-cpu-request",
				Detail:    "No CPU request set — unpredictable scheduling",
				Severity:  "critical",
			})
			score -= 5
		}
		if wl.MemRequest == "" {
			result.Anomalies = append(result.Anomalies, PerfAnomaly1920{
				Workload:  wl.Name,
				Namespace: wl.Namespace,
				Type:      "missing-mem-request",
				Detail:    "No memory request set — OOMKill risk",
				Severity:  "critical",
			})
			score -= 5
		}
		result.Workloads = append(result.Workloads, *wl)
	}
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].CPUTotal > result.Workloads[j].CPUTotal
	})

	result.Summary.TotalWorkloads = len(result.Workloads)
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgCPURequest = totalCPU / float64(result.Summary.TotalWorkloads)
		result.Summary.AvgMemRequestMB = totalMemMB / result.Summary.TotalWorkloads
	}
	result.Summary.HighCPUWorkloads = highCPU
	result.Summary.HighMemWorkloads = highMem
	result.Summary.TotalAnomalies = len(result.Anomalies)

	// Nodes
	nodeList, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodeList.Items {
			cpuCap := node.Status.Allocatable.Cpu()
			memCap := node.Status.Allocatable.Memory()
			result.Nodes = append(result.Nodes, PerfNodeBaseline1920{
				Name:        node.Name,
				CPUCapacity: cpuCap.String(),
				MemCapacity: fmt.Sprintf("%dGi", memCap.Value()/(1024*1024*1024)),
			})
		}
	}
	result.Summary.TotalNodes = len(result.Nodes)

	// Thresholds (recommended baselines)
	result.Thresholds = []PerfThreshold1920{
		{Metric: "cpu-request", Threshold: "0.5-2 cores/container", ActionType: "alert-if-exceeded"},
		{Metric: "memory-request", Threshold: "256Mi-4Gi/container", ActionType: "alert-if-exceeded"},
		{Metric: "cpu-limit", Threshold: "2x request", ActionType: "review-if-asymmetric"},
		{Metric: "memory-limit", Threshold: "1.5x request", ActionType: "review-if-missing"},
		{Metric: "node-alloc-pct", Threshold: "70-85%", ActionType: "scale-if-exceeded"},
	}

	// Score
	if result.Summary.HighCPUWorkloads > 5 {
		score -= 5
	}
	if result.Summary.HighMemWorkloads > 5 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if highCPU > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads with high CPU requests (>2 cores) — review for optimization", highCPU))
	}
	if highMem > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads with high memory requests (>4Gi) — verify actual usage", highMem))
	}
	missingReqs := 0
	for _, a := range result.Anomalies {
		if a.Type == "missing-cpu-request" || a.Type == "missing-mem-request" {
			missingReqs++
		}
	}
	if missingReqs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads missing resource requests — add for reliable scheduling", missingReqs))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// Helper: get part of split string safely
func getPart(parts []string, idx int) string {
	if idx < len(parts) {
		return parts[idx]
	}
	return ""
}

// Helper: determine pod owner kind
func getPodOwnerKind(pod corev1.Pod) string {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind != "" {
			return ownerRef.Kind
		}
	}
	return ""
}
