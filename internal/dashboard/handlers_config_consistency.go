package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigConsistencyResult is the configuration consistency & standardization auditor.
// It detects configuration drift across similar workloads, identifies non-conformant
// patterns, and scores standardization maturity per namespace and cluster-wide.
type ConfigConsistencyResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          ConfigConsSummary   `json:"summary"`
	ImageRegistry    []ImageRegEntry     `json:"imageRegistryAnalysis"`
	ResourceTiers    []ResourceTierEntry `json:"resourceTiers"`
	NonConformants   []NonConformant     `json:"nonConformants"`
	ByNamespace      []ConfigConsNS      `json:"byNamespace"`
	ConsistencyScore int                 `json:"consistencyScore"`
	Grade            string              `json:"grade"`
	Recommendations  []string            `json:"recommendations"`
}

// ConfigConsSummary aggregates consistency statistics.
type ConfigConsSummary struct {
	TotalWorkloads        int     `json:"totalWorkloads"`
	ConsistentWorkloads   int     `json:"consistentWorkloads"` // follow standard patterns
	NonConformantCount    int     `json:"nonConformantCount"`  // deviate from norms
	DistinctRegistries    int     `json:"distinctRegistries"`
	DistinctResourceTiers int     `json:"distinctResourceTiers"`
	DistinctProbeTypes    int     `json:"distinctProbeTypes"`
	InconsistentLabels    int     `json:"inconsistentLabels"`
	StandardizationPct    float64 `json:"standardizationPct"`
}

// ImageRegEntry shows image registry distribution.
type ImageRegEntry struct {
	Registry   string `json:"registry"`
	Count      int    `json:"count"`
	Percentage int    `json:"percentage"`
	IsInternal bool   `json:"isInternal"`
}

// ResourceTierEntry shows resource request patterns.
type ResourceTierEntry struct {
	Tier       string `json:"tier"` // nano, micro, small, medium, large, xl
	CPURequest string `json:"cpuRequest"`
	MemRequest string `json:"memRequest"`
	Count      int    `json:"count"`
	Percentage int    `json:"percentage"`
}

// NonConformant identifies a workload that deviates from standard patterns.
type NonConformant struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Issues    []string `json:"issues"`
	Score     int      `json:"score"` // 0-100, lower = more non-conformant
	Severity  string   `json:"severity"`
}

// ConfigConsNS shows per-namespace consistency stats.
type ConfigConsNS struct {
	Namespace      string  `json:"namespace"`
	TotalWorkloads int     `json:"totalWorkloads"`
	NonConformant  int     `json:"nonConformant"`
	ConsistencyPct float64 `json:"consistencyPct"`
}

