package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OwnershipMapResult is the workload ownership & accountability governance engine.
// It maps which teams own which workloads, detects orphaned resources lacking ownership
// metadata, and provides accountability scoring per namespace.
type OwnershipMapResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          OwnershipSummary    `json:"summary"`
	ByTeam           []TeamOwnership     `json:"byTeam"`
	OrphanedWorkloads []OrphanedWorkload `json:"orphanedWorkloads"`
	ByNamespace      []NSOwnership       `json:"byNamespace"`
	LabelCoverage    LabelCoverage       `json:"labelCoverage"`
	Recommendations  []string            `json:"recommendations"`
}

// OwnershipSummary aggregates ownership statistics.
type OwnershipSummary struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	WithOwnerLabel   int     `json:"withOwnerLabel"`   // has team/owner/app label
	WithoutOwnerLabel int    `json:"withoutOwnerLabel"` // orphaned (no ownership metadata)
	WithContactLabel int     `json:"withContactLabel"` // has slack/email/contact annotation
	UniqueTeams      int     `json:"uniqueTeams"`
	OrphanedNSCount  int     `json:"orphanedNamespaceCount"` // namespaces where >50% workloads lack ownership
	CoveragePct      float64 `json:"coveragePct"`            // % with ownership
	AccountabilityScore int  `json:"accountabilityScore"`    // 0-100
}

// TeamOwnership shows workloads owned by one team.
type TeamOwnership struct {
	Team         string   `json:"team"`
	WorkloadCount int     `json:"workloadCount"`
	Namespaces   []string `json:"namespaces"`
	Workloads    []string `json:"workloads,omitempty"` // sample workload names
	RiskLevel    string   `json:"riskLevel"`           // based on count concentration
}

// OrphanedWorkload identifies a workload without ownership metadata.
type OrphanedWorkload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Labels    map[string]string `json:"labels"`
	Age       string `json:"age"`
	Replicas  int32  `json:"replicas"`
	Severity  string `json:"severity"`
}

// NSOwnership shows ownership metadata per namespace.
type NSOwnership struct {
	Namespace       string  `json:"namespace"`
	TotalWorkloads  int     `json:"totalWorkloads"`
	WithOwner       int     `json:"withOwner"`
	CoveragePct     float64 `json:"coveragePct"`
	HasNSTeamLabel  bool    `json:"hasNSTeamLabel"`
	Status          string  `json:"status"` // healthy, partial, orphaned
}

// LabelCoverage analyzes metadata label adoption across key labels.
type LabelCoverage struct {
	AppLabel       int `json:"appLabel"`       // % with app label
	TeamLabel      int `json:"teamLabel"`      // % with team/owner label
	VersionLabel   int `json:"versionLabel"`   // % with version label
	ManagedByLabel int `json:"managedByLabel"` // % with app.kubernetes.io/managed-by
	InstanceLabel  int `json:"instanceLabel"`  // % with app.kubernetes.io/instance
}

// nsOwnershipData holds per-namespace ownership stats.
type nsOwnershipData struct {
	total     int
	withOwner int
}

