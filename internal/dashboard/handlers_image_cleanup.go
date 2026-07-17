package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageCleanupResult identifies unused and stale container images on nodes
// to free up disk space. It cross-references running pods with node image
// caches to find images that could be safely garbage collected.
type ImageCleanupResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ImgCleanupSummary `json:"summary"`
	UnusedImages    []ImgCleanupEntry `json:"unusedImages"`
	StaleImages     []ImgCleanupEntry `json:"staleImages"`
	ByNode          []ImgNodeSummary  `json:"byNode"`
	PotentialSave   ImgCleanupSave    `json:"potentialSave"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type ImgCleanupSummary struct {
	TotalImages   int `json:"totalImages"`
	RunningImages int `json:"runningImages"`
	UnusedImages  int `json:"unusedImages"`
	StaleImages   int `json:"staleImages"`
	DuplicateTags int `json:"duplicateTags"`
}

type ImgCleanupEntry struct {
	Image  string   `json:"image"`
	Status string   `json:"status"` // unused, stale, duplicate
	Size   string   `json:"estimatedSize"`
	Reason string   `json:"reason"`
	Nodes  []string `json:"nodes"`
}

type ImgNodeSummary struct {
	Node        string `json:"node"`
	TotalImages int    `json:"totalImages"`
	UnusedCount int    `json:"unusedCount"`
	DiskUsage   string `json:"estimatedDiskUsage"`
}

type ImgCleanupSave struct {
	EstimatedDiskGB  float64 `json:"estimatedDiskGB"`
	EstimatedPercent float64 `json:"estimatedPercent"`
}

// handleImageCleanup handles GET /api/scalability/image-cleanup
func (s *Server) handleImageCleanup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ImageCleanupResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Build set of running images
	runningImages := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Image != "" {
				runningImages[cs.Image] = true
			}
		}
	}

	// Build node image map from node status
	type nodeImageInfo struct {
		images []string
	}
	nodeImgMap := make(map[string]*nodeImageInfo)
	allNodeImages := make(map[string][]string) // image -> nodes

	for _, node := range nodes.Items {
		ni := &nodeImageInfo{}
		for _, img := range node.Status.Images {
			for _, name := range img.Names {
				ni.images = append(ni.images, name)
				allNodeImages[name] = append(allNodeImages[name], node.Name)
			}
		}
		nodeImgMap[node.Name] = ni
	}

	// Categorize images
	imageBaseMap := make(map[string][]string) // base name -> full refs
	for img := range allNodeImages {
		base := getImageBase(img)
		imageBaseMap[base] = append(imageBaseMap[base], img)
	}

	totalImages := len(allNodeImages)
	unusedCount := 0
	staleCount := 0
	dupCount := 0
	var unusedEntries []ImgCleanupEntry
	var staleEntries []ImgCleanupEntry

	for img, nodeList := range allNodeImages {
		result.Summary.TotalImages++

		if !runningImages[img] {
			// Check if any variant is running
			base := getImageBase(img)
			anyRunning := false
			for _, variant := range imageBaseMap[base] {
				if runningImages[variant] {
					anyRunning = true
					break
				}
			}
			if !anyRunning {
				unusedCount++
				unusedEntries = append(unusedEntries, ImgCleanupEntry{
					Image:  img,
					Status: "unused",
					Size:   "~500MB",
					Reason: "No running pod uses this image",
					Nodes:  nodeList,
				})
			}
		}

		// Stale: :latest or very old tag
		if isStaleImage(img) {
			staleCount++
			staleEntries = append(staleEntries, ImgCleanupEntry{
				Image:  img,
				Status: "stale",
				Reason: "Uses :latest or mutable tag",
				Nodes:  nodeList,
			})
		}

		// Duplicate: multiple tags for same base image
		base := getImageBase(img)
		if len(imageBaseMap[base]) > 2 {
			dupCount++
		}
	}

	result.Summary.RunningImages = len(runningImages)
	result.Summary.UnusedImages = unusedCount
	result.Summary.StaleImages = staleCount
	result.Summary.DuplicateTags = dupCount
	result.UnusedImages = unusedEntries
	result.StaleImages = staleEntries

	// Node summaries
	for nodeName, ni := range nodeImgMap {
		unusedOnNode := 0
		for _, img := range ni.images {
			if !runningImages[img] {
				unusedOnNode++
			}
		}
		estDisk := float64(len(ni.images)) * 0.5 // ~500MB avg per image
		result.ByNode = append(result.ByNode, ImgNodeSummary{
			Node:        nodeName,
			TotalImages: len(ni.images),
			UnusedCount: unusedOnNode,
			DiskUsage:   fmt.Sprintf("%.1f GB", estDisk),
		})
	}

	// Potential savings
	estSaveGB := float64(unusedCount) * 0.5
	estPercent := 0.0
	if totalImages > 0 {
		estPercent = float64(unusedCount) / float64(totalImages) * 100
	}
	result.PotentialSave = ImgCleanupSave{
		EstimatedDiskGB:  estSaveGB,
		EstimatedPercent: estPercent,
	}

	// Score
	if totalImages > 0 {
		result.HealthScore = (totalImages - unusedCount) * 100 / totalImages
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	sort.Slice(result.UnusedImages, func(i, j int) bool {
		return len(result.UnusedImages[i].Nodes) > len(result.UnusedImages[j].Nodes)
	})

	result.Recommendations = buildImgCleanupRecs(&result)
	writeJSON(w, result)
}

func getImageBase(img string) string {
	// Strip tag and digest
	for i := len(img) - 1; i >= 0; i-- {
		if img[i] == '/' {
			break
		}
		if img[i] == ':' || img[i] == '@' {
			return img[:i]
		}
	}
	return img
}

func isStaleImage(img string) bool {
	// Check for :latest
	for i := len(img) - 1; i >= 0; i-- {
		if img[i] == ':' {
			tag := img[i+1:]
			return tag == "latest"
		}
		if img[i] == '/' {
			break
		}
	}
	return false
}

func buildImgCleanupRecs(r *ImageCleanupResult) []string {
	recs := []string{}
	if r.Summary.UnusedImages > 0 {
		recs = append(recs, fmt.Sprintf("%d 个镜像未被任何 Pod 使用，可安全清理", r.Summary.UnusedImages))
	}
	if r.PotentialSave.EstimatedDiskGB > 1 {
		recs = append(recs, fmt.Sprintf("预计可释放 %.1f GB 磁盘空间 (%.0f%%)", r.PotentialSave.EstimatedDiskGB, r.PotentialSave.EstimatedPercent))
	}
	if r.Summary.StaleImages > 0 {
		recs = append(recs, fmt.Sprintf("%d 个镜像使用 :latest 标签，建议固定版本", r.Summary.StaleImages))
	}
	if r.Summary.DuplicateTags > 0 {
		recs = append(recs, fmt.Sprintf("%d 个镜像有多个版本标签，建议清理旧版本", r.Summary.DuplicateTags))
	}
	if len(recs) == 0 {
		recs = append(recs, "镜像管理良好，无清理需求")
	}
	return recs
}

var _ corev1.Node
