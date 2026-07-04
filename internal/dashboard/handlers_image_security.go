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

// ImageSecurityResult is the full image supply chain security analysis.
type ImageSecurityResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         ImageSecuritySummary `json:"summary"`
	Images          []ImageSecEntry      `json:"images"`
	ByRegistry      []RegistryStat       `json:"byRegistry"`
	TopRisks        []ImageRiskEntry     `json:"topRisks"`
	Recommendations []string             `json:"recommendations"`
}

// ImageSecuritySummary aggregates cluster-wide image security metrics.
type ImageSecuritySummary struct {
	TotalImages       int `json:"totalImages"`
	UniqueImages      int `json:"uniqueImages"`
	PinnedByDigest    int `json:"pinnedByDigest"`   // uses @sha256:
	NotPinned         int `json:"notPinned"`        // uses mutable tag
	UsingLatest       int `json:"usingLatest"`      // :latest tag
	NoTag             int `json:"noTag"`            // no tag at all
	PublicRegistries  int `json:"publicRegistries"` // Docker Hub without auth
	PrivateRegistries int `json:"privateRegistries"`
	UnknownRegistries int `json:"unknownRegistries"` // no registry prefix
	OldVersionTags    int `json:"oldVersionTags"`    // likely outdated versions
	SecurityScore     int `json:"securityScore"`     // 0-100
}

// ImageSecEntry describes security posture for one image reference.
type ImageSecEntry struct {
	Image        string   `json:"image"`
	Registry     string   `json:"registry"`
	Repository   string   `json:"repository"`
	Tag          string   `json:"tag"`
	DigestPinned bool     `json:"digestPinned"`
	IsLatest     bool     `json:"isLatest"`
	HasNoTag     bool     `json:"hasNoTag"`
	IsPublic     bool     `json:"isPublicRegistry"`
	IsUnknown    bool     `json:"isUnknownRegistry"`
	OldVersion   bool     `json:"oldVersion"`
	UsedBy       int      `json:"usedBy"` // number of pods using this image
	Namespaces   []string `json:"namespaces"`
	RiskLevel    string   `json:"riskLevel"` // critical / high / medium / low
	Issues       []string `json:"issues"`
}

// RegistryStat aggregates per-registry statistics.
type RegistryStat struct {
	Registry   string `json:"registry"`
	ImageCount int    `json:"imageCount"`
	IsPublic   bool   `json:"isPublic"`
	HasDigest  int    `json:"hasDigest"`
}

// ImageRiskEntry is a summary of top risk images.
type ImageRiskEntry struct {
	Image     string `json:"image"`
	RiskLevel string `json:"riskLevel"`
	Reasons   string `json:"reasons"`
	UsedBy    int    `json:"usedBy"`
}

// Known public registries that don't require authentication.
var knownPublicRegistries = map[string]bool{
	"docker.io":            true,
	"registry-1.docker.io": true,
	"gcr.io":               true,
	"quay.io":              true,
	"public.ecr.aws":       true,
	"ghcr.io":              true,
}

