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
// v18.98 — Product Dimension
// 1. Storage Class Audit
// 2. Workload Interdependency Map
// 3. DNS Resolution Health
// ============================================================

// ---------------------------------------------------------------
// 1. Storage Class Audit — storage class usage & tier analysis
// ---------------------------------------------------------------

type StorageClassAuditResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         StorageClassSummary `json:"summary"`
	Classes         []StorageClassEntry `json:"classes"`
	PVCsByClass     []PVCByClassEntry   `json:"pvcsByClass"`
	Misconfigured   []PVCByClassEntry   `json:"misconfigured"`
	Recommendations []string            `json:"recommendations"`
}

type StorageClassSummary struct {
	TotalPVCs       int `json:"totalPVCs"`
	TotalStorageGB  int `json:"totalStorageGB"`
	StorageClasses  int `json:"storageClasses"`
	DefaultClass    int `json:"defaultClass"`
	WithProvisioner int `json:"withProvisioner"`
	BoundPVCs       int `json:"boundPVCs"`
	PendingPVCs     int `json:"pendingPVCs"`
	NoStorageClass  int `json:"noStorageClass"`
}

type StorageClassEntry struct {
	Name           string `json:"name"`
	IsDefault      bool   `json:"isDefault"`
	Provisioner    string `json:"provisioner"`
	ReclaimPolicy  string `json:"reclaimPolicy"`
	VolumeBinding  string `json:"volumeBindingMode"`
	AllowExpansion bool   `json:"allowExpansion"`
	PVCCount       int    `json:"pvcCount"`
	TotalGB        int    `json:"totalGB"`
}

type PVCByClassEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	StorageClass string `json:"storageClass"`
	SizeGB       int    `json:"sizeGB"`
	Phase        string `json:"phase"`
	AccessMode   string `json:"accessMode"`
	RiskLevel    string `json:"riskLevel"`
	Issue        string `json:"issue,omitempty"`
}

