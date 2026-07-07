package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CRDResult is the API object count & CRD explosion risk analysis.
type CRDResult struct {
	ScannedAt       time.Time    `json:"scannedAt"`
	Summary         CRDSummary   `json:"summary"`
	ByResourceType  []CRDEntry   `json:"byResourceType"`
	HighCountCRDs   []CRDEntry   `json:"highCountCRDs"` // CRDs with >500 objects
	ByNamespace     []CRDNSEntry `json:"byNamespace"`
	Issues          []CRDIssue   `json:"issues"`
	Recommendations []string     `json:"recommendations"`
}

// CRDSummary aggregates object count statistics.
type CRDSummary struct {
	TotalCRDs            int `json:"totalCRDs"`
	CRDsWithObjects      int `json:"crdsWithObjects"`
	HighCountCRDs        int `json:"highCountCRDs"`     // >500 objects
	VeryHighCountCRDs    int `json:"veryHighCountCRDs"` // >1000 objects
	TotalConfigMaps      int `json:"totalConfigMaps"`
	TotalSecrets         int `json:"totalSecrets"`
	TotalServices        int `json:"totalServices"`
	TotalPods            int `json:"totalPods"`
	TotalNamespaces      int `json:"totalNamespaces"`
	LargestNSObjectCount int `json:"largestNSObjectCount"`
	ScalabilityScore     int `json:"scalabilityScore"` // 0-100
}

