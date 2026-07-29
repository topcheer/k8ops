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
// v20.33 — Security Dimension (Round 25)
// 1. Image Tag Immutability — mutable tag (latest) usage audit
// 2. RBAC Wildcard Audit — wildcard verbs/resources in ClusterRole
// 3. Pod Security Context Baseline — runAsNonRoot/runAsUser coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Image Tag Immutability
// ---------------------------------------------------------------

type ImageTagResult2033 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ImageTagSummary2033 `json:"summary"`
	MutableTags     []ImageTagEntry2033 `json:"mutableTags"`
	Recommendations []string            `json:"recommendations"`
}

type ImageTagSummary2033 struct {
	TotalImages  int `json:"totalImages"`
	MutableTags  int `json:"mutableTags"`
	DigestPinned int `json:"digestPinned"`
	NoTag        int `json:"noTag"`
}

type ImageTagEntry2033 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Tag       string `json:"tag"`
}

func (s *Server) handleImageTagImmutability(w http.ResponseWriter, r *http.Request) {
	result := ImageTagResult2033{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	mutableTags := map[string]bool{"latest": true, "main": true, "master": true, "edge": true, "stable": true, "nightly": true}
	seenImages := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			img := c.Image
			seenImages[img] = true

			// Check if digest-pinned (contains @sha256:)
			if len(img) > 7 && img[len(img)-7:][:1] == "@" {
				result.Summary.DigestPinned++
				continue
			}

			// Extract tag
			tag := ""
			if idx := lastSlash(img); idx >= 0 {
				rest := img[idx+1:]
				if colonIdx := indexByte(rest, ':'); colonIdx >= 0 {
					tag = rest[colonIdx+1:]
				}
			} else if colonIdx := indexByte(img, ':'); colonIdx >= 0 {
				tag = img[colonIdx+1:]
			}

			if tag == "" {
				result.Summary.NoTag++
				result.MutableTags = append(result.MutableTags, ImageTagEntry2033{
					Pod: pod.Name, Namespace: pod.Namespace,
					Image: img, Tag: "<none>",
				})
				score -= 3
			} else if mutableTags[tag] {
				result.Summary.MutableTags++
				result.MutableTags = append(result.MutableTags, ImageTagEntry2033{
					Pod: pod.Name, Namespace: pod.Namespace,
					Image: img, Tag: tag,
				})
				score -= 2
			}
		}
	}

	result.Summary.TotalImages = len(seenImages)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.MutableTags, func(i, j int) bool {
		return result.MutableTags[i].Namespace < result.MutableTags[j].Namespace
	})

	if result.Summary.MutableTags > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers use mutable tags (latest/main) — pin to specific versions or digests", result.Summary.MutableTags))
	}

	writeJSON(w, result)
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------
// 2. RBAC Wildcard Audit
// ---------------------------------------------------------------

type RBACWildcardResult2033 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RBACWildcardSummary2033 `json:"summary"`
	WildcardRoles   []RBACWildcardEntry2033 `json:"wildcardRoles"`
	Recommendations []string                `json:"recommendations"`
}

type RBACWildcardSummary2033 struct {
	TotalClusterRoles int `json:"totalClusterRoles"`
	WildcardVerbs     int `json:"wildcardVerbs"`
	WildcardResources int `json:"wildcardResources"`
}

type RBACWildcardEntry2033 struct {
	Name      string `json:"name"`
	Issue     string `json:"issue"`
	Verbs     string `json:"verbs"`
	Resources string `json:"resources"`
}

func (s *Server) handleRBACWildcardAudit(w http.ResponseWriter, r *http.Request) {
	result := RBACWildcardResult2033{ScannedAt: time.Now()}
	score := 100

	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalClusterRoles++

		for _, rule := range cr.Rules {
			// Check for wildcard verbs
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WildcardVerbs++
					result.WildcardRoles = append(result.WildcardRoles, RBACWildcardEntry2033{
						Name: cr.Name, Issue: "wildcard verb",
						Verbs: "*", Resources: joinStrs2033(rule.Resources),
					})
					score -= 3
					break
				}
			}

			// Check for wildcard resources
			for _, res := range rule.Resources {
				if res == "*" {
					result.Summary.WildcardResources++
					result.WildcardRoles = append(result.WildcardRoles, RBACWildcardEntry2033{
						Name: cr.Name, Issue: "wildcard resource",
						Verbs: joinStrs2033(rule.Verbs), Resources: "*",
					})
					score -= 3
					break
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.WildcardRoles, func(i, j int) bool {
		return result.WildcardRoles[i].Name < result.WildcardRoles[j].Name
	})

	if result.Summary.WildcardVerbs > 0 || result.Summary.WildcardResources > 0 {
		result.Recommendations = append(result.Recommendations,
			"Replace wildcard RBAC permissions with specific verbs and resources")
	}

	writeJSON(w, result)
}

func joinStrs2033(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for i := 1; i < len(ss); i++ {
		result += "," + ss[i]
	}
	return result
}

// ---------------------------------------------------------------
// 3. Pod Security Context Baseline
// ---------------------------------------------------------------

type SecCtxBaselineResult2033 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         SecCtxBaselineSummary2033 `json:"summary"`
	MissingSC       []SecCtxBaselineEntry2033 `json:"missingSecurityContext"`
	Recommendations []string                  `json:"recommendations"`
}

type SecCtxBaselineSummary2033 struct {
	TotalContainers int `json:"totalContainers"`
	WithNonRoot     int `json:"withNonRoot"`
	WithReadOnlyFS  int `json:"withReadOnlyFS"`
	NoSecurityCtx   int `json:"noSecurityContext"`
}

type SecCtxBaselineEntry2033 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleSecCtxBaseline(w http.ResponseWriter, r *http.Request) {
	result := SecCtxBaselineResult2033{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podHasNonRoot := false
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsNonRoot != nil && *pod.Spec.SecurityContext.RunAsNonRoot {
			podHasNonRoot = true
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if c.SecurityContext == nil {
				if !podHasNonRoot {
					result.Summary.NoSecurityCtx++
					result.MissingSC = append(result.MissingSC, SecCtxBaselineEntry2033{
						Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					})
					score -= 1
				}
				continue
			}

			sc := c.SecurityContext
			if sc.RunAsNonRoot != nil && *sc.RunAsNonRoot {
				result.Summary.WithNonRoot++
			} else if !podHasNonRoot {
				result.Summary.NoSecurityCtx++
				result.MissingSC = append(result.MissingSC, SecCtxBaselineEntry2033{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
				score -= 1
			}

			if sc.ReadOnlyRootFilesystem != nil && *sc.ReadOnlyRootFilesystem {
				result.Summary.WithReadOnlyFS++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.MissingSC, func(i, j int) bool {
		return result.MissingSC[i].Namespace < result.MissingSC[j].Namespace
	})

	if result.Summary.NoSecurityCtx > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers missing runAsNonRoot — enforce Pod Security 'restricted' baseline", result.Summary.NoSecurityCtx))
	}

	writeJSON(w, result)
}
