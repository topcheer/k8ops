package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.33 — Product Dimension (Round 8)
// 1. Service Mesh Readiness Checker — sidecar & mesh enrollment
// 2. Volume Access Mode Audit — RWO/ROX/RWX compatibility
// 3. Pod Disruption Budget Gap — PDB coverage analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Service Mesh Readiness Checker
// ---------------------------------------------------------------

type MeshReadyResult1933 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         MeshReadySummary1933 `json:"summary"`
	Pods            []MeshPodEntry1933   `json:"pods"`
	Gaps            []MeshGapEntry1933   `json:"gaps"`
	Recommendations []string             `json:"recommendations"`
}

type MeshReadySummary1933 struct {
	TotalPods       int `json:"totalPods"`
	WithSidecar     int `json:"withSidecar"`
	WithoutSidecar  int `json:"withoutSidecar"`
	MeshInjected    int `json:"meshInjected"`
	ManualSidecar   int `json:"manualSidecar"`
	WithAnnotations int `json:"withMeshAnnotations"`
}

type MeshPodEntry1933 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	HasSidecar  bool   `json:"hasSidecar"`
	SidecarName string `json:"sidecarName"`
	Injected    bool   `json:"injected"`
}

type MeshGapEntry1933 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

func (s *Server) handleMeshReadyCheck(w http.ResponseWriter, r *http.Request) {
	result := MeshReadyResult1933{ScannedAt: time.Now()}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	meshSidecars := map[string]bool{
		"istio-proxy": true, "envoy": true, "linkerd-proxy": true,
		"sidecar": true, "consul-connect": true, "cilium-proxy": true,
	}
	meshAnnotations := []string{"sidecar.istio.io", "linkerd.io", "consul.hashicorp.com"}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasSidecar := false
		sidecarName := ""
		injected := false

		for _, c := range pod.Spec.Containers {
			if meshSidecars[c.Name] {
				hasSidecar = true
				sidecarName = c.Name
				break
			}
		}

		for k := range pod.Annotations {
			for _, ma := range meshAnnotations {
				if containsStr1933(k, ma) {
					result.Summary.WithAnnotations++
					injected = true
					break
				}
			}
		}

		entry := MeshPodEntry1933{
			Name: pod.Name, Namespace: pod.Namespace,
			HasSidecar: hasSidecar, SidecarName: sidecarName, Injected: injected,
		}
		result.Pods = append(result.Pods, entry)

		if hasSidecar {
			result.Summary.WithSidecar++
			if injected {
				result.Summary.MeshInjected++
			} else {
				result.Summary.ManualSidecar++
			}
		} else {
			result.Summary.WithoutSidecar++
			if len(pod.Spec.Containers) > 0 {
				result.Gaps = append(result.Gaps, MeshGapEntry1933{
					Name: pod.Name, Namespace: pod.Namespace,
					Reason:   "Pod has no mesh sidecar — no mTLS, tracing, or traffic control",
					Severity: "low",
				})
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutSidecar > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods without mesh sidecar — enroll for mTLS & observability", result.Summary.WithoutSidecar))
	}
	if result.Summary.ManualSidecar > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with manual sidecar — use auto-injection for consistency", result.Summary.ManualSidecar))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume Access Mode Audit
// ---------------------------------------------------------------

type VolAccessResult1933 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         VolAccessSummary1933 `json:"summary"`
	Volumes         []VolAccessEntry1933 `json:"volumes"`
	Issues          []VolAccessIssue1933 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type VolAccessSummary1933 struct {
	TotalPVCs    int `json:"totalPVCs"`
	RWOCount     int `json:"rwoCount"`
	ROXCount     int `json:"roxCount"`
	RWXCount     int `json:"rwxCount"`
	MultiAccess  int `json:"multiAccessPVs"`
	SingleAccess int `json:"singleAccessPVs"`
	SharedPVs    int `json:"sharedPVs"`
}

type VolAccessEntry1933 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	AccessMode string `json:"accessMode"`
	SCName     string `json:"storageClass"`
	Size       string `json:"size"`
	Bound      bool   `json:"bound"`
}

type VolAccessIssue1933 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleVolAccessAudit(w http.ResponseWriter, r *http.Request) {
	result := VolAccessResult1933{ScannedAt: time.Now()}
	score := 100

	pvcList, err := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Count PVCs per PV to detect shared volumes
	pvRefCount := make(map[string]int)
	for _, pvc := range pvcList.Items {
		if pvc.Spec.VolumeName != "" {
			pvRefCount[pvc.Spec.VolumeName]++
		}
	}

	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		accessMode := ""
		if len(pvc.Spec.AccessModes) > 0 {
			accessMode = string(pvc.Spec.AccessModes[0])
		}
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		size := pvc.Spec.Resources.Requests.Storage().String()
		bound := pvc.Status.Phase == corev1.ClaimBound

		entry := VolAccessEntry1933{
			Name: pvc.Name, Namespace: pvc.Namespace,
			AccessMode: accessMode, SCName: scName, Size: size, Bound: bound,
		}
		result.Volumes = append(result.Volumes, entry)

		switch accessMode {
		case "ReadWriteOnce":
			result.Summary.RWOCount++
			result.Summary.SingleAccess++
		case "ReadOnlyMany":
			result.Summary.ROXCount++
		case "ReadWriteMany":
			result.Summary.RWXCount++
			result.Summary.MultiAccess++
			result.Summary.SharedPVs++
		}

		// Check for shared PV with RWO (potential conflict)
		if pvRefCount[pvc.Spec.VolumeName] > 1 && accessMode == "ReadWriteOnce" {
			result.Issues = append(result.Issues, VolAccessIssue1933{
				Name: pvc.Name, Namespace: pvc.Namespace,
				IssueType: "rwo-shared", Severity: "high",
				Detail: "RWO PVC appears shared by multiple pods — data corruption risk",
			})
			score -= 5
		}

		if !bound {
			result.Issues = append(result.Issues, VolAccessIssue1933{
				Name: pvc.Name, Namespace: pvc.Namespace,
				IssueType: "unbound", Severity: "medium",
				Detail: "PVC is not bound — storage not available",
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SharedPVs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d RWX volumes — ensure concurrent write safety", result.Summary.SharedPVs))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Disruption Budget Gap
// ---------------------------------------------------------------

type PDBGapResult1933 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PDBGapSummary1933     `json:"summary"`
	Covered         []PDBCoveredEntry1933 `json:"covered"`
	Gaps            []PDBGapEntry1933     `json:"gaps"`
	Recommendations []string              `json:"recommendations"`
}

type PDBGapSummary1933 struct {
	TotalWorkloads int `json:"totalWorkloads"`
	WithPDB        int `json:"withPDB"`
	WithoutPDB     int `json:"withoutPDB"`
	CriticalUnprot int `json:"criticalUnprotected"`
	MinAvailable   int `json:"pdbWithMinAvailable"`
	MaxUnavailable int `json:"pdbWithMaxUnavailable"`
}

type PDBCoveredEntry1933 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
	Value     string `json:"value"`
}

type PDBGapEntry1933 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	Severity  string `json:"severity"`
	Reason    string `json:"reason"`
}

func (s *Server) handlePDBGapAnalysisV2(w http.ResponseWriter, r *http.Request) {
	result := PDBGapResult1933{ScannedAt: time.Now()}
	score := 100

	pdbList, err := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Build PDB selector map
	pdbSelectors := make([]struct {
		namespace string
		labels    map[string]string
		name      string
		strategy  string
		value     string
	}, 0)
	pdbNS := make(map[string]bool)

	for _, pdb := range pdbList.Items {
		pdbNS[pdb.Namespace] = true
		strategy := "minAvailable"
		value := ""
		if pdb.Spec.MinAvailable != nil {
			value = pdb.Spec.MinAvailable.String()
		}
		if pdb.Spec.MaxUnavailable != nil {
			strategy = "maxUnavailable"
			value = pdb.Spec.MaxUnavailable.String()
		}
		pdbSelectors = append(pdbSelectors, struct {
			namespace string
			labels    map[string]string
			name      string
			strategy  string
			value     string
		}{pdb.Namespace, pdb.Spec.Selector.MatchLabels, pdb.Name, strategy, value})

		result.Summary.WithPDB++
		if strategy == "minAvailable" {
			result.Summary.MinAvailable++
		} else {
			result.Summary.MaxUnavailable++
		}
		result.Covered = append(result.Covered, PDBCoveredEntry1933{
			Name: pdb.Name, Namespace: pdb.Namespace, Strategy: strategy, Value: value,
		})
	}

	// Check deployments for PDB coverage
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}

		// Check if any PDB covers this deployment
		hasPDB := false
		for _, sel := range pdbSelectors {
			if sel.namespace != dep.Namespace {
				continue
			}
			if matchLabels1933(sel.labels, dep.Labels) {
				hasPDB = true
				break
			}
		}

		if !hasPDB {
			result.Summary.WithoutPDB++
			severity := "low"
			if replicas >= 3 {
				severity = "high"
			} else if replicas >= 2 {
				severity = "medium"
			}
			if replicas == 1 {
				severity = "critical"
			}
			result.Gaps = append(result.Gaps, PDBGapEntry1933{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas, Severity: severity,
				Reason: "No PDB — voluntary disruptions can cause downtime",
			})
			if severity == "critical" {
				result.Summary.CriticalUnprot++
				score -= 5
			} else if severity == "high" {
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutPDB > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads without PDB — add for voluntary disruption protection", result.Summary.WithoutPDB))
	}
	if result.Summary.CriticalUnprot > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d single-replica workloads without PDB — critical risk", result.Summary.CriticalUnprot))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// Helpers
func containsStr1933(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func matchLabels1933(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