// CRDEntry describes one resource type's object count.
type CRDEntry struct {
	Name        string `json:"name"` // resource type name
	Group       string `json:"group,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Namespaced  bool   `json:"namespaced"`
	ObjectCount int    `json:"objectCount"`
	Namespaces  int    `json:"namespaces,omitempty"` // how many namespaces have this
	RiskLevel   string `json:"riskLevel"`
}

// CRDNSEntry per-namespace object count.
type CRDNSEntry struct {
	Namespace    string `json:"namespace"`
	ConfigMaps   int    `json:"configMaps"`
	Secrets      int    `json:"secrets"`
	Services     int    `json:"services"`
	Pods         int    `json:"pods"`
	TotalObjects int    `json:"totalObjects"`
}

// CRDIssue is a detected scalability problem.
type CRDIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleCRDExplosion counts API objects per type and detects CRD explosion risk.
// GET /api/scalability/crd-explosion
func (s *Server) handleCRDExplosion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CRDResult{ScannedAt: time.Now()}

	// Count core resources
	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	result.Summary.TotalNamespaces = len(nsList.Items)

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if pods != nil {
		result.Summary.TotalPods = len(pods.Items)
	}

	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if svcs != nil {
		result.Summary.TotalServices = len(svcs.Items)
	}

	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	if cms != nil {
		result.Summary.TotalConfigMaps = len(cms.Items)
	}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if secrets != nil {
		result.Summary.TotalSecrets = len(secrets.Items)
	}

	// Add core resource types to the list
	coreTypes := []CRDEntry{
		{Name: "pods", Kind: "Pod", Namespaced: true, ObjectCount: result.Summary.TotalPods},
		{Name: "services", Kind: "Service", Namespaced: true, ObjectCount: result.Summary.TotalServices},
		{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, ObjectCount: result.Summary.TotalConfigMaps},
		{Name: "secrets", Kind: "Secret", Namespaced: true, ObjectCount: result.Summary.TotalSecrets},
	}
	for i := range coreTypes {
		coreTypes[i].RiskLevel = crdAssessRisk(coreTypes[i].ObjectCount)
		result.ByResourceType = append(result.ByResourceType, coreTypes[i])
	}

	// Count CRDs via discovery API
	apiResourceLists, err := rc.clientset.Discovery().ServerPreferredResources()
	result.Summary.TotalCRDs = 0
	if err == nil {
		for _, list := range apiResourceLists {
			if !strings.Contains(list.GroupVersion, ".") {
				continue // skip core resources
			}
			for range list.APIResources {
				result.Summary.TotalCRDs++
			}
		}
	}

	// Add CRD entries from discovery
	for _, list := range apiResourceLists {
		if !strings.Contains(list.GroupVersion, ".") {
			continue
		}
		parts := strings.SplitN(list.GroupVersion, "/", 2)
		group := ""
		if len(parts) == 2 {
			group = parts[0]
		}
		for _, res := range list.APIResources {
			if strings.Contains(res.Name, "/") {
				continue // skip subresources
			}
			entry := CRDEntry{
				Name:       res.Name,
				Group:      group,
				Kind:       res.Kind,
				Namespaced: res.Namespaced,
			}
			entry.RiskLevel = "low"
			result.ByResourceType = append(result.ByResourceType, entry)
			result.Summary.CRDsWithObjects++
		}
	}

	// Count per-namespace objects
	nsMap := make(map[string]*CRDNSEntry)
	for _, pod := range pods.Items {
		ns := crdGetOrCreateNS(nsMap, pod.Namespace)
		ns.Pods++
		ns.TotalObjects++
	}
	for _, svc := range svcs.Items {
		ns := crdGetOrCreateNS(nsMap, svc.Namespace)
		ns.Services++
		ns.TotalObjects++
	}
	for _, cm := range cms.Items {
		ns := crdGetOrCreateNS(nsMap, cm.Namespace)
		ns.ConfigMaps++
		ns.TotalObjects++
	}
	for _, sec := range secrets.Items {
		ns := crdGetOrCreateNS(nsMap, sec.Namespace)
		ns.Secrets++
		ns.TotalObjects++
	}

	for _, nsStat := range nsMap {
		if nsStat.TotalObjects > result.Summary.LargestNSObjectCount {
			result.Summary.LargestNSObjectCount = nsStat.TotalObjects
		}
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Identify high-count resource types
	for _, entry := range result.ByResourceType {
		if entry.ObjectCount > 1000 {
			result.Summary.VeryHighCountCRDs++
			result.Summary.HighCountCRDs++
			result.HighCountCRDs = append(result.HighCountCRDs, entry)
			result.Issues = append(result.Issues, CRDIssue{
				Severity: "warning", Type: "very-high-object-count",
				Resource: entry.Name,
				Message:  fmt.Sprintf("%s has %d objects — very high count may slow API server list/watch operations", entry.Name, entry.ObjectCount),
			})
		} else if entry.ObjectCount > 500 {
			result.Summary.HighCountCRDs++
			result.HighCountCRDs = append(result.HighCountCRDs, entry)
		}
	}

	// Namespace-level issues
	for _, nsStat := range result.ByNamespace {
		if nsStat.ConfigMaps > 200 {
			result.Issues = append(result.Issues, CRDIssue{
				Severity: "info", Type: "high-configmap-count",
				Resource: nsStat.Namespace,
				Message:  fmt.Sprintf("Namespace %s has %d ConfigMaps — consider cleanup to reduce API server load", nsStat.Namespace, nsStat.ConfigMaps),
			})
		}
		if nsStat.Secrets > 100 {
			result.Issues = append(result.Issues, CRDIssue{
				Severity: "warning", Type: "high-secret-count",
				Resource: nsStat.Namespace,
				Message:  fmt.Sprintf("Namespace %s has %d Secrets — excessive secrets increase API server encryption overhead", nsStat.Namespace, nsStat.Secrets),
			})
		}
	}

	// Sort
	sort.Slice(result.ByResourceType, func(i, j int) bool {
		return result.ByResourceType[i].ObjectCount > result.ByResourceType[j].ObjectCount
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalObjects > result.ByNamespace[j].TotalObjects
	})
	if len(result.ByNamespace) > 15 {
		result.ByNamespace = result.ByNamespace[:15]
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return crdIssueRank(result.Issues[i].Severity) < crdIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ScalabilityScore = crdScore(result.Summary)
	result.Recommendations = crdGenRecs(result.Summary, result.HighCountCRDs)

	writeJSON(w, result)
}

// crdAssessRisk determines risk level based on object count.
func crdAssessRisk(count int) string {
	if count > 1000 {
		return "critical"
	}
	if count > 500 {
		return "high"
	}
	if count > 200 {
		return "medium"
	}
	return "low"
}

// crdScore computes scalability score 0-100.
func crdScore(s CRDSummary) int {
	score := 100
	score -= s.VeryHighCountCRDs * 10
	score -= (s.HighCountCRDs - s.VeryHighCountCRDs) * 5
	if s.TotalConfigMaps > 1000 {
		score -= 5
	}
	if s.TotalSecrets > 500 {
		score -= 5
	}
	if s.TotalCRDs > 50 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	return score
}

// crdGenRecs produces actionable advice.
func crdGenRecs(s CRDSummary, highCount []CRDEntry) []string {
	var recs []string

	if s.VeryHighCountCRDs > 0 {
		recs = append(recs, fmt.Sprintf("%d resource type(s) have >1000 objects — consider cleanup, pagination, or sharding to reduce API server load", s.VeryHighCountCRDs))
	}
	if s.HighCountCRDs > 0 {
		recs = append(recs, fmt.Sprintf("%d resource type(s) have >500 objects — monitor list/watch latency closely", s.HighCountCRDs))
	}
	if s.TotalConfigMaps > 500 {
		recs = append(recs, fmt.Sprintf("%d total ConfigMaps — prune unused ones to reduce etcd size and API server overhead", s.TotalConfigMaps))
	}
	if s.TotalSecrets > 200 {
		recs = append(recs, fmt.Sprintf("%d total Secrets — consider external secret management (Vault, Sealed Secrets) for large counts", s.TotalSecrets))
	}
	if s.TotalCRDs > 30 {
		recs = append(recs, fmt.Sprintf("%d CRDs installed — each CRD adds API server overhead, consolidate or remove unused CRDs", s.TotalCRDs))
	}
	if s.LargestNSObjectCount > 500 {
		recs = append(recs, fmt.Sprintf("Largest namespace has %d objects — consider splitting into smaller namespaces for better isolation", s.LargestNSObjectCount))
	}
	if s.ScalabilityScore < 70 {
		recs = append(recs, fmt.Sprintf("Scalability score is %d/100 — cluster has object count patterns that may impact API server performance", s.ScalabilityScore))
	}
	if s.VeryHighCountCRDs == 0 && s.TotalCRDs <= 30 {
		recs = append(recs, "Object counts are within healthy range — good scalability posture")
	}

	return recs
}

func crdGetOrCreateNS(m map[string]*CRDNSEntry, ns string) *CRDNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &CRDNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func crdIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
