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

// CMResult is the ConfigMap/Secret size & memory pressure analysis.
type CMResult struct {
	ScannedAt        time.Time   `json:"scannedAt"`
	Summary          CMSummary   `json:"summary"`
	OversizedCMs     []CMEntry   `json:"oversizedCMs"`
	OversizedSecrets []CMEntry   `json:"oversizedSecrets"`
	ByNamespace      []CMNSEntry `json:"byNamespace"`
	MountedToPods    []CMEntry   `json:"mountedToPods"` // large configs mounted as volumes
	Issues           []CMIssue   `json:"issues"`
	Recommendations  []string    `json:"recommendations"`
}

// CMSummary aggregates size statistics.
type CMSummary struct {
	TotalConfigMaps   int     `json:"totalConfigMaps"`
	TotalSecrets      int     `json:"totalSecrets"`
	OversizedCMs      int     `json:"oversizedCMs"`     // >1MB
	OversizedSecrets  int     `json:"oversizedSecrets"` // >1MB
	LargestCMSizeKB   float64 `json:"largestCMSizeKB"`
	LargestSecretKB   float64 `json:"largestSecretKB"`
	TotalCMSizeMB     float64 `json:"totalCMSizeMB"`
	TotalSecretSizeMB float64 `json:"totalSecretSizeMB"`
	HealthScore       int     `json:"healthScore"` // 0-100
}

// CMEntry describes one ConfigMap or Secret's size.
type CMEntry struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Kind         string  `json:"kind"` // ConfigMap or Secret
	SizeKB       float64 `json:"sizeKB"`
	KeyCount     int     `json:"keyCount"`
	LargestKey   string  `json:"largestKey,omitempty"`
	LargestKeyKB float64 `json:"largestKeyKB,omitempty"`
	IsMounted    bool    `json:"isMounted"`
	RiskLevel    string  `json:"riskLevel"`
}

// CMNSEntry per-namespace size stats.
type CMNSEntry struct {
	Namespace   string  `json:"namespace"`
	CMCount     int     `json:"cmCount"`
	SecretCount int     `json:"secretCount"`
	TotalSizeMB float64 `json:"totalSizeMB"`
}

// CMIssue is a detected size problem.
type CMIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleConfigMapSize audits ConfigMap/Secret sizes for etcd pressure.
// GET /api/product/configmap-size
func (s *Server) handleConfigMapSize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	cms, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build set of mounted ConfigMap/Secret names from pods
	mountedCMs := make(map[string]bool)
	mountedSecrets := make(map[string]bool)
	pods, _ := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if pods != nil {
		for _, pod := range pods.Items {
			for _, vol := range pod.Spec.Volumes {
				if vol.ConfigMap != nil {
					mountedCMs[pod.Namespace+"/"+vol.ConfigMap.Name] = true
				}
				if vol.Secret != nil {
					mountedSecrets[pod.Namespace+"/"+vol.Secret.SecretName] = true
				}
			}
		}
	}

	result := CMResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*CMNSEntry)

	// Analyze ConfigMaps
	for _, cm := range cms.Items {
		result.Summary.TotalConfigMaps++

		entry := cmsAnalyzeEntry(cm.Name, cm.Namespace, "ConfigMap", cm.Data, cm.BinaryData)
		entry.IsMounted = mountedCMs[cm.Namespace+"/"+cm.Name]

		nsStat := cmsGetOrCreateNS(nsMap, cm.Namespace)
		nsStat.CMCount++
		nsStat.TotalSizeMB += entry.SizeKB / 1024
		result.Summary.TotalCMSizeMB += entry.SizeKB / 1024

		if entry.SizeKB > result.Summary.LargestCMSizeKB {
			result.Summary.LargestCMSizeKB = entry.SizeKB
		}

		if entry.SizeKB > 1024 { // >1MB
			result.Summary.OversizedCMs++
			result.OversizedCMs = append(result.OversizedCMs, entry)
			result.Issues = append(result.Issues, CMIssue{
				Severity: "warning", Type: "oversized-configmap",
				Resource: fmt.Sprintf("%s/%s", cm.Namespace, cm.Name),
				Message:  fmt.Sprintf("ConfigMap %s/%s is %.1fKB (>1MB) — approaching etcd max value size of 1.5MB", cm.Namespace, cm.Name, entry.SizeKB),
			})
			if entry.IsMounted {
				result.MountedToPods = append(result.MountedToPods, entry)
				result.Issues = append(result.Issues, CMIssue{
					Severity: "warning", Type: "large-mounted-configmap",
					Resource: fmt.Sprintf("%s/%s", cm.Namespace, cm.Name),
					Message:  fmt.Sprintf("ConfigMap %s/%s (%.1fKB) is mounted as volume — large configs increase kubelet memory and API server traffic", cm.Namespace, cm.Name, entry.SizeKB),
				})
			}
		}

		entry.RiskLevel = cmsAssessRisk(entry)
	}

	// Analyze Secrets
	for _, sec := range secrets.Items {
		// Skip system-managed secret types
		if sec.Type == corev1.SecretTypeServiceAccountToken ||
			sec.Type == corev1.SecretTypeDockercfg ||
			sec.Type == corev1.SecretTypeDockerConfigJson {
			continue
		}
		result.Summary.TotalSecrets++

		entry := cmsAnalyzeEntry(sec.Name, sec.Namespace, "Secret", sec.StringData, sec.Data)
		entry.IsMounted = mountedSecrets[sec.Namespace+"/"+sec.Name]

		nsStat := cmsGetOrCreateNS(nsMap, sec.Namespace)
		nsStat.SecretCount++
		nsStat.TotalSizeMB += entry.SizeKB / 1024
		result.Summary.TotalSecretSizeMB += entry.SizeKB / 1024

		if entry.SizeKB > result.Summary.LargestSecretKB {
			result.Summary.LargestSecretKB = entry.SizeKB
		}

		if entry.SizeKB > 1024 { // >1MB
			result.Summary.OversizedSecrets++
			result.OversizedSecrets = append(result.OversizedSecrets, entry)
			result.Issues = append(result.Issues, CMIssue{
				Severity: "warning", Type: "oversized-secret",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Secret %s/%s is %.1fKB (>1MB) — large secrets increase API server encryption overhead", sec.Namespace, sec.Name, entry.SizeKB),
			})
		}

		entry.RiskLevel = cmsAssessRisk(entry)
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.OversizedCMs, func(i, j int) bool {
		return result.OversizedCMs[i].SizeKB > result.OversizedCMs[j].SizeKB
	})
	sort.Slice(result.OversizedSecrets, func(i, j int) bool {
		return result.OversizedSecrets[i].SizeKB > result.OversizedSecrets[j].SizeKB
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalSizeMB > result.ByNamespace[j].TotalSizeMB
	})
	if len(result.ByNamespace) > 15 {
		result.ByNamespace = result.ByNamespace[:15]
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return cmsIssueRank(result.Issues[i].Severity) < cmsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = cmsScore(result.Summary)
	result.Recommendations = cmsGenRecs(result.Summary, result.OversizedCMs, result.OversizedSecrets)

	writeJSON(w, result)
}