// handleConfigConsistency provides configuration consistency & standardization auditing.
// GET /api/deployment/config-consistency
func (s *Server) handleConfigConsistency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ConfigConsistencyResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deploys, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Track patterns
	registryCounts := make(map[string]int)
	resourceTiers := make(map[string]int)
	probeTypes := make(map[string]int)
	labelKeys := make(map[string]int)

	type nsConsData struct {
		total      int
		nonConform int
	}
	nsStats := make(map[string]*nsConsData)

	totalWorkloads := 0

	for _, dep := range deploys.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		totalWorkloads++
		result.Summary.TotalWorkloads++

		nsd, ok := nsStats[dep.Namespace]
		if !ok {
			nsd = &nsConsData{}
			nsStats[dep.Namespace] = nsd
		}
		nsd.total++

		issues := []string{}
		score := 100

		// === Image registry analysis ===
		for _, c := range dep.Spec.Template.Spec.Containers {
			imageRef := c.Image
			registry := extractImageRegistry(imageRef)
			registryCounts[registry]++

			// Check for Docker Hub default (untrusted for prod)
			if registry == "docker.io" || (registry == "" && !strings.Contains(imageRef, ".")) {
				if !strings.Contains(imageRef, ".") {
					issues = append(issues, fmt.Sprintf("Image '%s' from default Docker Hub — should use private registry", c.Name))
					score -= 15
				}
			}

			// Latest tag check
			if strings.HasSuffix(imageRef, ":latest") || !strings.Contains(imageRef, ":") {
				issues = append(issues, fmt.Sprintf("Image '%s' uses mutable tag — should pin to specific version", c.Name))
				score -= 10
			}

			// Resource tier
			if c.Resources.Requests != nil {
				cpuReq := c.Resources.Requests[corev1.ResourceCPU]
				memReq := c.Resources.Requests[corev1.ResourceMemory]
				if !cpuReq.IsZero() && !memReq.IsZero() {
					tier := classifyResourceTier(cpuReq, memReq)
					resourceTiers[tier]++
				}
			} else {
				issues = append(issues, fmt.Sprintf("Container '%s' has no resource requests", c.Name))
				score -= 15
			}

			// Resource limits
			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
				issues = append(issues, fmt.Sprintf("Container '%s' has no resource limits", c.Name))
				score -= 10
			}

			// Probe types
			if c.ReadinessProbe != nil {
				pt := probeTypeString(c.ReadinessProbe)
				probeTypes[pt]++
			} else {
				issues = append(issues, fmt.Sprintf("Container '%s' missing readiness probe", c.Name))
				score -= 10
			}
			if c.LivenessProbe != nil {
				pt := probeTypeString(c.LivenessProbe)
				probeTypes["liveness:"+pt]++
			} else {
				issues = append(issues, fmt.Sprintf("Container '%s' missing liveness probe", c.Name))
				score -= 5
			}

			// Security context
			if c.SecurityContext == nil {
				issues = append(issues, fmt.Sprintf("Container '%s' missing security context", c.Name))
				score -= 10
			}

			// Image pull policy
			if c.ImagePullPolicy == "" || c.ImagePullPolicy == corev1.PullNever {
				issues = append(issues, fmt.Sprintf("Container '%s' has problematic imagePullPolicy", c.Name))
				score -= 5
			}
		}

		// Labels analysis
		hasAppLabel := false
		for k := range dep.Labels {
			labelKeys[k]++
			kl := strings.ToLower(k)
			if kl == "app" || kl == "app.kubernetes.io/name" {
				hasAppLabel = true
			}
		}
		if !hasAppLabel {
			issues = append(issues, "Missing app label — inconsistent with labeling standard")
			score -= 10
			result.Summary.InconsistentLabels++
		}

		// Strategy consistency
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			issues = append(issues, "Uses Recreate strategy — should use RollingUpdate for zero-downtime")
			score -= 10
		}

		// Revision history
		if dep.Spec.RevisionHistoryLimit == nil || (dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0) {
			issues = append(issues, "No revision history — rollback impossible")
			score -= 5
		}

		// Clamp score
		if score < 0 {
			score = 0
		}

		// Determine if non-conformant
		if len(issues) > 0 {
			severity := "low"
			switch {
			case score < 30:
				severity = "critical"
			case score < 50:
				severity = "high"
			case score < 70:
				severity = "medium"
			}

			result.NonConformants = append(result.NonConformants, NonConformant{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Issues:    issues,
				Score:     score,
				Severity:  severity,
			})
			result.Summary.NonConformantCount++
			nsd.nonConform++
		} else {
			result.Summary.ConsistentWorkloads++
		}
	}

	// Standardization percentage
	if totalWorkloads > 0 {
		result.Summary.StandardizationPct = float64(result.Summary.ConsistentWorkloads) / float64(totalWorkloads) * 100
	}

	// Distinct counts
	result.Summary.DistinctRegistries = len(registryCounts)
	result.Summary.DistinctResourceTiers = len(resourceTiers)
	result.Summary.DistinctProbeTypes = len(probeTypes)

	// Build image registry analysis
	for reg, count := range registryCounts {
		pct := 0
		if totalWorkloads > 0 {
			pct = count * 100 / totalWorkloads
		}
		isInternal := strings.Contains(reg, ".") && !strings.Contains(reg, "docker.io")
		result.ImageRegistry = append(result.ImageRegistry, ImageRegEntry{
			Registry: reg, Count: count, Percentage: pct, IsInternal: isInternal,
		})
	}
	sort.Slice(result.ImageRegistry, func(i, j int) bool {
		return result.ImageRegistry[i].Count > result.ImageRegistry[j].Count
	})

	// Build resource tiers analysis
	for tier, count := range resourceTiers {
		pct := 0
		if totalWorkloads > 0 {
			pct = count * 100 / totalWorkloads
		}
		cpu, mem := tierToResources(tier)
		result.ResourceTiers = append(result.ResourceTiers, ResourceTierEntry{
			Tier: tier, CPURequest: cpu, MemRequest: mem, Count: count, Percentage: pct,
		})
	}
	sort.Slice(result.ResourceTiers, func(i, j int) bool {
		return result.ResourceTiers[i].Count > result.ResourceTiers[j].Count
	})

	// Sort non-conformants by score ascending (worst first)
	sort.Slice(result.NonConformants, func(i, j int) bool {
		return result.NonConformants[i].Score < result.NonConformants[j].Score
	})
	if len(result.NonConformants) > 30 {
		result.NonConformants = result.NonConformants[:30]
	}

	// By namespace
	for nsName, nsd := range nsStats {
		consPct := 0.0
		if nsd.total > 0 {
			consPct = float64(nsd.total-nsd.nonConform) / float64(nsd.total) * 100
		}
		result.ByNamespace = append(result.ByNamespace, ConfigConsNS{
			Namespace:      nsName,
			TotalWorkloads: nsd.total,
			NonConformant:  nsd.nonConform,
			ConsistencyPct: consPct,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ConsistencyPct < result.ByNamespace[j].ConsistencyPct
	})

	// Consistency score = standardization percentage
	result.ConsistencyScore = int(result.Summary.StandardizationPct)
	result.Grade = goldenScoreToGrade(result.ConsistencyScore)

	result.Recommendations = generateConfigConsRecs(result)

	writeJSON(w, result)
}

