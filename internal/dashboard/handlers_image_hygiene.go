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

// ImgHygieneResult is the container image deployment hygiene analysis.
type ImgHygieneResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ImgHygieneSummary `json:"summary"`
	Images          []ImgHygEntry     `json:"images"`
	ByRegistry      []ImgHygRegStat   `json:"byRegistry"`
	LatestImages    []ImgHygEntry     `json:"latestImages"`
	Duplicates      []ImgHygDuplicate `json:"duplicates"`
	Issues          []ImgHygIssue     `json:"issues"`
	Recommendations []string          `json:"recommendations"`
}

// ImgHygieneSummary aggregates image deployment hygiene.
type ImgHygieneSummary struct {
	TotalContainers int `json:"totalContainers"`
	UniqueImages    int `json:"uniqueImages"`
	LatestTagCount  int `json:"latestTagCount"`
	DigestPinned    int `json:"digestPinned"`
	VersionTagged   int `json:"versionTagged"`
	UntaggedOrEmpty int `json:"untaggedOrEmpty"`
	Registries      int `json:"registries"`
	DuplicateImages int `json:"duplicateImages"`
	UntrustedReg    int `json:"untrustedRegistries"`
	HygieneScore    int `json:"hygieneScore"`
}

// ImgHygEntry describes one unique container image.
type ImgHygEntry struct {
	Image        string   `json:"image"`
	Registry     string   `json:"registry"`
	Repository   string   `json:"repository"`
	Tag          string   `json:"tag"`
	HasDigest    bool     `json:"hasDigest"`
	IsLatest     bool     `json:"isLatest"`
	IsVersioned  bool     `json:"isVersioned"`
	ReplicaCount int      `json:"replicaCount"`
	Namespaces   []string `json:"namespaces"`
	Pods         []string `json:"pods"`
	RiskLevel    string   `json:"riskLevel"`
}

// ImgHygRegStat per-registry statistics.
type ImgHygRegStat struct {
	Registry    string `json:"registry"`
	ImageCount  int    `json:"imageCount"`
	PodCount    int    `json:"podCount"`
	LatestCount int    `json:"hasLatest"`
	IsTrusted   bool   `json:"isTrusted"`
}

// ImgHygDuplicate flags same base image with multiple tags.
type ImgHygDuplicate struct {
	BaseImage string   `json:"baseImage"`
	Variants  []string `json:"variants"`
	TotalPods int      `json:"totalPods"`
}

// ImgHygIssue is a detected problem.
type ImgHygIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

// handleImageHygiene analyzes container image deployment practices.
// GET /api/deployment/image-hygiene
func (s *Server) handleImageHygiene(w http.ResponseWriter, r *http.Request) {
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

	result := ImgHygieneResult{ScannedAt: time.Now()}
	imageMap := make(map[string]*ImgHygEntry)
	regMap := make(map[string]*ImgHygRegStat)
	baseVariants := make(map[string]map[string]bool)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			fullImage := c.Image
			entry := imgHygParse(fullImage)

			if existing, ok := imageMap[fullImage]; ok {
				existing.ReplicaCount++
				if !imgHygContains(existing.Namespaces, pod.Namespace) {
					existing.Namespaces = append(existing.Namespaces, pod.Namespace)
				}
				existing.Pods = append(existing.Pods, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			} else {
				entry.ReplicaCount = 1
				entry.Namespaces = []string{pod.Namespace}
				entry.Pods = []string{fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)}
				entry.RiskLevel = imgHygAssessRisk(entry)
				imageMap[fullImage] = &entry
			}

			// Registry stats
			regStat := imgHygGetOrCreateReg(regMap, entry.Registry)
			regStat.PodCount++
			if entry.IsLatest {
				regStat.LatestCount++
			}

			// Aggregate summary
			if entry.IsLatest {
				result.Summary.LatestTagCount++
			}
			if entry.HasDigest {
				result.Summary.DigestPinned++
			}
			if entry.IsVersioned {
				result.Summary.VersionTagged++
			}

			// Duplicate detection
			base := entry.Registry + "/" + entry.Repository
			if baseVariants[base] == nil {
				baseVariants[base] = make(map[string]bool)
			}
			baseVariants[base][fullImage] = true
		}
	}

	// Build unique images list
	for _, entry := range imageMap {
		result.Images = append(result.Images, *entry)
		if entry.IsLatest {
			result.LatestImages = append(result.LatestImages, *entry)
		}
	}
	result.Summary.UniqueImages = len(result.Images)

	// Update registry image counts
	for _, rs := range regMap {
		for _, e := range imageMap {
			if e.Registry == rs.Registry {
				rs.ImageCount++
			}
		}
		rs.IsTrusted = imgHygIsTrustedReg(rs.Registry)
		if !rs.IsTrusted {
			result.Summary.UntrustedReg++
		}
		result.ByRegistry = append(result.ByRegistry, *rs)
	}
	result.Summary.Registries = len(regMap)

	// Sort images by risk
	sort.Slice(result.Images, func(i, j int) bool {
		return imgHygRiskRank(result.Images[i].RiskLevel) < imgHygRiskRank(result.Images[j].RiskLevel)
	})

	// Detect duplicates
	for base, variants := range baseVariants {
		if len(variants) > 1 {
			variantList := make([]string, 0, len(variants))
			totalPods := 0
			for v := range variants {
				variantList = append(variantList, v)
				if e, ok := imageMap[v]; ok {
					totalPods += e.ReplicaCount
				}
			}
			sort.Strings(variantList)
			result.Duplicates = append(result.Duplicates, ImgHygDuplicate{
				BaseImage: base, Variants: variantList, TotalPods: totalPods,
			})
			result.Summary.DuplicateImages++
		}
	}
	sort.Slice(result.Duplicates, func(i, j int) bool {
		return len(result.Duplicates[i].Variants) > len(result.Duplicates[j].Variants)
	})

	// Sort registry stats
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].PodCount > result.ByRegistry[j].PodCount
	})

	// Generate issues
	if result.Summary.LatestTagCount > 0 {
		result.Issues = append(result.Issues, ImgHygIssue{
			Severity: "warning", Type: "latest-tag",
			Message: fmt.Sprintf("%d container(s) use :latest — non-reproducible deployments", result.Summary.LatestTagCount),
		})
	}
	if result.Summary.DuplicateImages > 0 {
		result.Issues = append(result.Issues, ImgHygIssue{
			Severity: "info", Type: "duplicates",
			Message: fmt.Sprintf("%d base image(s) have multiple tag variants — consolidate versions", result.Summary.DuplicateImages),
		})
	}
	if result.Summary.UntrustedReg > 0 {
		result.Issues = append(result.Issues, ImgHygIssue{
			Severity: "info", Type: "untrusted-registry",
			Message: fmt.Sprintf("%d untrusted registry/registries — consider private registry", result.Summary.UntrustedReg),
		})
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return imgHygIssueRank(result.Issues[i].Severity) < imgHygIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HygieneScore = imgHygCalcScore(result.Summary)
	result.Recommendations = imgHygGenRecs(result.Summary, result.Duplicates)

	writeJSON(w, result)
}

