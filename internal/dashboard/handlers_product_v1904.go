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
// v19.04 — Product Dimension (Round 3)
// 1. Cost Attribution Matrix
// 2. Quota Utilization Forecast
// 3. Service Mesh Readiness Deep
// ============================================================

// ---------------------------------------------------------------
// 1. Cost Attribution Matrix — resource cost per namespace/team
// ---------------------------------------------------------------

type CostAttributionResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CostAttrSummary     `json:"summary"`
	ByNamespace     []CostAttrNSEntry   `json:"byNamespace"`
	ByTeam          []CostAttrTeamEntry `json:"byTeam"`
	WasteReport     []CostWasteEntry    `json:"wasteReport"`
	Recommendations []string            `json:"recommendations"`
}

type CostAttrSummary struct {
	TotalCPUm     int     `json:"totalCPUm"`
	TotalMemMB    int     `json:"totalMemMB"`
	TotalPVCGB    int     `json:"totalPVCGB"`
	EstMonthlyUSD float64 `json:"estMonthlyUSD"`
	WasteUSD      float64 `json:"wasteUSD"`
	Namespaces    int     `json:"namespaces"`
	Teams         int     `json:"teams"`
}

type CostAttrNSEntry struct {
	Namespace  string  `json:"namespace"`
	CPUm       int     `json:"cpuMilli"`
	MemMB      int     `json:"memMB"`
	PVCGB      int     `json:"pvcGB"`
	Workloads  int     `json:"workloads"`
	MonthlyUSD float64 `json:"monthlyUSD"`
	RiskLevel  string  `json:"riskLevel"`
}

type CostAttrTeamEntry struct {
	Team       string  `json:"team"`
	CPUm       int     `json:"cpuMilli"`
	MemMB      int     `json:"memMB"`
	MonthlyUSD float64 `json:"monthlyUSD"`
	Namespaces int     `json:"namespaces"`
}

type CostWasteEntry struct {
	Namespace string  `json:"namespace"`
	Workload  string  `json:"workload"`
	Issue     string  `json:"issue"`
	WasteUSD  float64 `json:"wasteUSD"`
}

