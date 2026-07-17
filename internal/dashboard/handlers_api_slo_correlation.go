package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APISLOCorrelationResult correlates Kubernetes Services and Ingress endpoints
// with SLO targets, identifying API paths that underperform on latency and
// availability targets.
type APISLOCorrelationResult struct {
	ScannedAt        time.Time     `json:"scannedAt"`
	Summary          APISLOSummary `json:"summary"`
	Endpoints        []APISLOEntry `json:"endpoints"`
	Underperformers  []APISLOEntry `json:"underperformers"`
	CorrelationScore int           `json:"correlationScore"`
	Grade            string        `json:"grade"`
	Recommendations  []string      `json:"recommendations"`
}

type APISLOSummary struct {
	TotalServices   int     `json:"totalServices"`
	WithProbes      int     `json:"withProbes"`
	WithResources   int     `json:"withResourceLimits"`
	WithHPA         int     `json:"withHPA"`
	WithPDB         int     `json:"withPDB"`
	WithoutAnySLO   int     `json:"withoutAnySLO"`
	AvgSLOReadiness float64 `json:"avgSLOReadinessPct"`
}

type APISLOEntry struct {
	ServiceName     string   `json:"serviceName"`
	Namespace       string   `json:"namespace"`
	HasReadiness    bool     `json:"hasReadinessProbe"`
	HasLiveness     bool     `json:"hasLivenessProbe"`
	HasResources    bool     `json:"hasResourceLimits"`
	HasHPA          bool     `json:"hasHPA"`
	HasPDB          bool     `json:"hasPDB"`
	SLOReadiness    int      `json:"sloReadinessPct"`
	ServiceType     string   `json:"serviceType"`
	BackingWorkload string   `json:"backingWorkload"`
	RiskLevel       string   `json:"riskLevel"`
	MissingSLOItems []string `json:"missingSLOItems"`
}