// cmsAnalyzeEntry computes size from data maps.
func cmsAnalyzeEntry(name, namespace, kind string, stringData map[string]string, binaryData map[string][]byte) CMEntry {
	entry := CMEntry{
		Name:      name,
		Namespace: namespace,
		Kind:      kind,
	}

	var totalBytes float64
	var largestKey string
	var largestKeyBytes float64

	for key, val := range stringData {
		keyBytes := float64(len(val))
		totalBytes += keyBytes
		entry.KeyCount++
		if keyBytes > largestKeyBytes {
			largestKeyBytes = keyBytes
			largestKey = key
		}
	}

	for key, val := range binaryData {
		keyBytes := float64(len(val))
		totalBytes += keyBytes
		entry.KeyCount++
		if keyBytes > largestKeyBytes {
			largestKeyBytes = keyBytes
			largestKey = key
		}
	}

	entry.SizeKB = totalBytes / 1024
	entry.LargestKey = largestKey
	entry.LargestKeyKB = largestKeyBytes / 1024

	return entry
}

// cmsAssessRisk determines risk level.
func cmsAssessRisk(entry CMEntry) string {
	if entry.SizeKB > 1024 { // >1MB
		return "high"
	}
	if entry.SizeKB > 512 { // >512KB
		return "medium"
	}
	return "low"
}

// cmsScore computes health score 0-100.
func cmsScore(s CMSummary) int {
	score := 100
	score -= s.OversizedCMs * 5
	score -= s.OversizedSecrets * 5
	if s.TotalCMSizeMB > 100 {
		score -= 5
	}
	if s.TotalSecretSizeMB > 50 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	return score
}

// cmsGenRecs produces actionable advice.
func cmsGenRecs(s CMSummary, oversizedCMs []CMEntry, oversizedSecrets []CMEntry) []string {
	var recs []string

	if s.OversizedCMs > 0 {
		top := ""
		if len(oversizedCMs) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %.1fKB)", oversizedCMs[0].Namespace, oversizedCMs[0].Name, oversizedCMs[0].SizeKB)
		}
		recs = append(recs, fmt.Sprintf("%d ConfigMap(s) exceed 1MB%s — move large data to external storage (database, object store)", s.OversizedCMs, top))
	}
	if s.OversizedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d Secret(s) exceed 1MB — use external secret management (Vault, Sealed Secrets)", s.OversizedSecrets))
	}
	if s.TotalCMSizeMB > 50 {
		recs = append(recs, fmt.Sprintf("Total ConfigMap data is %.1fMB — large configs slow API server list operations", s.TotalCMSizeMB))
	}
	if s.TotalSecretSizeMB > 20 {
		recs = append(recs, fmt.Sprintf("Total Secret data is %.1fMB — consider external secret stores to reduce etcd load", s.TotalSecretSizeMB))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Config storage health score is %d/100 — review ConfigMap/Secret sizes", s.HealthScore))
	}
	if s.OversizedCMs == 0 && s.OversizedSecrets == 0 {
		recs = append(recs, fmt.Sprintf("All ConfigMaps/Secrets are within healthy size range (total: %.1fMB CM, %.1fMB Secret)", s.TotalCMSizeMB, s.TotalSecretSizeMB))
	}

	return recs
}

func cmsGetOrCreateNS(m map[string]*CMNSEntry, ns string) *CMNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &CMNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func cmsIssueRank(s string) int {
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

var _ = strings.Contains