// imgHygParse parses a container image reference into components.
func imgHygParse(image string) ImgHygEntry {
	entry := ImgHygEntry{Image: image}

	if idx := strings.Index(image, "@"); idx >= 0 {
		entry.HasDigest = true
		image = image[:idx]
	}

	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		entry.Tag = image[lastColon+1:]
		image = image[:lastColon]
	} else {
		entry.Tag = "latest"
		entry.IsLatest = true
	}

	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		entry.Registry = parts[0]
		entry.Repository = parts[1]
	} else {
		entry.Registry = "docker.io"
		entry.Repository = image
	}

	if entry.Tag == "latest" {
		entry.IsLatest = true
	} else if imgHygIsVersion(entry.Tag) {
		entry.IsVersioned = true
	}

	return entry
}

func imgHygIsVersion(tag string) bool {
	if tag == "" || tag == "latest" {
		return false
	}
	for _, c := range tag {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func imgHygAssessRisk(entry ImgHygEntry) string {
	risk := 0
	if entry.IsLatest {
		risk += 15
	}
	if !entry.HasDigest && entry.IsLatest {
		risk += 10
	}
	if entry.Registry == "docker.io" {
		risk += 5
	}
	if !entry.IsVersioned && !entry.HasDigest {
		risk += 5
	}

	switch {
	case risk >= 25:
		return "high"
	case risk >= 15:
		return "medium"
	default:
		return "low"
	}
}

func imgHygCalcScore(s ImgHygieneSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.LatestTagCount * 8
	score -= s.UntaggedOrEmpty * 5
	score -= s.DuplicateImages * 3
	score -= s.UntrustedReg * 2
	if score < 0 {
		score = 0
	}
	return score
}

func imgHygGenRecs(s ImgHygieneSummary, dups []ImgHygDuplicate) []string {
	var recs []string

	if s.LatestTagCount > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) use :latest — pin specific version tags", s.LatestTagCount))
	}
	if s.DigestPinned > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) digest-pinned (@sha256) — best practice", s.DigestPinned))
	} else if s.VersionTagged > 0 {
		recs = append(recs, "Consider pinning with @sha256 digests for immutability")
	}
	if s.DuplicateImages > 0 {
		top := ""
		if len(dups) > 0 {
			top = fmt.Sprintf(" (e.g. %s has %d variants)", dups[0].BaseImage, len(dups[0].Variants))
		}
		recs = append(recs, fmt.Sprintf("%d base image(s) with multiple variants%s — consolidate", s.DuplicateImages, top))
	}
	if s.UntrustedReg > 0 {
		recs = append(recs, fmt.Sprintf("%d untrusted registry/registries — migrate to private registry", s.UntrustedReg))
	}
	if s.HygieneScore < 60 {
		recs = append(recs, fmt.Sprintf("Image hygiene score %d/100 — improve tagging strategy", s.HygieneScore))
	}

	return recs
}

func imgHygIsTrustedReg(reg string) bool {
	trusted := []string{"registry.k8s.io", "k8s.gcr.io", "gcr.io", "quay.io", "registry.iot2.win", "ghcr.io", "public.ecr.aws", "docker.io"}
	for _, t := range trusted {
		if reg == t {
			return true
		}
	}
	return false
}

func imgHygGetOrCreateReg(m map[string]*ImgHygRegStat, reg string) *ImgHygRegStat {
	if e, ok := m[reg]; ok {
		return e
	}
	e := &ImgHygRegStat{Registry: reg, IsTrusted: imgHygIsTrustedReg(reg)}
	m[reg] = e
	return e
}

func imgHygContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func imgHygRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func imgHygIssueRank(s string) int {
	switch s {
	case "warning":
		return 0
	case "info":
		return 1
	default:
		return 2
	}
}
