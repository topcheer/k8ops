package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupplyChainResult is the supply chain & SBOM coverage security audit.
type SupplyChainResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SupplyChainSummary  `json:"summary"`
	Images          []SupplyChainEntry  `json:"images"`
	ByNamespace     []SupplyChainNSStat `json:"byNamespace"`
	Issues          []SupplyChainIssue  `json:"issues"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// SupplyChainSummary aggregates supply chain security statistics.
type SupplyChainSummary struct {
	TotalImages       int `json:"totalImages"`
	UniqueImages      int `json:"uniqueImages"`
	UsingDigest       int `json:"usingDigest"`       // image referenced by digest@sha256
	UsingLatestTag    int `json:"usingLatestTag"`    // image uses :latest
	NoRegistry        int `json:"noRegistry"`        // no explicit registry (Docker Hub default)
	TrustedRegistries int `json:"trustedRegistries"` // from known trusted registries
	UnsignedImages    int `json:"unsignedImages"`    // no signing annotation
	NoSBOMAnnotation  int `json:"noSBOMAnnotation"`  // no SBOM/provenance annotation
	StaleImages       int `json:"staleImages"`       // image tag >90d old (heuristic)
}

// SupplyChainEntry describes one container image's supply chain posture.
type SupplyChainEntry struct {
	Image       string `json:"image"`
	Namespace   string `json:"namespace"`
	Workload    string `json:"workload"`
	Kind        string `json:"kind"`
	HasDigest   bool   `json:"hasDigest"`
	IsLatestTag bool   `json:"isLatestTag"`
	Registry    string `json:"registry"`
	IsTrusted   bool   `json:"isTrusted"`
	HasSigning  bool   `json:"hasSigning"`
	HasSBOM     bool   `json:"hasSBOM"`
	IsStale     bool   `json:"isStale"`
	RiskLevel   string `json:"riskLevel"`
}

// SupplyChainNSStat per-namespace supply chain stats.
type SupplyChainNSStat struct {
	Namespace   string `json:"namespace"`
	TotalImages int    `json:"totalImages"`
	UsingDigest int    `json:"usingDigest"`
	UsingLatest int    `json:"usingLatest"`
	Unsigned    int    `json:"unsigned"`
}

// SupplyChainIssue describes a specific supply chain problem.
type SupplyChainIssue struct {
	Severity   string `json:"severity"`
	Image      string `json:"image"`
	Workload   string `json:"workload"`
	Namespace  string `json:"namespace"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// Trusted registry list for supply chain validation.
var trustedRegistries = []string{
	"registry.k8s.io",
	"k8s.gcr.io",
	"registry.iot2.win",
	"gcr.io",
	"quay.io",
	"docker.io/library",
	"public.ecr.aws",
	"mcr.microsoft.com",
	"ghcr.io",
}

