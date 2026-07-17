package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvLeakScannerResult scans all container environment variables for sensitive
// data leaked in plaintext: passwords, tokens, API keys, private keys.
// Provides per-workload findings with remediation commands.
type EnvLeakScannerResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         EnvLeakSummary       `json:"summary"`
	Findings        []EnvLeakFinding     `json:"findings"`
	ByNamespace     []EnvLeakNS          `json:"byNamespace"`
	TopPatterns     []EnvLeakPatternStat `json:"topPatterns"`
	Severity        string               `json:"overallSeverity"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type EnvLeakSummary struct {
	TotalContainers int `json:"totalContainers"`
	WithEnvVars     int `json:"withEnvVars"`
	PlaintextLeaks  int `json:"plaintextLeaks"`
	HighRisk        int `json:"highRisk"`
	MediumRisk      int `json:"mediumRisk"`
	LowRisk         int `json:"lowRisk"`
	SafeRef         int `json:"safeSecretRef"`
}

type EnvLeakFinding struct {
	Workload   string `json:"workload"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	EnvVar     string `json:"envVar"`
	Value      string `json:"valuePreview"` // masked, first 3 chars only
	Pattern    string `json:"pattern"`
	Severity   string `json:"severity"`
	UsesSecret bool   `json:"usesSecretRef"`
	FixCommand string `json:"fixCommand"`
}

type EnvLeakNS struct {
	Namespace string `json:"namespace"`
	Leaks     int    `json:"leakCount"`
	Workloads int    `json:"workloads"`
}

type EnvLeakPatternStat struct {
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
	Example string `json:"exampleVarName"`
}

// handleEnvLeakScanner handles GET /api/security/env-leak-scanner
func (s *Server) handleEnvLeakScanner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EnvLeakScannerResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	var findings []EnvLeakFinding
	patternCounts := make(map[string]int)
	patternExamples := make(map[string]string)
	nsMap := make(map[string]*EnvLeakNS)

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if _, ok := nsMap[d.Namespace]; !ok {
			nsMap[d.Namespace] = &EnvLeakNS{Namespace: d.Namespace}
		}
		nsMap[d.Namespace].Workloads++

		for _, c := range d.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			if len(c.Env) == 0 && len(c.EnvFrom) == 0 {
				continue
			}
			result.Summary.WithEnvVars++

			for _, env := range c.Env {
				if env.ValueFrom != nil {
					// Secret/ConfigMap reference is safe
					if env.ValueFrom.SecretKeyRef != nil {
						result.Summary.SafeRef++
					}
					continue
				}

				// Plaintext value - check if sensitive
				if env.Value == "" {
					continue
				}

				pattern := classifyEnvLeak(env.Name, env.Value)
				if pattern == "" {
					continue // not sensitive
				}

				severity := "medium"
				if pattern == "private-key" || pattern == "connection-string" {
					severity = "high"
				} else if pattern == "generic-secret" {
					severity = "low"
				}

				preview := maskValue(env.Value)

				finding := EnvLeakFinding{
					Workload: d.Name, Namespace: d.Namespace,
					Container: c.Name, EnvVar: env.Name,
					Value: preview, Pattern: pattern,
					Severity: severity, UsesSecret: false,
					FixCommand: fmt.Sprintf(
						"kubectl create secret generic %s-env --from-literal=%s=%s -n %s && "+
							"kubectl patch deployment %s -n %s --type=strategic -p "+
							"'{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"%s\",\"env\":[{\"name\":\"%s\",\"valueFrom\":{\"secretKeyRef\":{\"name\":\"%s-env\",\"key\":\"%s\"}}}]}}]}}}}'",
						d.Name, env.Name, "***", d.Namespace,
						d.Name, d.Namespace, c.Name, env.Name, d.Name, env.Name),
				}

				findings = append(findings, finding)
				result.Summary.PlaintextLeaks++
				nsMap[d.Namespace].Leaks++

				switch severity {
				case "high":
					result.Summary.HighRisk++
				case "medium":
					result.Summary.MediumRisk++
				default:
					result.Summary.LowRisk++
				}

				patternCounts[pattern]++
				if _, ok := patternExamples[pattern]; !ok {
					patternExamples[pattern] = env.Name
				}
			}
		}
	}

	// Top patterns
	for p, c := range patternCounts {
		result.TopPatterns = append(result.TopPatterns, EnvLeakPatternStat{
			Pattern: p, Count: c, Example: patternExamples[p],
		})
	}
	sort.Slice(result.TopPatterns, func(i, j int) bool {
		return result.TopPatterns[i].Count > result.TopPatterns[j].Count
	})

	// NS breakdown
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Leaks > result.ByNamespace[j].Leaks
	})

	sort.Slice(findings, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[findings[i].Severity] < sevOrder[findings[j].Severity]
	})
	result.Findings = findings

	// Score
	if result.Summary.TotalContainers > 0 {
		result.HealthScore = 100 - result.Summary.PlaintextLeaks*5
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
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

	if result.Summary.HighRisk > 5 {
		result.Severity = "critical"
	} else if result.Summary.HighRisk > 0 {
		result.Severity = "high"
	} else if result.Summary.PlaintextLeaks > 0 {
		result.Severity = "medium"
	} else {
		result.Severity = "low"
	}

	result.Recommendations = buildEnvLeakRecs(&result)
	writeJSON(w, result)
}

