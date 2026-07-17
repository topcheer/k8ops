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

// EnvVarAuditResult audits environment variables across all workloads for
// security risks (secrets in env), consistency issues, missing best-practice
// vars, and configuration sprawl.
type EnvVarAuditResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         EnvAuditSummary `json:"summary"`
	Risks           []EnvVarRisk    `json:"risks"`
	Sprawl          []EnvVarSprawl  `json:"sprawl"`
	MissingBestPrac []EnvVarMissing `json:"missingBestPractices"`
	ByNamespace     []EnvAuditNS    `json:"byNamespace"`
	TopEnvVars      []EnvVarFreq    `json:"topEnvVars"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type EnvAuditSummary struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	TotalEnvVars     int     `json:"totalEnvVars"`
	SecretInEnv      int     `json:"secretInEnv"`
	PlaintextSecrets int     `json:"plaintextSecrets"`
	ConfigMapRefs    int     `json:"configMapRefs"`
	SecretRefs       int     `json:"secretRefs"`
	DuplicateKeys    int     `json:"duplicateKeys"`
	AvgVarsPerPod    float64 `json:"avgVarsPerPod"`
}

type EnvVarRisk struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	VarName   string `json:"varName"`
	RiskType  string `json:"riskType"` // plaintext-secret, sensitive-name, hardcoded-url
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type EnvVarSprawl struct {
	Namespace string   `json:"namespace"`
	VarName   string   `json:"varName"`
	Count     int      `json:"count"`
	Values    []string `json:"values"`
}

type EnvVarMissing struct {
	VarName string `json:"varName"`
	Reason  string `json:"reason"`
	Missing int    `json:"missingCount"`
}

type EnvAuditNS struct {
	Namespace string  `json:"namespace"`
	Workloads int     `json:"workloads"`
	EnvVars   int     `json:"envVars"`
	Secrets   int     `json:"secretsInEnv"`
	AvgVars   float64 `json:"avgVars"`
}

type EnvVarFreq struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Sensitive env var name patterns
var sensitivePatterns = []string{"password", "passwd", "secret", "token", "key", "credential", "auth", "apikey", "api_key"}

// handleEnvVarAudit handles GET /api/product/env-var-audit
func (s *Server) handleEnvVarAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EnvVarAuditResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})

	// Sensitive value patterns (values that look like secrets)
	nsStats := map[string]*EnvAuditNS{}
	varFreq := map[string]int{}
	totalVars := 0
	totalWorkloads := 0

	analyzePodSpec := func(name, ns, kind string, spec corev1.PodSpec) {
		if isSystemNamespace(ns) {
			return
		}
		totalWorkloads++
		if nsStats[ns] == nil {
			nsStats[ns] = &EnvAuditNS{Namespace: ns}
		}
		nsStats[ns].Workloads++

		for _, c := range spec.Containers {
			// Direct env vars
			for _, ev := range c.Env {
				totalVars++
				nsStats[ns].EnvVars++
				varFreq[ev.Name]++

				nameLower := strings.ToLower(ev.Name)

				// Check for sensitive names with plaintext values
				for _, pat := range sensitivePatterns {
					if strings.Contains(nameLower, pat) && ev.Value != "" {
						result.Summary.PlaintextSecrets++
						nsStats[ns].Secrets++
						result.Risks = append(result.Risks, EnvVarRisk{
							Workload: name, Namespace: ns, VarName: ev.Name,
							RiskType: "plaintext-secret", Severity: "critical",
							Detail: fmt.Sprintf("Env var %s appears to contain a plaintext secret — use secretKeyRef instead", ev.Name),
						})
						break
					}
				}

				// Check for hardcoded URLs (should be in ConfigMap)
				if strings.HasPrefix(ev.Value, "http://") || strings.HasPrefix(ev.Value, "https://") {
					result.Risks = append(result.Risks, EnvVarRisk{
						Workload: name, Namespace: ns, VarName: ev.Name,
						RiskType: "hardcoded-url", Severity: "low",
						Detail: fmt.Sprintf("Env var %s contains hardcoded URL — consider ConfigMap", ev.Name),
					})
				}
			}

			// EnvFrom (ConfigMap/Secret refs)
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					result.Summary.SecretRefs++
				}
				if ef.ConfigMapRef != nil {
					result.Summary.ConfigMapRefs++
				}
			}

			// SecretKeyRef in env
			for _, ev := range c.Env {
				if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
					result.Summary.SecretInEnv++
				}
			}
		}
	}

	for _, dep := range deployments.Items {
		analyzePodSpec(dep.Name, dep.Namespace, "Deployment", dep.Spec.Template.Spec)
	}
	for _, sts := range statefulsets.Items {
		analyzePodSpec(sts.Name, sts.Namespace, "StatefulSet", sts.Spec.Template.Spec)
	}
	for _, ds := range daemonsets.Items {
		analyzePodSpec(ds.Name, ds.Namespace, "DaemonSet", ds.Spec.Template.Spec)
	}

	// Summary
	result.Summary.TotalWorkloads = totalWorkloads
	result.Summary.TotalEnvVars = totalVars
	if totalWorkloads > 0 {
		result.Summary.AvgVarsPerPod = float64(totalVars) / float64(totalWorkloads)
	}

	// Detect sprawl: same env var with different values across workloads in same ns
	// Simplified: just flag most common env vars
	for name, count := range varFreq {
		if count > 3 {
			result.TopEnvVars = append(result.TopEnvVars, EnvVarFreq{Name: name, Count: count})
		}
	}
	sort.Slice(result.TopEnvVars, func(i, j int) bool { return result.TopEnvVars[i].Count > result.TopEnvVars[j].Count })
	if len(result.TopEnvVars) > 15 {
		result.TopEnvVars = result.TopEnvVars[:15]
	}

	// Missing best practices
	if totalWorkloads > 5 {
		if varFreq["POD_NAME"] == 0 && varFreq["HOSTNAME"] == 0 {
			result.MissingBestPrac = append(result.MissingBestPrac, EnvVarMissing{
				VarName: "POD_NAME/HOSTNAME", Missing: totalWorkloads,
				Reason: "Downward API for pod identity not configured",
			})
		}
		if varFreq["LOG_LEVEL"] == 0 {
			result.MissingBestPrac = append(result.MissingBestPrac, EnvVarMissing{
				VarName: "LOG_LEVEL", Missing: totalWorkloads,
				Reason: "No centralized log level configuration",
			})
		}
	}

	// NS stats
	for _, ns := range nsStats {
		if ns.Workloads > 0 {
			ns.AvgVars = float64(ns.EnvVars) / float64(ns.Workloads)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool { return result.ByNamespace[i].EnvVars > result.ByNamespace[j].EnvVars })

	// Sort risks by severity
	sort.Slice(result.Risks, func(i, j int) bool {
		if result.Risks[i].Severity != result.Risks[j].Severity {
			return result.Risks[i].Severity == "critical"
		}
		return false
	})
	if len(result.Risks) > 30 {
		result.Risks = result.Risks[:30]
	}

	result.HealthScore = computeEnvAuditScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generateEnvAuditRecs(result)

	writeJSON(w, result)
}

func computeEnvAuditScore(s EnvAuditSummary) int {
	score := 100
	if s.TotalWorkloads == 0 {
		return score
	}
	score -= minInt(s.PlaintextSecrets*5, 40)
	if score < 0 {
		score = 0
	}
	return score
}

func generateEnvAuditRecs(r EnvVarAuditResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Env var audit: %d workloads, %d env vars (%.1f avg), %d plaintext secrets — score %d/100",
		r.Summary.TotalWorkloads, r.Summary.TotalEnvVars, r.Summary.AvgVarsPerPod, r.Summary.PlaintextSecrets, r.HealthScore))
	if r.Summary.PlaintextSecrets > 0 {
		recs = append(recs, fmt.Sprintf("CRITICAL: %d plaintext secret(s) in env vars — migrate to secretKeyRef immediately", r.Summary.PlaintextSecrets))
	}
	for _, risk := range r.Risks {
		if risk.Severity == "critical" {
			recs = append(recs, fmt.Sprintf("%s/%s: %s", risk.Namespace, risk.Workload, risk.Detail))
		}
	}
	if len(r.TopEnvVars) > 5 {
		recs = append(recs, fmt.Sprintf("%d env vars shared across multiple workloads — consider ConfigMap consolidation", len(r.TopEnvVars)))
	}
	return recs
}

var _ appsv1.DeploymentList