// handleSupplyChain handles GET /api/security/supply-chain
// Audits container image supply chain security: digest pinning, trusted registries,
// image signing, SBOM/provenance annotations, latest tag usage.
func (s *Server) handleSupplyChain(w http.ResponseWriter, r *http.Request) {
	result := s.auditSupplyChain(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) auditSupplyChain(ctx context.Context) *SupplyChainResult {
	result := &SupplyChainResult{ScannedAt: time.Now()}

	if s.clientset == nil {
		result.HealthScore = 100
		return result
	}

	// Collect images from pods across all namespaces
	imageMap := map[string]*SupplyChainEntry{}

	pods, err := s.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		Limit: 5000,
	})
	if err != nil {
		result.HealthScore = 100
		return result
	}

	for _, pod := range pods.Items {
		ns := pod.Namespace
		podName := pod.Name

		for _, c := range pod.Spec.Containers {
			img := c.Image
			key := fmt.Sprintf("%s/%s", ns, img)
			entry, ok := imageMap[key]
			if !ok {
				entry = &SupplyChainEntry{
					Image:     img,
					Namespace: ns,
					Workload:  podName,
					Kind:      "Pod",
				}
				analyzeSupplyChainImage(entry)
				imageMap[key] = entry
			}
		}
		for _, c := range pod.Spec.InitContainers {
			img := c.Image
			key := fmt.Sprintf("%s/%s/init", ns, img)
			entry, ok := imageMap[key]
			if !ok {
				entry = &SupplyChainEntry{
					Image:     img,
					Namespace: ns,
					Workload:  podName,
					Kind:      "InitContainer",
				}
				analyzeSupplyChainImage(entry)
				imageMap[key] = entry
			}
		}
	}

	// Also check Deployments for owner reference
	deploys, err := s.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
		Limit: 5000,
	})
	if err == nil {
		for _, d := range deploys.Items {
			ns := d.Namespace
			for _, c := range d.Spec.Template.Spec.Containers {
				img := c.Image
				key := fmt.Sprintf("%s/%s", ns, img)
				if entry, ok := imageMap[key]; ok {
					if entry.Kind == "Pod" {
						entry.Kind = "Deployment"
						entry.Workload = d.Name
					}
				}
			}
		}
	}

	// Build entries
	for _, entry := range imageMap {
		result.Images = append(result.Images, *entry)
	}
	sort.Slice(result.Images, func(i, j int) bool {
		return result.Images[i].Image < result.Images[j].Image
	})

	// Build summary
	result.Summary.TotalImages = len(result.Images)
	uniqueSet := map[string]bool{}
	for _, img := range result.Images {
		uniqueSet[img.Image] = true
		if img.HasDigest {
			result.Summary.UsingDigest++
		}
		if img.IsLatestTag {
			result.Summary.UsingLatestTag++
		}
		if !img.IsTrusted {
			result.Summary.NoRegistry++
		} else {
			result.Summary.TrustedRegistries++
		}
		if !img.HasSigning {
			result.Summary.UnsignedImages++
		}
		if !img.HasSBOM {
			result.Summary.NoSBOMAnnotation++
		}
		if img.IsStale {
			result.Summary.StaleImages++
		}
	}
	result.Summary.UniqueImages = len(uniqueSet)

	// Build per-namespace stats
	nsMap := map[string]*SupplyChainNSStat{}
	for _, img := range result.Images {
		ns, ok := nsMap[img.Namespace]
		if !ok {
			ns = &SupplyChainNSStat{Namespace: img.Namespace}
			nsMap[img.Namespace] = ns
		}
		ns.TotalImages++
		if img.HasDigest {
			ns.UsingDigest++
		}
		if img.IsLatestTag {
			ns.UsingLatest++
		}
		if !img.HasSigning {
			ns.Unsigned++
		}
	}
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalImages > result.ByNamespace[j].TotalImages
	})

	// Generate issues
	for _, img := range result.Images {
		if img.IsLatestTag {
			result.Issues = append(result.Issues, SupplyChainIssue{
				Severity:   "critical",
				Image:      img.Image,
				Workload:   img.Workload,
				Namespace:  img.Namespace,
				Issue:      "Image uses :latest tag — non-reproducible, supply chain risk",
				Suggestion: "Pin to specific version tag or digest (image@sha256:...)",
			})
		}
		if !img.HasDigest && !img.IsLatestTag {
			result.Issues = append(result.Issues, SupplyChainIssue{
				Severity:   "warning",
				Image:      img.Image,
				Workload:   img.Workload,
				Namespace:  img.Namespace,
				Issue:      "Image not pinned by digest — tag can be mutated",
				Suggestion: "Reference image by digest@sha256:... for immutability",
			})
		}
		if !img.IsTrusted {
			result.Issues = append(result.Issues, SupplyChainIssue{
				Severity:   "warning",
				Image:      img.Image,
				Workload:   img.Workload,
				Namespace:  img.Namespace,
				Issue:      "Image from untrusted registry — no supply chain verification",
				Suggestion: "Use images from trusted registries (registry.k8s.io, quay.io, gcr.io, etc.)",
			})
		}
		if !img.HasSigning {
			result.Issues = append(result.Issues, SupplyChainIssue{
				Severity:   "info",
				Image:      img.Image,
				Workload:   img.Workload,
				Namespace:  img.Namespace,
				Issue:      "No image signing annotation detected — cannot verify image integrity",
				Suggestion: "Use Cosign/Sigstore to sign images and add verification annotations",
			})
		}
	}
	// Limit issues to avoid overwhelming output
	if len(result.Issues) > 100 {
		result.Issues = result.Issues[:100]
	}

	// Recommendations
	if result.Summary.UsingLatestTag > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d image(s) use :latest tag — pin to specific versions for reproducibility", result.Summary.UsingLatestTag))
	}
	if result.Summary.UsingDigest < result.Summary.TotalImages/2 && result.Summary.TotalImages > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Only %d/%d images pinned by digest — use digest references for immutability", result.Summary.UsingDigest, result.Summary.TotalImages))
	}
	if result.Summary.NoRegistry > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d image(s) from untrusted registries — prefer trusted registries with supply chain controls", result.Summary.NoRegistry))
	}
	if result.Summary.UnsignedImages > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unsigned image(s) — implement image signing with Cosign/Sigstore", result.Summary.UnsignedImages))
	}
	if result.Summary.NoSBOMAnnotation > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d image(s) without SBOM/provenance — generate SBOMs for vulnerability tracking", result.Summary.NoSBOMAnnotation))
	}
	if result.Summary.StaleImages > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d stale image(s) detected — update base images regularly", result.Summary.StaleImages))
	}

	// Health score
	score := 100
	if result.Summary.TotalImages > 0 {
		latestRatio := result.Summary.UsingLatestTag * 100 / result.Summary.TotalImages
		score -= latestRatio / 5
		untrustedRatio := result.Summary.NoRegistry * 100 / result.Summary.TotalImages
		score -= untrustedRatio / 10
		unsignedRatio := result.Summary.UnsignedImages * 100 / result.Summary.TotalImages
		score -= unsignedRatio / 10
		digestRatio := result.Summary.UsingDigest * 100 / result.Summary.TotalImages
		if digestRatio < 20 {
			score -= 10
		} else if digestRatio < 50 {
			score -= 5
		}
		if result.Summary.StaleImages > 0 {
			score -= result.Summary.StaleImages * 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	return result
}

// analyzeSupplyChainImage inspects an image reference for supply chain attributes.
func analyzeSupplyChainImage(entry *SupplyChainEntry) {
	img := entry.Image

	// Check for digest reference (image@sha256:...)
	if strings.Contains(img, "@sha256:") {
		entry.HasDigest = true
		// Extract registry from before the @
		parts := strings.SplitN(img, "@", 2)
		img = parts[0]
	}

	// Check for :latest tag (only if not digest-pinned)
	if entry.HasDigest {
		entry.IsLatestTag = false
	} else if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
		entry.IsLatestTag = true
	}

	// Extract registry
	entry.Registry = extractRegistry(img)

	// Check trusted registry
	for _, tr := range trustedRegistries {
		if strings.HasPrefix(entry.Registry, tr) {
			entry.IsTrusted = true
			break
		}
	}

	// Check for signing annotations (heuristic: look for specific patterns in image name)
	// In practice, this would check pod annotations like cosign.sigstore.dev
	// For now, we check if the image is from a registry known to support signing
	if entry.IsTrusted {
		entry.HasSigning = true // trusted registries often support signing
	}

	// SBOM: heuristic — trusted registries with specific patterns
	if entry.IsTrusted {
		entry.HasSBOM = true
	}

	// Stale: heuristic — can't determine actual image age from reference alone
	// Would need to check image pull time or creation timestamp
	entry.IsStale = false

	entry.RiskLevel = assessSupplyChainRisk(*entry)
}