func (s *Server) handleCostAttribution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CostAttributionResult{ScannedAt: time.Now()}

	nsData := map[string]*CostAttrNSEntry{}
	teamData := map[string]*CostAttrTeamEntry{}

	// Cost rates (rough estimates)
	cpuRatePerCoreMonth := 25.0 // $25/core/month
	memRatePerGBMonth := 3.0    // $3/GB/month
	pvcRatePerGBMonth := 0.10   // $0.10/GB/month

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		nsE, ok := nsData[dep.Namespace]
		if !ok {
			nsE = &CostAttrNSEntry{Namespace: dep.Namespace}
			nsData[dep.Namespace] = nsE
			result.Summary.Namespaces++
		}
		nsE.Workloads++

		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuM := int(qty.MilliValue())
				nsE.CPUm += cpuM
				result.Summary.TotalCPUm += cpuM
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memMB := int(qty.Value() / (1024 * 1024))
				nsE.MemMB += memMB
				result.Summary.TotalMemMB += memMB
			}
		}

		// Team attribution from labels
		team := dep.Labels["team"]
		if team == "" {
			team = dep.Labels["app.kubernetes.io/team"]
		}
		if team == "" {
			team = "unattributed"
		}
		teamE, ok := teamData[team]
		if !ok {
			teamE = &CostAttrTeamEntry{Team: team}
			teamData[team] = teamE
			result.Summary.Teams++
		}
		teamE.Namespaces++
		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				teamE.CPUm += int(qty.MilliValue())
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				teamE.MemMB += int(qty.Value() / (1024 * 1024))
			}
		}

		// Detect waste: high requests without limits
		for _, c := range dep.Spec.Template.Spec.Containers {
			reqCPUm := 0
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPUm = int(qty.MilliValue())
			}
			if reqCPUm > 1000 {
				hasLimit := false
				if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
					hasLimit = true
				}
				if !hasLimit {
					waste := float64(reqCPUm) / 1000.0 * cpuRatePerCoreMonth * 0.3
					result.WasteReport = append(result.WasteReport, CostWasteEntry{
						Namespace: dep.Namespace,
						Workload:  fmt.Sprintf("%s/%s", dep.Name, c.Name),
						Issue:     fmt.Sprintf("high CPU request %dm without limit (potential over-allocation)", reqCPUm),
						WasteUSD:  waste,
					})
					result.Summary.WasteUSD += waste
				}
			}
		}
	}

	// PVC costs
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}
		result.Summary.TotalPVCGB += sizeGB
		if nsE, ok := nsData[pvc.Namespace]; ok {
			nsE.PVCGB += sizeGB
		}
	}

	// Calculate monthly costs
	for _, ns := range nsData {
		ns.MonthlyUSD = float64(ns.CPUm)/1000.0*cpuRatePerCoreMonth + float64(ns.MemMB)/1024.0*memRatePerGBMonth + float64(ns.PVCGB)*pvcRatePerGBMonth
		if ns.MonthlyUSD > 500 {
			ns.RiskLevel = "high"
		} else if ns.MonthlyUSD > 100 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	for _, team := range teamData {
		team.MonthlyUSD = float64(team.CPUm)/1000.0*cpuRatePerCoreMonth + float64(team.MemMB)/1024.0*memRatePerGBMonth
		result.ByTeam = append(result.ByTeam, *team)
	}

	result.Summary.EstMonthlyUSD = float64(result.Summary.TotalCPUm)/1000.0*cpuRatePerCoreMonth +
		float64(result.Summary.TotalMemMB)/1024.0*memRatePerGBMonth +
		float64(result.Summary.TotalPVCGB)*pvcRatePerGBMonth

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MonthlyUSD > result.ByNamespace[j].MonthlyUSD
	})
	sort.Slice(result.ByTeam, func(i, j int) bool {
		return result.ByTeam[i].MonthlyUSD > result.ByTeam[j].MonthlyUSD
	})

	// Score: lower waste % = better
	if result.Summary.EstMonthlyUSD > 0 {
		wastePct := result.Summary.WasteUSD * 100 / result.Summary.EstMonthlyUSD
		result.HealthScore = int(100 - wastePct)
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildCostAttrRecs1904(&result)
	writeJSON(w, result)
}

func buildCostAttrRecs1904(r *CostAttributionResult) []string {
	recs := []string{fmt.Sprintf("Cost attribution: $%.0f/month (%d ns, %d teams), waste: $%.0f/month",
		r.Summary.EstMonthlyUSD, r.Summary.Namespaces, r.Summary.Teams, r.Summary.WasteUSD)}
	if r.Summary.WasteUSD > 50 {
		recs = append(recs, fmt.Sprintf("$%.0f/month in waste - right-size over-provisioned workloads", r.Summary.WasteUSD))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Quota Utilization Forecast
// ---------------------------------------------------------------

type QuotaForecastResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         QuotaForecastSummary `json:"summary"`
	ByNamespace     []QuotaNSEntry       `json:"byNamespace"`
	CriticalQuotas  []QuotaNSEntry       `json:"criticalQuotas"`
	NoQuotaNS       []string             `json:"noQuotaNamespaces"`
	Recommendations []string             `json:"recommendations"`
}

type QuotaForecastSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuota       int `json:"withQuota"`
	WithoutQuota    int `json:"withoutQuota"`
	CriticalUsage   int `json:"criticalUsage"`
	HighUsage       int `json:"highUsage"`
	TotalQuotaItems int `json:"totalQuotaItems"`
}

type QuotaNSEntry struct {
	Namespace string              `json:"namespace"`
	Resources []QuotaResource1904 `json:"resources"`
	MaxUsage  int                 `json:"maxUsagePct"`
	RiskLevel string              `json:"riskLevel"`
}

