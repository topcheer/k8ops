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

// SecretMountAuditResult audits how secrets are mounted and detects risky patterns.
type SecretMountAuditResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         SecretMountSummary   `json:"summary"`
	ByNamespace     []SecretMountNSEntry `json:"byNamespace"`
	RiskyMounts     []SecretMountEntry   `json:"riskyMounts"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type SecretMountSummary struct {
	TotalMounts      int `json:"totalSecretMounts"`
	EnvVarMounts     int `json:"envVarMounts"`
	VolumeMounts     int `json:"volumeMounts"`
	DockerJSONMounts int `json:"dockerJSONMounts"`
	PlaintextEnv     int `json:"plaintextEnvSecrets"`
	Unreferenced     int `json:"unreferencedSecrets"`
}

type SecretMountNSEntry struct {
	Namespace  string `json:"namespace"`
	MountCount int    `json:"mountCount"`
	RiskCount  int    `json:"riskCount"`
	RiskLevel  string `json:"riskLevel"`
}

type SecretMountEntry struct {
	PodName    string   `json:"podName"`
	Namespace  string   `json:"namespace"`
	SecretName string   `json:"secretName"`
	MountType  string   `json:"mountType"`
	RiskLevel  string   `json:"riskLevel"`
	Issues     []string `json:"issues"`
}

// handleSecretMountAudit handles GET /api/product/secret-mount-audit
func (s *Server) handleSecretMountAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SecretMountAuditResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Build set of referenced secret names
	referencedSecrets := make(map[string]bool)

	nsMap := make(map[string]*SecretMountNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		// Check env var secret references
		for _, c := range pod.Spec.Containers {
			for _, es := range c.EnvFrom {
				if es.SecretRef != nil {
					result.Summary.TotalMounts++
					result.Summary.EnvVarMounts++
					referencedSecrets[pod.Namespace+"/"+es.SecretRef.Name] = true

					entry := SecretMountEntry{
						PodName: pod.Name, Namespace: pod.Namespace,
						SecretName: es.SecretRef.Name, MountType: "envFrom",
					}
					entry.Issues = []string{"secret-exposed-as-env"}
					entry.RiskLevel = "medium"
					result.Summary.PlaintextEnv++
					result.RiskyMounts = append(result.RiskyMounts, entry)
					trackNS(nsMap, pod.Namespace, 1, 1)
				}
			}
			for _, e := range c.Env {
				if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
					result.Summary.TotalMounts++
					result.Summary.EnvVarMounts++
					result.Summary.PlaintextEnv++
					referencedSecrets[pod.Namespace+"/"+e.ValueFrom.SecretKeyRef.Name] = true
					result.RiskyMounts = append(result.RiskyMounts, SecretMountEntry{
						PodName: pod.Name, Namespace: pod.Namespace,
						SecretName: e.ValueFrom.SecretKeyRef.Name, MountType: "envVar",
						RiskLevel: "medium", Issues: []string{"secret-as-env-var"},
					})
					trackNS(nsMap, pod.Namespace, 1, 1)
				}
			}
		}

		// Check volume-mounted secrets
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				result.Summary.TotalMounts++
				result.Summary.VolumeMounts++
				referencedSecrets[pod.Namespace+"/"+vol.Secret.SecretName] = true
				trackNS(nsMap, pod.Namespace, 1, 0)
			} else if strings.Contains(vol.Name, "docker") && vol.Secret != nil {
				result.Summary.DockerJSONMounts++
			}
		}
	}

	// Count unreferenced secrets
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		if !referencedSecrets[sec.Namespace+"/"+sec.Name] {
			result.Summary.Unreferenced++
		}
	}

	for _, e := range nsMap {
		switch {
		case e.RiskCount > 10:
			e.RiskLevel = "critical"
		case e.RiskCount > 5:
			e.RiskLevel = "high"
		case e.RiskCount > 0:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskCount > result.ByNamespace[j].RiskCount
	})

	if result.Summary.TotalMounts > 0 {
		volRatio := float64(result.Summary.VolumeMounts) / float64(result.Summary.TotalMounts)
		result.HealthScore = int(volRatio * 100)
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Secret 挂载审计: %d 总挂载, %d env注入, %d volume挂载, %d 未引用",
			result.Summary.TotalMounts, result.Summary.EnvVarMounts,
			result.Summary.VolumeMounts, result.Summary.Unreferenced),
	}
	if result.Summary.PlaintextEnv > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Secret 通过环境变量注入, 建议改用 volume 挂载", result.Summary.PlaintextEnv))
	}
	if result.Summary.Unreferenced > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个未引用的 Secret, 建议清理", result.Summary.Unreferenced))
	}
	writeJSON(w, result)
}

func trackNS(m map[string]*SecretMountNSEntry, ns string, mounts, risks int) {
	if m[ns] == nil {
		m[ns] = &SecretMountNSEntry{Namespace: ns}
	}
	m[ns].MountCount += mounts
	m[ns].RiskCount += risks
}
