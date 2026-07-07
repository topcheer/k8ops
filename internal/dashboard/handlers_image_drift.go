package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IDResult is the deployment image drift analysis.
type IDResult struct {
	ScannedAt        time.Time `json:"scannedAt"`
	Summary          IDSummary `json:"summary"`
	DriftedWorkloads []IDEntry `json:"driftedWorkloads"` // workloads with mixed images
	UsingLatestTag   []IDEntry `json:"usingLatestTag"`   // workloads using :latest
	NoDigest         []IDEntry `json:"noDigest"`         // images without digest
	ByWorkload       []IDEntry `json:"byWorkload"`
	Issues           []IDIssue `json:"issues"`
	Recommendations  []string  `json:"recommendations"`
}

// IDSummary aggregates image drift statistics.
type IDSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	DriftedWorkloads int `json:"driftedWorkloads"` // pods running different images in same workload
	UsingLatestTag   int `json:"usingLatestTag"`
	NoDigest         int `json:"noDigest"`         // images without sha256 digest
	StalledRollouts  int `json:"stalledRollouts"`  // updatedReplicas < replicas (image mismatch)
	ConsistencyScore int `json:"consistencyScore"` // 0-100
}

// IDEntry describes one workload's image consistency status.
type IDEntry struct {
	Name            string           `json:"name"`
	Namespace       string           `json:"namespace"`
	Kind            string           `json:"kind"`
	Replicas        int32            `json:"replicas"`
	ReadyReplicas   int32            `json:"readyReplicas"`
	UpdatedReplicas int32            `json:"updatedReplicas"`
	ImageVariants   []IDImageVariant `json:"imageVariants"` // distinct images found
	HasDrift        bool             `json:"hasDrift"`
	UsesLatest      bool             `json:"usesLatest"`
	HasDigest       bool             `json:"hasDigest"`
	RiskLevel       string           `json:"riskLevel"`
}

// IDImageVariant describes one distinct image version found.
type IDImageVariant struct {
	Image     string `json:"image"`
	PodCount  int    `json:"podCount"`
	IsLatest  bool   `json:"isLatest"`
	HasDigest bool   `json:"hasDigest"`
}

// IDIssue is a detected drift problem.
type IDIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleImageDrift detects image version drift within workloads.
// GET /api/deployment/image-drift
func (s *Server) handleImageDrift(w http.ResponseWriter, r *http.Request) {
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

	// Build pod → workload map and workload → pods map
	type wlKey struct{ ns, name, kind string }
	wlPods := make(map[wlKey][]corev1.Pod)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, ref := range pod.OwnerReferences {
			kind := ref.Kind
			if kind == "ReplicaSet" {
				kind = "Deployment" // ReplicaSet is usually owned by Deployment
			}
			if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
				key := wlKey{pod.Namespace, ref.Name, kind}
				wlPods[key] = append(wlPods[key], pod)
				break
			}
		}
	}

	// Get deployment/sts/ds specs for replica counts
	depRepl := make(map[string]int32)
	deployments, _ := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if deployments != nil {
		for _, d := range deployments.Items {
			r := int32(1)
			if d.Spec.Replicas != nil {
				r = *d.Spec.Replicas
			}
			depRepl[d.Namespace+"/"+d.Name] = r
		}
	}

	result := IDResult{ScannedAt: time.Now()}

	for key, podList := range wlPods {
		result.Summary.TotalWorkloads++

		entry := IDEntry{
			Name:      key.name,
			Namespace: key.ns,
			Kind:      key.kind,
		}

		if r, ok := depRepl[key.ns+"/"+key.name]; ok {
			entry.Replicas = r
		} else {
			entry.Replicas = int32(len(podList))
		}

		// Collect all images from all pods
		imageMap := make(map[string]*IDImageVariant)
		for _, pod := range podList {
			for _, c := range pod.Spec.Containers {
				img := c.Image
				if imageMap[img] == nil {
					imageMap[img] = &IDImageVariant{Image: img}
				}
				imageMap[img].PodCount++
				// Check for :latest tag
				if idIsLatestTag(img) {
					imageMap[img].IsLatest = true
					entry.UsesLatest = true
				}
				// Check for digest
				if strings.Contains(img, "@sha256:") {
					imageMap[img].HasDigest = true
					entry.HasDigest = true
				}
			}
		}

		// Build variants list
		for _, v := range imageMap {
			entry.ImageVariants = append(entry.ImageVariants, *v)
		}
		sort.Slice(entry.ImageVariants, func(i, j int) bool {
			return entry.ImageVariants[i].PodCount > entry.ImageVariants[j].PodCount
		})

		// Check for drift (more than 1 distinct image)
		if len(imageMap) > 1 {
			entry.HasDrift = true
			result.Summary.DriftedWorkloads++
			result.DriftedWorkloads = append(result.DriftedWorkloads, entry)
			images := make([]string, 0, len(imageMap))
			for img := range imageMap {
				images = append(images, img)
			}
			result.Issues = append(result.Issues, IDIssue{
				Severity: "warning", Type: "image-drift",
				Resource: fmt.Sprintf("%s/%s", key.ns, key.name),
				Message:  fmt.Sprintf("%s %s/%s has %d distinct images across pods: %s — rollout may be stalled", key.kind, key.ns, key.name, len(imageMap), strings.Join(images, ", ")),
			})
		}

		// Check for latest tag
		if entry.UsesLatest {
			result.Summary.UsingLatestTag++
			result.UsingLatestTag = append(result.UsingLatestTag, entry)
			result.Issues = append(result.Issues, IDIssue{
				Severity: "warning", Type: "latest-tag",
				Resource: fmt.Sprintf("%s/%s", key.ns, key.name),
				Message:  fmt.Sprintf("%s %s/%s uses :latest tag — not reproducible, use pinned versions", key.kind, key.ns, key.name),
			})
		}

		// Check for no digest
		if !entry.HasDigest {
			result.Summary.NoDigest++
			result.NoDigest = append(result.NoDigest, entry)
		}

		entry.RiskLevel = idAssessRisk(entry)
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort
	sort.Slice(result.DriftedWorkloads, func(i, j int) bool {
		return len(result.DriftedWorkloads[i].ImageVariants) > len(result.DriftedWorkloads[j].ImageVariants)
	})
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return idRiskRank(result.ByWorkload[i].RiskLevel) < idRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return idIssueRank(result.Issues[i].Severity) < idIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ConsistencyScore = idScore(result.Summary)
	result.Recommendations = idGenRecs(result.Summary, result.DriftedWorkloads)

	writeJSON(w, result)
}

