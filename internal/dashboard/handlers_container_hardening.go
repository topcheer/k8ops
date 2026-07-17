package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerHardeningResult scans all containers for security hardening gaps
// and generates strategic patch commands. Covers securityContext fields
// that are commonly missing: runAsNonRoot, readOnlyRootFilesystem, capabilities.
type ContainerHardeningResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         HardeningSummary   `json:"summary"`
	Findings        []HardeningFinding `json:"findings"`
	BatchPatch      []HardeningPatch   `json:"batchPatch"`
	BySeverity      []HardeningSevStat `json:"bySeverity"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type HardeningSummary struct {
	TotalContainers  int `json:"totalContainers"`
	NonRoot          int `json:"runAsNonRoot"`
	ReadOnlyRootFS   int `json:"readOnlyRootFilesystem"`
	DropAllCaps      int `json:"dropAllCapabilities"`
	NoPrivEscalation int `json:"noPrivilegeEscalation"`
	MissingAll       int `json:"missingAllHardening"`
}

type HardeningFinding struct {
	Workload  string   `json:"workload"`
	Namespace string   `json:"namespace"`
	Container string   `json:"container"`
	Issues    []string `json:"issues"`
	Severity  string   `json:"severity"`
	PatchJSON string   `json:"patchJSON"`
}

type HardeningPatch struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Command   string `json:"command"`
}

type HardeningSevStat struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

// handleContainerHardening handles GET /api/security/container-hardening
func (s *Server) handleContainerHardening(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ContainerHardeningResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var findings []HardeningFinding
	var patches []HardeningPatch
	sevMap := make(map[string]int)

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			var issues []string

			sc := c.SecurityContext

			// Check runAsNonRoot
			if sc == nil || sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				issues = append(issues, "missing runAsNonRoot=true")
			} else {
				result.Summary.NonRoot++
			}

			// Check readOnlyRootFilesystem
			if sc == nil || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				issues = append(issues, "missing readOnlyRootFilesystem=true")
			} else {
				result.Summary.ReadOnlyRootFS++
			}

			// Check capabilities drop ALL
			if sc == nil || sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 {
				issues = append(issues, "missing capabilities.drop=ALL")
			} else {
				hasAll := false
				for _, cap := range sc.Capabilities.Drop {
					if string(cap) == "ALL" {
						hasAll = true
						break
					}
				}
				if hasAll {
					result.Summary.DropAllCaps++
				} else {
					issues = append(issues, "capabilities.drop incomplete (need ALL)")
				}
			}

			// Check allowPrivilegeEscalation=false
			if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
				issues = append(issues, "missing allowPrivilegeEscalation=false")
			} else {
				result.Summary.NoPrivEscalation++
			}

			if len(issues) == 0 {
				continue
			}

			if len(issues) == 4 {
				result.Summary.MissingAll++
			}

			severity := "medium"
			if len(issues) >= 3 {
				severity = "high"
			}
			if len(issues) >= 4 {
				severity = "critical"
			}

			sevMap[severity]++

			patchJSON := generateHardeningPatch(c.Name, issues)

			finding := HardeningFinding{
				Workload: d.Name, Namespace: d.Namespace,
				Container: c.Name, Issues: issues,
				Severity: severity, PatchJSON: patchJSON,
			}
			findings = append(findings, finding)

			cmd := fmt.Sprintf("kubectl patch deployment %s -n %s --type=strategic -p '%s'",
				d.Name, d.Namespace, patchJSON)
			patches = append(patches, HardeningPatch{
				Workload: d.Name, Namespace: d.Namespace, Command: cmd,
			})
		}
	}

	// Severity stats
	for sev, count := range sevMap {
		result.BySeverity = append(result.BySeverity, HardeningSevStat{Severity: sev, Count: count})
	}
	sort.Slice(result.BySeverity, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return sevOrder[result.BySeverity[i].Severity] < sevOrder[result.BySeverity[j].Severity]
	})

	sort.Slice(findings, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return sevOrder[findings[i].Severity] < sevOrder[findings[j].Severity]
	})

	result.Findings = findings
	result.BatchPatch = patches

	// Score
	if result.Summary.TotalContainers > 0 {
		fullyHardened := 0
		for _, f := range findings {
			if len(f.Issues) == 0 {
				fullyHardened++
			}
		}
		result.HealthScore = fullyHardened * 100 / result.Summary.TotalContainers
		// Actually calculate from summary
		minScore := result.Summary.TotalContainers - len(findings)
		result.HealthScore = minScore * 100 / result.Summary.TotalContainers
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildContainerHardeningRecs(&result)
	writeJSON(w, result)
}

func generateHardeningPatch(container string, issues []string) string {
	parts := ""
	for _, issue := range issues {
		switch {
		case containsStrSimple(issue, "runAsNonRoot"):
			if parts != "" {
				parts += ","
			}
			parts += `"runAsNonRoot":true`
		case containsStrSimple(issue, "readOnlyRootFilesystem"):
			if parts != "" {
				parts += ","
			}
			parts += `"readOnlyRootFilesystem":true`
		case containsStrSimple(issue, "capabilities.drop"):
			if parts != "" {
				parts += ","
			}
			parts += `"capabilities":{"drop":["ALL"]}`
		case containsStrSimple(issue, "allowPrivilegeEscalation"):
			if parts != "" {
				parts += ","
			}
			parts += `"allowPrivilegeEscalation":false`
		}
	}
	return fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s","securityContext":{%s}}]}}}}`, container, parts)
}

func containsStrSimple(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func buildContainerHardeningRecs(r *ContainerHardeningResult) []string {
	recs := []string{}
	if r.Summary.MissingAll > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器完全没有安全上下文加固", r.Summary.MissingAll))
	}
	recs = append(recs, fmt.Sprintf("runAsNonRoot: %d/%d", r.Summary.NonRoot, r.Summary.TotalContainers))
	recs = append(recs, fmt.Sprintf("readOnlyRootFilesystem: %d/%d", r.Summary.ReadOnlyRootFS, r.Summary.TotalContainers))
	recs = append(recs, fmt.Sprintf("drop ALL capabilities: %d/%d", r.Summary.DropAllCaps, r.Summary.TotalContainers))
	if len(r.BatchPatch) > 0 {
		recs = append(recs, fmt.Sprintf("已生成 %d 个 kubectl patch 命令", len(r.BatchPatch)))
	}
	return recs
}

var _ corev1.SecurityContext
