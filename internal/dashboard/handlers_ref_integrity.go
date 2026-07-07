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

// RIResult is the secret/configmap reference integrity analysis.
type RIResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         RISummary `json:"summary"`
	BrokenRefs      []RIEntry `json:"brokenRefs"`   // references to non-existent resources
	OptionalRefs    []RIEntry `json:"optionalRefs"` // optional=true references (may be missing)
	ByWorkload      []RIEntry `json:"byWorkload"`
	Issues          []RIIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// RISummary aggregates reference integrity statistics.
type RISummary struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	TotalRefs          int `json:"totalRefs"`    // total secret/configmap references
	BrokenRefs         int `json:"brokenRefs"`   // references to non-existent resources
	OptionalRefs       int `json:"optionalRefs"` // optional=true references
	WorkloadsWithIssue int `json:"workloadsWithIssue"`
	IntegrityScore     int `json:"integrityScore"` // 0-100
}

// RIEntry describes one reference and its status.
type RIEntry struct {
	Workload   string   `json:"workload"`
	Namespace  string   `json:"namespace"`
	Kind       string   `json:"kind"`    // Deployment / StatefulSet / DaemonSet
	RefType    string   `json:"refType"` // Secret / ConfigMap
	RefName    string   `json:"refName"`
	RefSource  string   `json:"refSource"` // envFrom / env / volume
	Optional   bool     `json:"optional"`
	Exists     bool     `json:"exists"`
	Containers []string `json:"containers,omitempty"` // which containers reference it
	RiskLevel  string   `json:"riskLevel"`
}

// RIIssue is a detected reference problem.
type RIIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleRefIntegrity checks Secret/ConfigMap reference integrity for all workloads.
// GET /api/deployment/ref-integrity
func (s *Server) handleRefIntegrity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	// Build existing ConfigMap and Secret name sets per namespace
	cmNames := make(map[string]map[string]bool) // ns → name → true
	secretNames := make(map[string]map[string]bool)

	cms, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, cm := range cms.Items {
			if cmNames[cm.Namespace] == nil {
				cmNames[cm.Namespace] = make(map[string]bool)
			}
			cmNames[cm.Namespace][cm.Name] = true
		}
	}

	secrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, sec := range secrets.Items {
			if secretNames[sec.Namespace] == nil {
				secretNames[sec.Namespace] = make(map[string]bool)
			}
			secretNames[sec.Namespace][sec.Name] = true
		}
	}

	// Helper to check existence
	cmExists := func(ns, name string) bool {
		return cmNames[ns] != nil && cmNames[ns][name]
	}
	secretExists := func(ns, name string) bool {
		return secretNames[ns] != nil && secretNames[ns][name]
	}

	result := RIResult{ScannedAt: time.Now()}
	processedWorkloads := make(map[string]bool)

	// Check Deployments
	deployments, _ := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if deployments != nil {
		for _, dep := range deployments.Items {
			wlKey := dep.Namespace + "/" + dep.Name
			if processedWorkloads[wlKey] {
				continue
			}
			processedWorkloads[wlKey] = true
			result.Summary.TotalWorkloads++
			riCheckPodSpec(dep.Name, dep.Namespace, "Deployment", dep.Spec.Template.Spec, cmExists, secretExists, &result)
		}
	}

	// Check StatefulSets
	stsList, _ := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if stsList != nil {
		for _, sts := range stsList.Items {
			wlKey := sts.Namespace + "/" + sts.Name
			if processedWorkloads[wlKey] {
				continue
			}
			processedWorkloads[wlKey] = true
			result.Summary.TotalWorkloads++
			riCheckPodSpec(sts.Name, sts.Namespace, "StatefulSet", sts.Spec.Template.Spec, cmExists, secretExists, &result)
		}
	}

	// Check DaemonSets
	dsList, _ := rc.clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if dsList != nil {
		for _, ds := range dsList.Items {
			wlKey := ds.Namespace + "/" + ds.Name
			if processedWorkloads[wlKey] {
				continue
			}
			processedWorkloads[wlKey] = true
			result.Summary.TotalWorkloads++
			riCheckPodSpec(ds.Name, ds.Namespace, "DaemonSet", ds.Spec.Template.Spec, cmExists, secretExists, &result)
		}
	}

	// Sort
	sort.Slice(result.BrokenRefs, func(i, j int) bool {
		return result.BrokenRefs[i].Workload < result.BrokenRefs[j].Workload
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return riIssueRank(result.Issues[i].Severity) < riIssueRank(result.Issues[j].Severity)
	})

	result.Summary.IntegrityScore = riScore(result.Summary)
	result.Recommendations = riGenRecs(result.Summary, result.BrokenRefs)

	writeJSON(w, result)
}