// extractImageRegistry extracts the registry from an image reference.
func extractImageRegistry(image string) string {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		return parts[0]
	}
	if len(parts) == 2 && (strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		return parts[0]
	}
	return "docker.io"
}

// classifyResourceTier maps CPU/memory requests to a named tier.
func classifyResourceTier(cpu, mem resource.Quantity) string {
	cpuMilli := cpu.MilliValue()
	memMi := mem.Value() / (1024 * 1024)

	switch {
	case cpuMilli <= 50 && memMi <= 64:
		return "nano"
	case cpuMilli <= 100 && memMi <= 128:
		return "micro"
	case cpuMilli <= 250 && memMi <= 256:
		return "small"
	case cpuMilli <= 500 && memMi <= 512:
		return "medium"
	case cpuMilli <= 1000 && memMi <= 1024:
		return "large"
	default:
		return "xl"
	}
}

// tierToResources returns representative resource values for a tier.
func tierToResources(tier string) (string, string) {
	switch tier {
	case "nano":
		return "25m", "64Mi"
	case "micro":
		return "50m", "128Mi"
	case "small":
		return "100m", "256Mi"
	case "medium":
		return "250m", "512Mi"
	case "large":
		return "500m", "1Gi"
	case "xl":
		return "1+", "2Gi+"
	default:
		return "?", "?"
	}
}

// probeTypeString returns the probe type as a string.
func probeTypeString(p *corev1.Probe) string {
	if p.HTTPGet != nil {
		return "http"
	}
	if p.TCPSocket != nil {
		return "tcp"
	}
	if p.Exec != nil {
		return "exec"
	}
	if p.GRPC != nil {
		return "grpc"
	}
	return "none"
}

// generateConfigConsRecs produces actionable recommendations.
func generateConfigConsRecs(result ConfigConsistencyResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Configuration consistency: %d/100 (grade %s) — %.0f%% workloads conform to standards", result.ConsistencyScore, result.Grade, result.Summary.StandardizationPct))

	if result.Summary.NonConformantCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads have configuration deviations — standardize patterns for easier operations", result.Summary.NonConformantCount))
	}

	if result.Summary.DistinctRegistries > 3 {
		recs = append(recs, fmt.Sprintf("%d distinct image registries in use — consolidate to 1-2 trusted registries", result.Summary.DistinctRegistries))
	}

	if result.Summary.InconsistentLabels > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads missing standard labels — implement mandatory app label policy", result.Summary.InconsistentLabels))
	}

	if len(result.NonConformants) > 0 {
		worst := result.NonConformants[0]
		recs = append(recs, fmt.Sprintf("Most non-conformant: '%s/%s' (score %d) — %s", worst.Namespace, worst.Name, worst.Score, strings.Join(worst.Issues[:min(2, len(worst.Issues))], "; ")))
	}

	// Check for missing resource tiers diversity
	if result.Summary.DistinctResourceTiers > 5 {
		recs = append(recs, fmt.Sprintf("%d distinct resource tiers — standardize to 3-4 canonical tiers (S/M/L/XL)", result.Summary.DistinctResourceTiers))
	}

	if len(recs) == 1 {
		recs = append(recs, "All workloads follow consistent configuration patterns — maintain current standards")
	}

	return recs
}