// idIsLatestTag checks if image uses :latest tag.
func idIsLatestTag(image string) bool {
	// Remove digest part if present
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return true // no tag = implicitly latest
	}
	return parts[len(parts)-1] == "latest"
}

// idAssessRisk determines risk level.
func idAssessRisk(entry IDEntry) string {
	if entry.HasDrift {
		return "high"
	}
	if entry.UsesLatest {
		return "medium"
	}
	if !entry.HasDigest {
		return "low"
	}
	return "low"
}

// idScore computes consistency score 0-100.
func idScore(s IDSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.DriftedWorkloads * 15
	score -= s.UsingLatestTag * 8
	score -= s.NoDigest * 2
	if score < 0 {
		score = 0
	}
	return score
}

// idGenRecs produces actionable advice.
func idGenRecs(s IDSummary, drifted []IDEntry) []string {
	var recs []string

	if s.DriftedWorkloads > 0 {
		top := ""
		if len(drifted) > 0 {
			images := make([]string, 0)
			for _, v := range drifted[0].ImageVariants {
				images = append(images, fmt.Sprintf("%s (%d pods)", v.Image, v.PodCount))
			}
			top = fmt.Sprintf(" (e.g. %s/%s: %s)", drifted[0].Namespace, drifted[0].Name, strings.Join(images, ", "))
		}
		recs = append(recs, fmt.Sprintf("%d workload(s) have image drift%s — pods running different versions, check rollout status", s.DriftedWorkloads, top))
	}
	if s.UsingLatestTag > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) use :latest tag — pin to specific versions for reproducibility", s.UsingLatestTag))
	}
	if s.NoDigest > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) lack image digests — use digest references (image@sha256:...) for immutability", s.NoDigest))
	}
	if s.ConsistencyScore < 70 {
		recs = append(recs, fmt.Sprintf("Image consistency score is %d/100 — review deployment image practices", s.ConsistencyScore))
	}
	if s.DriftedWorkloads == 0 && s.UsingLatestTag == 0 {
		recs = append(recs, "All workloads have consistent images — good deployment posture")
	}

	return recs
}

func idRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

func idIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

var _ = appsv1.Deployment{}
