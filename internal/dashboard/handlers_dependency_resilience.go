package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DependencyResilienceResult analyzes service-to-service dependency resilience,
// identifying critical paths, cascade failure risks, and missing circuit breakers.
type DependencyResilienceResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	Summary          DepResSummary      `json:"summary"`
	CriticalPaths    []CriticalPath     `json:"criticalPaths"`
	ResilienceGaps   []ResilienceGap    `json:"resilienceGaps"`
	ByNamespace      []DepResNS         `json:"byNamespace"`
	ResilienceScore  int                `json:"resilienceScore"`
	Grade            string             `json:"grade"`
	Recommendations  []string           `json:"recommendations"`
}

// DepResSummary aggregates dependency resilience statistics.
type DepResSummary struct {
	TotalServices     int `json:"totalServices"`
	ServicesWithSelectors int `json:"servicesWithSelectors"`
	SinglePodServices int `json:"singlePodServices"` // services pointing to single pod
	NoPDBServices     int `json:"noPDBServices"`     // services without PDB protection
	CrossNSServices   int `json:"crossNSServices"`   // services referenced cross-namespace
	OrphanedServices  int `json:"orphanedServices"`  // no backing pods
	MultiBackendSvc   int `json:"multiBackendSvc"`   // service with multiple endpoints
	StaleEndpoints    int `json:"staleEndpoints"`    // endpoints not ready
}

// CriticalPath identifies a high-risk dependency chain.
type CriticalPath struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Type        string `json:"type"` // ingress->svc, svc->svc
	RiskLevel   string `json:"riskLevel"`
	Reason      string `json:"reason"`
}

// ResilienceGap identifies missing resilience patterns.
type ResilienceGap struct {
	Service    string `json:"service"`
	Namespace  string `json:"namespace"`
	GapType    string `json:"gapType"` // no-pdb, single-pod, no-probe, orphaned
	Severity   string `json:"severity"`
	Impact     string `json:"impact"`
}

// DepResNS shows per-namespace dependency health.
type DepResNS struct {
	Namespace      string  `json:"namespace"`
	TotalServices  int     `json:"totalServices"`
	Gaps           int     `json:"gaps"`
	CriticalPaths  int     `json:"criticalPaths"`
	ResiliencePct  float64 `json:"resiliencePct"`
}