func (s *Server) handleStorageClassAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := StorageClassAuditResult{ScannedAt: time.Now()}

	// Get StorageClasses
	scList, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	scInfo := map[string]*StorageClassEntry{}
	for _, sc := range scList.Items {
		entry := &StorageClassEntry{
			Name:           sc.Name,
			Provisioner:    sc.Provisioner,
			ReclaimPolicy:  string(*sc.ReclaimPolicy),
			AllowExpansion: sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion,
		}
		if sc.VolumeBindingMode != nil {
			entry.VolumeBinding = string(*sc.VolumeBindingMode)
		}
		// Check annotations for default
		if sc.Annotations != nil {
			if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				entry.IsDefault = true
				result.Summary.DefaultClass++
			}
		}
		scInfo[sc.Name] = entry
		result.Summary.StorageClasses++
		if sc.Provisioner != "" {
			result.Summary.WithProvisioner++
		}
	}

	// Analyze PVCs
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pvcCountBySC := map[string]int{}
	totalGBBySC := map[string]int{}

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}
		result.Summary.TotalStorageGB += sizeGB

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		if scName == "" {
			scName = "<default>"
		}

		entry := PVCByClassEntry{
			Name:         pvc.Name,
			Namespace:    pvc.Namespace,
			StorageClass: scName,
			SizeGB:       sizeGB,
			Phase:        string(pvc.Status.Phase),
		}
		if len(pvc.Spec.AccessModes) > 0 {
			entry.AccessMode = string(pvc.Spec.AccessModes[0])
		}

		switch string(pvc.Status.Phase) {
		case "Bound":
			result.Summary.BoundPVCs++
		case "Pending":
			result.Summary.PendingPVCs++
			entry.RiskLevel = "high"
			entry.Issue = "PVC pending - storage provisioning may be failing"
			result.Misconfigured = append(result.Misconfigured, entry)
		}

		if scName == "<default>" && result.Summary.DefaultClass == 0 {
			result.Summary.NoStorageClass++
			entry.RiskLevel = "medium"
			entry.Issue = "no storage class specified and no default configured"
			if entry.Phase == "Bound" {
				result.Misconfigured = append(result.Misconfigured, entry)
			}
		}

		pvcCountBySC[scName]++
		totalGBBySC[scName] += sizeGB
		result.PVCsByClass = append(result.PVCsByClass, entry)
	}

	// Finalize storage class entries
	for _, sc := range scInfo {
		sc.PVCCount = pvcCountBySC[sc.Name]
		sc.TotalGB = totalGBBySC[sc.Name]
		result.Classes = append(result.Classes, *sc)
	}
	sort.Slice(result.Classes, func(i, j int) bool {
		return result.Classes[i].PVCCount > result.Classes[j].PVCCount
	})

	// Score
	if result.Summary.TotalPVCs > 0 {
		boundPct := result.Summary.BoundPVCs * 100 / result.Summary.TotalPVCs
		result.HealthScore = boundPct
		if result.Summary.PendingPVCs > 0 {
			result.HealthScore -= result.Summary.PendingPVCs * 10
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildStorageClassRecs1898(&result)
	writeJSON(w, result)
}

func buildStorageClassRecs1898(result *StorageClassAuditResult) []string {
	recs := []string{
		fmt.Sprintf("Storage class audit: %d PVCs (%dGB), %d storage classes (%d default), %d pending",
			result.Summary.TotalPVCs, result.Summary.TotalStorageGB,
			result.Summary.StorageClasses, result.Summary.DefaultClass,
			result.Summary.PendingPVCs),
	}
	if result.Summary.PendingPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVCs in Pending state - check storage provisioner health", result.Summary.PendingPVCs))
	}
	if result.Summary.NoStorageClass > 0 {
		recs = append(recs, fmt.Sprintf("%d PVCs without explicit storage class - define a default StorageClass", result.Summary.NoStorageClass))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Workload Interdependency Map — service-to-service dependency graph
// ---------------------------------------------------------------

type InterdependencyResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         InterdepSummary   `json:"summary"`
	Dependencies    []InterdepEntry   `json:"dependencies"`
	HubServices     []HubServiceEntry `json:"hubServices"`
	OrphanServices  []OrphanSvcEntry  `json:"orphanServices"`
	CircularDeps    []string          `json:"circularDeps"`
	Recommendations []string          `json:"recommendations"`
}

type InterdepSummary struct {
	TotalServices    int `json:"totalServices"`
	WithDependencies int `json:"withDependencies"`
	IsolatedServices int `json:"isolatedServices"`
	HubServices      int `json:"hubServices"`
	MaxFanIn         int `json:"maxFanIn"`
	MaxFanOut        int `json:"maxFanOut"`
	OrphanedServices int `json:"orphanedServices"`
}

type InterdepEntry struct {
	Source   string `json:"source"`
	SourceNs string `json:"sourceNamespace"`
	Target   string `json:"target"`
	TargetNs string `json:"targetNamespace"`
	DepType  string `json:"dependencyType"`
}

type HubServiceEntry struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	FanIn     int    `json:"fanIn"`
	FanOut    int    `json:"fanOut"`
	RiskLevel string `json:"riskLevel"`
}

type OrphanSvcEntry struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

