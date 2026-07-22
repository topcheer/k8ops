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
// v19.27 — Product Dimension (Round 7)
// 1. Secret Version History — secret rotation & age tracking
// 2. CRD Health Monitor — CRD usage & operator health
// 3. Workload Autosize Recommender — right-size based on spec analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Secret Version History — secret rotation & age tracking
// ---------------------------------------------------------------

type SecretVersionResult1927 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         SecretVersionSummary1927 `json:"summary"`
	Secrets         []SecretVersionEntry1927 `json:"secrets"`
	Stale           []SecretStaleEntry1927   `json:"stale"`
	ByType          []SecretTypeStat1927     `json:"byType"`
	Recommendations []string                 `json:"recommendations"`
}

type SecretVersionSummary1927 struct {
	TotalSecrets  int     `json:"totalSecrets"`
	OlderThan90d  int     `json:"olderThan90d"`
	OlderThan180d int     `json:"olderThan180d"`
	NeverRotated  int     `json:"neverRotated"`
	EmptySecrets  int     `json:"emptySecrets"`
	DuplicateKeys int     `json:"duplicateKeys"`
	AvgAgeDays    float64 `json:"avgAgeDays"`
}

type SecretVersionEntry1927 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Type       string  `json:"type"`
	KeyCount   int     `json:"keyCount"`
	AgeDays    float64 `json:"ageDays"`
	DataSize   int     `json:"dataSizeBytes"`
	MountCount int     `json:"mountCount"`
}

type SecretStaleEntry1927 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	AgeDays   float64 `json:"ageDays"`
	Reason    string  `json:"reason"`
}

type SecretTypeStat1927 struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (s *Server) handleSecretVersionHistory(w http.ResponseWriter, r *http.Request) {
	result := SecretVersionResult1927{
		ScannedAt: time.Now(),
	}
	score := 100

	secList, err := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Count mounts per secret from pods
	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	mountCount := make(map[string]int) // "ns/name" -> count
	if err == nil {
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for _, vol := range pod.Spec.Volumes {
				if vol.Secret != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
					mountCount[key]++
				}
			}
		}
	}

	typeStats := make(map[string]int)
	var totalAge float64

	for _, sec := range secList.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		ageDays := time.Since(sec.CreationTimestamp.Time).Hours() / 24
		keyCount := len(sec.Data)
		dataSize := 0
		for _, v := range sec.Data {
			dataSize += len(v)
		}
		mc := mountCount[fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)]

		entry := SecretVersionEntry1927{
			Name:       sec.Name,
			Namespace:  sec.Namespace,
			Type:       string(sec.Type),
			KeyCount:   keyCount,
			AgeDays:    ageDays,
			DataSize:   dataSize,
			MountCount: mc,
		}
		result.Secrets = append(result.Secrets, entry)
		result.Summary.TotalSecrets++
		totalAge += ageDays

		typeStats[string(sec.Type)]++

		// Stale checks
		if ageDays > 180 {
			result.Stale = append(result.Stale, SecretStaleEntry1927{
				Name: sec.Name, Namespace: sec.Namespace, AgeDays: ageDays,
				Reason: "Secret older than 180 days — rotate immediately",
			})
			result.Summary.OlderThan180d++
			result.Summary.NeverRotated++
			score -= 3
		} else if ageDays > 90 {
			result.Stale = append(result.Stale, SecretStaleEntry1927{
				Name: sec.Name, Namespace: sec.Namespace, AgeDays: ageDays,
				Reason: "Secret older than 90 days — consider rotation",
			})
			result.Summary.OlderThan90d++
			score -= 1
		}

		if keyCount == 0 {
			result.Summary.EmptySecrets++
			score -= 2
		}

		// Unused secrets (not mounted anywhere)
		if mc == 0 {
			result.Stale = append(result.Stale, SecretStaleEntry1927{
				Name: sec.Name, Namespace: sec.Namespace, AgeDays: ageDays,
				Reason: "Secret not mounted by any pod — candidate for cleanup",
			})
		}
	}

	if result.Summary.TotalSecrets > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(result.Summary.TotalSecrets)
	}

	for t, c := range typeStats {
		result.ByType = append(result.ByType, SecretTypeStat1927{Type: t, Count: c})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OlderThan90d > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d secrets older than 90 days — establish rotation policy", result.Summary.OlderThan90d))
	}
	if result.Summary.EmptySecrets > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d empty secrets — remove unused artifacts", result.Summary.EmptySecrets))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. CRD Health Monitor — CRD usage & operator health