// riCheckPodSpec inspects a pod spec for broken Secret/ConfigMap references.
func riCheckPodSpec(name, namespace, kind string, spec corev1.PodSpec, cmExists, secretExists func(ns, name string) bool, result *RIResult) {
	hasIssue := false

	// Check volumes
	for _, vol := range spec.Volumes {
		if vol.ConfigMap != nil {
			result.Summary.TotalRefs++
			entry := RIEntry{
				Workload: name, Namespace: namespace, Kind: kind,
				RefType: "ConfigMap", RefName: vol.ConfigMap.Name, RefSource: "volume",
				Optional: vol.ConfigMap.Optional != nil && *vol.ConfigMap.Optional,
				Exists:   cmExists(namespace, vol.ConfigMap.Name),
			}
			entry.RiskLevel = riAssessRisk(entry)
			if !entry.Exists {
				if entry.Optional {
					result.Summary.OptionalRefs++
					result.OptionalRefs = append(result.OptionalRefs, entry)
				} else {
					result.Summary.BrokenRefs++
					result.BrokenRefs = append(result.BrokenRefs, entry)
					hasIssue = true
					result.Issues = append(result.Issues, RIIssue{
						Severity: "critical", Type: "missing-configmap-volume",
						Resource: fmt.Sprintf("%s/%s", namespace, name),
						Message:  fmt.Sprintf("%s %s/%s references ConfigMap '%s' in volume '%s' but it does not exist — pod will fail to start", kind, namespace, name, vol.ConfigMap.Name, vol.Name),
					})
				}
			}
			result.ByWorkload = append(result.ByWorkload, entry)
		}
		if vol.Secret != nil {
			result.Summary.TotalRefs++
			entry := RIEntry{
				Workload: name, Namespace: namespace, Kind: kind,
				RefType: "Secret", RefName: vol.Secret.SecretName, RefSource: "volume",
				Optional: vol.Secret.Optional != nil && *vol.Secret.Optional,
				Exists:   secretExists(namespace, vol.Secret.SecretName),
			}
			entry.RiskLevel = riAssessRisk(entry)
			if !entry.Exists {
				if entry.Optional {
					result.Summary.OptionalRefs++
					result.OptionalRefs = append(result.OptionalRefs, entry)
				} else {
					result.Summary.BrokenRefs++
					result.BrokenRefs = append(result.BrokenRefs, entry)
					hasIssue = true
					result.Issues = append(result.Issues, RIIssue{
						Severity: "critical", Type: "missing-secret-volume",
						Resource: fmt.Sprintf("%s/%s", namespace, name),
						Message:  fmt.Sprintf("%s %s/%s references Secret '%s' in volume '%s' but it does not exist — pod will fail to start", kind, namespace, name, vol.Secret.SecretName, vol.Name),
					})
				}
			}
			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	// Check envFrom
	for _, c := range spec.Containers {
		for _, from := range c.EnvFrom {
			if from.ConfigMapRef != nil {
				result.Summary.TotalRefs++
				entry := RIEntry{
					Workload: name, Namespace: namespace, Kind: kind,
					RefType: "ConfigMap", RefName: from.ConfigMapRef.Name, RefSource: "envFrom",
					Optional:   from.ConfigMapRef.Optional != nil && *from.ConfigMapRef.Optional,
					Exists:     cmExists(namespace, from.ConfigMapRef.Name),
					Containers: []string{c.Name},
				}
				entry.RiskLevel = riAssessRisk(entry)
				if !entry.Exists {
					if entry.Optional {
						result.Summary.OptionalRefs++
						result.OptionalRefs = append(result.OptionalRefs, entry)
					} else {
						result.Summary.BrokenRefs++
						result.BrokenRefs = append(result.BrokenRefs, entry)
						hasIssue = true
						result.Issues = append(result.Issues, RIIssue{
							Severity: "critical", Type: "missing-configmap-envfrom",
							Resource: fmt.Sprintf("%s/%s", namespace, name),
							Message:  fmt.Sprintf("%s %s/%s container '%s' envFrom ConfigMap '%s' does not exist — pod will fail to start", kind, namespace, name, c.Name, from.ConfigMapRef.Name),
						})
					}
				}
				result.ByWorkload = append(result.ByWorkload, entry)
			}
			if from.SecretRef != nil {
				result.Summary.TotalRefs++
				entry := RIEntry{
					Workload: name, Namespace: namespace, Kind: kind,
					RefType: "Secret", RefName: from.SecretRef.Name, RefSource: "envFrom",
					Optional:   from.SecretRef.Optional != nil && *from.SecretRef.Optional,
					Exists:     secretExists(namespace, from.SecretRef.Name),
					Containers: []string{c.Name},
				}
				entry.RiskLevel = riAssessRisk(entry)
				if !entry.Exists {
					if entry.Optional {
						result.Summary.OptionalRefs++
						result.OptionalRefs = append(result.OptionalRefs, entry)
					} else {
						result.Summary.BrokenRefs++
						result.BrokenRefs = append(result.BrokenRefs, entry)
						hasIssue = true
						result.Issues = append(result.Issues, RIIssue{
							Severity: "critical", Type: "missing-secret-envfrom",
							Resource: fmt.Sprintf("%s/%s", namespace, name),
							Message:  fmt.Sprintf("%s %s/%s container '%s' envFrom Secret '%s' does not exist — pod will fail to start", kind, namespace, name, c.Name, from.SecretRef.Name),
						})
					}
				}
				result.ByWorkload = append(result.ByWorkload, entry)
			}
		}

		// Check env valueFrom
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil && env.ValueFrom.ConfigMapKeyRef.Name != "" {
				result.Summary.TotalRefs++
				entry := RIEntry{
					Workload: name, Namespace: namespace, Kind: kind,
					RefType: "ConfigMap", RefName: env.ValueFrom.ConfigMapKeyRef.Name, RefSource: "env",
					Optional:   env.ValueFrom.ConfigMapKeyRef.Optional != nil && *env.ValueFrom.ConfigMapKeyRef.Optional,
					Exists:     cmExists(namespace, env.ValueFrom.ConfigMapKeyRef.Name),
					Containers: []string{c.Name},
				}
				entry.RiskLevel = riAssessRisk(entry)
				if !entry.Exists && !entry.Optional {
					result.Summary.BrokenRefs++
					result.BrokenRefs = append(result.BrokenRefs, entry)
					hasIssue = true
					result.Issues = append(result.Issues, RIIssue{
						Severity: "critical", Type: "missing-configmap-env",
						Resource: fmt.Sprintf("%s/%s", namespace, name),
						Message:  fmt.Sprintf("%s %s/%s container '%s' env var references ConfigMap '%s' which does not exist", kind, namespace, name, c.Name, env.ValueFrom.ConfigMapKeyRef.Name),
					})
				}
				result.ByWorkload = append(result.ByWorkload, entry)
			}
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
				result.Summary.TotalRefs++
				entry := RIEntry{
					Workload: name, Namespace: namespace, Kind: kind,
					RefType: "Secret", RefName: env.ValueFrom.SecretKeyRef.Name, RefSource: "env",
					Optional:   env.ValueFrom.SecretKeyRef.Optional != nil && *env.ValueFrom.SecretKeyRef.Optional,
					Exists:     secretExists(namespace, env.ValueFrom.SecretKeyRef.Name),
					Containers: []string{c.Name},
				}
				entry.RiskLevel = riAssessRisk(entry)
				if !entry.Exists && !entry.Optional {
					result.Summary.BrokenRefs++
					result.BrokenRefs = append(result.BrokenRefs, entry)
					hasIssue = true
					result.Issues = append(result.Issues, RIIssue{
						Severity: "critical", Type: "missing-secret-env",
						Resource: fmt.Sprintf("%s/%s", namespace, name),
						Message:  fmt.Sprintf("%s %s/%s container '%s' env var references Secret '%s' which does not exist", kind, namespace, name, c.Name, env.ValueFrom.SecretKeyRef.Name),
					})
				}
				result.ByWorkload = append(result.ByWorkload, entry)
			}
		}
	}

	if hasIssue {
		result.Summary.WorkloadsWithIssue++
	}
}