func classifyEnvLeak(name, value string) string {
	// Check variable name for sensitive keywords
	sensitiveNames := map[string]string{
		"password": "password", "passwd": "password", "pwd": "password",
		"secret": "generic-secret", "token": "token",
		"api_key": "api-key", "apikey": "api-key", "access_key": "api-key",
		"private_key": "private-key", "privatekey": "private-key",
		"credential": "credential",
	}
	for keyword, pattern := range sensitiveNames {
		if envContainsLower(name, keyword) {
			return pattern
		}
	}

	// Check value patterns
	if len(value) > 20 && (envContainsLower(value, "begin") && envContainsLower(value, "private")) {
		return "private-key"
	}
	if envContainsLower(value, "mongodb://") || envContainsLower(value, "postgres://") || envContainsLower(value, "mysql://") {
		if envContainsLower(value, "password=") || envContainsLower(value, ":") {
			return "connection-string"
		}
	}
	return ""
}

func envContainsLower(s, substr string) bool {
	return len(s) >= len(substr) && envFindLower(s, substr)
}

func envFindLower(s, substr string) bool {
	sLower := ""
	subLower := ""
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			sLower += string(c + 32)
		} else {
			sLower += string(c)
		}
	}
	for _, c := range substr {
		if c >= 'A' && c <= 'Z' {
			subLower += string(c + 32)
		} else {
			subLower += string(c)
		}
	}
	for i := 0; i <= len(sLower)-len(subLower); i++ {
		if sLower[i:i+len(subLower)] == subLower {
			return true
		}
	}
	return false
}

func maskValue(val string) string {
	if len(val) <= 3 {
		return "***"
	}
	return val[:3] + "***"
}

func buildEnvLeakRecs(r *EnvLeakScannerResult) []string {
	recs := []string{}
	if r.Summary.PlaintextLeaks == 0 {
		recs = append(recs, "未检测到明文敏感环境变量")
		return recs
	}
	recs = append(recs, fmt.Sprintf("发现 %d 个明文敏感环境变量，应迁移到 Secret", r.Summary.PlaintextLeaks))
	if r.Summary.HighRisk > 0 {
		recs = append(recs, fmt.Sprintf("%d 个高风险泄露（私钥/连接字符串），需立即修复", r.Summary.HighRisk))
	}
	if len(r.TopPatterns) > 0 {
		top := r.TopPatterns[0]
		recs = append(recs, fmt.Sprintf("最常见模式: %s (%d 个，例如 %s)", top.Pattern, top.Count, top.Example))
	}
	recs = append(recs, "使用 finding 中的 fixCommand 批量迁移到 Secret")
	return recs
}

var _ corev1.EnvVar
