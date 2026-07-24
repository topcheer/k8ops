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

// ============================================================
// v19.59 — Deployment Dimension (Round 13)
// 1. Pod Anti-Affinity Gap — multi-replica co-location risk
// 2. Container Command Audit — entrypoint/command security & consistency
// 3. Deploy Annotation Signal — automation annotation coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Anti-Affinity Gap
// ---------------------------------------------------------------

type AntiAffGapResult1959 struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	HealthScore      int                   `json:"healthScore"`
	Grade            string                `json:"grade"`
	Summary          AntiAffGapSummary1959 `json:"summary"`
	AtRiskWorkloads  []AntiAffGapEntry1959 `json:"atRiskWorkloads"`
	HealthyWorkloads []AntiAffGapEntry1959 `json:"healthyWorkloads"`
	Recommendations  []string              `json:"recommendations"`
}

type AntiAffGapSummary1959 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithAntiAffinity  int `json:"withAntiAffinity"`
	WithoutAntiAff    int `json:"withoutAntiAffinity"`
	CoLocatedRisk     int `json:"coLocatedRisk"`
	TotalReplicas     int `json:"totalReplicas"`
}

type AntiAffGapEntry1959 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Replicas   int    `json:"replicas"`
	HasPodAnti bool   `json:"hasPodAntiAffinity"`
	RiskLevel  string `json:"riskLevel"`
}

func (s *Server) handleAntiAffGap1959(w http.ResponseWriter, r *http.Request) {
	result := AntiAffGapResult1959{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build pod -> node map
	podNode := make(map[string]string) // ns/name -> node
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podNode[pod.Namespace+"/"+pod.Name] = pod.Spec.NodeName
		}
	}

	// Check Deployments
	for _, dep := range depList.Items {
		if dep.Spec.Replicas == nil || *dep.Spec.Replicas < 2 {
			continue
		}
		replicas := int(*dep.Spec.Replicas)
		result.Summary.TotalMultiReplica++
		result.Summary.TotalReplicas += replicas

		hasAnti := false
		if dep.Spec.Template.Spec.Affinity != nil &&
			dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAnti = true
		}
		// Also check topologySpreadConstraints
		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			hasAnti = true
		}

		entry := AntiAffGapEntry1959{
			Name: dep.Name, Namespace: dep.Namespace,
			Kind: "Deployment", Replicas: replicas,
			HasPodAnti: hasAnti,
		}

		if hasAnti {
			result.HealthyWorkloads = append(result.HealthyWorkloads, entry)
			result.Summary.WithAntiAffinity++
		} else {
			// Check if pods are co-located on same node
			nodeCount := make(map[string]int)
			for _, pod := range podList.Items {
				if pod.Namespace == dep.Namespace {
					for _, or := range pod.OwnerReferences {
						if or.Kind == "ReplicaSet" && strings.HasPrefix(pod.Name, dep.Name) {
							if node, ok := podNode[pod.Namespace+"/"+pod.Name]; ok {
								nodeCount[node]++
							}
						}
					}
				}
			}
			coLocated := false
			for _, cnt := range nodeCount {
				if cnt >= 2 {
					coLocated = true
					break
				}
			}

			entry.RiskLevel = "medium"
			if coLocated {
				entry.RiskLevel = "high"
				result.Summary.CoLocatedRisk++
				score -= 5
			} else {
				score -= 2
			}
			result.AtRiskWorkloads = append(result.AtRiskWorkloads, entry)
			result.Summary.WithoutAntiAff++
		}
	}

	// Check StatefulSets
	for _, sts := range stsList.Items {
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas < 2 {
			continue
		}
		replicas := int(*sts.Spec.Replicas)
		result.Summary.TotalMultiReplica++
		result.Summary.TotalReplicas += replicas

		hasAnti := false
		if sts.Spec.Template.Spec.Affinity != nil &&
			sts.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAnti = true
		}

		entry := AntiAffGapEntry1959{
			Name: sts.Name, Namespace: sts.Namespace,
			Kind: "StatefulSet", Replicas: replicas,
			HasPodAnti: hasAnti,
		}

		if hasAnti {
			result.HealthyWorkloads = append(result.HealthyWorkloads, entry)
			result.Summary.WithAntiAffinity++
		} else {
			entry.RiskLevel = "medium"
			result.AtRiskWorkloads = append(result.AtRiskWorkloads, entry)
			result.Summary.WithoutAntiAff++
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.CoLocatedRisk > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads with co-located pods on same node — add podAntiAffinity", result.Summary.CoLocatedRisk))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d multi-replica workloads have anti-affinity rules", result.Summary.WithAntiAffinity, result.Summary.TotalMultiReplica))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Command Audit
// ---------------------------------------------------------------

