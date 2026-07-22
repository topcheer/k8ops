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

// ============================================================
// v19.23 — Deployment Dimension (Round 7)
// 1. Annotation Compliance — required metadata on workloads
// 2. Multi-Arch Image Audit — multi-architecture image support
// 3. Container Dependency Mapper — inter-container dependency & start order
// ============================================================

// ---------------------------------------------------------------
// 1. Annotation Compliance — required metadata on workloads
// ---------------------------------------------------------------

type AnnotationComplianceResult1923 struct {
	ScannedAt       time.Time                       `json:"scannedAt"`
	HealthScore     int                             `json:"healthScore"`
	Grade           string                          `json:"grade"`
	Summary         AnnotationComplianceSummary1923 `json:"summary"`
	Workloads       []AnnotationEntry1923           `json:"workloads"`
	MissingAnnots   []AnnotationMissingEntry1923    `json:"missingAnnotations"`
	Recommendations []string                        `json:"recommendations"`
}

type AnnotationComplianceSummary1923 struct {
	TotalWorkloads int      `json:"totalWorkloads"`
	FullyCompliant int      `json:"fullyCompliant"`
	PartiallyComp  int      `json:"partiallyCompliant"`
	NonCompliant   int      `json:"nonCompliant"`
	ComplianceRate float64  `json:"complianceRate"`
	CheckedAnnots  []string `json:"checkedAnnotations"`
	MissingCount   int      `json:"missingCount"`
}

type AnnotationEntry1923 struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Kind        string            `json:"kind"`
	Annotations map[string]string `json:"annotations"`
	Compliant   bool              `json:"compliant"`
	Score       int               `json:"score"`
}

type AnnotationMissingEntry1923 struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	Missing   []string `json:"missing"`
	Severity  string   `json:"severity"`
}

func (s *Server) handleAnnotationCompliance(w http.ResponseWriter, r *http.Request) {
	result := AnnotationComplianceResult1923{
		ScannedAt: time.Now(),
	}

	// Required annotations for production compliance
	requiredAnnots := []string{
		"owner", "contact",
		"app.kubernetes.io/managed-by",
		"deployment.kubernetes.io/revision",
	}
	result.Summary.CheckedAnnots = requiredAnnots
	score := 100

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		annots := make(map[string]string)
		for k, v := range dep.Annotations {
			annots[k] = v
		}
		// Also check labels for managed-by
		if lbl, ok := dep.Labels["app.kubernetes.io/managed-by"]; ok && annots["app.kubernetes.io/managed-by"] == "" {
			annots["app.kubernetes.io/managed-by"] = lbl
		}

		missing := make([]string, 0)
		for _, req := range requiredAnnots {
			found := false
			for k := range annots {
				if strings.Contains(k, req) || k == req {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, req)
			}
		}

		wlScore := (len(requiredAnnots) - len(missing)) * 100 / len(requiredAnnots)
		compliant := len(missing) == 0

		entry := AnnotationEntry1923{
			Name:        dep.Name,
			Namespace:   dep.Namespace,
			Kind:        "Deployment",
			Annotations: annots,
			Compliant:   compliant,
			Score:       wlScore,
		}
		result.Workloads = append(result.Workloads, entry)
		result.Summary.TotalWorkloads++

		if compliant {
			result.Summary.FullyCompliant++
		} else if len(missing) < len(requiredAnnots) {
			result.Summary.PartiallyComp++
		} else {
			result.Summary.NonCompliant++
		}

		if len(missing) > 0 {
			severity := "low"
			if len(missing) >= 3 {
				severity = "high"
			} else if len(missing) >= 2 {
				severity = "medium"
			}
			result.MissingAnnots = append(result.MissingAnnots, AnnotationMissingEntry1923{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				Missing: missing, Severity: severity,
			})
			result.Summary.MissingCount += len(missing)
		}
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.ComplianceRate = float64(result.Summary.FullyCompliant) * 100 / float64(result.Summary.TotalWorkloads)
	}
	if result.Summary.NonCompliant > 5 {
		score -= 15
	}
	if result.Summary.PartiallyComp > 10 {
		score -= 10
	}
	if result.Summary.MissingCount > 20 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.NonCompliant > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads have zero required annotations — add owner/contact metadata", result.Summary.NonCompliant))
	}
	if result.Summary.PartiallyComp > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads partially compliant — add missing annotations", result.Summary.PartiallyComp))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Multi-Arch Image Audit — multi-architecture image support
// ---------------------------------------------------------------

type MultiArchResult1923 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         MultiArchSummary1923 `json:"summary"`
	Images          []MultiArchEntry1923 `json:"images"`
	ArchRisks       []MultiArchRisk1923  `json:"archRisks"`
	Recommendations []string             `json:"recommendations"`
}

