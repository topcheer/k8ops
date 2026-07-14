package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistryRateLimitResult is the container image registry rate limit & pull reliability audit.
type RegistryRateLimitResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         RegistrySummary `json:"summary"`
	Registries      []RegistryEntry `json:"registries"`
	Risks           []RegistryRisk  `json:"risks"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// RegistrySummary aggregates registry statistics.
type RegistrySummary struct {
	TotalImages        int `json:"totalImages"`
	UniqueRegistries   int `json:"uniqueRegistries"`
	UsingDockerHub     int `json:"usingDockerHub"`  // images from docker.io (rate-limited)
	UsingGHCR          int `json:"usingGHCR"`       // GitHub Container Registry
	UsingPrivate       int `json:"usingPrivate"`    // private registries
	UsingPublic        int `json:"usingPublic"`     // public registries
	WithPullSecrets    int `json:"withPullSecrets"` // pods with imagePullSecrets
	WithoutPullSecrets int `json:"withoutPullSecrets"`
	DuplicateImages    int `json:"duplicateImages"` // same image used by multiple pods
	RateLimitRisk      int `json:"rateLimitRisk"`   // pods using Docker Hub without auth
}

// RegistryEntry describes a registry's usage.
type RegistryEntry struct {
	Registry      string `json:"registry"`
	ImageCount    int    `json:"imageCount"`
	PodCount      int    `json:"podCount"`
	HasPullSecret bool   `json:"hasPullSecret"`
	RateLimited   bool   `json:"rateLimited"`
	RiskLevel     string `json:"riskLevel"`
}

// RegistryRisk describes a specific risk.
type RegistryRisk struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName,omitempty"`
	Image     string `json:"image"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleRegistryRateLimit audits container image registry rate limit & pull reliability.
// GET /api/operations/registry-rate-limit
func (s *Server) handleRegistryRateLimit(w http.ResponseWriter, r *http.Request) {
	result := RegistryRateLimitResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// Known rate-limited registries
	rateLimitedRegistries := map[string]bool{
		"docker.io":            true,
		"registry-1.docker.io": true,
		"index.docker.io":      true,
	}

	// Known public registries
	publicRegistries := map[string]bool{
		"registry.k8s.io":      true,
		"k8s.gcr.io":           true,
		"gcr.io":               true,
		"quay.io":              true,
		"ghcr.io":              true,
		"public.ecr.aws":       true,
		"docker.io":            true,
		"registry-1.docker.io": true,
		"index.docker.io":      true,
	}

	registryMap := make(map[string]*RegistryEntry)
	imagePodMap := make(map[string][]string) // image -> []pod names

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}

			hasPullSecrets := len(pod.Spec.ImagePullSecrets) > 0
			if hasPullSecrets {
				result.Summary.WithPullSecrets++
			} else {
				result.Summary.WithoutPullSecrets++
			}

			for _, c := range pod.Spec.Containers {
				image := c.Image
				result.Summary.TotalImages++

				// Extract registry from image
				registry := "docker.io"
				if strings.Contains(image, "/") {
					parts := strings.SplitN(image, "/", 2)
					if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
						registry = parts[0]
					}
				}

				// Track image usage
				imagePodMap[image] = append(imagePodMap[image], fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))

				// Track registry stats
				entry, ok := registryMap[registry]
				if !ok {
					entry = &RegistryEntry{Registry: registry}
					registryMap[registry] = entry
					result.Summary.UniqueRegistries++
				}
				entry.ImageCount++
				entry.PodCount++

				if hasPullSecrets {
					entry.HasPullSecret = true
				}

				// Check if rate-limited
				isRateLimited := false
				for rlReg := range rateLimitedRegistries {
					if registry == rlReg || strings.HasPrefix(image, rlReg+"/") {
						isRateLimited = true
						break
					}
				}
				if isRateLimited {
					entry.RateLimited = true
					result.Summary.UsingDockerHub++
					if !hasPullSecrets {
						result.Summary.RateLimitRisk++
						result.Risks = append(result.Risks, RegistryRisk{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							Image:     image,
							Issue:     "Using Docker Hub without imagePullSecrets — rate limited (100 pulls/6h anonymous)",
							Severity:  "high",
						})
					}
				}

				// Classify registry
				isPublic := false
				for pubReg := range publicRegistries {
					if registry == pubReg {
						isPublic = true
						break
					}
				}
				if isPublic {
					result.Summary.UsingPublic++
					if registry == "ghcr.io" || strings.HasPrefix(image, "ghcr.io/") {
						result.Summary.UsingGHCR++
					}
				} else {
					result.Summary.UsingPrivate++
				}
			}
		}
	}

	// Check for duplicate images
	for _, pods := range imagePodMap {
		if len(pods) > 5 {
			result.Summary.DuplicateImages++
		}
	}

	// Build registry entries and assign risk levels
	for _, entry := range registryMap {
		entry.RiskLevel = "low"
		if entry.RateLimited && !entry.HasPullSecret {
			entry.RiskLevel = "high"
		} else if entry.RateLimited {
			entry.RiskLevel = "medium"
		}
		result.Registries = append(result.Registries, *entry)
	}
	sort.Slice(result.Registries, func(i, j int) bool {
		return result.Registries[i].RiskLevel > result.Registries[j].RiskLevel
	})
	sort.Slice(result.Risks, func(i, j int) bool {
		return result.Risks[i].Severity > result.Risks[j].Severity
	})

	// Recommendations
	if result.Summary.RateLimitRisk > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods use Docker Hub without authentication — add imagePullSecrets to avoid rate limiting", result.Summary.RateLimitRisk))
	}
	if result.Summary.UsingDockerHub > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d images from Docker Hub — consider mirroring to private registry for reliability", result.Summary.UsingDockerHub))
	}
	if result.Summary.WithoutPullSecrets > 0 && result.Summary.UsingPrivate > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods without imagePullSecrets using private registries — add secrets for authentication", result.Summary.WithoutPullSecrets))
	}

	// Health score
	score := 100
	score -= result.Summary.RateLimitRisk * 5
	if result.Summary.UsingDockerHub > 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
