package dashboard

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.06 — Product Dimension (additional)
// 1. Env Var Secret Leak Detector
// 2. Probe Coverage Gap
// 3. GPU/Accelerator Audit
// ============================================================

// ---------------------------------------------------------------
// 1. Env Var Secret Leak Detector — hardcoded secrets in env vars
// ---------------------------------------------------------------

type EnvSecretLeakResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EnvSecretLeakSummary `json:"summary"`
	LeakedSecrets   []EnvSecretLeakEntry `json:"leakedSecrets"`
	ByNamespace     []EnvSecretLeakNS    `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type EnvSecretLeakSummary struct {
	TotalContainers  int `json:"totalContainers"`
	WithSecretRef    int `json:"withSecretRef"`
	HardcodedSecrets int `json:"hardcodedSecrets"`
	HighRisk         int `json:"highRisk"`
	MediumRisk       int `json:"mediumRisk"`
	LowRisk          int `json:"lowRisk"`
	NamespacesWith   int `json:"namespacesWithLeaks"`
}

type EnvSecretLeakEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Container       string `json:"container"`
	EnvName         string `json:"envName"`
	EnvValuePreview string `json:"envValuePreview"`
	Pattern         string `json:"pattern"`
	RiskLevel       string `json:"riskLevel"`
}

type EnvSecretLeakNS struct {
	Namespace string `json:"namespace"`
	LeakCount int    `json:"leakCount"`
}

// Patterns that indicate hardcoded secrets
var secretPatterns1906 = []struct {
	Name  string
	Regex *regexp.Regexp
	Risk  string
}{
	{"password", regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*\S+`), "high"},
	{"api-key", regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*\S+`), "high"},
	{"token", regexp.MustCompile(`(?i)(token|secret|auth)\s*[=:]\s*\S+`), "high"},
	{"private-key", regexp.MustCompile(`-----BEGIN.*PRIVATE KEY-----`), "critical"},
	{"connection-string", regexp.MustCompile(`(?i)(mongodb|postgres|mysql|redis)://\S+:\S+@`), "high"},
	{"base64-blob", regexp.MustCompile(`^[A-Za-z0-9+/]{40,}={0,2}$`), "medium"},
	{"aws-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "critical"},
}

func (s *Server) handleEnvSecretLeakDetector(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EnvSecretLeakResult{ScannedAt: time.Now()}

	nsLeakCount := map[string]int{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			hasSecretRef := false

			for _, env := range c.Env {
				// Track secretKeyRef usage
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					hasSecretRef = true
					continue
				}
				if env.Value == "" {
					continue
				}

				// Check env name for sensitive keywords
				envNameLower := strings.ToLower(env.Name)
				isSensitiveName := strings.Contains(envNameLower, "password") ||
					strings.Contains(envNameLower, "passwd") ||
					strings.Contains(envNameLower, "token") ||
					strings.Contains(envNameLower, "secret") ||
					strings.Contains(envNameLower, "api_key") ||
					strings.Contains(envNameLower, "apikey") ||
					strings.Contains(envNameLower, "auth")

				// Check value against patterns
				for _, pattern := range secretPatterns1906 {
					if pattern.Regex.MatchString(env.Value) || (isSensitiveName && len(env.Value) > 6) {
						result.Summary.HardcodedSecrets++
						risk := pattern.Risk
						if isSensitiveName && risk == "" {
							risk = "medium"
						}
						preview := env.Value
						if len(preview) > 20 {
							preview = preview[:8] + "..." + preview[len(preview)-4:]
						}
						entry := EnvSecretLeakEntry{
							Name: dep.Name, Namespace: dep.Namespace,
							Container: c.Name, EnvName: env.Name,
							EnvValuePreview: preview,
							Pattern:         pattern.Name,
							RiskLevel:       risk,
						}
						if risk == "" {
							entry.Pattern = "sensitive-name"
							entry.RiskLevel = "medium"
							risk = "medium"
						}
						switch risk {
						case "critical":
							result.Summary.HighRisk++
						case "high":
							result.Summary.HighRisk++
						case "medium":
							result.Summary.MediumRisk++
						default:
							result.Summary.LowRisk++
						}
						result.LeakedSecrets = append(result.LeakedSecrets, entry)
						nsLeakCount[dep.Namespace]++
						break
					}
				}
			}

			if hasSecretRef {
				result.Summary.WithSecretRef++
			}
		}
	}

	for ns, count := range nsLeakCount {
		if count > 0 {
			result.Summary.NamespacesWith++
			result.ByNamespace = append(result.ByNamespace, EnvSecretLeakNS{
				Namespace: ns, LeakCount: count,
			})
		}
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].LeakCount > result.ByNamespace[j].LeakCount
	})

	// Score: fewer hardcoded secrets = better
	if result.Summary.TotalContainers > 0 {
		leakRate := result.Summary.HardcodedSecrets * 100 / result.Summary.TotalContainers
		result.HealthScore = 100 - leakRate
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildEnvSecretLeakRecs1906(&result)
	writeJSON(w, result)
}