type MultiArchSummary1923 struct {
	TotalImages      int `json:"totalImages"`
	UniqueImages     int `json:"uniqueImages"`
	Arm64Images      int `json:"arm64Images"`
	Amd64Images      int `json:"amd64Images"`
	MultiArchImages  int `json:"multiArchImages"`
	SingleArchCount  int `json:"singleArchCount"`
	UnknownArchCount int `json:"unknownArchCount"`
}

type MultiArchEntry1923 struct {
	Image       string   `json:"image"`
	ArchGuess   string   `json:"archGuess"`
	IsMultiArch bool     `json:"isMultiArch"`
	Workloads   []string `json:"workloads"`
	TagType     string   `json:"tagType"`
}

type MultiArchRisk1923 struct {
	Image    string `json:"image"`
	RiskType string `json:"riskType"`
	Detail   string `json:"detail"`
}

func (s *Server) handleMultiArchAudit(w http.ResponseWriter, r *http.Request) {
	result := MultiArchResult1923{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Get node architecture
	nodeList, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	nodeArch := make(map[string]int)
	for _, node := range nodeList.Items {
		arch := node.Status.NodeInfo.Architecture
		nodeArch[arch]++
	}

	type imgInfo struct {
		workloads map[string]bool
	}
	imgMap := make(map[string]*imgInfo)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			appName = pod.Name
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			info, exists := imgMap[img]
			if !exists {
				info = &imgInfo{workloads: make(map[string]bool)}
				imgMap[img] = info
			}
			info.workloads[appName] = true
		}
	}

	for img, info := range imgMap {
		wlList := make([]string, 0, len(info.workloads))
		for wl := range info.workloads {
			wlList = append(wlList, wl)
		}

		// Guess architecture from image name patterns
		archGuess := "unknown"
		isMultiArch := false
		tagType := "versioned"
		imgLower := strings.ToLower(img)

		if strings.Contains(img, "@sha256:") {
			tagType = "pinned"
		} else if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
			tagType = "floating"
		}

		// Heuristic: official images and major registries tend to be multi-arch
		if strings.HasPrefix(img, "docker.io/") || strings.HasPrefix(img, "registry.k8s.io/") ||
			strings.HasPrefix(img, "ghcr.io/") || strings.HasPrefix(img, "quay.io/") ||
			(!strings.Contains(img, "/") && strings.Contains(img, ":")) {
			archGuess = "multi-arch"
			isMultiArch = true
		} else {
			archGuess = "single-arch"
		}

		// Check for arch-specific tags
		if strings.Contains(imgLower, "arm64") || strings.Contains(imgLower, "aarch64") {
			archGuess = "arm64"
			isMultiArch = false
		} else if strings.Contains(imgLower, "amd64") || strings.Contains(imgLower, "x86_64") {
			archGuess = "amd64"
			isMultiArch = false
		}

		entry := MultiArchEntry1923{
			Image:       img,
			ArchGuess:   archGuess,
			IsMultiArch: isMultiArch,
			Workloads:   wlList,
			TagType:     tagType,
		}
		result.Images = append(result.Images, entry)
		result.Summary.TotalImages++
		result.Summary.UniqueImages++

		if isMultiArch {
			result.Summary.MultiArchImages++
		} else {
			result.Summary.SingleArchCount++
			if archGuess == "arm64" {
				result.Summary.Arm64Images++
			} else if archGuess == "amd64" {
				result.Summary.Amd64Images++
			} else {
				result.Summary.UnknownArchCount++
			}

			// Risk: single-arch image on multi-arch cluster
			if len(nodeArch) > 1 {
				result.ArchRisks = append(result.ArchRisks, MultiArchRisk1923{
					Image:    img,
					RiskType: "single-arch-on-multi-arch-cluster",
					Detail:   fmt.Sprintf("Image appears single-arch (%s) but cluster has %d architectures", archGuess, len(nodeArch)),
				})
			}
		}

		// Risk: floating tag
		if tagType == "floating" {
			result.ArchRisks = append(result.ArchRisks, MultiArchRisk1923{
				Image:    img,
				RiskType: "floating-tag",
				Detail:   "Floating tag (:latest) — architecture may change unpredictably",
			})
		}
	}

	sort.Slice(result.Images, func(i, j int) bool {
		return result.Images[i].Image < result.Images[j].Image
	})

	// Score
	if result.Summary.SingleArchCount > result.Summary.MultiArchImages {
		score -= 15
	}
	if result.Summary.UnknownArchCount > 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SingleArchCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images appear single-architecture — build multi-arch images for portability", result.Summary.SingleArchCount))
	}
	if result.Summary.UnknownArchCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images with unknown architecture — verify with 'docker manifest inspect'", result.Summary.UnknownArchCount))
	}
	if len(nodeArch) > 1 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Cluster has %d architectures — ensure all images support each", len(nodeArch)))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Dependency Mapper — inter-container dependency & start order
