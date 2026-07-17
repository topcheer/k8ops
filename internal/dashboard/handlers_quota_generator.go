package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QuotaGeneratorResult generates ResourceQuota and LimitRange YAML manifests
// for namespaces that lack resource governance. Provides ready-to-apply
// kubectl commands with sensible default limits.
type QuotaGeneratorResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         QuotaGenSummary `json:"summary"`
	Generated       []QuotaManifest `json:"generated"`
	BatchApply      []string        `json:"batchApply"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type QuotaGenSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuota       int `json:"withQuota"`
	WithLimitRange  int `json:"withLimitRange"`
	MissingQuota    int `json:"missingQuota"`
}

type QuotaManifest struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Type         string `json:"type"` // ResourceQuota or LimitRange
	ManifestYAML string `json:"manifestYAML"`
	ApplyCommand string `json:"applyCommand"`
}

// handleQuotaGenerator handles GET /api/scalability/quota-generator
func (s *Server) handleQuotaGenerator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := QuotaGeneratorResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})

	quotaNS := make(map[string]bool)
	for _, q := range quotas.Items {
		quotaNS[q.Namespace] = true
	}
	lrNS := make(map[string]bool)
	for _, lr := range limitRanges.Items {
		lrNS[lr.Namespace] = true
	}

	var manifests []QuotaManifest
	var batchCmds []string

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) || ns.Status.Phase != corev1.NamespaceActive {
			continue
		}
		result.Summary.TotalNamespaces++

		hasQuota := quotaNS[ns.Name]
		hasLR := lrNS[ns.Name]
		if hasQuota {
			result.Summary.WithQuota++
		}
		if hasLR {
			result.Summary.WithLimitRange++
		}

		if !hasQuota {
			result.Summary.MissingQuota++
			// Generate ResourceQuota
			quotaName := ns.Name + "-quota"
			quotaYAML := generateQuotaYAML(quotaName, ns.Name)
			quotaCmd := fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", quotaYAML)
			manifests = append(manifests, QuotaManifest{
				Name: quotaName, Namespace: ns.Name, Type: "ResourceQuota",
				ManifestYAML: quotaYAML, ApplyCommand: quotaCmd,
			})
			batchCmds = append(batchCmds, quotaCmd)
		}

		if !hasLR {
			// Generate LimitRange
			lrName := ns.Name + "-limits"
			lrYAML := generateLimitRangeYAML(lrName, ns.Name)
			lrCmd := fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", lrYAML)
			manifests = append(manifests, QuotaManifest{
				Name: lrName, Namespace: ns.Name, Type: "LimitRange",
				ManifestYAML: lrYAML, ApplyCommand: lrCmd,
			})
			batchCmds = append(batchCmds, lrCmd)
		}
	}

	if result.Summary.TotalNamespaces > 0 {
		result.HealthScore = result.Summary.WithQuota * 100 / result.Summary.TotalNamespaces
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 50:
		result.Grade = "B"
	case result.HealthScore >= 25:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Namespace < manifests[j].Namespace
	})

	result.Generated = manifests
	result.BatchApply = batchCmds
	result.Recommendations = buildQuotaGenRecs(&result)
	writeJSON(w, result)
}

func generateQuotaYAML(name, ns string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ResourceQuota
metadata:
  name: %s
  namespace: %s
spec:
  hard:
    requests.cpu: "10"
    requests.memory: 20Gi
    limits.cpu: "20"
    limits.memory: 40Gi
    persistentvolumeclaims: "10"
    services.loadbalancers: "2"
    pods: "50"`, name, ns)
}

func generateLimitRangeYAML(name, ns string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: LimitRange
metadata:
  name: %s
  namespace: %s
spec:
  limits:
  - default:
      cpu: 500m
      memory: 512Mi
    defaultRequest:
      cpu: 100m
      memory: 128Mi
    max:
      cpu: "4"
      memory: 8Gi
    type: Container`, name, ns)
}

func buildQuotaGenRecs(r *QuotaGeneratorResult) []string {
	recs := []string{}
	if r.Summary.MissingQuota == 0 {
		recs = append(recs, "所有命名空间都有 ResourceQuota 保护")
		return recs
	}
	recs = append(recs, fmt.Sprintf("%d/%d 个命名空间缺少 ResourceQuota", r.Summary.MissingQuota, r.Summary.TotalNamespaces))
	recs = append(recs, fmt.Sprintf("已生成 %d 个 YAML（ResourceQuota + LimitRange）", len(r.Generated)))
	recs = append(recs, "建议根据命名空间实际负载调整 hard 限制值")
	return recs
}