func (s *Server) handleWorkloadInterdependency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := InterdependencyResult{ScannedAt: time.Now()}

	// Build service registry: ns/svc -> selector
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	svcByNsName := map[string]string{} // ns/name -> selector string
	svcExists := map[string]bool{}
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++
		key := svc.Namespace + "/" + svc.Name
		svcExists[key] = true
		if len(svc.Spec.Selector) > 0 {
			selParts := []string{}
			for k, v := range svc.Spec.Selector {
				selParts = append(selParts, k+"="+v)
			}
			sort.Strings(selParts)
			svcByNsName[key] = strings.Join(selParts, ",")
		}
	}

	// Track fan-in/fan-out
	fanIn := map[string]int{}  // service -> number of dependents
	fanOut := map[string]int{} // service -> number of dependencies

	// Analyze env vars for service references (common patterns: DB_HOST, API_URL, etc)
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		srcKey := dep.Namespace + "/" + dep.Name
		hasDep := false

		// Check env vars for service references
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				val := env.Value
				if env.ValueFrom != nil {
					continue
				}
				// Look for service name patterns in env values
				for svcKey := range svcExists {
					svcName := svcKey[strings.Index(svcKey, "/")+1:]
					if val != "" && (strings.Contains(val, svcName) || strings.Contains(strings.ToLower(env.Name), strings.ToLower(svcName))) {
						if svcKey != srcKey {
							result.Dependencies = append(result.Dependencies, InterdepEntry{
								Source: dep.Name, SourceNs: dep.Namespace,
								Target: svcName, TargetNs: svcKey[:strings.Index(svcKey, "/")],
								DepType: "env-ref",
							})
							fanIn[svcKey]++
							fanOut[srcKey]++
							hasDep = true
						}
					}
				}
			}
		}

		// Check for external service references (DB hosts, API endpoints in env)
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.Value == "" {
					continue
				}
				lowerName := strings.ToLower(env.Name)
				if strings.Contains(lowerName, "host") || strings.Contains(lowerName, "url") || strings.Contains(lowerName, "endpoint") {
					hasDep = true
				}
			}
		}

		if hasDep {
			result.Summary.WithDependencies++
		} else {
			result.Summary.IsolatedServices++
		}
	}

	// Build hub services (high fan-in)
	maxFanIn := 0
	maxFanOut := 0
	for svc, count := range fanIn {
		if count > maxFanIn {
			maxFanIn = count
		}
		out := fanOut[svc]
		if out > maxFanOut {
			maxFanOut = out
		}
		risk := "low"
		if count >= 5 {
			risk = "high"
		} else if count >= 3 {
			risk = "medium"
		}
		ns := svc[:strings.Index(svc, "/")]
		name := svc[strings.Index(svc, "/")+1:]
		result.HubServices = append(result.HubServices, HubServiceEntry{
			Service: name, Namespace: ns, FanIn: count, FanOut: out, RiskLevel: risk,
		})
		if count >= 3 {
			result.Summary.HubServices++
		}
	}
	result.Summary.MaxFanIn = maxFanIn
	result.Summary.MaxFanOut = maxFanOut
	sort.Slice(result.HubServices, func(i, j int) bool {
		return result.HubServices[i].FanIn > result.HubServices[j].FanIn
	})

	// Find orphaned services (no fan-in at all = nobody depends on them)
	for svcKey := range svcExists {
		if fanIn[svcKey] == 0 {
			ns := svcKey[:strings.Index(svcKey, "/")]
			name := svcKey[strings.Index(svcKey, "/")+1:]
			result.Summary.OrphanedServices++
			result.OrphanServices = append(result.OrphanServices, OrphanSvcEntry{
				Service: name, Namespace: ns, Type: "no-dependents",
			})
		}
	}

	// Score
	if result.Summary.TotalServices > 0 {
		connectedPct := result.Summary.WithDependencies * 100 / result.Summary.TotalServices
		result.HealthScore = connectedPct
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildInterdepRecs1898(&result)
	writeJSON(w, result)
}

func buildInterdepRecs1898(result *InterdependencyResult) []string {
	recs := []string{
		fmt.Sprintf("Interdependency: %d services (%d with deps, %d isolated), %d hub services, max fan-in %d",
			result.Summary.TotalServices, result.Summary.WithDependencies,
			result.Summary.IsolatedServices, result.Summary.HubServices,
			result.Summary.MaxFanIn),
	}
	if result.Summary.HubServices > 0 {
		recs = append(recs, fmt.Sprintf("%d hub services with high fan-in - ensure HA and capacity for these critical dependencies", result.Summary.HubServices))
	}
	if result.Summary.OrphanedServices > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned services with no dependents - verify if still needed or clean up", result.Summary.OrphanedServices))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. DNS Resolution Health — CoreDNS resolution health & config
// ---------------------------------------------------------------

type DNSHealthResult1898 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSHealthSummary1898 `json:"summary"`
	ConfigIssues    []DNSConfigIssue1898 `json:"configIssues"`
	ServiceDNS      []ServiceDNSEntry    `json:"serviceDNS"`
	Recommendations []string             `json:"recommendations"`
}

type DNSHealthSummary1898 struct {
	CoreDNSServers   int `json:"corednsServers"`
	TotalServices    int `json:"totalServices"`
	ServicesWithDNS  int `json:"servicesWithDNS"`
	HeadlessServices int `json:"headlessServices"`
	ExternalNames    int `json:"externalNames"`
	LongNames        int `json:"longNames"`
	ConfigIssues     int `json:"configIssues"`
}