// ---------------------------------------------------------------

type ContainerDepResult1923 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ContainerDepSummary1923 `json:"summary"`
	Pods            []ContainerDepEntry1923 `json:"pods"`
	Risks           []ContainerDepRisk1923  `json:"risks"`
	Recommendations []string                `json:"recommendations"`
}

type ContainerDepSummary1923 struct {
	TotalPods          int `json:"totalPods"`
	MultiContainerPods int `json:"multiContainerPods"`
	WithInitContainers int `json:"withInitContainers"`
	WithSidecars       int `json:"withSidecars"`
	SharedVolumeDeps   int `json:"sharedVolumeDeps"`
	TotalContainers    int `json:"totalContainers"`
	RiskCount          int `json:"riskCount"`
}

type ContainerDepEntry1923 struct {
	PodName         string   `json:"podName"`
	Namespace       string   `json:"namespace"`
	ContainerCount  int      `json:"containerCount"`
	InitContainers  []string `json:"initContainers"`
	MainContainers  []string `json:"mainContainers"`
	SharedVolumes   []string `json:"sharedVolumes"`
	HasDependencies bool     `json:"hasDependencies"`
}

type ContainerDepRisk1923 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
}

func (s *Server) handleContainerDeps(w http.ResponseWriter, r *http.Request) {
	result := ContainerDepResult1923{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		containerCount := len(pod.Spec.Containers)
		result.Summary.TotalPods++
		result.Summary.TotalContainers += containerCount

		if containerCount <= 1 && len(pod.Spec.InitContainers) == 0 {
			continue
		}

		result.Summary.MultiContainerPods++

		initContainers := make([]string, 0)
		for _, ic := range pod.Spec.InitContainers {
			initContainers = append(initContainers, ic.Name)
		}
		if len(initContainers) > 0 {
			result.Summary.WithInitContainers++
		}

		mainContainers := make([]string, 0)
		for _, c := range pod.Spec.Containers {
			mainContainers = append(mainContainers, c.Name)
		}

		// Detect sidecar pattern (2+ main containers)
		isSidecar := containerCount >= 2
		if isSidecar {
			result.Summary.WithSidecars++
		}

		// Find shared volumes between containers
		sharedVols := make([]string, 0)
		volUsage := make(map[string]int)
		for _, c := range pod.Spec.Containers {
			for _, vm := range c.VolumeMounts {
				volUsage[vm.Name]++
			}
		}
		for volName, count := range volUsage {
			if count > 1 {
				sharedVols = append(sharedVols, volName)
				result.Summary.SharedVolumeDeps++
			}
		}

		hasDeps := len(initContainers) > 0 || len(sharedVols) > 0 || isSidecar

		entry := ContainerDepEntry1923{
			PodName:         pod.Name,
			Namespace:       pod.Namespace,
			ContainerCount:  containerCount,
			InitContainers:  initContainers,
			MainContainers:  mainContainers,
			SharedVolumes:   sharedVols,
			HasDependencies: hasDeps,
		}
		result.Pods = append(result.Pods, entry)

		// Risk: many init containers (slow startup)
		if len(initContainers) > 3 {
			result.Risks = append(result.Risks, ContainerDepRisk1923{
				PodName: pod.Name, Namespace: pod.Namespace,
				RiskType: "many-init-containers", Severity: "medium",
				Detail: fmt.Sprintf("%d init containers — sequential startup may be slow", len(initContainers)),
			})
			score -= 2
		}

		// Risk: many sidecars (complexity)
		if containerCount > 4 {
			result.Risks = append(result.Risks, ContainerDepRisk1923{
				PodName: pod.Name, Namespace: pod.Namespace,
				RiskType: "many-containers", Severity: "medium",
				Detail: fmt.Sprintf("%d containers in single pod — consider splitting into separate pods", containerCount),
			})
			score -= 3
		}

		// Risk: no resource limits on any container in multi-container pod
		if containerCount > 1 {
			missingLimits := 0
			for _, c := range pod.Spec.Containers {
				if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
					missingLimits++
				}
			}
			if missingLimit := missingLimits; missingLimit > 0 {
				result.Risks = append(result.Risks, ContainerDepRisk1923{
					PodName: pod.Name, Namespace: pod.Namespace,
					RiskType: "missing-resource-limits", Severity: "high",
					Detail: fmt.Sprintf("%d containers without resource limits in multi-container pod", missingLimit),
				})
				score -= 5
			}
		}
	}

	result.Summary.RiskCount = len(result.Risks)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.MultiContainerPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d multi-container pods — ensure resource isolation between containers", result.Summary.MultiContainerPods))
	}
	if result.Summary.WithInitContainers > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use init containers — optimize startup sequence", result.Summary.WithInitContainers))
	}
	if result.Summary.RiskCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d container dependency risks detected", result.Summary.RiskCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
