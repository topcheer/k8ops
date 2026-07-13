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

// NodeLifecycleResult is the node lifecycle & infrastructure health audit.
type NodeLifecycleResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         NodeLifecycleSummary `json:"summary"`
	ByKernelVersion []NodeKernelEntry    `json:"byKernelVersion"`
	ByOSImage       []NodeOSEntry        `json:"byOSImage"`
	ByArch          []NodeArchEntry      `json:"byArch"`
	GPUNodes        []GPUNodeEntry       `json:"gpuNodes,omitempty"`
	OldNodes        []NodeAgeEntry       `json:"oldNodes"`
	Recommendations []string             `json:"recommendations"`
}

// NodeLifecycleSummary aggregates node lifecycle statistics.
type NodeLifecycleSummary struct {
	TotalNodes         int  `json:"totalNodes"`
	KernelVersions     int  `json:"kernelVersions"` // distinct kernel versions
	OSImages           int  `json:"osImages"`       // distinct OS images
	Archs              int  `json:"archs"`          // distinct architectures
	GPUNodes           int  `json:"gpuNodes"`
	NodesOlderThan90d  int  `json:"nodesOlderThan90d"` // nodes created >90 days ago
	NodesOlderThan180d int  `json:"nodesOlderThan180d"`
	HasGPU             bool `json:"hasGPU"`
	KernelDrift        bool `json:"kernelDrift"`  // multiple kernel versions detected
	OSImageDrift       bool `json:"osImageDrift"` // multiple OS images detected
	HealthScore        int  `json:"healthScore"`
}

// NodeKernelEntry shows nodes grouped by kernel version.
type NodeKernelEntry struct {
	KernelVersion string   `json:"kernelVersion"`
	NodeCount     int      `json:"nodeCount"`
	NodeNames     []string `json:"nodeNames,omitempty"`
}

// NodeOSEntry shows nodes grouped by OS image.
type NodeOSEntry struct {
	OSImage   string   `json:"osImage"`
	NodeCount int      `json:"nodeCount"`
	NodeNames []string `json:"nodeNames,omitempty"`
}

// NodeArchEntry shows nodes grouped by architecture.
type NodeArchEntry struct {
	Arch      string `json:"arch"`
	NodeCount int    `json:"nodeCount"`
}

// GPUNodeEntry describes a node with GPU resources.
type GPUNodeEntry struct {
	Name        string `json:"name"`
	GPUCount    int    `json:"gpuCount"`
	GPUType     string `json:"gpuType,omitempty"`
	GPUMemory   string `json:"gpuMemory,omitempty"`
	Allocatable string `json:"allocatable,omitempty"`
}

// NodeAgeEntry describes an old node.
type NodeAgeEntry struct {
	Name string `json:"name"`
	Age  string `json:"age"`
	Days int    `json:"days"`
}