type DNSConfigIssue1898 struct {
	Component string `json:"component"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

type ServiceDNSEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	DNSName   string `json:"dnsName"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleDNSResolutionHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DNSHealthResult1898{ScannedAt: time.Now()}

	// Check CoreDNS deployment health
	corednsDeps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range corednsDeps.Items {
		if strings.Contains(strings.ToLower(dep.Name), "coredns") {
			result.Summary.CoreDNSServers += int(dep.Status.ReadyReplicas)
			if dep.Status.ReadyReplicas == 0 {
				result.ConfigIssues = append(result.ConfigIssues, DNSConfigIssue1898{
					Component: dep.Name,
					Issue:     "CoreDNS deployment has 0 ready replicas",
					Severity:  "critical",
				})
				result.Summary.ConfigIssues++
			}
			if dep.Spec.Replicas != nil && *dep.Spec.Replicas < 2 {
				result.ConfigIssues = append(result.ConfigIssues, DNSConfigIssue1898{
					Component: dep.Name,
					Issue:     "CoreDNS should have at least 2 replicas for HA",
					Severity:  "high",
				})
				result.Summary.ConfigIssues++
			}
		}
	}

	// Analyze service DNS names
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++
		result.Summary.ServicesWithDNS++

		entry := ServiceDNSEntry{
			Name: svc.Name, Namespace: svc.Namespace,
			DNSName: fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
		}

		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			if svc.Spec.ClusterIP == "None" {
				entry.Type = "headless"
				result.Summary.HeadlessServices++
				entry.RiskLevel = "medium"
			} else {
				entry.Type = "cluster-ip"
				entry.RiskLevel = "low"
			}
		case corev1.ServiceTypeExternalName:
			entry.Type = "external-name"
			result.Summary.ExternalNames++
			entry.DNSName = svc.Spec.ExternalName
			entry.RiskLevel = "medium"
		case corev1.ServiceTypeLoadBalancer, corev1.ServiceTypeNodePort:
			entry.Type = "external"
			entry.RiskLevel = "low"
		}

		// Check for long service names (DNS label max 63 chars)
		if len(svc.Name) > 50 {
			result.Summary.LongNames++
			entry.RiskLevel = "medium"
		}

		result.ServiceDNS = append(result.ServiceDNS, entry)
	}

	// Check CoreDNS ConfigMap for custom config
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	for _, cm := range cms.Items {
		if cm.Namespace == "kube-system" && strings.Contains(cm.Name, "coredns") {
			// Check for custom stub domains or forwarding
			if cm.Data["Corefile"] != "" {
				corefile := cm.Data["Corefile"]
				if !strings.Contains(corefile, "forward") && !strings.Contains(corefile, "proxy") {
					result.ConfigIssues = append(result.ConfigIssues, DNSConfigIssue1898{
						Component: cm.Name,
						Issue:     "CoreDNS Corefile missing forward/proxy directive - upstream resolution may fail",
						Severity:  "medium",
					})
					result.Summary.ConfigIssues++
				}
			}
		}
	}

	// Score
	if result.Summary.CoreDNSServers == 0 {
		result.HealthScore = 0
	} else {
		result.HealthScore = 90
		result.HealthScore -= result.Summary.ConfigIssues * 10
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildDNSHealthRecs1898(&result)
	writeJSON(w, result)
}

func buildDNSHealthRecs1898(result *DNSHealthResult1898) []string {
	recs := []string{
		fmt.Sprintf("DNS health: %d CoreDNS servers, %d services (%d headless, %d external-name), %d config issues",
			result.Summary.CoreDNSServers, result.Summary.TotalServices,
			result.Summary.HeadlessServices, result.Summary.ExternalNames,
			result.Summary.ConfigIssues),
	}
	if result.Summary.CoreDNSServers < 2 {
		recs = append(recs, fmt.Sprintf("Only %d CoreDNS replicas - scale to 2+ for DNS HA", result.Summary.CoreDNSServers))
	}
	if result.Summary.ConfigIssues > 0 {
		recs = append(recs, fmt.Sprintf("%d DNS config issues detected - review CoreDNS configuration", result.Summary.ConfigIssues))
	}
	if result.Summary.LongNames > 0 {
		recs = append(recs, fmt.Sprintf("%d service names > 50 chars - risk of DNS truncation", result.Summary.LongNames))
	}
	return recs
}