// handleImageSecurityAudit scans all running container images for supply chain risks.
// GET /api/security/images?namespace=xxx
func (s *Server) handleImageSecurityAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := ImageSecurityResult{ScannedAt: time.Now()}

	// Aggregate images
	imageMap := make(map[string]*ImageSecEntry)
	registryMap := make(map[string]*RegistryStat)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, c := range pod.Spec.Containers {
			entry := getOrCreateImageEntry(imageMap, c.Image)

			// Track usage
			entry.UsedBy++
			found := false
			for _, ns2 := range entry.Namespaces {
				if ns2 == pod.Namespace {
					found = true
					break
				}
			}
			if !found {
				entry.Namespaces = append(entry.Namespaces, pod.Namespace)
			}
		}
	}

	// Analyze each unique image
	for _, entry := range imageMap {
		analyzeImageSecurity(entry)

		result.Summary.TotalImages++
		if entry.DigestPinned {
			result.Summary.PinnedByDigest++
		} else {
			result.Summary.NotPinned++
		}
		if entry.IsLatest {
			result.Summary.UsingLatest++
		}
		if entry.HasNoTag {
			result.Summary.NoTag++
		}
		if entry.IsPublic {
			result.Summary.PublicRegistries++
		}
		if entry.IsUnknown {
			result.Summary.UnknownRegistries++
		}
		if !entry.IsPublic && !entry.IsUnknown {
			result.Summary.PrivateRegistries++
		}
		if entry.OldVersion {
			result.Summary.OldVersionTags++
		}

		// Registry stats
		regStat := getOrCreateRegistryStat(registryMap, entry.Registry)
		regStat.ImageCount++
		if entry.DigestPinned {
			regStat.HasDigest++
		}
		regStat.IsPublic = entry.IsPublic || entry.IsUnknown

		result.Images = append(result.Images, *entry)
	}
	result.Summary.UniqueImages = len(result.Images)

	// Calculate security score
	result.Summary.SecurityScore = calculateImageSecScore(result.Summary)

	// Sort images by risk
	sort.Slice(result.Images, func(i, j int) bool {
		return imageRiskRank(result.Images[i].RiskLevel) < imageRiskRank(result.Images[j].RiskLevel)
	})

	// Build registry stats
	for _, reg := range registryMap {
		result.ByRegistry = append(result.ByRegistry, *reg)
	}
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].ImageCount > result.ByRegistry[j].ImageCount
	})

	// Build top risks
	for _, img := range result.Images {
		if img.RiskLevel == "critical" || img.RiskLevel == "high" {
			result.TopRisks = append(result.TopRisks, ImageRiskEntry{
				Image:     img.Image,
				RiskLevel: img.RiskLevel,
				Reasons:   strings.Join(img.Issues, "; "),
				UsedBy:    img.UsedBy,
			})
		}
	}
	if len(result.TopRisks) > 20 {
		result.TopRisks = result.TopRisks[:20]
	}

	// Recommendations
	result.Recommendations = generateImageSecRecommendations(result)

	writeJSON(w, result)
}

// getOrCreateImageEntry gets or creates an image entry from the map.
func getOrCreateImageEntry(m map[string]*ImageSecEntry, image string) *ImageSecEntry {
	if e, ok := m[image]; ok {
		return e
	}
	e := &ImageSecEntry{Image: image}
	m[image] = e
	return e
}

// analyzeImageSecurity fills in security analysis fields for an image.
func analyzeImageSecurity(entry *ImageSecEntry) {
	image := entry.Image

	// Check digest pinning
	if strings.Contains(image, "@sha256:") {
		entry.DigestPinned = true
		// Strip digest for parsing
		if idx := strings.Index(image, "@"); idx >= 0 {
			image = image[:idx]
		}
	}

	// Parse registry, repository, tag
	registry, repository, tag := parseImageReference(image)
	entry.Registry = registry
	entry.Repository = repository
	entry.Tag = tag

	// Check tag (skip if digest-pinned — tag is irrelevant with immutable digest)
	if tag == "latest" && !entry.DigestPinned {
		entry.IsLatest = true
	}
	if tag == "" && !entry.DigestPinned {
		entry.HasNoTag = true
	}

	// Check old version (v1, 1.0, very old numeric patterns)
	if isOldVersionTag(tag) {
		entry.OldVersion = true
	}

	// Registry classification
	if registry == "" {
		entry.IsUnknown = true
	} else if knownPublicRegistries[registry] {
		entry.IsPublic = true
	}

	// Build issues list
	var issues []string
	if entry.IsLatest {
		issues = append(issues, "uses :latest tag — mutable and non-reproducible")
	}
	if entry.HasNoTag {
		issues = append(issues, "no tag specified — defaults to :latest")
	}
	if !entry.DigestPinned && !entry.IsLatest && !entry.HasNoTag {
		issues = append(issues, "not pinned by digest — image content can change silently")
	}
	if entry.IsPublic {
		issues = append(issues, fmt.Sprintf("from public registry (%s) — no provenance guarantee", registry))
	}
	if entry.IsUnknown {
		issues = append(issues, "no registry prefix — defaults to Docker Hub, verify trust")
	}
	if entry.OldVersion {
		issues = append(issues, fmt.Sprintf("old version tag (%s) — may contain known vulnerabilities", tag))
	}
	entry.Issues = issues

	entry.RiskLevel = assessImageRiskLevel(*entry)
}

