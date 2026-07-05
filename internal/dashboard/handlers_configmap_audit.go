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

// ConfigAuditResult is the ConfigMap & Secret configuration audit.
type ConfigAuditResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ConfigAuditSummary  `json:"summary"`
	ConfigMaps      []ConfigMapEntry    `json:"configMaps"`
	Secrets         []SecretEntry       `json:"secrets"`
	Unreferenced    []UnreferencedEntry `json:"unreferenced"`
	LargeConfigs    []ConfigMapEntry    `json:"largeConfigs"`
	Issues          []ConfigIssue       `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

// ConfigAuditSummary aggregates ConfigMap/Secret audit statistics.
type ConfigAuditSummary struct {
	TotalConfigMaps  int `json:"totalConfigMaps"`
	TotalSecrets     int `json:"totalSecrets"`
	UnreferencedCMs  int `json:"unreferencedCMs"`
	UnreferencedSecs int `json:"unreferencedSecrets"`
	LargeCMs         int `json:"largeCMs"`         // >1MB
	PlainTextSecrets int `json:"plainTextSecrets"` // type=Opaque with plaintext-like keys
	EmptyConfigs     int `json:"emptyConfigs"`     // no data keys
	OldSecrets       int `json:"oldSecrets"`       // created >180d ago without rotation
	ImmutableCount   int `json:"immutableCount"`   // immutable=true
	HealthScore      int `json:"healthScore"`      // 0-100
}

// ConfigMapEntry describes one ConfigMap.
type ConfigMapEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	DataKeys     int      `json:"dataKeys"`
	SizeBytes    int      `json:"sizeBytes"`
	IsLarge      bool     `json:"isLarge"`
	IsImmutable  bool     `json:"isImmutable"`
	IsReferenced bool     `json:"isReferenced"`
	AgeDays      int      `json:"ageDays"`
	RiskLevel    string   `json:"riskLevel"`
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

// SecretEntry describes one Secret.
type SecretEntry struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Type          string   `json:"type"`
	DataKeys      int      `json:"dataKeys"`
	SizeBytes     int      `json:"sizeBytes"`
	IsImmutable   bool     `json:"isImmutable"`
	IsReferenced  bool     `json:"isReferenced"`
	AgeDays       int      `json:"ageDays"`
	NeedsRotation bool     `json:"needsRotation"`
	RiskLevel     string   `json:"riskLevel"`
	ReferencedBy  []string `json:"referencedBy,omitempty"`
}

// UnreferencedEntry describes an unreferenced ConfigMap or Secret.
type UnreferencedEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // ConfigMap / Secret
	AgeDays   int    `json:"ageDays"`
	SizeBytes int    `json:"sizeBytes"`
}

// ConfigIssue is a detected configuration problem.
type ConfigIssue struct {
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Message   string `json:"message"`
}

// handleConfigAudit audits ConfigMaps and Secrets for best practices.
// GET /api/product/config-audit
func (s *Server) handleConfigAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	configMaps, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build reference map: what ConfigMaps/Secrets are actually used by pods
	cmRefs := make(map[string]bool) // ns/name → referenced
	secretRefs := make(map[string]bool)
	cmRefBy := make(map[string][]string) // ns/name → []pod names
	secretRefBy := make(map[string][]string)

	for _, pod := range pods.Items {
		podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

		// Volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.ConfigMap.Name)
				cmRefs[key] = true
				cmRefBy[key] = append(cmRefBy[key], podKey)
			}
			if vol.Secret != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
				secretRefs[key] = true
				secretRefBy[key] = append(secretRefBy[key], podKey)
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, src.ConfigMap.Name)
						cmRefs[key] = true
						cmRefBy[key] = append(cmRefBy[key], podKey)
					}
					if src.Secret != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, src.Secret.Name)
						secretRefs[key] = true
						secretRefBy[key] = append(secretRefBy[key], podKey)
					}
				}
			}
		}

		// Env vars
		for _, c := range pod.Spec.Containers {
			for _, ev := range c.Env {
				if ev.ValueFrom != nil {
					if ev.ValueFrom.ConfigMapKeyRef != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, ev.ValueFrom.ConfigMapKeyRef.Name)
						cmRefs[key] = true
						cmRefBy[key] = append(cmRefBy[key], podKey)
					}
					if ev.ValueFrom.SecretKeyRef != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, ev.ValueFrom.SecretKeyRef.Name)
						secretRefs[key] = true
						secretRefBy[key] = append(secretRefBy[key], podKey)
					}
				}
			}
			for _, envFrom := range c.EnvFrom {
				if envFrom.ConfigMapRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, envFrom.ConfigMapRef.Name)
					cmRefs[key] = true
					cmRefBy[key] = append(cmRefBy[key], podKey)
				}
				if envFrom.SecretRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, envFrom.SecretRef.Name)
					secretRefs[key] = true
					secretRefBy[key] = append(secretRefBy[key], podKey)
				}
			}
		}
	}

	result := ConfigAuditResult{ScannedAt: time.Now()}
	now := time.Now()

	// Analyze ConfigMaps
	for _, cm := range configMaps.Items {
		// Skip system ConfigMaps
		if cm.Namespace == "kube-system" && (strings.HasPrefix(cm.Name, "coredns") ||
			strings.HasPrefix(cm.Name, "kubeadm") || strings.HasPrefix(cm.Name, "extension-apiserver")) {
			continue
		}

		entry := ConfigMapEntry{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		}

		// Data keys and size
		size := 0
		for k, v := range cm.Data {
			size += len(k) + len(v)
		}
		for k, v := range cm.BinaryData {
			size += len(k) + len(v)
		}
		entry.DataKeys = len(cm.Data) + len(cm.BinaryData)
		entry.SizeBytes = size
		entry.IsLarge = size > 1024*1024 // 1MB

		// Immutable
		if cm.Immutable != nil && *cm.Immutable {
			entry.IsImmutable = true
			result.Summary.ImmutableCount++
		}

		// Reference check
		key := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
		entry.IsReferenced = cmRefs[key]
		entry.ReferencedBy = cmRefBy[key]

		// Age
		entry.AgeDays = int(now.Sub(cm.CreationTimestamp.Time).Hours() / 24)

		// Empty
		if entry.DataKeys == 0 {
			result.Summary.EmptyConfigs++
		}

		// Risk
		entry.RiskLevel = cmAuditRisk(entry)
		result.Summary.TotalConfigMaps++

		if entry.IsLarge {
			result.Summary.LargeCMs++
			result.LargeConfigs = append(result.LargeConfigs, entry)
			result.Issues = append(result.Issues, ConfigIssue{
				Severity:  "warning",
				Type:      "large-configmap",
				Namespace: cm.Namespace,
				Resource:  cm.Name,
				Message:   fmt.Sprintf("ConfigMap %s/%s is %.1fKB — large ConfigMaps slow down API and consume etcd space", cm.Namespace, cm.Name, float64(size)/1024),
			})
		}

		if !entry.IsReferenced {
			result.Summary.UnreferencedCMs++
			result.Unreferenced = append(result.Unreferenced, UnreferencedEntry{
				Name: cm.Name, Namespace: cm.Namespace, Kind: "ConfigMap",
				AgeDays: entry.AgeDays, SizeBytes: size,
			})
			result.Issues = append(result.Issues, ConfigIssue{
				Severity:  "info",
				Type:      "unreferenced-configmap",
				Namespace: cm.Namespace,
				Resource:  cm.Name,
				Message:   fmt.Sprintf("ConfigMap %s/%s is not referenced by any pod — candidate for cleanup", cm.Namespace, cm.Name),
			})
		}

		result.ConfigMaps = append(result.ConfigMaps, entry)
	}

	// Analyze Secrets
	for _, sec := range secrets.Items {
		// Skip system service-account tokens
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}

		entry := SecretEntry{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Type:      string(sec.Type),
		}

		size := 0
		for k, v := range sec.Data {
			size += len(k) + len(v)
		}
		entry.DataKeys = len(sec.Data)
		entry.SizeBytes = size

		if sec.Immutable != nil && *sec.Immutable {
			entry.IsImmutable = true
			result.Summary.ImmutableCount++
		}

		key := fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)
		entry.IsReferenced = secretRefs[key]
		entry.ReferencedBy = secretRefBy[key]

		entry.AgeDays = int(now.Sub(sec.CreationTimestamp.Time).Hours() / 24)
		if entry.AgeDays > 180 {
			entry.NeedsRotation = true
			result.Summary.OldSecrets++
			result.Issues = append(result.Issues, ConfigIssue{
				Severity:  "warning",
				Type:      "stale-secret",
				Namespace: sec.Namespace,
				Resource:  sec.Name,
				Message:   fmt.Sprintf("Secret %s/%s is %d days old — consider rotating credentials", sec.Namespace, sec.Name, entry.AgeDays),
			})
		}

		// Detect plaintext-like keys in Opaque secrets
		if sec.Type == corev1.SecretTypeOpaque {
			for k := range sec.Data {
				kl := strings.ToLower(k)
				if strings.Contains(kl, "password") || strings.Contains(kl, "passwd") ||
					strings.Contains(kl, "token") || strings.Contains(kl, "key") ||
					strings.Contains(kl, "secret") {
					result.Summary.PlainTextSecrets++
					break
				}
			}
		}

		entry.RiskLevel = secretAuditRisk(entry)
		result.Summary.TotalSecrets++

		if !entry.IsReferenced {
			result.Summary.UnreferencedSecs++
			result.Unreferenced = append(result.Unreferenced, UnreferencedEntry{
				Name: sec.Name, Namespace: sec.Namespace, Kind: "Secret",
				AgeDays: entry.AgeDays, SizeBytes: size,
			})
		}

		result.Secrets = append(result.Secrets, entry)
	}

	// Sort by risk
	sort.Slice(result.ConfigMaps, func(i, j int) bool {
		return cmAuditRank(result.ConfigMaps[i].RiskLevel) < cmAuditRank(result.ConfigMaps[j].RiskLevel)
	})
	sort.Slice(result.Secrets, func(i, j int) bool {
		return cmAuditRank(result.Secrets[i].RiskLevel) < cmAuditRank(result.Secrets[j].RiskLevel)
	})
	sort.Slice(result.LargeConfigs, func(i, j int) bool {
		return result.LargeConfigs[i].SizeBytes > result.LargeConfigs[j].SizeBytes
	})
	sort.Slice(result.Unreferenced, func(i, j int) bool {
		return result.Unreferenced[i].AgeDays > result.Unreferenced[j].AgeDays
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return cmIssueRank(result.Issues[i].Severity) < cmIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = cmAuditScore(result.Summary)
	result.Recommendations = cmAuditRecs(result.Summary)

	writeJSON(w, result)
}

// cmAuditRisk determines risk level for a ConfigMap.
func cmAuditRisk(entry ConfigMapEntry) string {
	risk := 0
	if entry.IsLarge {
		risk += 15
	}
	if !entry.IsReferenced {
		risk += 5
	}
	if entry.DataKeys == 0 {
		risk += 5
	}
	switch {
	case risk >= 15:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// secretAuditRisk determines risk level for a Secret.
func secretAuditRisk(entry SecretEntry) string {
	risk := 0
	if entry.NeedsRotation {
		risk += 15
	}
	if !entry.IsReferenced {
		risk += 5
	}
	if !entry.IsImmutable {
		risk += 5
	}
	switch {
	case risk >= 15:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// cmAuditScore computes 0-100.
func cmAuditScore(s ConfigAuditSummary) int {
	total := s.TotalConfigMaps + s.TotalSecrets
	if total == 0 {
		return 100
	}
	score := 100
	score -= s.LargeCMs * 8
	score -= s.OldSecrets * 6
	score -= s.UnreferencedCMs * 2
	score -= s.UnreferencedSecs * 3
	score -= s.EmptyConfigs * 2
	if score < 0 {
		score = 0
	}
	return score
}

// cmAuditRecs produces actionable advice.
func cmAuditRecs(s ConfigAuditSummary) []string {
	var recs []string

	if s.LargeCMs > 0 {
		recs = append(recs, fmt.Sprintf("%d ConfigMap(s) exceed 1MB — split data or use external config store (etcd has 1.5MB limit per object)", s.LargeCMs))
	}
	if s.OldSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d Secret(s) are >180 days old — implement rotation policy with External Secrets Operator or Sealed Secrets", s.OldSecrets))
	}
	if s.UnreferencedCMs > 0 {
		recs = append(recs, fmt.Sprintf("%d ConfigMap(s) not referenced by any pod — clean up with kubectl delete configmap", s.UnreferencedCMs))
	}
	if s.UnreferencedSecs > 0 {
		recs = append(recs, fmt.Sprintf("%d Secret(s) not referenced by any pod — safe to remove after verification", s.UnreferencedSecs))
	}
	if s.EmptyConfigs > 0 {
		recs = append(recs, fmt.Sprintf("%d empty ConfigMap(s) — remove or populate with data", s.EmptyConfigs))
	}
	if s.PlainTextSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d Secret(s) contain plaintext credential keys — ensure proper RBAC access and consider encryption at rest", s.PlainTextSecrets))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Config audit score is %d/100 — review configuration management practices", s.HealthScore))
	}

	return recs
}

func cmAuditRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func cmIssueRank(s string) int {
	switch s {
	case "warning":
		return 0
	case "info":
		return 1
	default:
		return 2
	}
}
