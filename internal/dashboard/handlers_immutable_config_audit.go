package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImmutableConfigResult audits whether ConfigMaps and Secrets are set to immutable.
type ImmutableConfigResult struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	Summary         ImmutableConfigSummary   `json:"summary"`
	MutableCMs      []ImmutableConfigEntry   `json:"mutableConfigMaps"`
	MutableSecrets  []ImmutableConfigEntry   `json:"mutableSecrets"`
	ByNamespace     []ImmutableConfigNsEntry `json:"byNamespace"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Recommendations []string                 `json:"recommendations"`
}

type ImmutableConfigSummary struct {
	TotalCMs         int `json:"totalConfigMaps"`
	ImmutableCMs     int `json:"immutableConfigMaps"`
	TotalSecrets     int `json:"totalSecrets"`
	ImmutableSecrets int `json:"immutableSecrets"`
	LargeMutableCMs  int `json:"largeMutableCMs"`
	HotReloadCMs     int `json:"hotReloadCMs"`
}

type ImmutableConfigEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	DataKeys    int    `json:"dataKeys"`
	IsImmutable bool   `json:"isImmutable"`
	RiskLevel   string `json:"riskLevel"`
}

type ImmutableConfigNsEntry struct {
	Namespace   string  `json:"namespace"`
	MutableCMs  int     `json:"mutableCMs"`
	MutableSecs int     `json:"mutableSecrets"`
	RiskScore   float64 `json:"riskScore"`
}

// handleImmutableConfigAudit handles GET /api/deployment/immutable-config-audit
func (s *Server) handleImmutableConfigAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImmutableConfigResult{ScannedAt: time.Now()}

	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*ImmutableConfigNsEntry)

	for _, cm := range cms.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		result.Summary.TotalCMs++
		entry := ImmutableConfigEntry{
			Name: cm.Name, Namespace: cm.Namespace, Kind: "ConfigMap",
			DataKeys: len(cm.Data),
		}
		entry.IsImmutable = cm.Immutable != nil && *cm.Immutable

		if !entry.IsImmutable {
			result.MutableCMs = append(result.MutableCMs, entry)
			if entry.DataKeys > 10 {
				entry.RiskLevel = "high"
				result.Summary.LargeMutableCMs++
			} else {
				entry.RiskLevel = "medium"
			}
		} else {
			result.Summary.ImmutableCMs++
			entry.RiskLevel = "low"
		}

		if _, ok := nsMap[cm.Namespace]; !ok {
			nsMap[cm.Namespace] = &ImmutableConfigNsEntry{Namespace: cm.Namespace}
		}
		if !entry.IsImmutable {
			nsMap[cm.Namespace].MutableCMs++
		}
	}

	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		result.Summary.TotalSecrets++
		entry := ImmutableConfigEntry{
			Name: sec.Name, Namespace: sec.Namespace, Kind: "Secret",
			DataKeys: len(sec.Data) + len(sec.StringData),
		}
		entry.IsImmutable = sec.Immutable != nil && *sec.Immutable

		if !entry.IsImmutable {
			result.MutableSecrets = append(result.MutableSecrets, entry)
			entry.RiskLevel = "medium"
		} else {
			result.Summary.ImmutableSecrets++
			entry.RiskLevel = "low"
		}

		if _, ok := nsMap[sec.Namespace]; !ok {
			nsMap[sec.Namespace] = &ImmutableConfigNsEntry{Namespace: sec.Namespace}
		}
		if !entry.IsImmutable {
			nsMap[sec.Namespace].MutableSecs++
		}
	}

	for _, e := range nsMap {
		e.RiskScore = float64(e.MutableCMs+e.MutableSecs) * 10
		if e.RiskScore > 100 {
			e.RiskScore = 100
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskScore > result.ByNamespace[j].RiskScore
	})

	totalRes := result.Summary.TotalCMs + result.Summary.TotalSecrets
	immutableRes := result.Summary.ImmutableCMs + result.Summary.ImmutableSecrets
	if totalRes > 0 {
		result.HealthScore = immutableRes * 100 / totalRes
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("不可变配置审计: %d ConfigMap (%d 不可变), %d Secret (%d 不可变)",
			result.Summary.TotalCMs, result.Summary.ImmutableCMs, result.Summary.TotalSecrets, result.Summary.ImmutableSecrets),
	}
	if result.Summary.LargeMutableCMs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个大型可变 ConfigMap, 建议设为不可变", result.Summary.LargeMutableCMs))
	}
	if result.HealthScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 对不频繁变更的 ConfigMap/Secret 设置 immutable: true, 减少 kube-apiserver 负载")
	}
	writeJSON(w, result)
}