// ---------------------------------------------------------------

type CRDHealthResult1927 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CRDHealthSummary1927 `json:"summary"`
	CRDs            []CRDEntry1927       `json:"crds"`
	Issues          []CRDIssue1927       `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type CRDHealthSummary1927 struct {
	TotalCRDs      int `json:"totalCRDs"`
	WithInstances  int `json:"withInstances"`
	EmptyCRDs      int `json:"emptyCRDs"`
	OperatorsFound int `json:"operatorsFound"`
	StaleCRDs      int `json:"staleCRDs"`
	DeprecatedVer  int `json:"deprecatedVersions"`
}

type CRDEntry1927 struct {
	Name             string   `json:"name"`
	Group            string   `json:"group"`
	Kind             string   `json:"kind"`
	VersionCount     int      `json:"versionCount"`
	ServedVersions   []string `json:"servedVersions"`
	StorageVersion   string   `json:"storageVersion"`
	Scope            string   `json:"scope"`
	HasInstances     bool     `json:"hasInstances"`
	EstInstanceCount int      `json:"estInstanceCount"`
}

type CRDIssue1927 struct {
	Name      string `json:"name"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleCRDHealth(w http.ResponseWriter, r *http.Request) {
	result := CRDHealthResult1927{
		ScannedAt: time.Now(),
	}
	score := 100

	// CRDs: use discovery API to list custom resources
	_, apiResList, err := s.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, apiGroup := range apiResList {
		if !strings.Contains(apiGroup.GroupVersion, "/") {
			continue // skip core API groups
		}
		for _, res := range apiGroup.APIResources {
			if strings.Contains(res.Group, ".") {
				key := fmt.Sprintf("%s/%s", res.Group, res.Kind)
				scope := "Cluster"
				if res.Namespaced {
					scope = "Namespaced"
				}
				entry := CRDEntry1927{
					Name:  key,
					Group: res.Group,
					Kind:  res.Kind,
					Scope: scope,
				}
				result.CRDs = append(result.CRDs, entry)
				result.Summary.TotalCRDs++
			}
		}
	}

	// Check for operators
	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range depList.Items {
			nameLower := strings.ToLower(dep.Name)
			if strings.Contains(nameLower, "operator") || strings.Contains(nameLower, "controller-manager") {
				result.Summary.OperatorsFound++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DeprecatedVer > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deprecated CRD versions — migrate to current API version", result.Summary.DeprecatedVer))
	}
	if result.Summary.TotalCRDs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d CRDs registered — monitor operator reconciliation health", result.Summary.TotalCRDs))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Workload Autosize Recommender — right-size based on spec analysis
// -----------------------------------------------------------

type AutosizeResult1927 struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	HealthScore      int                 `json:"healthScore"`
	Grade            string              `json:"grade"`
	Summary          AutosizeSummary1927 `json:"summary"`
	OverProvisioned  []AutosizeEntry1927 `json:"overProvisioned"`
	UnderProvisioned []AutosizeEntry1927 `json:"underProvisioned"`
	NoRequests       []AutosizeEntry1927 `json:"noRequests"`
	Recommendations  []string            `json:"recommendations"`
}

type AutosizeSummary1927 struct {
	TotalWorkloads     int     `json:"totalWorkloads"`
	OverProvisioned    int     `json:"overProvisioned"`
	UnderProvisioned   int     `json:"underProvisioned"`
	NoRequests         int     `json:"noRequests"`
	EstSavingsCPUCores float64 `json:"estSavingsCPUCores"`
	EstSavingsMemMB    int     `json:"estSavingsMemMB"`
	EstMonthlySavings  float64 `json:"estMonthlySavingsUSD"`
}

type AutosizeEntry1927 struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	Kind           string  `json:"kind"`
	CurrentCPU     string  `json:"currentCPU"`
	CurrentMem     string  `json:"currentMem"`
	RecommendedCPU string  `json:"recommendedCPU"`
	RecommendedMem string  `json:"recommendedMem"`
	Reason         string  `json:"reason"`
	SavingsUSD     float64 `json:"estSavingsUSD"`
}

