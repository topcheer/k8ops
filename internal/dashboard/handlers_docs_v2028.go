package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.28 — Documentation Dimension (Round 24)
// 1. Helm Release Inventory — Helm-managed resources documentation
// 2. Pod Disruption Budget Doc — PDB coverage inventory
// 3. Service Account Token Age — stale SA token documentation
// ============================================================

// ---------------------------------------------------------------
// 1. Helm Release Inventory
// ---------------------------------------------------------------

type HelmInvResult2028 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         HelmInvSummary2028 `json:"summary"`
	Releases        []HelmInvEntry2028 `json:"releases"`
	Recommendations []string           `json:"recommendations"`
}

type HelmInvSummary2028 struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	HelmNamespaces   int `json:"helmNamespaces"`
	HelmWorkloads    int `json:"helmWorkloads"`
	OrphanedReleases int `json:"orphanedReleases"`
}

type HelmInvEntry2028 struct {
	Namespace string `json:"namespace"`
	Release   string `json:"release"`
	Workloads int    `json:"workloads"`
	ChartVer  string `json:"chartVersion"`
}

func (s *Server) handleHelmReleaseInv(w http.ResponseWriter, r *http.Request) {
	result := HelmInvResult2028{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	// Build map of namespace -> helm release name
	nsReleases := make(map[string]map[string]*HelmInvEntry2028)

	for _, dep := range deployList.Items {
		rel := extractHelmRelease(dep.Annotations)
		if rel != "" {
			if nsReleases[dep.Namespace] == nil {
				nsReleases[dep.Namespace] = make(map[string]*HelmInvEntry2028)
			}
			if nsReleases[dep.Namespace][rel] == nil {
				chartVer := ""
				if v, ok := dep.Annotations["meta.helm.sh/release-name"]; ok {
					chartVer = v
				}
				nsReleases[dep.Namespace][rel] = &HelmInvEntry2028{
					Namespace: dep.Namespace, Release: rel, ChartVer: chartVer,
				}
			}
			nsReleases[dep.Namespace][rel].Workloads++
		}
	}

	for _, sts := range stsList.Items {
		rel := extractHelmRelease(sts.Annotations)
		if rel != "" {
			if nsReleases[sts.Namespace] == nil {
				nsReleases[sts.Namespace] = make(map[string]*HelmInvEntry2028)
			}
			if nsReleases[sts.Namespace][rel] == nil {
				nsReleases[sts.Namespace][rel] = &HelmInvEntry2028{
					Namespace: sts.Namespace, Release: rel,
				}
			}
			nsReleases[sts.Namespace][rel].Workloads++
		}
	}

	result.Summary.TotalNamespaces = len(nsList.Items)
	for ns, releases := range nsReleases {
		_ = ns
		result.Summary.HelmNamespaces++
		for _, entry := range releases {
			result.Summary.HelmWorkloads += entry.Workloads
			result.Releases = append(result.Releases, *entry)
		}
	}

	// No helm releases is not necessarily bad
	if result.Summary.HelmWorkloads == 0 {
		result.Recommendations = append(result.Recommendations,
			"No Helm-managed workloads detected — consider using Helm for lifecycle management")
	}

	score = 100 // Docs dimension is informational
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Releases, func(i, j int) bool {
		return result.Releases[i].Namespace < result.Releases[j].Namespace
	})

	writeJSON(w, result)
}

