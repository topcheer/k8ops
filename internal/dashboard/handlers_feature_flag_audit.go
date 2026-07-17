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

// FeatureFlagAuditResult scans ConfigMaps, annotations, and environment
// variables for feature flags, identifying stale, uncontrolled, or risky toggles.
type FeatureFlagAuditResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         FeatureFlagSummary `json:"summary"`
	ByFlag          []FeatureFlagEntry `json:"byFlag"`
	StaleFlags      []FeatureFlagEntry `json:"staleFlags"`
	CoverageScore   int                `json:"coverageScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type FeatureFlagSummary struct {
	TotalFlags      int `json:"totalFlags"`
	ConfigMapFlags  int `json:"configMapFlags"`
	AnnotationFlags int `json:"annotationFlags"`
	EnvVarFlags     int `json:"envVarFlags"`
	EnabledFlags    int `json:"enabledFlags"`
	DisabledFlags   int `json:"disabledFlags"`
	StaleFlags      int `json:"staleFlags"`
	UnmanagedFlags  int `json:"unmanagedFlags"`
}

type FeatureFlagEntry struct {
	FlagName          string   `json:"flagName"`
	Namespace         string   `json:"namespace"`
	Source            string   `json:"source"`
	SourceName        string   `json:"sourceName"`
	Value             string   `json:"value"`
	IsEnabled         bool     `json:"isEnabled"`
	AffectedWorkloads []string `json:"affectedWorkloads"`
	IsStale           bool     `json:"isStale"`
	IsManaged         bool     `json:"isManaged"`
	RiskLevel         string   `json:"riskLevel"`
}

// Known feature flag patterns
var flagKeyPatterns = []string{
	"feature.",
	"FEATURE_",
	"flag.",
	"FLAG_",
	"enable.",
	"ENABLE_",
	"toggle.",
	"beta.",
	"experimental.",
	"canary.",
	"preview.",
	"config.feature",
}

func isFeatureFlagKey(key string) bool {
	upperKey := strings.ToUpper(key)
	for _, pattern := range flagKeyPatterns {
		if strings.HasPrefix(upperKey, strings.ToUpper(pattern)) {
			return true
		}
	}
	lowerKey := strings.ToLower(key)
	if strings.Contains(lowerKey, "feature-flag") || strings.Contains(lowerKey, "feature_flag") {
		return true
	}
	return false
}

func isFlagEnabled(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "true" || v == "1" || v == "yes" || v == "on" || v == "enabled"
}

// handleFeatureFlagAudit handles GET /api/product/feature-flag-audit
func (s *Server) handleFeatureFlagAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := FeatureFlagAuditResult{ScannedAt: time.Now()}

	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	flagMap := make(map[string]*FeatureFlagEntry)

	// 1. Scan ConfigMaps for feature flags
	for _, cm := range cms.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		// Check if CM name suggests it's a feature flag config
		cmName := strings.ToLower(cm.Name)
		isFlagCM := strings.Contains(cmName, "feature") || strings.Contains(cmName, "flag") || strings.Contains(cmName, "config")

		for k, v := range cm.Data {
			if isFlagCM || isFeatureFlagKey(k) {
				key := cm.Namespace + "/cm:" + cm.Name + "/" + k
				if _, ok := flagMap[key]; !ok {
					flagMap[key] = &FeatureFlagEntry{
						FlagName:   k,
						Namespace:  cm.Namespace,
						Source:     "ConfigMap",
						SourceName: cm.Name,
					}
				}
				flagMap[key].Value = v
				flagMap[key].IsEnabled = isFlagEnabled(v)
				result.Summary.ConfigMapFlags++

				// Check if managed (has annotation or label)
				if _, ok := cm.Annotations["featureflag.managed"]; ok {
					flagMap[key].IsManaged = true
				}
			}
		}
	}

	// 2. Scan deployment annotations
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for k, v := range d.Annotations {
			if isFeatureFlagKey(k) || strings.Contains(strings.ToLower(k), "feature") {
				key := d.Namespace + "/ann:" + d.Name + "/" + k
				if _, ok := flagMap[key]; !ok {
					flagMap[key] = &FeatureFlagEntry{
						FlagName:   k,
						Namespace:  d.Namespace,
						Source:     "Annotation",
						SourceName: d.Name,
						IsManaged:  true, // annotations on deployments are managed
					}
				}
				flagMap[key].Value = v
				flagMap[key].IsEnabled = isFlagEnabled(v)
				flagMap[key].AffectedWorkloads = appendUniqueSecretStr(flagMap[key].AffectedWorkloads, d.Name)
				result.Summary.AnnotationFlags++
			}
		}
	}

	// 3. Scan pod env vars for feature flags
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}

		for _, c := range pod.Spec.Containers {
			for _, ev := range c.Env {
				if isFeatureFlagKey(ev.Name) {
					key := pod.Namespace + "/env:" + pod.Name + "/" + c.Name + "/" + ev.Name
					val := ev.Value
					if val == "" && ev.ValueFrom != nil {
						val = "<from-ref>"
					}
					if _, ok := flagMap[key]; !ok {
						flagMap[key] = &FeatureFlagEntry{
							FlagName:   ev.Name,
							Namespace:  pod.Namespace,
							Source:     "EnvVar",
							SourceName: wlName + "/" + c.Name,
						}
					}
					flagMap[key].Value = val
					flagMap[key].IsEnabled = isFlagEnabled(val)
					if wlName != "" {
						flagMap[key].AffectedWorkloads = appendUniqueSecretStr(flagMap[key].AffectedWorkloads, wlName)
					}
					result.Summary.EnvVarFlags++
				}
			}
		}
	}

	// Build entries and classify
	var entries []FeatureFlagEntry
	for _, e := range flagMap {
		result.Summary.TotalFlags++
		if e.IsEnabled {
			result.Summary.EnabledFlags++
		} else {
			result.Summary.DisabledFlags++
		}

		// Stale flag: env var flag not managed, or CM flag with "temp" or "test" in name
		lowerFlag := strings.ToLower(e.FlagName)
		if strings.Contains(lowerFlag, "temp") || strings.Contains(lowerFlag, "test") || strings.Contains(lowerFlag, "debug") {
			e.IsStale = true
			result.Summary.StaleFlags++
		}
		if !e.IsManaged {
			result.Summary.UnmanagedFlags++
			if e.Source == "EnvVar" {
				e.RiskLevel = "high" // env var flags are hard to change without redeployment
			} else {
				e.RiskLevel = "medium"
			}
		} else {
			e.RiskLevel = "low"
		}

		if e.IsStale {
			e.RiskLevel = "high"
		}

		entries = append(entries, *e)
	}

	// Sort by risk level
	riskRank := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(entries, func(i, j int) bool {
		return riskRank[entries[i].RiskLevel] < riskRank[entries[j].RiskLevel]
	})
	result.ByFlag = entries

	// Collect stale flags
	for _, e := range entries {
		if e.IsStale {
			result.StaleFlags = append(result.StaleFlags, e)
		}
	}

	// Coverage score: managed + not stale = good
	if result.Summary.TotalFlags > 0 {
		managedRatio := float64(result.Summary.TotalFlags-result.Summary.UnmanagedFlags) / float64(result.Summary.TotalFlags)
		stalePenalty := float64(result.Summary.StaleFlags) / float64(result.Summary.TotalFlags)
		result.CoverageScore = int(managedRatio*100 - stalePenalty*30)
		if result.CoverageScore < 0 {
			result.CoverageScore = 0
		}
	} else {
		result.CoverageScore = 100
	}

	switch {
	case result.CoverageScore >= 80:
		result.Grade = "A"
	case result.CoverageScore >= 60:
		result.Grade = "B"
	case result.CoverageScore >= 40:
		result.Grade = "C"
	case result.CoverageScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildFeatureFlagRecs(&result)
	writeJSON(w, result)
}

func buildFeatureFlagRecs(r *FeatureFlagAuditResult) []string {
	recs := []string{
		fmt.Sprintf("特性开关审计: %d 个开关 (%d 启用, %d 禁用), %d 通过 ConfigMap, %d 注解, %d 环境变量",
			r.Summary.TotalFlags, r.Summary.EnabledFlags, r.Summary.DisabledFlags,
			r.Summary.ConfigMapFlags, r.Summary.AnnotationFlags, r.Summary.EnvVarFlags),
	}
	if r.Summary.StaleFlags > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个过期/测试开关需要清理", r.Summary.StaleFlags))
	}
	if r.Summary.UnmanagedFlags > 0 {
		recs = append(recs, fmt.Sprintf("%d 个开关未通过集中管理 (建议使用 ConfigMap 或 Flag 系统)", r.Summary.UnmanagedFlags))
	}
	if r.Summary.EnvVarFlags > 0 {
		recs = append(recs, fmt.Sprintf("%d 个开关通过环境变量设置 (修改需重新部署), 建议迁移到 ConfigMap", r.Summary.EnvVarFlags))
	}
	if len(r.StaleFlags) > 0 {
		top := r.StaleFlags[0]
		recs = append(recs, fmt.Sprintf("建议清理: %s/%s = '%s' (过期)", top.Namespace, top.FlagName, top.Value))
	}
	return recs
}
