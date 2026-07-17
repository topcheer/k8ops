package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretSprayResult analyzes how widely each Secret is mounted across pods.
// Over-sprayed secrets (mounted on many pods) increase the blast radius of
// credential compromise and lateral movement risk.
type SecretSprayResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         SecretSpraySummary `json:"summary"`
	BySecret        []SecretSprayEntry `json:"bySecret"`
	CriticalSpray   []SecretSprayEntry `json:"criticalSpray"`
	ExposureScore   int                `json:"exposureScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type SecretSpraySummary struct {
	TotalSecrets     int `json:"totalSecrets"`
	MountedSecrets   int `json:"mountedSecrets"`
	OrphanedSecrets  int `json:"orphanedSecrets"`
	HighSpray        int `json:"highSprayCount"`
	MaxMountCount    int `json:"maxMountCount"`
	TotalMountPoints int `json:"totalMountPoints"`
}

type SecretSprayEntry struct {
	SecretName        string   `json:"secretName"`
	Namespace         string   `json:"namespace"`
	Type              string   `json:"type"`
	MountCount        int      `json:"mountCount"`
	AffectedPods      []string `json:"affectedPods"`
	AffectedWorkloads []string `json:"affectedWorkloads"`
	Namespaces        []string `json:"spreadNamespaces"`
	SprayLevel        string   `json:"sprayLevel"`
	RiskScore         int      `json:"riskScore"`
	IsSensitive       bool     `json:"isSensitive"`
}

// handleSecretSpray handles GET /api/security/secret-spray
func (s *Server) handleSecretSpray(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecretSprayResult{ScannedAt: time.Now()}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build secret mount map: ns/name -> entry
	secretMap := make(map[string]*SecretSprayEntry)
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		key := sec.Namespace + "/" + sec.Name
		isSensitive := isSensitiveSecretType(sec.Type)
		secretMap[key] = &SecretSprayEntry{
			SecretName:  sec.Name,
			Namespace:   sec.Namespace,
			Type:        string(sec.Type),
			IsSensitive: isSensitive,
		}
	}

	// Scan pods for secret mounts
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		// Get workload name
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			wlName = pod.Name
		}

		mountedSecrets := make(map[string]bool)

		// Check volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				key := pod.Namespace + "/" + vol.Secret.SecretName
				mountedSecrets[key] = true
			}
			// Check projected volumes with secret sources
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						key := pod.Namespace + "/" + src.Secret.Name
						mountedSecrets[key] = true
					}
				}
			}
		}

		// Check env vars referencing secrets
		for _, c := range pod.Spec.Containers {
			for _, ev := range c.Env {
				if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
					key := pod.Namespace + "/" + ev.ValueFrom.SecretKeyRef.Name
					mountedSecrets[key] = true
				}
			}
			for _, envFrom := range c.EnvFrom {
				if envFrom.SecretRef != nil {
					key := pod.Namespace + "/" + envFrom.SecretRef.Name
					mountedSecrets[key] = true
				}
			}
		}

		// Check imagePullSecrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			key := pod.Namespace + "/" + ips.Name
			mountedSecrets[key] = true
		}

		// Record mounts
		for key := range mountedSecrets {
			entry, ok := secretMap[key]
			if !ok {
				// Secret not in our map (might be system ns)
				continue
			}
			entry.MountCount++
			entry.AffectedPods = appendUniqueSecretStr(entry.AffectedPods, pod.Name)
			if wlName != "" {
				entry.AffectedWorkloads = appendUniqueSecretStr(entry.AffectedWorkloads, wlName)
			}
		}
	}

	// Build result entries
	result.Summary.TotalSecrets = len(secretMap)
	var entries []SecretSprayEntry
	maxMount := 0
	totalMounts := 0

	for _, e := range secretMap {
		if e.MountCount > 0 {
			result.Summary.MountedSecrets++
			totalMounts += e.MountCount
		} else {
			result.Summary.OrphanedSecrets++
		}

		// Determine spray level
		switch {
		case e.MountCount >= 20:
			e.SprayLevel = "critical"
			result.Summary.HighSpray++
		case e.MountCount >= 10:
			e.SprayLevel = "high"
			result.Summary.HighSpray++
		case e.MountCount >= 5:
			e.SprayLevel = "medium"
		case e.MountCount >= 1:
			e.SprayLevel = "low"
		default:
			e.SprayLevel = "none"
		}

		// Risk score: sensitive secrets with high spray = higher risk
		baseRisk := e.MountCount * 5
		if e.IsSensitive {
			baseRisk = e.MountCount * 8
		}
		if baseRisk > 100 {
			baseRisk = 100
		}
		e.RiskScore = baseRisk

		if e.MountCount > maxMount {
			maxMount = e.MountCount
		}

		entries = append(entries, *e)
	}

	result.Summary.MaxMountCount = maxMount
	result.Summary.TotalMountPoints = totalMounts

	// Sort by mount count descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].MountCount > entries[j].MountCount
	})
	result.BySecret = entries

	// Collect critical spray
	for _, e := range entries {
		if e.SprayLevel == "critical" || e.SprayLevel == "high" {
			result.CriticalSpray = append(result.CriticalSpray, e)
		}
	}

	// Exposure score: lower is worse
	if result.Summary.TotalSecrets > 0 {
		sprayRatio := float64(result.Summary.HighSpray) / float64(result.Summary.TotalSecrets)
		orphanRatio := float64(result.Summary.OrphanedSecrets) / float64(result.Summary.TotalSecrets)
		result.ExposureScore = int((1 - sprayRatio - orphanRatio*0.3) * 100)
		if result.ExposureScore < 0 {
			result.ExposureScore = 0
		}
	}

	switch {
	case result.ExposureScore >= 80:
		result.Grade = "A"
	case result.ExposureScore >= 60:
		result.Grade = "B"
	case result.ExposureScore >= 40:
		result.Grade = "C"
	case result.ExposureScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildSecretSprayRecs(&result)
	writeJSON(w, result)
}

func isSensitiveSecretType(t corev1.SecretType) bool {
	switch t {
	case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
		return true
	case corev1.SecretTypeTLS:
		return true
	case corev1.SecretTypeServiceAccountToken:
		return true
	}
	return false
}

func appendUniqueSecretStr(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

func buildSecretSprayRecs(r *SecretSprayResult) []string {
	recs := []string{
		fmt.Sprintf("Secret 喷射: %d 个密钥, %d 已挂载, %d 孤立, %d 高喷射", r.Summary.TotalSecrets, r.Summary.MountedSecrets, r.Summary.OrphanedSecrets, r.Summary.HighSpray),
	}
	if r.Summary.HighSpray > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个 Secret 被大量 Pod 挂载 (>=10), 泄露影响范围大", r.Summary.HighSpray))
	}
	if r.Summary.OrphanedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d 个孤立 Secret 未被任何 Pod 使用", r.Summary.OrphanedSecrets))
	}
	if len(r.CriticalSpray) > 0 {
		top := r.CriticalSpray[0]
		recs = append(recs, fmt.Sprintf("最高喷射: %s/%s 挂载 %d 次", top.Namespace, top.SecretName, top.MountCount))
	}
	if r.ExposureScore < 60 {
		recs = append(recs, "建议: 拆分高喷射 Secret 为每命名空间独立凭据, 使用 External Secrets 或 Vault")
	}
	return recs
}