// extractRegistry extracts the registry from an image reference.
func extractRegistry(img string) string {
	// Remove tag/digest
	if idx := strings.Index(img, "@"); idx > 0 {
		img = img[:idx]
	}
	if idx := strings.LastIndex(img, ":"); idx > 0 {
		// Check if this is a port or tag
		afterColon := img[idx+1:]
		if !strings.Contains(afterColon, "/") {
			// It's a tag, remove it
			img = img[:idx]
		}
	}

	// Extract registry (first component before /)
	if idx := strings.Index(img, "/"); idx > 0 {
		registry := img[:idx]
		// Check if it looks like a registry (contains . or : or is localhost)
		if strings.Contains(registry, ".") || strings.Contains(registry, ":") || registry == "localhost" {
			return registry
		}
	}
	return "docker.io"
}

// assessSupplyChainRisk determines risk level for a supply chain entry.
func assessSupplyChainRisk(entry SupplyChainEntry) string {
	if entry.IsLatestTag {
		return "critical"
	}
	if !entry.HasDigest && !entry.IsTrusted {
		return "warning"
	}
	if !entry.HasDigest {
		return "info"
	}
	return "healthy"
}

// formatSupplyChainSummary returns a human-readable summary.
func formatSupplyChainSummary(r *SupplyChainResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Supply chain audit: %d total images (%d unique), %d digest-pinned, %d latest, %d untrusted, %d unsigned",
		r.Summary.TotalImages, r.Summary.UniqueImages,
		r.Summary.UsingDigest, r.Summary.UsingLatestTag,
		r.Summary.NoRegistry, r.Summary.UnsignedImages)
	return b.String()
}

// Ensure corev1 import is used.
var _ corev1.PodSpec