// handleOwnershipMap provides workload ownership & accountability governance analysis.
// GET /api/product/ownership-map
func (s *Server) handleOwnershipMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := OwnershipMapResult{ScannedAt: time.Now()}
	now := time.Now()
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Collect deployments and statefulsets
	deploys, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	stss, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Namespace labels for team-level ownership
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	nsTeamLabel := make(map[string]string)
	for _, ns := range nsList.Items {
		for k, v := range ns.Labels {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "team") || strings.Contains(kl, "owner") ||
				strings.Contains(kl, "dept") || strings.Contains(kl, "department") {
				nsTeamLabel[ns.Name] = v
				break
			}
		}
	}

	// Label tracking
	labelStats := struct{ app, team, version, managedBy, instance int }{}
	teamMap := make(map[string]*TeamOwnership)
	nsStats := make(map[string]*nsOwnershipData)

	totalWorkloads := 0
	withOwner := 0
	withContact := 0

	processWorkload := func(name, ns, kind string, replicas int32, labels map[string]string, annotations map[string]string, creationTime time.Time) {
		if systemNS[ns] {
			return
		}
		totalWorkloads++

		nsd, ok := nsStats[ns]
		if !ok {
			nsd = &nsOwnershipData{}
			nsStats[ns] = nsd
		}
		nsd.total++

		// Track label coverage
		hasApp := false
		hasTeam := false
		hasVersion := false
		hasManagedBy := false
		hasInstance := false
		teamName := ""

		for k, v := range labels {
			kl := strings.ToLower(k)
			if kl == "app" || kl == "app.kubernetes.io/name" || kl == "k8s-app" {
				hasApp = true
			}
			if strings.Contains(kl, "team") || strings.Contains(kl, "owner") ||
				strings.Contains(kl, "dept") || strings.Contains(kl, "department") ||
				strings.Contains(kl, "cost-center") || strings.Contains(kl, "cost_center") {
				hasTeam = true
				teamName = v
			}
			if kl == "version" || kl == "app.kubernetes.io/version" {
				hasVersion = true
			}
			if strings.Contains(kl, "managed-by") {
				hasManagedBy = true
			}
			if strings.Contains(kl, "instance") {
				hasInstance = true
			}
		}

		if hasApp {
			labelStats.app++
		}
		if hasTeam {
			labelStats.team++
		}
		if hasVersion {
			labelStats.version++
		}
		if hasManagedBy {
			labelStats.managedBy++
		}
		if hasInstance {
			labelStats.instance++
		}

		// Check for ownership (team label or namespace team label)
		hasOwnership := hasTeam
		if !hasOwnership && nsTeamLabel[ns] != "" {
			hasOwnership = true
			teamName = nsTeamLabel[ns]
		}

		if hasOwnership {
			withOwner++
			nsd.withOwner++

			// Track team
			if teamName == "" {
				teamName = "unknown"
			}
			team, ok := teamMap[teamName]
			if !ok {
				team = &TeamOwnership{Team: teamName}
				teamMap[teamName] = team
			}
			team.WorkloadCount++
			if !containsNS(team.Namespaces, ns) {
				team.Namespaces = append(team.Namespaces, ns)
			}
			if len(team.Workloads) < 10 {
				team.Workloads = append(team.Workloads, fmt.Sprintf("%s/%s", ns, name))
			}
		} else {
			// Orphaned workload
			severity := "medium"
			if replicas > 3 {
				severity = "high"
			}
			age := now.Sub(creationTime)
			if age > 90*24*time.Hour {
				severity = "high"
			}
			result.OrphanedWorkloads = append(result.OrphanedWorkloads, OrphanedWorkload{
				Name:      name,
				Namespace: ns,
				Kind:      kind,
				Labels:    labels,
				Age:       formatDuration(age),
				Replicas:  replicas,
				Severity:  severity,
			})
		}

		// Check for contact annotation
		if annotations != nil {
			for k := range annotations {
				kl := strings.ToLower(k)
				if strings.Contains(kl, "contact") || strings.Contains(kl, "slack") ||
					strings.Contains(kl, "email") || strings.Contains(kl, "oncall") {
					withContact++
					break
				}
			}
		}
	}

	for _, dep := range deploys.Items {
		reps := int32(0)
		if dep.Spec.Replicas != nil {
			reps = *dep.Spec.Replicas
		}
		processWorkload(dep.Name, dep.Namespace, "Deployment", reps, dep.Labels, dep.Annotations, dep.CreationTimestamp.Time)
	}
	for _, sts := range stss.Items {
		reps := int32(0)
		if sts.Spec.Replicas != nil {
			reps = *sts.Spec.Replicas
		}
		processWorkload(sts.Name, sts.Namespace, "StatefulSet", reps, sts.Labels, sts.Annotations, sts.CreationTimestamp.Time)
	}

	// Build summary
	result.Summary.TotalWorkloads = totalWorkloads
	result.Summary.WithOwnerLabel = withOwner
	result.Summary.WithoutOwnerLabel = totalWorkloads - withOwner
	result.Summary.WithContactLabel = withContact
	result.Summary.UniqueTeams = len(teamMap)
	if totalWorkloads > 0 {
		result.Summary.CoveragePct = float64(withOwner) / float64(totalWorkloads) * 100
	}

	// Accountability score
	score := 0
	if totalWorkloads > 0 {
		score = int(result.Summary.CoveragePct * 0.5) // 50% from coverage
		contactPct := float64(withContact) / float64(totalWorkloads) * 100
		score += int(contactPct * 0.3) // 30% from contact info
		// 20% from team diversity (more teams = better accountability distribution)
		teamDiversity := float64(len(teamMap)) / float64(totalWorkloads) * 100
		if teamDiversity > 0 {
			score += min(20, int(teamDiversity*2))
		}
	}
	result.Summary.AccountabilityScore = score

	// Build byTeam
	for _, team := range teamMap {
		risk := "low"
		if team.WorkloadCount > 20 {
			risk = "high" // too concentrated
		} else if team.WorkloadCount > 10 {
			risk = "moderate"
		}
		team.RiskLevel = risk
		result.ByTeam = append(result.ByTeam, *team)
	}
	sort.Slice(result.ByTeam, func(i, j int) bool {
		return result.ByTeam[i].WorkloadCount > result.ByTeam[j].WorkloadCount
	})

	// Sort orphaned workloads
	sort.Slice(result.OrphanedWorkloads, func(i, j int) bool {
		if result.OrphanedWorkloads[i].Severity != result.OrphanedWorkloads[j].Severity {
			return severityRankMap(result.OrphanedWorkloads[i].Severity) > severityRankMap(result.OrphanedWorkloads[j].Severity)
		}
		return result.OrphanedWorkloads[i].Replicas > result.OrphanedWorkloads[j].Replicas
	})
	if len(result.OrphanedWorkloads) > 50 {
		result.OrphanedWorkloads = result.OrphanedWorkloads[:50]
	}

	// Build byNamespace
	orphanedNS := 0
	for nsName, nsd := range nsStats {
		coverage := 0.0
		if nsd.total > 0 {
			coverage = float64(nsd.withOwner) / float64(nsd.total) * 100
		}
		status := "healthy"
		if coverage < 50 {
			status = "orphaned"
			orphanedNS++
		} else if coverage < 80 {
			status = "partial"
		}
		result.ByNamespace = append(result.ByNamespace, NSOwnership{
			Namespace:      nsName,
			TotalWorkloads: nsd.total,
			WithOwner:      nsd.withOwner,
			CoveragePct:    coverage,
			HasNSTeamLabel: nsTeamLabel[nsName] != "",
			Status:         status,
		})
	}
	result.Summary.OrphanedNSCount = orphanedNS
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	// Label coverage percentages
	if totalWorkloads > 0 {
		result.LabelCoverage = LabelCoverage{
			AppLabel:       labelStats.app * 100 / totalWorkloads,
			TeamLabel:      labelStats.team * 100 / totalWorkloads,
			VersionLabel:   labelStats.version * 100 / totalWorkloads,
			ManagedByLabel: labelStats.managedBy * 100 / totalWorkloads,
			InstanceLabel:  labelStats.instance * 100 / totalWorkloads,
		}
	}

	// Recommendations
	result.Recommendations = generateOwnershipRecs(result)

	writeJSON(w, result)
}