func extractHelmRelease(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	for k, v := range annotations {
		if k == "meta.helm.sh/release-name" {
			return v
		}
		if strings.Contains(k, "helm") && strings.Contains(k, "release") {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------
// 2. Pod Disruption Budget Doc
// ---------------------------------------------------------------

type PDBDocResult2028 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PDBDocSummary2028 `json:"summary"`
	Workloads       []PDBDocEntry2028 `json:"workloads"`
	Recommendations []string          `json:"recommendations"`
}

type PDBDocSummary2028 struct {
	TotalWorkloads int `json:"totalWorkloads"`
	WithPDB        int `json:"withPDB"`
	WithoutPDB     int `json:"withoutPDB"`
}

type PDBDocEntry2028 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	HasPDB    bool   `json:"hasPDB"`
	Replicas  int    `json:"replicas"`
}

func (s *Server) handlePDBDoc(w http.ResponseWriter, r *http.Request) {
	result := PDBDocResult2028{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	// Get PDBs
	pdbList, err := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	pdbSelectors := make(map[string]bool) // namespace -> has PDB

	if err == nil {
		for _, pdb := range pdbList.Items {
			key := pdb.Namespace + "/" + pdb.Name
			pdbSelectors[key] = true
		}
	}

	// Check deployments
	for _, dep := range deployList.Items {
		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}
		entry := PDBDocEntry2028{
			Name: dep.Name, Namespace: dep.Namespace,
			Kind: "Deployment", Replicas: replicas,
		}
		result.Summary.TotalWorkloads++
		entry.HasPDB = hasMatchingPDB(dep.Namespace, dep.Name, dep.Labels, pdbList)
		if entry.HasPDB {
			result.Summary.WithPDB++
		} else {
			result.Summary.WithoutPDB++
			if replicas > 1 {
				score -= 2
			}
		}
		result.Workloads = append(result.Workloads, entry)
	}

	// Check statefulsets
	for _, sts := range stsList.Items {
		replicas := 1
		if sts.Spec.Replicas != nil {
			replicas = int(*sts.Spec.Replicas)
		}
		entry := PDBDocEntry2028{
			Name: sts.Name, Namespace: sts.Namespace,
			Kind: "StatefulSet", Replicas: replicas,
		}
		result.Summary.TotalWorkloads++
		entry.HasPDB = hasMatchingPDB(sts.Namespace, sts.Name, sts.Labels, pdbList)
		if entry.HasPDB {
			result.Summary.WithPDB++
		} else {
			result.Summary.WithoutPDB++
			if replicas > 1 {
				score -= 2
			}
		}
		result.Workloads = append(result.Workloads, entry)
	}

	_ = appsv1.Deployment{} // keep import

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].Namespace < result.Workloads[j].Namespace
	})

	if result.Summary.WithoutPDB > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica workloads lack PDB — add PodDisruptionBudget for voluntary disruption protection", result.Summary.WithoutPDB))
	}

	writeJSON(w, result)
}

func hasMatchingPDB(ns, name string, labels map[string]string, pdbList *policyv1.PodDisruptionBudgetList) bool {
	_ = ns
	_ = name
	if pdbList == nil {
		return false
	}
	for _, pdb := range pdbList.Items {
		if pdb.Spec.Selector == nil {
			continue
		}
		sel := pdb.Spec.Selector.MatchLabels
		if len(sel) == 0 {
			continue
		}
		matched := true
		for k, v := range sel {
			if labels[k] != v {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 3. Service Account Token Age
// ---------------------------------------------------------------

type SATokenAgeResult2028 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenAgeSummary2028 `json:"summary"`
	OldTokens       []SATokenAgeEntry2028 `json:"oldTokens"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenAgeSummary2028 struct {
	TotalSAs      int `json:"totalServiceAccounts"`
	OldTokens     int `json:"oldTokens"`
	AncientTokens int `json:"ancientTokens"`
}

type SATokenAgeEntry2028 struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	CreatedAt time.Time `json:"createdAt"`
	AgeDays   int       `json:"ageDays"`
}

func (s *Server) handleSATokenAgeDoc(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult2028{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		ageDays := int(now.Sub(sa.CreationTimestamp.Time).Hours() / 24)

		if ageDays > 365 {
			result.Summary.OldTokens++
			if ageDays > 730 {
				result.Summary.AncientTokens++
				score -= 1
			}
			result.OldTokens = append(result.OldTokens, SATokenAgeEntry2028{
				Name: sa.Name, Namespace: sa.Namespace,
				CreatedAt: sa.CreationTimestamp.Time, AgeDays: ageDays,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OldTokens, func(i, j int) bool {
		return result.OldTokens[i].AgeDays > result.OldTokens[j].AgeDays
	})

	if result.Summary.AncientTokens > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d service accounts older than 2 years — review and rotate tokens", result.Summary.AncientTokens))
	}

	writeJSON(w, result)
}