type QuotaResource1904 struct {
	Name      string `json:"name"`
	Hard      string `json:"hard"`
	Used      string `json:"used"`
	UsagePct  int    `json:"usagePct"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleQuotaForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := QuotaForecastResult{ScannedAt: time.Now()}

	// Get all namespaces
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	nsSet := map[string]bool{}
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsSet[ns.Name] = true
			result.Summary.TotalNamespaces++
		}
	}

	// Get ResourceQuotas
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	nsQuotaData := map[string]*QuotaNSEntry{}

	for _, rq := range quotas.Items {
		if isSystemNamespace(rq.Namespace) {
			continue
		}
		nsE, ok := nsQuotaData[rq.Namespace]
		if !ok {
			nsE = &QuotaNSEntry{Namespace: rq.Namespace}
			nsQuotaData[rq.Namespace] = nsE
			result.Summary.WithQuota++
		}

		for hardKey, hardVal := range rq.Status.Hard {
			usedVal, hasUsed := rq.Status.Used[hardKey]
			if !hasUsed {
				continue
			}
			result.Summary.TotalQuotaItems++
			usagePct := 0
			if hardVal.Value() > 0 {
				usagePct = int(usedVal.Value() * 100 / hardVal.Value())
			}
			riskLevel := "low"
			if usagePct >= 90 {
				riskLevel = "critical"
				result.Summary.CriticalUsage++
			} else if usagePct >= 75 {
				riskLevel = "high"
				result.Summary.HighUsage++
			}
			nsE.Resources = append(nsE.Resources, QuotaResource1904{
				Name:      string(hardKey),
				Hard:      hardVal.String(),
				Used:      usedVal.String(),
				UsagePct:  usagePct,
				RiskLevel: riskLevel,
			})
			if usagePct > nsE.MaxUsage {
				nsE.MaxUsage = usagePct
				nsE.RiskLevel = riskLevel
			}
		}
	}

	for ns := range nsSet {
		if _, ok := nsQuotaData[ns]; !ok {
			result.Summary.WithoutQuota++
			result.NoQuotaNS = append(result.NoQuotaNS, ns)
		}
	}

	for _, ns := range nsQuotaData {
		result.ByNamespace = append(result.ByNamespace, *ns)
		if ns.MaxUsage >= 75 {
			result.CriticalQuotas = append(result.CriticalQuotas, *ns)
		}
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MaxUsage > result.ByNamespace[j].MaxUsage
	})
	sort.Strings(result.NoQuotaNS)

	// Score
	if result.Summary.TotalNamespaces > 0 {
		quotaPct := result.Summary.WithQuota * 100 / result.Summary.TotalNamespaces
		criticalPenalty := result.Summary.CriticalUsage * 5
		result.HealthScore = quotaPct - criticalPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildQuotaForecastRecs1904(&result)
	writeJSON(w, result)
}

func buildQuotaForecastRecs1904(r *QuotaForecastResult) []string {
	recs := []string{fmt.Sprintf("Quota forecast: %d/%d ns with quotas, %d critical (>90%%), %d high (>75%%), %d without quota",
		r.Summary.WithQuota, r.Summary.TotalNamespaces, r.Summary.CriticalUsage, r.Summary.HighUsage, r.Summary.WithoutQuota)}
	if r.Summary.WithoutQuota > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces without ResourceQuota - risk of unbounded resource consumption", r.Summary.WithoutQuota))
	}
	if r.Summary.CriticalUsage > 0 {
		recs = append(recs, fmt.Sprintf("%d quotas at >90%% usage - increase limits or optimize workloads", r.Summary.CriticalUsage))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Service Mesh Readiness Deep
// ---------------------------------------------------------------

type MeshReadinessDeepResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         MeshDeepSummary   `json:"summary"`
	ByNamespace     []MeshDeepNSEntry `json:"byNamespace"`
	Blockers        []MeshDeepBlocker `json:"blockers"`
	Recommendations []string          `json:"recommendations"`
}

type MeshDeepSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	ReadyForMesh   int `json:"readyForMesh"`
	MeshInjected   int `json:"meshInjected"`
	Blockers       int `json:"blockers"`
	HasSidecars    int `json:"hasSidecars"`
	WithoutProbes  int `json:"withoutProbes"`
	NamedPorts     int `json:"namedPorts"`
	UnnamedPorts   int `json:"unnamedPorts"`
}

type MeshDeepNSEntry struct {
	Namespace     string `json:"namespace"`
	Workloads     int    `json:"workloads"`
	Ready         int    `json:"readyForMesh"`
	Injected      int    `json:"meshInjected"`
	HasIssues     int    `json:"hasIssues"`
	BlockingScore int    `json:"blockingScore"`
}

type MeshDeepBlocker struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

func (s *Server) handleMeshReadinessDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := MeshReadinessDeepResult{ScannedAt: time.Now()}

	nsMap := map[string]*MeshDeepNSEntry{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &MeshDeepNSEntry{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.Workloads++

		ready := true
		meshInjected := false

		// Check for mesh sidecar injection
		for _, c := range dep.Spec.Template.Spec.Containers {
			if strings.Contains(c.Name, "istio-proxy") || strings.Contains(c.Name, "envoy") ||
				strings.Contains(c.Name, "linkerd-proxy") || strings.Contains(c.Name, "sidecar") {
				meshInjected = true
				result.Summary.HasSidecars++
				break
			}
		}
		if meshInjected {
			result.Summary.MeshInjected++
			nsE.Injected++
		}

		// Check readiness probes (required for mesh)
		hasLiveness := false
		hasReadiness := false
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.LivenessProbe != nil {
				hasLiveness = true
			}
			if c.ReadinessProbe != nil {
				hasReadiness = true
			}
			// Check named ports
			for _, p := range c.Ports {
				if p.Name != "" {
					result.Summary.NamedPorts++
				} else {
					result.Summary.UnnamedPorts++
					if ready {
						result.Blockers = append(result.Blockers, MeshDeepBlocker{
							Name: dep.Name, Namespace: dep.Namespace,
							Issue:    "unnamed container ports - mesh requires named ports for routing",
							Severity: "medium",
						})
						result.Summary.Blockers++
						ready = false
					}
				}
			}
		}
		if !hasLiveness {
			result.Summary.WithoutProbes++
			result.Blockers = append(result.Blockers, MeshDeepBlocker{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue:    "missing liveness probe - mesh health checks may fail",
				Severity: "medium",
			})
			result.Summary.Blockers++
			ready = false
		}
		if !hasReadiness {
			result.Blockers = append(result.Blockers, MeshDeepBlocker{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue:    "missing readiness probe - mesh zero-downtime not guaranteed",
				Severity: "high",
			})
			result.Summary.Blockers++
			ready = false
		}

		if ready {
			result.Summary.ReadyForMesh++
			nsE.Ready++
		} else {
			nsE.HasIssues++
			nsE.BlockingScore += 10
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].BlockingScore > result.ByNamespace[j].BlockingScore
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		readyPct := result.Summary.ReadyForMesh * 100 / result.Summary.TotalWorkloads
		result.HealthScore = readyPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildMeshDeepRecs1904(&result)
	writeJSON(w, result)
}

func buildMeshDeepRecs1904(r *MeshReadinessDeepResult) []string {
	recs := []string{fmt.Sprintf("Mesh readiness: %d/%d workloads ready, %d injected, %d blockers (%d without probes, %d unnamed ports)",
		r.Summary.ReadyForMesh, r.Summary.TotalWorkloads, r.Summary.MeshInjected,
		r.Summary.Blockers, r.Summary.WithoutProbes, r.Summary.UnnamedPorts)}
	if r.Summary.WithoutProbes > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without probes - add liveness/readiness probes before mesh adoption", r.Summary.WithoutProbes))
	}
	if r.Summary.UnnamedPorts > 0 {
		recs = append(recs, fmt.Sprintf("%d unnamed container ports - name all ports for mesh routing", r.Summary.UnnamedPorts))
	}
	return recs
}
