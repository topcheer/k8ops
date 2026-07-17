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

// SecretExposureScanResult scans for secrets that are exposed through
// environment variables, volume mounts, or insecure handling. It
// identifies plaintext secret usage, overly permissive access, and
// recommends migration to external secret management.
type SecretExposureScanResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         SecretExposureSummary `json:"summary"`
	Exposures       []SecretExposureEntry `json:"exposures"`
	ByType          []SecretExposureType  `json:"byType"`
	HighRisk        []SecretExposureEntry `json:"highRisk"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type SecretExposureSummary struct {
	TotalSecrets      int `json:"totalSecrets"`
	ReferencedSecrets int `json:"referencedSecrets"`
	OrphanedSecrets   int `json:"orphanedSecrets"`
	PlaintextInEnv    int `json:"plaintextInEnv"`
	VolumeMounted     int `json:"volumeMounted"`
	EnvVarRef         int `json:"envVarRef"`
	DockerConfig      int `json:"dockerConfig"`
	TLS               int `json:"tls"`
	Opaque            int `json:"opaque"`
	HighRiskCount     int `json:"highRiskCount"`
}

type SecretExposureEntry struct {
	SecretName   string   `json:"secretName"`
	Namespace    string   `json:"namespace"`
	Type         string   `json:"type"`
	ExposureType string   `json:"exposureType"`
	UsedBy       []string `json:"usedBy"`
	RiskLevel    string   `json:"riskLevel"`
	Detail       string   `json:"detail"`
}

type SecretExposureType struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
	Risk  string `json:"risk"`
}

// handleSecretExposure handles GET /api/security/secret-exposure
func (s *Server) handleSecretExposure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecretExposureScanResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Build secret usage map: ns/name -> []workload
	secretUsage := make(map[string][]string)
	plaintextEnvs := 0

	scanContainers := func(workload, ns string, spec corev1.PodSpec) {
		for _, vol := range spec.Volumes {
			if vol.Secret != nil {
				key := ns + "/" + vol.Secret.SecretName
				secretUsage[key] = appendUniqueVal(secretUsage[key], workload)
			}
		}
		for _, c := range spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					key := ns + "/" + env.ValueFrom.SecretKeyRef.Name
					secretUsage[key] = appendUniqueVal(secretUsage[key], workload)
				}
				// Detect plaintext sensitive env vars
				if env.Value != "" && isSensitiveEnvVar(env.Name) {
					plaintextEnvs++
				}
			}
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					key := ns + "/" + ef.SecretRef.Name
					secretUsage[key] = appendUniqueVal(secretUsage[key], workload)
				}
			}
			// Check for imagePullSecrets containing credentials
			for _, ips := range spec.ImagePullSecrets {
				key := ns + "/" + ips.Name
				secretUsage[key] = appendUniqueVal(secretUsage[key], workload+"(imagePull)")
			}
		}
	}

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		scanContainers(d.Name, d.Namespace, d.Spec.Template.Spec)
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		scanContainers(ss.Name, ss.Namespace, ss.Spec.Template.Spec)
	}

	// Analyze secrets
	typeCounts := make(map[string]int)
	var exposures []SecretExposureEntry

	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}

		result.Summary.TotalSecrets++
		typeStr := string(sec.Type)
		typeCounts[typeStr]++

		switch sec.Type {
		case corev1.SecretTypeOpaque:
			result.Summary.Opaque++
		case corev1.SecretTypeTLS:
			result.Summary.TLS++
		case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
			result.Summary.DockerConfig++
		}

		key := sec.Namespace + "/" + sec.Name
		users := secretUsage[key]

		entry := SecretExposureEntry{
			SecretName: sec.Name,
			Namespace:  sec.Namespace,
			Type:       typeStr,
			UsedBy:     users,
		}

		if len(users) > 0 {
			result.Summary.ReferencedSecrets++
			entry.ExposureType = "referenced"
			entry.RiskLevel = "low"

			// Check if mounted as volume (all keys visible)
			for _, d := range deployments.Items {
				if d.Namespace != sec.Namespace {
					continue
				}
				for _, vol := range d.Spec.Template.Spec.Volumes {
					if vol.Secret != nil && vol.Secret.SecretName == sec.Name {
						entry.ExposureType = "volume-mount"
						result.Summary.VolumeMounted++
						if vol.Secret.DefaultMode != nil && *vol.Secret.DefaultMode > 0644 {
							entry.RiskLevel = "medium"
							entry.Detail = fmt.Sprintf("文件权限 %o 过于宽松", *vol.Secret.DefaultMode)
						}
					}
				}
			}
		} else {
			result.Summary.OrphanedSecrets++
			entry.ExposureType = "orphaned"
			entry.RiskLevel = "high"
			entry.Detail = "无工作负载引用的孤立 Secret"
			result.Summary.HighRiskCount++
		}

		// High risk: OPAQUE secret with many data keys
		if sec.Type == corev1.SecretTypeOpaque && len(sec.Data) > 5 {
			if entry.RiskLevel != "high" {
				entry.RiskLevel = "medium"
			}
			entry.Detail = fmt.Sprintf("%d 个数据键，建议拆分", len(sec.Data))
		}

		exposures = append(exposures, entry)
	}

	result.Summary.PlaintextInEnv = plaintextEnvs

	// By type
	for t, c := range typeCounts {
		risk := "low"
		if t == string(corev1.SecretTypeOpaque) {
			risk = "medium"
		}
		result.ByType = append(result.ByType, SecretExposureType{Type: t, Count: c, Risk: risk})
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// High risk secrets
	for _, e := range exposures {
		if e.RiskLevel == "high" || e.RiskLevel == "medium" {
			result.HighRisk = append(result.HighRisk, e)
		}
	}
	sort.Slice(result.HighRisk, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[result.HighRisk[i].RiskLevel] < sevOrder[result.HighRisk[j].RiskLevel]
	})

	// Score
	if result.Summary.TotalSecrets > 0 {
		score := result.Summary.ReferencedSecrets * 100 / result.Summary.TotalSecrets
		score -= result.Summary.OrphanedSecrets * 2
		score -= plaintextEnvs * 5
		if score < 0 {
			score = 0
		}
		result.HealthScore = score
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

	result.Exposures = exposures
	sort.Slice(result.Exposures, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[result.Exposures[i].RiskLevel] < sevOrder[result.Exposures[j].RiskLevel]
	})

	result.Recommendations = buildSecretExpRecs(&result)
	writeJSON(w, result)
}

func appendUniqueVal(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

func buildSecretExpRecs(r *SecretExposureScanResult) []string {
	recs := []string{}
	if r.Summary.OrphanedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d 个孤立 Secret 未被任何工作负载引用，建议清理", r.Summary.OrphanedSecrets))
	}
	if r.Summary.PlaintextInEnv > 0 {
		recs = append(recs, fmt.Sprintf("%d 个明文敏感环境变量，应迁移到 Secret 引用", r.Summary.PlaintextInEnv))
	}
	if r.Summary.DockerConfig > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Docker registry Secret，建议使用 cloud IAM 替代", r.Summary.DockerConfig))
	}
	if r.Summary.TLS > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 TLS Secret，建议使用 cert-manager 自动管理", r.Summary.TLS))
	}
	if len(recs) == 0 {
		recs = append(recs, "Secret 管理状态良好")
	}
	return recs
}

var _ = strings.Contains