func (s *Server) handleAutosizeRecommender(w http.ResponseWriter, r *http.Request) {
	result := AutosizeResult1927{
		ScannedAt: time.Now(),
	}
	score := 100
	var savedCPU float64
	var savedMemMB int

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}

		var totalCPU float64
		var totalMemMB int
		hasRequest := false
		var cpuReqStr, memReqStr string

		for _, c := range dep.Spec.Template.Spec.Containers {
			cpuQ := c.Resources.Requests.Cpu()
			memQ := c.Resources.Requests.Memory()
			if !cpuQ.IsZero() {
				totalCPU += cpuQ.AsApproximateFloat64()
				cpuReqStr = cpuQ.String()
				hasRequest = true
			}
			if !memQ.IsZero() {
				totalMemMB += int(memQ.Value() / (1024 * 1024))
				memReqStr = memQ.String()
			}
		}

		totalCPU *= float64(replicas)
		totalMemMB *= replicas

		if !hasRequest {
			result.NoRequests = append(result.NoRequests, AutosizeEntry1927{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				Reason: "No CPU/memory requests set — unpredictable scheduling",
			})
			result.Summary.NoRequests++
			score -= 3
			continue
		}

		// Over-provisioned heuristics: >2 cores or >4Gi per replica
		perReplicaCPU := totalCPU / float64(replicas)
		perReplicaMem := totalMemMB / replicas

		if perReplicaCPU > 2.0 || perReplicaMem > 4096 {
			recCPU := perReplicaCPU * 0.5 // recommend 50% reduction
			if recCPU < 0.1 {
				recCPU = 0.1
			}
			recMemMB := perReplicaMem / 2
			if recMemMB < 128 {
				recMemMB = 128
			}
			savedCPUTotal := (perReplicaCPU - recCPU) * float64(replicas)
			savedMemTotal := (perReplicaMem - recMemMB) * replicas
			savingsUSD := savedCPUTotal*28 + float64(savedMemTotal)/1024*3.5

			result.OverProvisioned = append(result.OverProvisioned, AutosizeEntry1927{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				CurrentCPU: cpuReqStr, CurrentMem: memReqStr,
				RecommendedCPU: fmt.Sprintf("%.1f", recCPU),
				RecommendedMem: fmt.Sprintf("%dMi", int(recMemMB)),
				Reason:         fmt.Sprintf("High resource request: %.2f CPU, %dMB per replica", perReplicaCPU, perReplicaMem),
				SavingsUSD:     savingsUSD,
			})
			result.Summary.OverProvisioned++
			savedCPU += savedCPUTotal
			savedMemMB += int(savedMemTotal)
		}

		// Under-provisioned: <100m CPU or <128Mi
		if perReplicaCPU < 0.1 && perReplicaCPU > 0 {
			result.UnderProvisioned = append(result.UnderProvisioned, AutosizeEntry1927{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				CurrentCPU: cpuReqStr, CurrentMem: memReqStr,
				RecommendedCPU: "250m", RecommendedMem: "256Mi",
				Reason: "Very low CPU request (<100m) — may cause throttling",
			})
			result.Summary.UnderProvisioned++
			score -= 1
		}
		if perReplicaMem > 0 && perReplicaMem < 128 {
			result.UnderProvisioned = append(result.UnderProvisioned, AutosizeEntry1927{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				CurrentCPU: cpuReqStr, CurrentMem: memReqStr,
				RecommendedCPU: "500m", RecommendedMem: "256Mi",
				Reason: "Very low memory request (<128Mi) — OOMKill risk",
			})
			result.Summary.UnderProvisioned++
			score -= 1
		}
	}

	result.Summary.EstSavingsCPUCores = savedCPU
	result.Summary.EstSavingsMemMB = savedMemMB
	result.Summary.EstMonthlySavings = savedCPU*28 + float64(savedMemMB)/1024*3.5

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OverProvisioned > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d over-provisioned workloads — right-size to save $%.2f/month", result.Summary.OverProvisioned, result.Summary.EstMonthlySavings))
	}
	if result.Summary.UnderProvisioned > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d under-provisioned workloads — increase requests to prevent throttling", result.Summary.UnderProvisioned))
	}
	if result.Summary.NoRequests > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads with no requests — add for reliable scheduling", result.Summary.NoRequests))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
