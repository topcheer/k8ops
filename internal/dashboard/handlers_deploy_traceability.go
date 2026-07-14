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

// DeployTraceabilityResult is the deployment reproducibility & CI/CD traceability audit.
type DeployTraceabilityResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         TraceabilitySummary  `json:"summary"`
	ByWorkload      []TraceabilityEntry  `json:"byWorkload"`
	LowTraceability []TraceabilityEntry  `json:"lowTraceability"`
	ByNamespace     []TraceabilityNSStat `json:"byNamespace"`
	Gaps            []TraceabilityGap    `json:"gaps"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// TraceabilitySummary aggregates CI/CD traceability metrics.
type TraceabilitySummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	HasVersionLabel int `json:"hasVersionLabel"` // app.kubernetes.io/version
	HasGitCommit    int `json:"hasGitCommit"`    // git-commit annotation or image digest
	HasBuildTime    int `json:"hasBuildTime"`    // build-timestamp annotation
	HasImageDigest  int `json:"hasImageDigest"`  // image@sha256:...
	HasManagedBy    int `json:"hasManagedBy"`    // app.kubernetes.io/managed-by
	HasPartOf       int `json:"hasPartOf"`       // app.kubernetes.io/part-of
	HasCreatedBy    int `json:"hasCreatedBy"`    // app.kubernetes.io/created-by
	WithFullTrace   int `json:"withFullTrace"`   // has version + git + build + digest
	WithNoTrace     int `json:"withNoTrace"`     // no traceability metadata at all
	Deployments     int `json:"deployments"`
	StatefulSets    int `json:"statefulSets"`
	DaemonSets      int `json:"daemonSets"`
}

// TraceabilityEntry per-workload traceability assessment.
type TraceabilityEntry struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Kind          string   `json:"kind"` // Deployment, StatefulSet, DaemonSet
	Version       string   `json:"version,omitempty"`
	GitCommit     string   `json:"gitCommit,omitempty"`
	BuildTime     string   `json:"buildTime,omitempty"`
	ImageDigest   bool     `json:"imageDigest"`
	ManagedBy     string   `json:"managedBy,omitempty"`
	PartOf        string   `json:"partOf,omitempty"`
	CreatedBy     string   `json:"createdBy,omitempty"`
	Score         int      `json:"score"` // 0-100
	MissingFields []string `json:"missingFields,omitempty"`
}

// TraceabilityNSStat per-namespace traceability stats.
type TraceabilityNSStat struct {
	Namespace      string `json:"namespace"`
	TotalWorkloads int    `json:"totalWorkloads"`
	WithFullTrace  int    `json:"withFullTrace"`
	WithNoTrace    int    `json:"withNoTrace"`
	AvgScore       int    `json:"avgScore"`
	RiskLevel      string `json:"riskLevel"`
}

// TraceabilityGap describes a traceability gap.
type TraceabilityGap struct {
	Namespace string `json:"namespace,omitempty"`
	Workload  string `json:"workload,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleDeployTraceability audits deployment reproducibility & CI/CD traceability.
