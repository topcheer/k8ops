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

// ============================================================
// v20.34 — Documentation Dimension (Round 25)
// 1. Label Taxonomy Report — label key cardinality and standardization
// 2. Annotation Inventory Doc — annotation key usage catalog
// 3. Resource Quota Cross-Ref — quota vs actual namespace usage doc
// ============================================================

// ---------------------------------------------------------------
// 1. Label Taxonomy Report
// ---------------------------------------------------------------

type LabelTaxResult2034 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LabelTaxSummary2034 `json:"summary"`
	HighCardinality []LabelTaxEntry2034 `json:"highCardinalityLabels"`
	Recommendations []string            `json:"recommendations"`
}

type LabelTaxSummary2034 struct {
	TotalResources  int `json:"totalResources"`
	UniqueLabelKeys int `json:"uniqueLabelKeys"`
	HighCardinality int `json:"highCardinality"`
}

type LabelTaxEntry2034 struct {
	Label        string `json:"label"`
	UniqueValues int    `json:"uniqueValues"`
}

func (s *Server) handleLabelTaxonomy(w http.ResponseWriter, r *http.Request) {
	result := LabelTaxResult2034{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	labelValues := make(map[string]map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalResources++

		for k, v := range pod.Labels {
			if labelValues[k] == nil {
				labelValues[k] = make(map[string]bool)
			}
			labelValues[k][v] = true
		}
	}

	result.Summary.UniqueLabelKeys = len(labelValues)

	for label, values := range labelValues {
		count := len(values)
		if count > 20 {
			result.Summary.HighCardinality++
			result.HighCardinality = append(result.HighCardinality, LabelTaxEntry2034{
				Label: label, UniqueValues: count,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HighCardinality, func(i, j int) bool {
		return result.HighCardinality[i].UniqueValues > result.HighCardinality[j].UniqueValues
	})

	if result.Summary.HighCardinality > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d labels have high cardinality (>20 values) — may cause etcd performance issues", result.Summary.HighCardinality))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Annotation Inventory Doc
// ---------------------------------------------------------------

type AnnotInvResult2034 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AnnotInvSummary2034 `json:"summary"`
	TopAnnotations  []AnnotInvEntry2034 `json:"topAnnotations"`
	Recommendations []string            `json:"recommendations"`
}

type AnnotInvSummary2034 struct {
	TotalResources  int `json:"totalResources"`
	UniqueAnnotKeys int `json:"uniqueAnnotationKeys"`
	TotalAnnots     int `json:"totalAnnotations"`
}

type AnnotInvEntry2034 struct {
	Key      string `json:"key"`
	Count    int    `json:"count"`
	Category string `json:"category"`
}

func (s *Server) handleAnnotationInventoryDoc(w http.ResponseWriter, r *http.Request) {
	result := AnnotInvResult2034{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	annotCount := make(map[string]int)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalResources++
		for k := range pod.Annotations {
			annotCount[k]++
			result.Summary.TotalAnnots++
		}
	}

	for _, svc := range svcList.Items {
		result.Summary.TotalResources++
		for k := range svc.Annotations {
			annotCount[k]++
			result.Summary.TotalAnnots++
		}
	}

	result.Summary.UniqueAnnotKeys = len(annotCount)

	// Top annotations by count
	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range annotCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for i, s2 := range sorted {
		if i >= 15 {
			break
		}
		cat := "other"
		if strings.Contains(s2.key, "kubectl.kubernetes.io") {
			cat = "kubectl"
		} else if strings.Contains(s2.key, "helm.sh") {
			cat = "helm"
		} else if strings.Contains(s2.key, "kubernetes.io") {
			cat = "k8s-native"
		}
		result.TopAnnotations = append(result.TopAnnotations, AnnotInvEntry2034{
			Key: s2.key, Count: s2.count, Category: cat,
		})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.UniqueAnnotKeys > 100 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unique annotation keys — consider standardizing annotation usage", result.Summary.UniqueAnnotKeys))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Resource Quota Cross-Ref
// ---------------------------------------------------------------

type QuotaXRefResult2034 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         QuotaXRefSummary2034 `json:"summary"`
	QuotaUsage      []QuotaXRefEntry2034 `json:"quotaUsage"`
	Recommendations []string             `json:"recommendations"`
}

type QuotaXRefSummary2034 struct {
	TotalNamespaces     int `json:"totalNamespaces"`
	NamespacesWithQuota int `json:"namespacesWithQuota"`
	NearLimit           int `json:"nearLimit"`
}

type QuotaXRefEntry2034 struct {
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Used      string `json:"used"`
	Hard      string `json:"hard"`
	UsagePct  int    `json:"usagePercent"`
}

func (s *Server) handleQuotaXRef(w http.ResponseWriter, r *http.Request) {
	result := QuotaXRefResult2034{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	quotaList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNamespaces = len(nsList.Items)

	// Group quotas by namespace
	nsQuotas := make(map[string][]corev1.ResourceQuota)
	for _, rq := range quotaList.Items {
		nsQuotas[rq.Namespace] = append(nsQuotas[rq.Namespace], rq)
	}

	result.Summary.NamespacesWithQuota = len(nsQuotas)

	for ns, quotas := range nsQuotas {
		for _, rq := range quotas {
			for res, hard := range rq.Status.Hard {
				used, ok := rq.Status.Used[res]
				if !ok {
					continue
				}

				usagePct := 0
				if hard.MilliValue() > 0 {
					usagePct = int(used.MilliValue() * 100 / hard.MilliValue())
				}

				entry := QuotaXRefEntry2034{
					Namespace: ns,
					Resource:  string(res),
					Used:      used.String(),
					Hard:      hard.String(),
					UsagePct:  usagePct,
				}

				if usagePct > 80 {
					result.Summary.NearLimit++
					result.QuotaUsage = append(result.QuotaUsage, entry)
					score -= 3
				} else if usagePct > 60 {
					result.QuotaUsage = append(result.QuotaUsage, entry)
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.QuotaUsage, func(i, j int) bool {
		return result.QuotaUsage[i].UsagePct > result.QuotaUsage[j].UsagePct
	})

	if result.Summary.NearLimit > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d quotas are near limit (>80%%) — increase quota or clean up resources", result.Summary.NearLimit))
	}

	writeJSON(w, result)
}