// parseImageReference extracts registry, repository, and tag from an image string.
func parseImageReference(image string) (registry, repository, tag string) {
	// Split tag (last colon after last slash)
	lastSlash := strings.LastIndex(image, "/")
	searchPart := image
	if lastSlash >= 0 {
		searchPart = image[lastSlash+1:]
	}

	colonIdx := strings.LastIndex(searchPart, ":")
	if colonIdx >= 0 {
		tag = searchPart[colonIdx+1:]
		image = image[:len(image)-len(searchPart)+colonIdx]
	}

	// Split registry from repository
	// Registry is everything before the first slash IF it contains a dot, colon, or is "localhost"
	firstSlash := strings.Index(image, "/")
	if firstSlash > 0 {
		prefix := image[:firstSlash]
		if strings.Contains(prefix, ".") || strings.Contains(prefix, ":") || prefix == "localhost" {
			registry = prefix
			repository = image[firstSlash+1:]
		} else {
			repository = image
		}
	} else {
		repository = image
	}

	return registry, repository, tag
}

// isOldVersionTag checks if a tag looks like an old version number.
func isOldVersionTag(tag string) bool {
	if tag == "" || tag == "latest" {
		return false
	}
	// Check for very old version patterns like v1, 1.0, 2
	if len(tag) <= 4 {
		lower := strings.ToLower(tag)
		// Single digit or v + single digit
		if strings.HasPrefix(lower, "v") {
			numPart := lower[1:]
			if isSimpleNumber(numPart) {
				return true
			}
		}
		if isSimpleNumber(lower) {
			return true
		}
	}
	return false
}

// isSimpleNumber checks if string is a simple number (1, 1.0, 12).
func isSimpleNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return true
}

// assessImageRiskLevel determines risk level for an image.
func assessImageRiskLevel(entry ImageSecEntry) string {
	risk := 0
	if entry.HasNoTag {
		risk += 25
	}
	if entry.IsLatest {
		risk += 15
	}
	if !entry.DigestPinned {
		risk += 10
	}
	if entry.IsUnknown {
		risk += 10
	}
	if entry.OldVersion {
		risk += 15
	}
	if entry.IsPublic && !entry.DigestPinned {
		risk += 5
	}

	switch {
	case risk >= 35:
		return "critical"
	case risk >= 20:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// calculateImageSecScore computes 0-100.
func calculateImageSecScore(s ImageSecuritySummary) int {
	if s.UniqueImages == 0 {
		return 100
	}
	score := 100
	score -= s.UsingLatest * 5
	score -= s.NoTag * 8
	score -= s.NotPinned * 2
	score -= s.OldVersionTags * 3
	score -= s.UnknownRegistries * 3
	if score < 0 {
		score = 0
	}
	return score
}

// generateImageSecRecommendations produces actionable advice.
func generateImageSecRecommendations(result ImageSecurityResult) []string {
	var recs []string
	s := result.Summary

	if s.NoTag > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) have no tag — always specify explicit version tags for reproducibility", s.NoTag))
	}
	if s.UsingLatest > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) use :latest — pin to specific versions to prevent unexpected changes", s.UsingLatest))
	}
	if s.NotPinned > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) are not pinned by digest — use @sha256: digests for immutable deployments", s.NotPinned))
	}
	if s.OldVersionTags > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) use old version tags — update to latest stable versions for security patches", s.OldVersionTags))
	}
	if s.UnknownRegistries > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) have no registry prefix — specify full registry path for supply chain clarity", s.UnknownRegistries))
	}
	if s.PublicRegistries > 0 {
		recs = append(recs, fmt.Sprintf("%d image(s) from public registries — consider mirroring to private registry with vulnerability scanning", s.PublicRegistries))
	}
	if s.SecurityScore < 50 {
		recs = append(recs, fmt.Sprintf("Image security score is %d/100 — implement image admission policies and registry scanning", s.SecurityScore))
	}

	return recs
}

func getOrCreateRegistryStat(m map[string]*RegistryStat, registry string) *RegistryStat {
	if e, ok := m[registry]; ok {
		return e
	}
	displayName := registry
	if registry == "" {
		displayName = "<none> (Docker Hub)"
	}
	e := &RegistryStat{Registry: displayName}
	m[registry] = e
	return e
}

func imageRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}