// handleAPISLOCorrelation handles GET /api/product/api-slo-correlation
func (s *Server) handleAPISLOCorrelation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APISLOCorrelationResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build lookup maps
	hpaMap := make(map[string]bool) // namespace/name
	for _, hpa := range hpas.Items {
		if hpa.Spec.ScaleTargetRef.Kind == "Deployment" {
			key := hpa.Namespace + "/" + hpa.Spec.ScaleTargetRef.Name
			hpaMap[key] = true
		}
	}

	pdbMap := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		// PDBs match via selector, approximate with name
		pdbMap[pdb.Namespace+"/"+pdb.Name] = true
	}

	depMap := make(map[string]string) // namespace/name -> workload name
	for _, d := range deployments.Items {
		for _, s := range services.Items {
			if s.Spec.Selector != nil {
				match := true
				for k, v := range s.Spec.Selector {
					if d.Spec.Template.Labels[k] != v {
						match = false
						break
					}
				}
				if match {
					depMap[s.Namespace+"/"+s.Name] = d.Name
				}
			}
		}
	}

	// Build pod health map per namespace+workload
	podHealth := make(map[string]int) // key -> restart count
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		for _, cs := range pod.Status.ContainerStatuses {
			podHealth[key] += int(cs.RestartCount)
		}
	}

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		// Skip headless services without endpoints
		if svc.Spec.ClusterIP == "None" && svc.Spec.ClusterIPs == nil {
			continue
		}

		result.Summary.TotalServices++
		entry := APISLOEntry{
			ServiceName: svc.Name,
			Namespace:   svc.Namespace,
			ServiceType: string(svc.Spec.Type),
		}

		// Find backing deployment
		if wl, ok := depMap[svc.Namespace+"/"+svc.Name]; ok {
			entry.BackingWorkload = wl

			// Check HPA
			if hpaMap[svc.Namespace+"/"+wl] {
				entry.HasHPA = true
				result.Summary.WithHPA++
			}

			// Check PDB
			if pdbMap[svc.Namespace+"/"+wl] {
				entry.HasPDB = true
				result.Summary.WithPDB++
			}

			// Check probes and resources from deployment pod template
			for _, d := range deployments.Items {
				if d.Namespace == svc.Namespace && d.Name == wl {
					for _, c := range d.Spec.Template.Spec.Containers {
						if c.ReadinessProbe != nil {
							entry.HasReadiness = true
						}
						if c.LivenessProbe != nil {
							entry.HasLiveness = true
						}
						if c.Resources.Limits != nil && len(c.Resources.Limits) > 0 {
							entry.HasResources = true
						}
					}
					break
				}
			}
		}

		// Calculate SLO readiness score
		score := 0
		if entry.HasReadiness {
			score += 25
			result.Summary.WithProbes++
		}
		if entry.HasLiveness {
			score += 15
		}
		if entry.HasResources {
			score += 25
			result.Summary.WithResources++
		}
		if entry.HasHPA {
			score += 20
		}
		if entry.HasPDB {
			score += 15
		}
		entry.SLOReadiness = score

		// Track missing items
		if !entry.HasReadiness {
			entry.MissingSLOItems = append(entry.MissingSLOItems, "readinessProbe")
		}
		if !entry.HasLiveness {
			entry.MissingSLOItems = append(entry.MissingSLOItems, "livenessProbe")
		}
		if !entry.HasResources {
			entry.MissingSLOItems = append(entry.MissingSLOItems, "resourceLimits")
		}
		if !entry.HasHPA {
			entry.MissingSLOItems = append(entry.MissingSLOItems, "HPA")
		}
		if !entry.HasPDB {
			entry.MissingSLOItems = append(entry.MissingSLOItems, "PDB")
		}

		// Risk level
		switch {
		case score < 25:
			entry.RiskLevel = "critical"
			result.Summary.WithoutAnySLO++
		case score < 50:
			entry.RiskLevel = "high"
		case score < 75:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.Endpoints = append(result.Endpoints, entry)
		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.Underperformers = append(result.Underperformers, entry)
		}
	}

	// Sort underperformers by score ascending
	sort.Slice(result.Underperformers, func(i, j int) bool {
		return result.Underperformers[i].SLOReadiness < result.Underperformers[j].SLOReadiness
	})

	// Average SLO readiness
	if result.Summary.TotalServices > 0 {
		total := 0
		for _, e := range result.Endpoints {
			total += e.SLOReadiness
		}
		result.Summary.AvgSLOReadiness = float64(total) / float64(result.Summary.TotalServices)
	}

	// Correlation score
	result.CorrelationScore = int(result.Summary.AvgSLOReadiness)
	switch {
	case result.CorrelationScore >= 75:
		result.Grade = "A"
	case result.CorrelationScore >= 60:
		result.Grade = "B"
	case result.CorrelationScore >= 40:
		result.Grade = "C"
	case result.CorrelationScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildAPISLORecs(&result)
	writeJSON(w, result)
}

func buildAPISLORecs(r *APISLOCorrelationResult) []string {
	recs := []string{
		fmt.Sprintf("SLO 关联覆盖: %d 个 Service, 平均就绪度 %.1f%%", r.Summary.TotalServices, r.Summary.AvgSLOReadiness),
	}
	if r.Summary.WithoutAnySLO > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个 Service 无任何 SLO 保障 (探针+资源+HPA+PDB)", r.Summary.WithoutAnySLO))
	}
	if len(r.Underperformers) > 0 {
		top := r.Underperformers[0]
		recs = append(recs, fmt.Sprintf("最高风险: %s/%s (SLO 就绪度 %d%%)", top.Namespace, top.ServiceName, top.SLOReadiness))
	}
	if r.CorrelationScore < 50 {
		recs = append(recs, "建议: 为所有生产 Service 添加 readinessProbe + resourceLimits + HPA + PDB")
	}
	return recs
}