// handleDependencyResilience provides service dependency resilience analysis.
// GET /api/product/dependency-resilience
func (s *Server) handleDependencyResilience(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DependencyResilienceResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})

	// Build endpoint map
	type epInfo struct {
		readyCount   int
		notReadyCount int
		podNames     map[string]bool
	}
	epMap := make(map[string]*epInfo)
	for _, ep := range endpoints.Items {
		if systemNS[ep.Namespace] {
			continue
		}
		key := ep.Namespace + "/" + ep.Name
		info := &epInfo{podNames: make(map[string]bool)}
		for _, sub := range ep.Subsets {
			for _, addr := range sub.Addresses {
				info.readyCount++
				if addr.TargetRef != nil {
					info.podNames[addr.TargetRef.Name] = true
				}
			}
			for range sub.NotReadyAddresses {
				info.notReadyCount++
			}
		}
		epMap[key] = info
	}

	// Build pod-per-namespace map
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning {
			nsPodCount[pod.Namespace]++
		}
	}

	// Namespace stats
	type nsDepData struct {
		total    int
		gaps     int
		critical int
	}
	nsStats := make(map[string]*nsDepData)

	// Analyze each service
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] || svc.Spec.ClusterIP == "None" && svc.Spec.Selector == nil {
			continue
		}
		result.Summary.TotalServices++

		nsd, ok := nsStats[svc.Namespace]
		if !ok {
			nsd = &nsDepData{}
			nsStats[svc.Namespace] = nsd
		}
		nsd.total++

		key := svc.Namespace + "/" + svc.Name
		ep := epMap[key]

		// Service has selector
		if len(svc.Spec.Selector) > 0 {
			result.Summary.ServicesWithSelectors++
		}

		// Check backing pods
		backingPods := 0
		if ep != nil {
			backingPods = ep.readyCount
		}

		if backingPods == 0 {
			// Orphaned service
			if len(svc.Spec.Selector) > 0 {
				result.Summary.OrphanedServices++
				result.ResilienceGaps = append(result.ResilienceGaps, ResilienceGap{
					Service: svc.Name, Namespace: svc.Namespace,
					GapType: "orphaned", Severity: "high",
					Impact: "Service has selector but no ready endpoints — traffic will fail",
				})
				nsd.gaps++
			}
		} else if backingPods == 1 {
			result.Summary.SinglePodServices++
			result.ResilienceGaps = append(result.ResilienceGaps, ResilienceGap{
				Service: svc.Name, Namespace: svc.Namespace,
				GapType: "single-pod", Severity: "medium",
				Impact: "Service routes to single pod — no HA, any pod failure causes outage",
			})
			nsd.gaps++
		} else {
			result.Summary.MultiBackendSvc++
		}

		// Stale endpoints
		if ep != nil && ep.notReadyCount > 0 {
			result.Summary.StaleEndpoints += ep.notReadyCount
		}

		// Cross-namespace reference check (via ExternalName or manual endpoints)
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.CrossNSServices++
			extName := svc.Spec.ExternalName
			result.CriticalPaths = append(result.CriticalPaths, CriticalPath{
				Source: svc.Namespace + "/" + svc.Name,
				Target: extName,
				Type: "external-name",
				RiskLevel: "medium",
				Reason: "ExternalName service — external dependency not controlled by Kubernetes",
			})
			nsd.critical++
		}
	}

	// Analyze ingress -> service chains
	for _, ing := range ingresses.Items {
		if systemNS[ing.Namespace] {
			continue
		}
		for _, rule := range ing.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				backend := path.Backend.Service
				if backend == nil {
					continue
				}
				svcName := backend.Name
				svcPort := int32(0)
				if backend.Port.Number != 0 {
					svcPort = backend.Port.Number
				} else if backend.Port.Name != "" {
					svcPort = -1 // named port
				}

				// Check if service exists and has endpoints
				svcKey := ing.Namespace + "/" + svcName
				ep := epMap[svcKey]
				if ep == nil || ep.readyCount == 0 {
					result.CriticalPaths = append(result.CriticalPaths, CriticalPath{
						Source: fmt.Sprintf("ingress/%s", ing.Name),
						Target: svcKey,
						Type:   "ingress-to-service",
						RiskLevel: "critical",
						Reason: "Ingress routes to service with no ready endpoints",
					})
					if nsd, ok := nsStats[ing.Namespace]; ok {
						nsd.critical++
					}
				}

				_ = svcPort
			}
		}
	}

	// Sort critical paths by risk level
	sort.Slice(result.CriticalPaths, func(i, j int) bool {
		return severityRankMap(result.CriticalPaths[i].RiskLevel) > severityRankMap(result.CriticalPaths[j].RiskLevel)
	})
	if len(result.CriticalPaths) > 30 {
		result.CriticalPaths = result.CriticalPaths[:30]
	}

	// Sort gaps by severity
	sort.Slice(result.ResilienceGaps, func(i, j int) bool {
		return severityRankMap(result.ResilienceGaps[i].Severity) > severityRankMap(result.ResilienceGaps[j].Severity)
	})
	if len(result.ResilienceGaps) > 30 {
		result.ResilienceGaps = result.ResilienceGaps[:30]
	}

	// By namespace
	for nsName, nsd := range nsStats {
		resPct := 0.0
		if nsd.total > 0 {
			resPct = float64(nsd.total-nsd.gaps) / float64(nsd.total) * 100
		}
		result.ByNamespace = append(result.ByNamespace, DepResNS{
			Namespace:     nsName,
			TotalServices: nsd.total,
			Gaps:          nsd.gaps,
			CriticalPaths: nsd.critical,
			ResiliencePct: resPct,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ResiliencePct < result.ByNamespace[j].ResiliencePct
	})

	// Resilience score
	score := 100
	if result.Summary.TotalServices > 0 {
		orphanRate := float64(result.Summary.OrphanedServices) / float64(result.Summary.TotalServices)
		score -= int(orphanRate * 30)
		singleRate := float64(result.Summary.SinglePodServices) / float64(result.Summary.TotalServices)
		score -= int(singleRate * 20)
	}
	critCount := 0
	for _, cp := range result.CriticalPaths {
		if cp.RiskLevel == "critical" {
			critCount++
		}
	}
	score -= critCount * 5
	if score < 0 {
		score = 0
	}
	result.ResilienceScore = score
	result.Grade = goldenScoreToGrade(score)

	result.Recommendations = generateDepResRecs(result)

	writeJSON(w, result)
}

// generateDepResRecs produces actionable recommendations.
func generateDepResRecs(result DependencyResilienceResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Dependency resilience: %d/100 (grade %s)", result.ResilienceScore, result.Grade))

	if result.Summary.OrphanedServices > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned services with no backing pods — fix selectors or remove stale services", result.Summary.OrphanedServices))
	}

	if result.Summary.SinglePodServices > 0 {
		recs = append(recs, fmt.Sprintf("%d services route to single pod — scale up for HA or add PDB protection", result.Summary.SinglePodServices))
	}

	if result.Summary.StaleEndpoints > 0 {
		recs = append(recs, fmt.Sprintf("%d stale (not-ready) endpoints detected — pods may be crashing or unhealthy", result.Summary.StaleEndpoints))
	}

	if len(result.CriticalPaths) > 0 {
		critCount := 0
		for _, cp := range result.CriticalPaths {
			if cp.RiskLevel == "critical" {
				critCount++
			}
		}
		if critCount > 0 {
			recs = append(recs, fmt.Sprintf("%d critical dependency paths — ingress routing to dead services", critCount))
		}
	}

	if result.Summary.CrossNSServices > 0 {
		recs = append(recs, fmt.Sprintf("%d ExternalName services — external dependencies beyond Kubernetes control", result.Summary.CrossNSServices))
	}

	if len(recs) == 1 {
		recs = append(recs, "Service dependencies are resilient — maintain current HA practices")
	}

	return recs
}

// Suppress unused imports
var _ netv1.Ingress
var _ intstr.IntOrString
