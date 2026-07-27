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
// v19.99 — Product Dimension (Round 19)
// 1. Volume Lifecycle Age — PVC age distribution & staleness
// 2. Service Endpoint Health — backing pod readiness per service
// 3. Image Tag Freshness — image tag age & staleness estimator
// ============================================================

// ---------------------------------------------------------------
// 1. Volume Lifecycle Age
// ---------------------------------------------------------------

type VolLifeResult1999 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         VolLifeSummary1999 `json:"summary"`
	OldVolumes      []VolLifeEntry1999 `json:"oldVolumes"`
	Recommendations []string           `json:"recommendations"`
}

type VolLifeSummary1999 struct {
	TotalPVCs   int     `json:"totalPVCs"`
	BoundPVCs   int     `json:"boundPVCs"`
	UnboundPVCs int     `json:"unboundPVCs"`
	OldPVCs     int     `json:"oldPVCs90d"`
	AvgAgeDays  float64 `json:"avgAgeDays"`
}

type VolLifeEntry1999 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Status    string  `json:"status"`
	AgeDays   float64 `json:"ageDays"`
}

func (s *Server) handleVolLifecycleAge(w http.ResponseWriter, r *http.Request) {
	result := VolLifeResult1999{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	var totalAge float64
	var count int

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		} else {
			result.Summary.UnboundPVCs++
		}

		if pvc.CreationTimestamp.IsZero() {
			continue
		}
		ageDays := time.Since(pvc.CreationTimestamp.Time).Hours() / 24
		totalAge += ageDays
		count++

		entry := VolLifeEntry1999{
			Name: pvc.Name, Namespace: pvc.Namespace,
			Status: string(pvc.Status.Phase), AgeDays: ageDays,
		}

		if ageDays > 90 {
			result.Summary.OldPVCs++
			result.OldVolumes = append(result.OldVolumes, entry)
		}
	}

	if count > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(count)
	}

	if result.Summary.UnboundPVCs > 3 {
		score -= 5
	}
	if result.Summary.OldPVCs > 10 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d bound, %d unbound), %d old (>90d), avg %.0fd", result.Summary.TotalPVCs, result.Summary.BoundPVCs, result.Summary.UnboundPVCs, result.Summary.OldPVCs, result.Summary.AvgAgeDays))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Endpoint Health
// ---------------------------------------------------------------

type SvcEPResult1999 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         SvcEPSummary1999 `json:"summary"`
	Unhealthy       []SvcEPEntry1999 `json:"unhealthyServices"`
	Recommendations []string         `json:"recommendations"`
}

type SvcEPSummary1999 struct {
	TotalServices int `json:"totalServices"`
	WithEndpoints int `json:"withEndpoints"`
	NoEndpoints   int `json:"noEndpoints"`
	Unhealthy     int `json:"unhealthyServices"`
}

type SvcEPEntry1999 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Issue     string `json:"issue"`
}

func (s *Server) handleServiceEndpointHealth(w http.ResponseWriter, r *http.Request) {
	result := SvcEPResult1999{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})

	// Build endpoint map
	epMap := make(map[string]bool)
	for _, ep := range epList.Items {
		hasReady := false
		for _, sub := range ep.Subsets {
			if len(sub.Addresses) > 0 {
				hasReady = true
				break
			}
		}
		epMap[ep.Namespace+"/"+ep.Name] = hasReady
	}

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		entry := SvcEPEntry1999{
			Name: svc.Name, Namespace: svc.Namespace,
			Type: string(svc.Spec.Type),
		}

		key := svc.Namespace + "/" + svc.Name
		hasEP, found := epMap[key]

		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			// ExternalName services don't have endpoints
			continue
		}

		if !found || !hasEP {
			result.Summary.NoEndpoints++
			entry.Issue = "No ready endpoints"
			result.Unhealthy = append(result.Unhealthy, entry)
			result.Summary.Unhealthy++
			if svc.Spec.Type != corev1.ServiceTypeExternalName {
				score -= 1
			}
		} else {
			result.Summary.WithEndpoints++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services: %d with endpoints, %d no endpoints, %d unhealthy", result.Summary.TotalServices, result.Summary.WithEndpoints, result.Summary.NoEndpoints, result.Summary.Unhealthy))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Image Tag Freshness
// ---------------------------------------------------------------

type ImgTagResult1999 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         ImgTagSummary1999 `json:"summary"`
	StaleImages     []ImgTagEntry1999 `json:"staleImages"`
	Recommendations []string          `json:"recommendations"`
}

type ImgTagSummary1999 struct {
	TotalImages int `json:"totalUniqueImages"`
	UsingLatest int `json:"usingLatestTag"`
	UsingSHA    int `json:"usingSHADigest"`
	UsingSemver int `json:"usingSemverTag"`
	NoTag       int `json:"withoutTag"`
}

type ImgTagEntry1999 struct {
	Image string `json:"image"`
	Issue string `json:"issue"`
}

func (s *Server) handleImageTagFreshness(w http.ResponseWriter, r *http.Request) {
	result := ImgTagResult1999{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imageSet := make(map[string]string) // image -> tag classification

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			if _, exists := imageSet[img]; exists {
				continue
			}

			// Classify tag
			if isSHADigest1999(img) {
				imageSet[img] = "sha"
				result.Summary.UsingSHA++
			} else if hasNoTag1999(img) {
				imageSet[img] = "none"
				result.Summary.NoTag++
				result.StaleImages = append(result.StaleImages, ImgTagEntry1999{
					Image: img, Issue: "No tag specified (defaults to latest)",
				})
				score -= 3
			} else if getTag1999(img) == "latest" {
				imageSet[img] = "latest"
				result.Summary.UsingLatest++
				result.StaleImages = append(result.StaleImages, ImgTagEntry1999{
					Image: img, Issue: "Using 'latest' tag — non-reproducible",
				})
				score -= 2
			} else {
				imageSet[img] = "semver"
				result.Summary.UsingSemver++
			}
		}
	}

	result.Summary.TotalImages = len(imageSet)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images: %d semver, %d SHA, %d latest, %d no tag", result.Summary.TotalImages, result.Summary.UsingSemver, result.Summary.UsingSHA, result.Summary.UsingLatest, result.Summary.NoTag))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func isSHADigest1999(img string) bool {
	for i := len(img) - 1; i >= 0; i-- {
		if img[i] == '@' {
			return true
		}
		if img[i] == '/' {
			break
		}
	}
	return false
}

func hasNoTag1999(img string) bool {
	// Check if image has a colon before a tag
	// e.g. "nginx" (no tag) vs "nginx:1.25" (tagged)
	lastSlash := -1
	for i := len(img) - 1; i >= 0; i-- {
		if img[i] == '/' {
			lastSlash = i
			break
		}
	}
	// Look for colon after last slash
	for i := len(img) - 1; i > lastSlash; i-- {
		if img[i] == ':' {
			return false
		}
	}
	return true
}

func getTag1999(img string) string {
	lastSlash := -1
	for i := len(img) - 1; i >= 0; i-- {
		if img[i] == '/' {
			lastSlash = i
			break
		}
	}
	for i := len(img) - 1; i > lastSlash; i-- {
		if img[i] == ':' {
			return img[i+1:]
		}
	}
	return ""
}