// riAssessRisk determines risk level.
func riAssessRisk(entry RIEntry) string {
	if !entry.Exists && !entry.Optional {
		return "critical"
	}
	if !entry.Exists && entry.Optional {
		return "low"
	}
	return "low"
}

// riScore computes 0-100.
func riScore(s RISummary) int {
	if s.TotalRefs == 0 {
		return 100
	}
	score := 100
	score -= s.BrokenRefs * 15
	if score < 0 {
		score = 0
	}
	return score
}

// riGenRecs produces actionable advice.
func riGenRecs(s RISummary, broken []RIEntry) []string {
	var recs []string

	if s.BrokenRefs > 0 {
		top := ""
		if len(broken) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %s '%s')", broken[0].Namespace, broken[0].Workload, broken[0].RefType, broken[0].RefName)
		}
		recs = append(recs, fmt.Sprintf("%d broken reference(s) found%s — create the missing resources or fix the references", s.BrokenRefs, top))
	}
	if s.OptionalRefs > 0 {
		recs = append(recs, fmt.Sprintf("%d optional reference(s) are missing — verify this is intentional (optional: true)", s.OptionalRefs))
	}
	if s.WorkloadsWithIssue > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have broken references — these will CrashLoopBackOff on deploy", s.WorkloadsWithIssue))
	}
	if s.BrokenRefs == 0 {
		recs = append(recs, "All Secret/ConfigMap references are valid — good deployment integrity")
	}

	return recs
}

func riIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// Suppress unused import warning
var _ = strings.Contains