// GET /api/deployment/traceability
func (s *Server) handleDeployTraceability(w http.ResponseWriter, r *http.Request) {
	result := DeployTraceabilityResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nsStats := map[string]*TraceabilityNSStat{}

	// Process a single workload and add to result
	processWorkload := func(name, ns, kind string, labels, annotations map[string]string, containers []corev1.Container) {
		result.Summary.TotalWorkloads++
		if nsStats[ns] == nil {
			nsStats[ns] = &TraceabilityNSStat{Namespace: ns, RiskLevel: "low"}
		}
		nsStats[ns].TotalWorkloads++

		entry := TraceabilityEntry{
			Name:      name,
			Namespace: ns,
			Kind:      kind,
			Score:     0,
		}

		missing := []string{}

		// Check standard Kubernetes recommended labels
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
		if v, ok := labels["app.kubernetes.io/version"]; ok && v != "" {
			entry.Version = v
			result.Summary.HasVersionLabel++
			entry.Score += 20
		} else {
			missing = append(missing, "version")
		}

		if v, ok := labels["app.kubernetes.io/managed-by"]; ok && v != "" {
			entry.ManagedBy = v
			result.Summary.HasManagedBy++
			entry.Score += 15
		} else {
			missing = append(missing, "managed-by")
		}

		if v, ok := labels["app.kubernetes.io/part-of"]; ok && v != "" {
			entry.PartOf = v
			result.Summary.HasPartOf++
			entry.Score += 10
		} else {
			missing = append(missing, "part-of")
		}

		if v, ok := labels["app.kubernetes.io/created-by"]; ok && v != "" {
			entry.CreatedBy = v
			result.Summary.HasCreatedBy++
			entry.Score += 5
		}

		// Check CI/CD annotations
		if v, ok := annotations["app.kubernetes.io/git-commit"]; ok && v != "" {
			entry.GitCommit = v
			result.Summary.HasGitCommit++
			entry.Score += 20
		} else if v, ok := annotations["git-commit"]; ok && v != "" {
			entry.GitCommit = v
			result.Summary.HasGitCommit++
			entry.Score += 20
		} else if v, ok := annotations["revision"]; ok && v != "" {
			entry.GitCommit = v
			result.Summary.HasGitCommit++
			entry.Score += 15
		} else {
			missing = append(missing, "git-commit")
		}

		if v, ok := annotations["app.kubernetes.io/build-time"]; ok && v != "" {
			entry.BuildTime = v
			result.Summary.HasBuildTime++
			entry.Score += 15
		} else if v, ok := annotations["build-timestamp"]; ok && v != "" {
			entry.BuildTime = v
			result.Summary.HasBuildTime++
			entry.Score += 15
		} else if v, ok := annotations["build-time"]; ok && v != "" {
			entry.BuildTime = v
			result.Summary.HasBuildTime++
			entry.Score += 15
		} else {
			missing = append(missing, "build-time")
		}

		// Check image digest pinning
		hasDigest := false
		for _, c := range containers {
			if strings.Contains(c.Image, "@sha256:") {
				hasDigest = true
				break
			}
		}
		if hasDigest {
			entry.ImageDigest = true
			result.Summary.HasImageDigest++
			entry.Score += 15
		} else {
			missing = append(missing, "image-digest")
		}

		entry.MissingFields = missing

		// Full trace = version + git + build + digest
		if entry.Version != "" && entry.GitCommit != "" && entry.BuildTime != "" && entry.ImageDigest {
			result.Summary.WithFullTrace++
			nsStats[ns].WithFullTrace++
		}

		// No trace at all
		if entry.Score == 0 {
			result.Summary.WithNoTrace++
			nsStats[ns].WithNoTrace++
			result.Gaps = append(result.Gaps, TraceabilityGap{
				Namespace: ns,
				Workload:  fmt.Sprintf("%s/%s", kind, name),
				Issue:     "No CI/CD traceability metadata — cannot trace deployment to source code or build",
				Severity:  "high",
			})
		} else if entry.Score < 50 {
			result.LowTraceability = append(result.LowTraceability, entry)
			result.Gaps = append(result.Gaps, TraceabilityGap{
				Namespace: ns,
				Workload:  fmt.Sprintf("%s/%s", kind, name),
				Issue:     fmt.Sprintf("Low traceability (score %d) — missing: %s", entry.Score, strings.Join(missing, ", ")),
				Severity:  "warning",
			})
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// 1. Process Deployments
	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			result.Summary.Deployments++
			processWorkload(dep.Name, dep.Namespace, "Deployment", dep.Spec.Template.Labels, dep.Spec.Template.Annotations, dep.Spec.Template.Spec.Containers)
		}
	}

	// 2. Process StatefulSets
	statefulsets, err := rc.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range statefulsets.Items {
			result.Summary.StatefulSets++
			processWorkload(sts.Name, sts.Namespace, "StatefulSet", sts.Spec.Template.Labels, sts.Spec.Template.Annotations, sts.Spec.Template.Spec.Containers)
		}
	}

	// 3. Process DaemonSets
	daemonsets, err := rc.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range daemonsets.Items {
			result.Summary.DaemonSets++
			processWorkload(ds.Name, ds.Namespace, "DaemonSet", ds.Spec.Template.Labels, ds.Spec.Template.Annotations, ds.Spec.Template.Spec.Containers)
		}
	}

	// 4. Calculate namespace stats
	totalScore := 0
	for _, stat := range nsStats {
		nsTotalScore := 0
		nsWorkloadCount := 0
		for _, entry := range result.ByWorkload {
			if entry.Namespace == stat.Namespace {
				nsTotalScore += entry.Score
				nsWorkloadCount++
			}
		}
		if nsWorkloadCount > 0 {
			stat.AvgScore = nsTotalScore / nsWorkloadCount
			totalScore += stat.AvgScore
		}
		if stat.WithNoTrace > 0 {
			stat.RiskLevel = "high"
		} else if stat.AvgScore < 50 {
			stat.RiskLevel = "medium"
		} else {
			stat.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].AvgScore < result.ByNamespace[j].AvgScore
	})

	// Sort workloads by score (lowest first)
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].Score < result.ByWorkload[j].Score
	})

	// 5. Calculate health score
	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = totalScore / len(nsStats)
	} else {
		result.HealthScore = 100
	}

	// 6. Recommendations
	if result.Summary.WithNoTrace > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d workload(s) have no CI/CD traceability metadata — add version labels, git-commit, and build-time annotations", result.Summary.WithNoTrace))
	}
	if result.Summary.HasImageDigest == 0 && result.Summary.TotalWorkloads > 0 {
		result.Recommendations = append(result.Recommendations,
			"No workloads use image digest pinning — use image@sha256:... for reproducible deployments")
	} else if result.Summary.HasImageDigest < result.Summary.TotalWorkloads {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d of %d workload(s) use image digests — pin all images with @sha256 for immutability", result.Summary.HasImageDigest, result.Summary.TotalWorkloads))
	}
	if result.Summary.WithFullTrace < result.Summary.TotalWorkloads {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d of %d workload(s) have full CI/CD traceability — add missing metadata for production readiness", result.Summary.WithFullTrace, result.Summary.TotalWorkloads))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"All workloads have full CI/CD traceability metadata — deployments are reproducible and traceable")
	}

	writeJSON(w, result)
}
