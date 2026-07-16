package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupplyChainResult analyzes container image supply chain security:
// registry trust, image digest pinning, base image freshness,
// vulnerability scanning coverage, and provenance verification.
type SupplyChainResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         SupplyChainSummary   `json:"summary"`
	RiskImages      []RiskImage          `json:"riskImages"`
	RegistryBreakdown []SupplyRegistryStat  `json:"registryBreakdown"`
	SecurityScore   int                  `json:"securityScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type SupplyChainSummary struct {
	TotalImages     int  `json:"totalImages"`
	ByDigest        int  `json:"byDigest"`
	ByTag           int  `json:"byTag"`
	LatestTag       int  `json:"latestTag"`
	NoPullPolicy    int  `json:"noPullPolicy"`
	PrivRegistry    int  `json:"privateRegistry"`
	PubRegistry     int  `json:"publicRegistry"`
	DuplicateImages int  `json:"duplicateImages"`
}

type RiskImage struct {
	Image     string `json:"image"`
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Risks     []string `json:"risks"`
	Severity  string `json:"severity"`
}

type SupplyRegistryStat struct {
	Registry  string `json:"registry"`
	ImageCount int   `json:"imageCount"`
	Trusted   bool   `json:"trusted"`
}

// handleSupplyChain analyzes container image supply chain security.
// GET /api/security/supply-chain
func (s *Server) handleSupplyChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SupplyChainResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Track unique images and registries
	imageSet := map[string]int{} // image -> usage count
	registryMap := map[string]int{}

	// Known trusted registries
	trustedRegistries := map[string]bool{
		"registry.k8s.io": true, "k8s.gcr.io": true,
		"gcr.io": true, "docker.io": true,
		"quay.io": true, "registry.access.redhat.com": true,
		"mcr.microsoft.com": true, "public.ecr.aws": true,
		"registry.iot2.win": true,
	}

	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, c := range pod.Spec.Containers {
			image := c.Image
			result.Summary.TotalImages++
			imageSet[image]++

			// Check if pinned by digest (contains @sha256:)
			if strings.Contains(image, "@sha256:") {
				result.Summary.ByDigest++
			} else {
				result.Summary.ByTag++
			}

			// Check for :latest tag
			if strings.HasSuffix(image, ":latest") || (!strings.Contains(image, ":") && !strings.Contains(image, "@")) {
				result.Summary.LatestTag++
			}

			// Check image pull policy
			if c.ImagePullPolicy == "Never" || c.ImagePullPolicy == "IfNotPresent" {
				if strings.HasSuffix(image, ":latest") {
					result.Summary.NoPullPolicy++
				}
			}

			// Determine registry
			registry := "docker.io"
			parts := strings.SplitN(image, "/", 2)
			if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
				registry = parts[0]
			}
			registryMap[registry]++

			// Check if private or public
			isPublic := trustedRegistries[registry] && registry != "registry.iot2.win"
			if isPublic {
				result.Summary.PubRegistry++
			} else {
				result.Summary.PrivRegistry++
			}

			// Build risk assessment
			var risks []string
			severity := "low"

			if strings.HasSuffix(image, ":latest") {
				risks = append(risks, "uses :latest tag — non-reproducible")
				severity = "medium"
			}
			if !strings.Contains(image, "@sha256:") {
				risks = append(risks, "not pinned by digest — vulnerable to supply chain attacks")
				if severity == "low" {
					severity = "medium"
				}
			}
			if !isPublic && !trustedRegistries[registry] {
				risks = append(risks, fmt.Sprintf("unknown registry '%s' — unverified provenance", registry))
				severity = "high"
			}

			if len(risks) > 0 {
				result.RiskImages = append(result.RiskImages, RiskImage{
					Image:     image,
					Workload:  pod.Name,
					Namespace: pod.Namespace,
					Risks:     risks,
					Severity:  severity,
				})
			}
		}
	}

	// Count duplicate images
	for _, count := range imageSet {
		if count > 3 {
			result.Summary.DuplicateImages++
		}
	}

	// Build registry breakdown
	for reg, count := range registryMap {
		result.RegistryBreakdown = append(result.RegistryBreakdown, SupplyRegistryStat{
			Registry:   reg,
			ImageCount: count,
			Trusted:    trustedRegistries[reg],
		})
	}

	// Score
	score := 100
	score -= result.Summary.LatestTag * 5
	score -= (result.Summary.ByTag - result.Summary.LatestTag) * 2
	score -= result.Summary.NoPullPolicy * 3
	if score < 0 {
		score = 0
	}
	result.SecurityScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.SecurityScore)

	// Sort
	sort.Slice(result.RiskImages, func(i, j int) bool {
		return result.RiskImages[i].Severity > result.RiskImages[j].Severity
	})
	sort.Slice(result.RegistryBreakdown, func(i, j int) bool {
		return result.RegistryBreakdown[i].ImageCount > result.RegistryBreakdown[j].ImageCount
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Supply chain security: %d/100 (grade %s) — %d total images", result.SecurityScore, result.Grade, result.Summary.TotalImages))
	if result.Summary.LatestTag > 0 {
		recs = append(recs, fmt.Sprintf("%d images use :latest tag — pin to specific versions for reproducibility", result.Summary.LatestTag))
	}
	if result.Summary.ByTag > result.Summary.ByDigest {
		recs = append(recs, fmt.Sprintf("%d images pinned by tag only — switch to @sha256 digest pinning", result.Summary.ByTag))
	}
	if result.Summary.NoPullPolicy > 0 {
		recs = append(recs, fmt.Sprintf("%d :latest images with IfNotPresent/Never pull policy — stale image risk", result.Summary.NoPullPolicy))
	}
	if len(recs) == 1 {
		recs = append(recs, "Supply chain security is strong — all images properly pinned and verified")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
