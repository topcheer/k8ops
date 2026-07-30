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
// v21.10 — Deployment Dimension (Round 38)
// 1. Container Readiness Gate Audit
// 2. Pod Disruption Budget Min Available Validator
// 3. Deployment Container Image Digest Pinning
// ============================================================

type ReadyGateResult2110 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ReadyGateSummary2110 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type ReadyGateSummary2110 struct {
	TotalPods   int `json:"totalPods"`
	WithReadyGt int `json:"withReadinessGates"`
}

func (s *Server) handleReadyGate2110(w http.ResponseWriter, r *http.Request) {
	result := ReadyGateResult2110{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ReadinessGates) > 0 {
			result.Summary.WithReadyGt++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PDB Min Available Validator
type PDBMinResult2110 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PDBMinSummary2110 `json:"summary"`
	Issues          []PDBMinEntry2110 `json:"issues"`
	Recommendations []string          `json:"recommendations"`
}

type PDBMinSummary2110 struct {
	TotalPDBs int `json:"totalPDBs"`
	WithMin   int `json:"withMinAvailable"`
}

type PDBMinEntry2110 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePDBMin2110(w http.ResponseWriter, r *http.Request) {
	result := PDBMinResult2110{ScannedAt: time.Now()}
	score := 100
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})

	for _, pdb := range pdbList.Items {
		result.Summary.TotalPDBs++
		if pdb.Spec.MinAvailable != nil {
			result.Summary.WithMin++
		} else {
			result.Issues = append(result.Issues, PDBMinEntry2110{Name: pdb.Name, Namespace: pdb.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Namespace < result.Issues[j].Namespace })
	writeJSON(w, result)
}

// 3. Image Digest Pinning
type ImgDigestResult2110 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ImgDigestSummary2110 `json:"summary"`
	Unpinned        []ImgDigestEntry2110 `json:"unpinnedImages"`
	Recommendations []string             `json:"recommendations"`
}

type ImgDigestSummary2110 struct {
	TotalImages int `json:"totalImages"`
	Pinned      int `json:"pinnedByDigest"`
	Unpinned    int `json:"unpinned"`
}

type ImgDigestEntry2110 struct {
	Image string `json:"image"`
}

func (s *Server) handleImgDigest2110(w http.ResponseWriter, r *http.Request) {
	result := ImgDigestResult2110{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seen[c.Image] {
				continue
			}
			seen[c.Image] = true
			result.Summary.TotalImages++
			if containsStr2039(c.Image, "@sha256:") {
				result.Summary.Pinned++
			} else {
				result.Summary.Unpinned++
				result.Unpinned = append(result.Unpinned, ImgDigestEntry2110{Image: c.Image})
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Unpinned > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d images not pinned by digest — pin for reproducibility", result.Summary.Unpinned))
	}
	writeJSON(w, result)
}