func buildEnvSecretLeakRecs1906(r *EnvSecretLeakResult) []string {
	recs := []string{fmt.Sprintf("Env secret leak: %d containers scanned, %d hardcoded secrets found (%d high/critical, %d medium)",
		r.Summary.TotalContainers, r.Summary.HardcodedSecrets, r.Summary.HighRisk, r.Summary.MediumRisk)}
	if r.Summary.HardcodedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d hardcoded secrets detected - migrate to Kubernetes Secrets or external secret stores", r.Summary.HardcodedSecrets))
	}
	if r.Summary.WithSecretRef > 0 {
		recs = append(recs, fmt.Sprintf("%d containers properly use secretKeyRef - ensure remaining follow same pattern", r.Summary.WithSecretRef))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Probe Coverage Gap — deployments missing probes
// ---------------------------------------------------------------

type ProbeCoverageResult struct {
	ScannedAt        time.Time            `json:"scannedAt"`
	HealthScore      int                  `json:"healthScore"`
	Grade            string               `json:"grade"`
	Summary          ProbeCoverageSummary `json:"summary"`
	ByWorkload       []ProbeCoverageEntry `json:"byWorkload"`
	MissingLiveness  []ProbeCoverageEntry `json:"missingLiveness"`
	MissingReadiness []ProbeCoverageEntry `json:"missingReadiness"`
	MissingStartup   []ProbeCoverageEntry `json:"missingStartup"`
	Recommendations  []string             `json:"recommendations"`
}

type ProbeCoverageSummary struct {
	TotalContainers  int `json:"totalContainers"`
	WithLiveness     int `json:"withLiveness"`
	WithoutLiveness  int `json:"withoutLiveness"`
	WithReadiness    int `json:"withReadiness"`
	WithoutReadiness int `json:"withoutReadiness"`
	WithStartup      int `json:"withStartup"`
	WithoutStartup   int `json:"withoutStartup"`
	CriticalMissing  int `json:"criticalMissing"`
}

type ProbeCoverageEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	HasLiveness  bool   `json:"hasLiveness"`
	HasReadiness bool   `json:"hasReadiness"`
	HasStartup   bool   `json:"hasStartup"`
	Replicas     int32  `json:"replicas"`
	RiskLevel    string `json:"riskLevel"`
	Issue        string `json:"issue"`
}