type CmdAuditResult1959 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         CmdAuditSummary1959   `json:"summary"`
	Issues          []CmdAuditIssue1959   `json:"issues"`
	PerNamespace    []CmdAuditNSEntry1959 `json:"perNamespace"`
	Recommendations []string              `json:"recommendations"`
}

type CmdAuditSummary1959 struct {
	TotalContainers int `json:"totalContainers"`
	WithCommand     int `json:"withExplicitCommand"`
	WithArgs        int `json:"withExplicitArgs"`
	ShellEntrypoint int `json:"shellEntrypoint"`
	MissingReadonly int `json:"missingReadOnly"`
	PrivilegedCmd   int `json:"privilegedCommand"`
	NoResourceLimit int `json:"withoutResourceLimit"`
}

type CmdAuditIssue1959 struct {
	Container string `json:"container"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type CmdAuditNSEntry1959 struct {
	Namespace  string `json:"namespace"`
	Containers int    `json:"containers"`
	Issues     int    `json:"issues"`
}

func (s *Server) handleCmdAudit1959(w http.ResponseWriter, r *http.Request) {
	result := CmdAuditResult1959{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsStats := make(map[string]*CmdAuditNSEntry1959)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &CmdAuditNSEntry1959{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}

		// Check all containers including init
		allContainers := make([]corev1.Container, 0)
		allContainers = append(allContainers, pod.Spec.Containers...)
		allContainers = append(allContainers, pod.Spec.InitContainers...)

		for _, c := range allContainers {
			result.Summary.TotalContainers++
			ns.Containers++

			// Check for explicit command
			if len(c.Command) > 0 {
				result.Summary.WithCommand++
				// Check for shell entrypoint (security risk)
				cmdStr := strings.Join(c.Command, " ")
				if strings.Contains(cmdStr, "/bin/sh") || strings.Contains(cmdStr, "/bin/bash") {
					result.Summary.ShellEntrypoint++
					result.Issues = append(result.Issues, CmdAuditIssue1959{
						Container: c.Name, Namespace: pod.Namespace, Pod: pod.Name,
						IssueType: "shell-entrypoint", Severity: "medium",
						Detail: fmt.Sprintf("Container uses shell as entrypoint: %s", cmdStr),
					})
					ns.Issues++
					score -= 1
				}
			}

			// Check for explicit args
			if len(c.Args) > 0 {
				result.Summary.WithArgs++
			}

			// Check readOnlyRootFilesystem
			if c.SecurityContext == nil ||
				c.SecurityContext.ReadOnlyRootFilesystem == nil ||
				!*c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.MissingReadonly++
			}

			// Check privileged
			if c.SecurityContext != nil &&
				c.SecurityContext.Privileged != nil &&
				*c.SecurityContext.Privileged {
				result.Summary.PrivilegedCmd++
				result.Issues = append(result.Issues, CmdAuditIssue1959{
					Container: c.Name, Namespace: pod.Namespace, Pod: pod.Name,
					IssueType: "privileged", Severity: "high",
					Detail: "Container runs in privileged mode",
				})
				ns.Issues++
				score -= 5
			}

			// Check resource limits
			if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
				result.Summary.NoResourceLimit++
			}
		}
	}

	for _, ns := range nsStats {
		result.PerNamespace = append(result.PerNamespace, *ns)
	}
	sort.Slice(result.PerNamespace, func(i, j int) bool {
		return result.PerNamespace[i].Issues > result.PerNamespace[j].Issues
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers audited, %d with shell entrypoint, %d privileged", result.Summary.TotalContainers, result.Summary.ShellEntrypoint, result.Summary.PrivilegedCmd))
	if result.Summary.NoResourceLimit > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without resource limits — add limits for reliability", result.Summary.NoResourceLimit))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Deploy Annotation Signal
// ---------------------------------------------------------------

type AnnotSignalResult1959 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         AnnotSignalSummary1959  `json:"summary"`
	Coverage        AnnotSignalCoverage1959 `json:"coverage"`
	Workloads       []AnnotSignalEntry1959  `json:"workloads"`
	Recommendations []string                `json:"recommendations"`
}

type AnnotSignalSummary1959 struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	WithOwner       int `json:"withOwnerAnnotation"`
	WithManagedBy   int `json:"withManagedBy"`
	WithReloader    int `json:"withReloaderAnnotation"`
	WithGitRevision int `json:"withGitRevision"`
	WithChangeCause int `json:"withChangeCause"`
	MissingCritical int `json:"missingCriticalAnnotations"`
}

type AnnotSignalCoverage1959 struct {
	OwnerPct       float64 `json:"ownerCoveragePct"`
	ManagedByPct   float64 `json:"managedByPct"`
	ReloaderPct    float64 `json:"reloaderPct"`
	GitRevisionPct float64 `json:"gitRevisionPct"`
	ChangeCausePct float64 `json:"changeCausePct"`
}

type AnnotSignalEntry1959 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	HasOwner       bool   `json:"hasOwner"`
	HasManagedBy   bool   `json:"hasManagedBy"`
	HasReloader    bool   `json:"hasReloader"`
	HasGitRev      bool   `json:"hasGitRevision"`
	HasChangeCause bool   `json:"hasChangeCause"`
	Score          int    `json:"signalScore"`
}

func (s *Server) handleAnnotSignal1959(w http.ResponseWriter, r *http.Request) {
	result := AnnotSignalResult1959{ScannedAt: time.Now()}
	score := 100

	// Collect all workloads
	type workload struct {
		name, ns, kind string
		annotations    map[string]string
	}
	var workloads []workload

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, d := range depList.Items {
		workloads = append(workloads, workload{d.Name, d.Namespace, "Deployment", d.Annotations})
	}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, st := range stsList.Items {
		workloads = append(workloads, workload{st.Name, st.Namespace, "StatefulSet", st.Annotations})
	}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		workloads = append(workloads, workload{ds.Name, ds.Namespace, "DaemonSet", ds.Annotations})
	}

	// Owner annotation patterns
	ownerKeys := []string{"app.kubernetes.io/managed-by", "owner", "team", "contact", "maintainer"}
	reloaderKeys := []string{"reloader.helm.sh/auto-reload", "reloader.stakater.com/auto", "configmap.reloader.stakater.com/reload"}
	gitKeys := []string{"git-commit", "git-revision", "deployment.kubernetes.io/revision", "argocd.argoproj.io/sync-revision"}
	changeCauseKeys := []string{"kubernetes.io/change-cause", "change-cause"}

	for _, wl := range workloads {
		result.Summary.TotalWorkloads++
		entry := AnnotSignalEntry1959{
			Name: wl.name, Namespace: wl.ns, Kind: wl.kind,
		}

		// Check owner
		for _, k := range ownerKeys {
			if _, ok := wl.annotations[k]; ok {
				entry.HasOwner = true
				break
			}
		}
		if entry.HasOwner {
			result.Summary.WithOwner++
		}

		// Check managed-by
		if v, ok := wl.annotations["app.kubernetes.io/managed-by"]; ok && v != "" {
			entry.HasManagedBy = true
			result.Summary.WithManagedBy++
		}

		// Check reloader
		for _, k := range reloaderKeys {
			if _, ok := wl.annotations[k]; ok {
				entry.HasReloader = true
				break
			}
		}
		if entry.HasReloader {
			result.Summary.WithReloader++
		}

		// Check git revision
		for _, k := range gitKeys {
			if v, ok := wl.annotations[k]; ok && v != "" {
				entry.HasGitRev = true
				break
			}
		}
		if entry.HasGitRev {
			result.Summary.WithGitRevision++
		}

		// Check change cause
		for _, k := range changeCauseKeys {
			if v, ok := wl.annotations[k]; ok && v != "" {
				entry.HasChangeCause = true
				break
			}
		}
		if entry.HasChangeCause {
			result.Summary.WithChangeCause++
		}

		// Signal score: each annotation = 20 points
		sigScore := 0
		if entry.HasOwner {
			sigScore += 20
		}
		if entry.HasManagedBy {
			sigScore += 20
		}
		if entry.HasReloader {
			sigScore += 20
		}
		if entry.HasGitRev {
			sigScore += 20
		}
		if entry.HasChangeCause {
			sigScore += 20
		}
		entry.Score = sigScore

		if sigScore < 40 {
			result.Summary.MissingCritical++
			score -= 2
		}

		result.Workloads = append(result.Workloads, entry)
	}

	// Calculate coverage percentages
	total := float64(result.Summary.TotalWorkloads)
	if total > 0 {
		result.Coverage.OwnerPct = float64(result.Summary.WithOwner) / total * 100
		result.Coverage.ManagedByPct = float64(result.Summary.WithManagedBy) / total * 100
		result.Coverage.ReloaderPct = float64(result.Summary.WithReloader) / total * 100
		result.Coverage.GitRevisionPct = float64(result.Summary.WithGitRevision) / total * 100
		result.Coverage.ChangeCausePct = float64(result.Summary.WithChangeCause) / total * 100
	}

	// Sort by signal score ascending (worst first)
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].Score < result.Workloads[j].Score
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Annotation coverage — owner: %.0f%%, managed-by: %.0f%%, reloader: %.0f%%", result.Coverage.OwnerPct, result.Coverage.ManagedByPct, result.Coverage.ReloaderPct))
	if result.Summary.MissingCritical > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads missing critical annotations (owner/managed-by) — add for traceability", result.Summary.MissingCritical))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// suppress unused import warning
var _ appsv1.Deployment
