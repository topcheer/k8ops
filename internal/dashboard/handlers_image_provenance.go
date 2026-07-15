package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageProvenanceResult is the container image provenance & registry trust audit.
type ImageProvenanceResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ImageProvSummary    `json:"summary"`
	ByRegistry      []ImageProvRegistry `json:"byRegistry"`
	ByNamespace     []ImageProvNSStat   `json:"byNamespace"`
	Risks           []ImageProvRisk     `json:"risks"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// ImageProvSummary aggregates image provenance metrics.
type ImageProvSummary struct {
	TotalImages         int `json:"totalImages"`
	WithDigest          int `json:"withDigest"`          // pinned with @sha256
	WithTagOnly         int `json:"withTagOnly"`         // uses mutable tag
	LatestTag           int `json:"latestTag"`           // uses :latest
	TrustedRegistries   int `json:"trustedRegistries"`   // from trusted list
	UntrustedRegistries int `json:"untrustedRegistries"` // from public registries
	UniqueRegistries    int `json:"uniqueRegistries"`
	TotalContainers     int `json:"totalContainers"`
}

// ImageProvRegistry per-registry stats.
type ImageProvRegistry struct {
	Registry   string `json:"registry"`
	ImageCount int    `json:"imageCount"`
	Trusted    bool   `json:"trusted"`
}

// ImageProvNSStat per-namespace image provenance stats.
type ImageProvNSStat struct {
	Namespace   string `json:"namespace"`
	TotalImages int    `json:"totalImages"`
	WithDigest  int    `json:"withDigest"`
	LatestTag   int    `json:"latestTag"`
	Untrusted   int    `json:"untrusted"`
	RiskLevel   string `json:"riskLevel"`
}

// ImageProvRisk describes an image provenance risk.
type ImageProvRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Image     string `json:"image,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// Trusted registries list (configurable in future)
var imageProvTrustedRegistries = []string{
	"registry.iot2.win",
	"gcr.io",
	"quay.io",
	"registry.k8s.io",
	"k8s.gcr.io",
	"ecr.aws",
	"ghcr.io",
}

// Public registries (lower trust)
var publicRegistries = []string{
	"docker.io",
	"library/",
	"nginx",
	"redis",
	"postgres",
	"mysql",
	"busybox",
	"alpine",
	"ubuntu",
	"debian",
	"python",
	"node",
	"openjdk",
}

// handleImageProvenance audits container image provenance & registry trust.
// GET /api/security/image-provenance
func (s *Server) handleImageProvenance(w http.ResponseWriter, r *http.Request) {
	result := ImageProvenanceResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	registryMap := map[string]*ImageProvRegistry{}
	nsStats := map[string]*ImageProvNSStat{}
	seenImages := map[string]bool{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName == "" {
			continue
		}
		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &ImageProvNSStat{Namespace: ns, RiskLevel: "low"}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			img := c.Image
			if seenImages[img] {
				continue
			}
			seenImages[img] = true
			result.Summary.TotalImages++
			nsStats[ns].TotalImages++

			imgLower := strings.ToLower(img)
			hasDigest := strings.Contains(img, "@sha256:")
			isLatest := strings.HasSuffix(imgLower, ":latest") || !strings.Contains(img[strings.LastIndex(img, "/")+1:], ":")

			if hasDigest {
				result.Summary.WithDigest++
				nsStats[ns].WithDigest++
			} else {
				result.Summary.WithTagOnly++
				if isLatest {
					result.Summary.LatestTag++
					nsStats[ns].LatestTag++
					result.Risks = append(result.Risks, ImageProvRisk{
						Namespace: ns,
						Image:     img,
						Issue:     fmt.Sprintf("Container uses :latest tag or no tag — image is mutable and not reproducible"),
						Severity:  "warning",
					})
				}
			}

			// Determine registry
			registry := "docker.io"
			if idx := strings.Index(img, "/"); idx > 0 {
				prefix := img[:idx]
				if strings.Contains(prefix, ".") || strings.Contains(prefix, ":") {
					registry = prefix
				}
			}

			isTrusted := false
			for _, tr := range imageProvTrustedRegistries {
				if strings.HasPrefix(registry, tr) || strings.HasPrefix(tr, registry) {
					isTrusted = true
					break
				}
			}
			isPublic := false
			if !isTrusted {
				for _, pr := range publicRegistries {
					if strings.HasPrefix(imgLower, pr) || registry == "docker.io" {
						isPublic = true
						break
					}
				}
			}

			if isTrusted {
				result.Summary.TrustedRegistries++
			} else {
				result.Summary.UntrustedRegistries++
				nsStats[ns].Untrusted++
				if isPublic {
					result.Risks = append(result.Risks, ImageProvRisk{
						Namespace: ns,
						Image:     img,
						Issue:     fmt.Sprintf("Image from public untrusted registry (%s) — no provenance guarantee", registry),
						Severity:  "medium",
					})
				}
			}

			if registryMap[registry] == nil {
				registryMap[registry] = &ImageProvRegistry{Registry: registry, Trusted: isTrusted}
			}
			registryMap[registry].ImageCount++
		}
	}

	// Build registry slice
	for _, reg := range registryMap {
		result.ByRegistry = append(result.ByRegistry, *reg)
	}
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].ImageCount > result.ByRegistry[j].ImageCount
	})
	result.Summary.UniqueRegistries = len(registryMap)

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.Untrusted > 2 {
			stat.RiskLevel = "high"
		} else if stat.LatestTag > 2 {
			stat.RiskLevel = "medium"
		} else if stat.Untrusted > 0 || stat.LatestTag > 0 {
			stat.RiskLevel = "medium"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalImages > result.ByNamespace[j].TotalImages
	})

	// Health score
	score := 100
	if result.Summary.LatestTag > 0 {
		score -= min(15, result.Summary.LatestTag*3)
	}
	if result.Summary.UntrustedRegistries > 0 {
		score -= min(20, result.Summary.UntrustedRegistries*2)
	}
	if result.Summary.WithDigest == 0 && result.Summary.TotalImages > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.LatestTag > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d image(s) use :latest or no tag — pin to specific version or digest", result.Summary.LatestTag))
	}
	if result.Summary.UntrustedRegistries > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d image(s) from untrusted public registries — use private/trusted registry", result.Summary.UntrustedRegistries))
	}
	if result.Summary.WithDigest == 0 && result.Summary.TotalImages > 0 {
		result.Recommendations = append(result.Recommendations,
			"No images use digest pinning — pin images with @sha256 for immutability")
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"All images are from trusted registries with proper version pinning")
	}

	writeJSON(w, result)
}