func (s *Server) handleProbeCoverageGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ProbeCoverageResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			entry := ProbeCoverageEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Container: c.Name, Replicas: replicas,
				HasLiveness:  c.LivenessProbe != nil,
				HasReadiness: c.ReadinessProbe != nil,
				HasStartup:   c.StartupProbe != nil,
			}

			var issues []string
			if !entry.HasLiveness {
				result.Summary.WithoutLiveness++
				issues = append(issues, "missing liveness probe")
			} else {
				result.Summary.WithLiveness++
			}
			if !entry.HasReadiness {
				result.Summary.WithoutReadiness++
				issues = append(issues, "missing readiness probe")
			} else {
				result.Summary.WithReadiness++
			}
			if !entry.HasStartup {
				result.Summary.WithoutStartup++
				if replicas >= 3 {
					issues = append(issues, "missing startup probe (high replica count)")
				}
			} else {
				result.Summary.WithStartup++
			}

			if len(issues) > 0 {
				entry.Issue = strings.Join(issues, "; ")
				switch {
				case !entry.HasLiveness && !entry.HasReadiness:
					entry.RiskLevel = "high"
					result.Summary.CriticalMissing++
				case !entry.HasReadiness:
					entry.RiskLevel = "high"
					result.Summary.CriticalMissing++
				case !entry.HasLiveness:
					entry.RiskLevel = "medium"
				default:
					entry.RiskLevel = "low"
				}
				result.ByWorkload = append(result.ByWorkload, entry)
				if !entry.HasLiveness {
					result.MissingLiveness = append(result.MissingLiveness, entry)
				}
				if !entry.HasReadiness {
					result.MissingReadiness = append(result.MissingReadiness, entry)
				}
				if !entry.HasStartup {
					result.MissingStartup = append(result.MissingStartup, entry)
				}
			}
		}
	}

	// Score: probe coverage rate
	if result.Summary.TotalContainers > 0 {
		readyPct := result.Summary.WithReadiness * 100 / result.Summary.TotalContainers
		livePct := result.Summary.WithLiveness * 100 / result.Summary.TotalContainers
		result.HealthScore = (readyPct + livePct) / 2
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildProbeCoverageRecs1906(&result)
	writeJSON(w, result)
}

func buildProbeCoverageRecs1906(r *ProbeCoverageResult) []string {
	recs := []string{fmt.Sprintf("Probe coverage: %d containers, liveness %d/%d, readiness %d/%d, startup %d/%d",
		r.Summary.TotalContainers,
		r.Summary.WithLiveness, r.Summary.TotalContainers,
		r.Summary.WithReadiness, r.Summary.TotalContainers,
		r.Summary.WithStartup, r.Summary.TotalContainers)}
	if r.Summary.CriticalMissing > 0 {
		recs = append(recs, fmt.Sprintf("%d containers with critical probe gaps - add liveness AND readiness probes", r.Summary.CriticalMissing))
	}
	if r.Summary.WithoutReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d containers without readiness probe - traffic may route to unhealthy pods", r.Summary.WithoutReadiness))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. GPU/Accelerator Audit — GPU resource requests & node availability
// ---------------------------------------------------------------

type GPUAuditResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         GPUAuditSummary   `json:"summary"`
	GPURequests     []GPURequestEntry `json:"gpuRequests"`
	NodeGPU         []NodeGPUEntry    `json:"nodeGPU"`
	WorkloadsNoGPU  []string          `json:"workloadsRequestingGPUButUnavailable"`
	Recommendations []string          `json:"recommendations"`
}

type GPUAuditSummary struct {
	TotalNodes       int    `json:"totalNodes"`
	NodesWithGPU     int    `json:"nodesWithGPU"`
	TotalWorkloads   int    `json:"totalWorkloads"`
	WorkloadsWithGPU int    `json:"workloadsWithGPU"`
	GPUAllocated     int    `json:"gpuAllocated"`
	GPUCapacity      int    `json:"gpuCapacity"`
	GPUAvailable     int    `json:"gpuAvailable"`
	GPUVendor        string `json:"gpuVendor"`
}

type GPURequestEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	GPURequest string `json:"gpuRequest"`
	GPUType    string `json:"gpuType"`
	Limited    bool   `json:"hasGPULimit"`
	RiskLevel  string `json:"riskLevel"`
}

type NodeGPUEntry struct {
	Node         string `json:"node"`
	GPUCapacity  int    `json:"gpuCapacity"`
	GPUAllocated int    `json:"gpuAllocated"`
	Zone         string `json:"zone"`
}