// generateOwnershipRecs produces actionable recommendations.
func generateOwnershipRecs(result OwnershipMapResult) []string {
	var recs []string

	if result.Summary.WithoutOwnerLabel > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads lack ownership labels — add 'team' or 'owner' labels for accountability", result.Summary.WithoutOwnerLabel, result.Summary.TotalWorkloads))
	}

	if result.Summary.CoveragePct < 50 {
		recs = append(recs, fmt.Sprintf("Ownership coverage is only %.0f%% — cluster resources cannot be attributed to teams for troubleshooting or cost allocation", result.Summary.CoveragePct))
	}

	if result.Summary.WithContactLabel < result.Summary.TotalWorkloads/2 {
		recs = append(recs, fmt.Sprintf("Only %d workloads have contact annotations — add 'contact', 'slack', or 'oncall' annotations for incident response", result.Summary.WithContactLabel))
	}

	if result.Summary.OrphanedNSCount > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have <50%% ownership coverage — prioritize these for labeling campaigns", result.Summary.OrphanedNSCount))
	}

	if result.LabelCoverage.TeamLabel < 30 {
		recs = append(recs, fmt.Sprintf("Only %d%% of workloads have team labels — implement a labeling policy and admission webhook to enforce it", result.LabelCoverage.TeamLabel))
	}

	if len(result.ByTeam) > 0 {
		top := result.ByTeam[0]
		if top.WorkloadCount > 15 {
			recs = append(recs, fmt.Sprintf("Team '%s' owns %d workloads across %d namespaces — consider splitting ownership for better accountability", top.Team, top.WorkloadCount, len(top.Namespaces)))
		}
	}

	if result.Summary.AccountabilityScore < 40 {
		recs = append(recs, fmt.Sprintf("Accountability score is %d/100 — implement mandatory ownership labels via OPA/Kyverno admission policies", result.Summary.AccountabilityScore))
	}

	if len(recs) == 0 {
		recs = append(recs, "Ownership metadata is healthy — maintain current labeling practices")
	}

	return recs
}

// containsNS checks if a namespace is in a slice.
func containsNS(slice []string, ns string) bool {
	for _, s := range slice {
		if s == ns {
			return true
		}
	}
	return false
}

// severityRankMap returns a rank for severity strings.
func severityRankMap(sev string) int {
	switch sev {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// Suppress unused import
var _ appsv1.Deployment
var _ corev1.Pod