// nodeLifecycleAuditCore performs the audit on nodes (testable).
func nodeLifecycleAuditCore(nodes []corev1.Node) NodeLifecycleResult {
	result := NodeLifecycleResult{
		ScannedAt: time.Now(),
	}

	kernelMap := make(map[string][]string)
	osMap := make(map[string][]string)
	archMap := make(map[string]int)
	var gpuNodes []GPUNodeEntry
	var oldNodes []NodeAgeEntry

	for i := range nodes {
		node := &nodes[i]
		result.Summary.TotalNodes++

		// Kernel version
		kernelVer := node.Status.NodeInfo.KernelVersion
		if kernelVer == "" {
			kernelVer = "unknown"
		}
		kernelMap[kernelVer] = append(kernelMap[kernelVer], node.Name)

		// OS image
		osImage := node.Status.NodeInfo.OSImage
		if osImage == "" {
			osImage = "unknown"
		}
		osMap[osImage] = append(osMap[osImage], node.Name)

		// Architecture
		arch := node.Status.NodeInfo.Architecture
		if arch == "" {
			arch = "unknown"
		}
		archMap[arch]++

		// GPU resources
		gpuCount := 0
		gpuType := ""
		for resourceName, qty := range node.Status.Allocatable {
			if strings.HasPrefix(string(resourceName), "nvidia.com/gpu") ||
				strings.HasPrefix(string(resourceName), "amd.com/gpu") ||
				strings.HasPrefix(string(resourceName), "intel.com/gpu") {
				gpuCount += int(qty.Value())
				if gpuType == "" {
					gpuType = string(resourceName)
				}
			}
		}
		if gpuCount > 0 {
			result.Summary.HasGPU = true
			result.Summary.GPUNodes++
			gpuNodes = append(gpuNodes, GPUNodeEntry{
				Name:     node.Name,
				GPUCount: gpuCount,
				GPUType:  gpuType,
			})
		}

		// Node age
		age := time.Since(node.CreationTimestamp.Time)
		days := int(age.Hours() / 24)
		if days > 180 {
			result.Summary.NodesOlderThan180d++
			oldNodes = append(oldNodes, NodeAgeEntry{
				Name: node.Name, Days: days,
				Age: formatDurationAge(node.CreationTimestamp.Time),
			})
		} else if days > 90 {
			result.Summary.NodesOlderThan90d++
			oldNodes = append(oldNodes, NodeAgeEntry{
				Name: node.Name, Days: days,
				Age: formatDurationAge(node.CreationTimestamp.Time),
			})
		}
	}

	// Build kernel version entries
	result.Summary.KernelVersions = len(kernelMap)
	result.Summary.KernelDrift = len(kernelMap) > 1
	for kv, names := range kernelMap {
		result.ByKernelVersion = append(result.ByKernelVersion, NodeKernelEntry{
			KernelVersion: kv, NodeCount: len(names), NodeNames: names,
		})
	}
	sort.Slice(result.ByKernelVersion, func(i, j int) bool {
		return result.ByKernelVersion[i].NodeCount > result.ByKernelVersion[j].NodeCount
	})

	// Build OS image entries
	result.Summary.OSImages = len(osMap)
	result.Summary.OSImageDrift = len(osMap) > 1
	for os, names := range osMap {
		result.ByOSImage = append(result.ByOSImage, NodeOSEntry{
			OSImage: os, NodeCount: len(names), NodeNames: names,
		})
	}
	sort.Slice(result.ByOSImage, func(i, j int) bool {
		return result.ByOSImage[i].NodeCount > result.ByOSImage[j].NodeCount
	})

	// Build arch entries
	result.Summary.Archs = len(archMap)
	for arch, count := range archMap {
		result.ByArch = append(result.ByArch, NodeArchEntry{Arch: arch, NodeCount: count})
	}
	sort.Slice(result.ByArch, func(i, j int) bool {
		return result.ByArch[i].NodeCount > result.ByArch[j].NodeCount
	})

	result.GPUNodes = gpuNodes
	result.OldNodes = oldNodes
	sort.Slice(result.OldNodes, func(i, j int) bool {
		return result.OldNodes[i].Days > result.OldNodes[j].Days
	})

	result.Summary.HealthScore = nodeLifecycleScore(result.Summary)
	result.Recommendations = nodeLifecycleRecommendations(result.Summary)

	return result
}

// nodeLifecycleScore calculates health score.
func nodeLifecycleScore(s NodeLifecycleSummary) int {
	base := 100
	// Kernel drift penalty
	if s.KernelDrift {
		base -= (s.KernelVersions - 1) * 10
	}
	// OS image drift penalty
	if s.OSImageDrift {
		base -= (s.OSImages - 1) * 5
	}
	// Old nodes penalty
	base -= s.NodesOlderThan180d * 5
	base -= s.NodesOlderThan90d * 2
	// GPU is a bonus for capability, not health
	if base < 0 {
		base = 0
	}
	return base
}

// nodeLifecycleRecommendations generates recommendations.
func nodeLifecycleRecommendations(s NodeLifecycleSummary) []string {
	var recs []string
	if s.KernelDrift {
		recs = append(recs, fmt.Sprintf("kernel version drift detected: %d different kernel versions across %d nodes — standardize on a single kernel version", s.KernelVersions, s.TotalNodes))
	}
	if s.OSImageDrift {
		recs = append(recs, fmt.Sprintf("OS image drift detected: %d different OS images across %d nodes — standardize node images for consistency", s.OSImages, s.TotalNodes))
	}
	if s.NodesOlderThan180d > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes are older than 180 days — plan node rotation to get latest OS patches and kernel updates", s.NodesOlderThan180d))
	}
	if s.NodesOlderThan90d > 0 && s.NodesOlderThan180d == 0 {
		recs = append(recs, fmt.Sprintf("%d nodes are older than 90 days — consider scheduling node rotation for OS patch updates", s.NodesOlderThan90d))
	}
	if s.HasGPU {
		recs = append(recs, fmt.Sprintf("%d GPU nodes detected — ensure GPU drivers are up to date and node taints prevent non-GPU workloads from scheduling", s.GPUNodes))
	}
	if !s.KernelDrift && !s.OSImageDrift && s.NodesOlderThan90d == 0 {
		recs = append(recs, "node lifecycle is well managed — consistent kernel/OS versions and no stale nodes")
	}
	return recs
}

// handleNodeLifecycle audits node OS patch status, kernel version drift, and GPU resources.
// GET /api/scalability/node-lifecycle
func (s *Server) handleNodeLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := nodeLifecycleAuditCore(nodes.Items)
	writeJSON(w, result)
}