func (s *Server) handleGPUAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := GPUAuditResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Check node GPU capacity
	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++

		gpuCap := 0
		gpuVendor := ""
		for k, v := range node.Status.Capacity {
			kl := strings.ToLower(string(k))
			if strings.Contains(kl, "nvidia.com/gpu") || strings.Contains(kl, "amd.com/gpu") || strings.Contains(kl, "gpu") {
				gpuCap += int(v.Value())
				if strings.Contains(kl, "nvidia") {
					gpuVendor = "nvidia"
				} else if strings.Contains(kl, "amd") {
					gpuVendor = "amd"
				}
			}
		}
		if gpuCap > 0 {
			result.Summary.NodesWithGPU++
			result.Summary.GPUCapacity += gpuCap
			zone := node.Labels["topology.kubernetes.io/zone"]
			result.NodeGPU = append(result.NodeGPU, NodeGPUEntry{
				Node: node.Name, GPUCapacity: gpuCap,
				Zone: zone,
			})
			if result.Summary.GPUVendor == "" {
				result.Summary.GPUVendor = gpuVendor
			}
		}
	}

	// Check workload GPU requests
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		for _, c := range dep.Spec.Template.Spec.Containers {
			for k, v := range c.Resources.Requests {
				kl := strings.ToLower(string(k))
				if strings.Contains(kl, "gpu") {
					result.Summary.WorkloadsWithGPU++
					result.Summary.GPUAllocated += int(v.Value())

					gpuType := "unknown"
					if strings.Contains(kl, "nvidia") {
						gpuType = "nvidia"
					} else if strings.Contains(kl, "amd") {
						gpuType = "amd"
					}

					_, hasLimit := c.Resources.Limits[k]
					entry := GPURequestEntry{
						Name: dep.Name, Namespace: dep.Namespace,
						GPURequest: v.String(), GPUType: gpuType,
						Limited:   hasLimit,
						RiskLevel: "low",
					}
					if !hasLimit {
						entry.RiskLevel = "medium"
					}
					if result.Summary.NodesWithGPU == 0 {
						entry.RiskLevel = "critical"
						result.WorkloadsNoGPU = append(result.WorkloadsNoGPU,
							fmt.Sprintf("%s/%s", dep.Namespace, dep.Name))
					}
					result.GPURequests = append(result.GPURequests, entry)
					break
				}
			}
		}
	}

	result.Summary.GPUAvailable = result.Summary.GPUCapacity - result.Summary.GPUAllocated

	// Score
	if result.Summary.WorkloadsWithGPU == 0 {
		result.HealthScore = 100 // No GPU workloads = no GPU issues
	} else if result.Summary.NodesWithGPU == 0 {
		result.HealthScore = 0 // GPU workloads but no GPU nodes = critical
	} else {
		utilPct := result.Summary.GPUAllocated * 100 / result.Summary.GPUCapacity
		if utilPct > 100 {
			result.HealthScore = 0 // Over-subscribed
		} else {
			result.HealthScore = 100 - (100-utilPct)/2
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildGPUAuditRecs1906(&result)
	writeJSON(w, result)
}

func buildGPUAuditRecs1906(r *GPUAuditResult) []string {
	recs := []string{fmt.Sprintf("GPU audit: %d/%d nodes with GPU, %d workloads requesting GPU (%d allocated / %d capacity)",
		r.Summary.NodesWithGPU, r.Summary.TotalNodes,
		r.Summary.WorkloadsWithGPU, r.Summary.GPUAllocated, r.Summary.GPUCapacity)}
	if len(r.WorkloadsNoGPU) > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads requesting GPU but no GPU nodes available - add GPU node pool", len(r.WorkloadsNoGPU)))
	}
	if r.Summary.GPUAvailable < 0 {
		recs = append(recs, "GPU over-subscribed - workloads will be stuck in Pending")
	}
	return recs
}
