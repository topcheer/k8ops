package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeOSDriftResult deeply analyzes node OS and lifecycle:
// kernel version drift, OS image consistency, container runtime versions,
// node age, GPU availability, and rotation readiness.
type NodeOSDriftResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         NodeOSDriftSummary `json:"summary"`
	NodeDetails     []NodeOSDetail     `json:"nodeDetails"`
	DriftFindings   []OSDriftFinding   `json:"driftFindings"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type NodeOSDriftSummary struct {
	TotalNodes     int    `json:"totalNodes"`
	UniqueKernels  int    `json:"uniqueKernels"`
	UniqueOSImages int    `json:"uniqueOSImages"`
	UniqueRuntimes int    `json:"uniqueRuntimes"`
	OldestNodeDays int    `json:"oldestNodeDays"`
	HasGPU         bool   `json:"hasGPU"`
	GPUNodes       int    `json:"gpuNodes"`
	K8sVersion     string `json:"k8sVersion"`
}

type NodeOSDetail struct {
	Name             string `json:"name"`
	Kernel           string `json:"kernel"`
	OSImage          string `json:"osImage"`
	ContainerRuntime string `json:"containerRuntime"`
	Arch             string `json:"arch"`
	AgeDays          int    `json:"ageDays"`
	Status           string `json:"status"`
}

type OSDriftFinding struct {
	Node     string `json:"node"`
	Finding  string `json:"finding"`
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
}

// handleNodeOSDrift deeply analyzes node OS lifecycle and drift.
// GET /api/scalability/node-os-drift
func (s *Server) handleNodeOSDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeOSDriftResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	now := time.Now()

	kernelSet := map[string]bool{}
	osImageSet := map[string]bool{}
	runtimeSet := map[string]bool{}
	oldestDays := 0

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		kernel := node.Status.NodeInfo.KernelVersion
		osImage := node.Status.NodeInfo.OSImage
		runtime := node.Status.NodeInfo.ContainerRuntimeVersion
		arch := node.Status.NodeInfo.Architecture

		kernelSet[kernel] = true
		osImageSet[osImage] = true
		runtimeSet[runtime] = true

		ageDays := int(now.Sub(node.CreationTimestamp.Time).Hours() / 24)
		if ageDays > oldestDays {
			oldestDays = ageDays
		}

		status := "healthy"
		if ageDays > 365 {
			status = "old"
		}
		if ageDays > 730 {
			status = "critical"
		}

		// Check node conditions
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status != "True" {
				status = "not-ready"
			}
		}

		result.NodeDetails = append(result.NodeDetails, NodeOSDetail{
			Name:             node.Name,
			Kernel:           kernel,
			OSImage:          osImage,
			ContainerRuntime: runtime,
			Arch:             arch,
			AgeDays:          ageDays,
			Status:           status,
		})

		// Check for GPU
		for _, alloc := range node.Status.Allocatable {
			if strings.Contains(strings.ToLower(alloc.String()), "nvidia") ||
				strings.Contains(strings.ToLower(alloc.String()), "gpu") {
				result.Summary.HasGPU = true
				result.Summary.GPUNodes++
				break
			}
		}

		// Drift findings
		if ageDays > 365 {
			severity := "medium"
			if ageDays > 730 {
				severity = "high"
			}
			result.DriftFindings = append(result.DriftFindings, OSDriftFinding{
				Node:     node.Name,
				Finding:  fmt.Sprintf("Node %d days old — may need OS patching or rotation", ageDays),
				Severity: severity,
				Impact:   "Older nodes accumulate security debt and may lack recent kernel patches",
			})
		}

		// Check for Ready condition issues
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status != "True" {
				result.DriftFindings = append(result.DriftFindings, OSDriftFinding{
					Node:     node.Name,
					Finding:  "Node not in Ready state",
					Severity: "critical",
					Impact:   "Node is not accepting workloads — investigate kubelet/network issues",
				})
			}
		}
	}

	result.Summary.UniqueKernels = len(kernelSet)
	result.Summary.UniqueOSImages = len(osImageSet)
	result.Summary.UniqueRuntimes = len(runtimeSet)
	result.Summary.OldestNodeDays = oldestDays

	if len(nodes.Items) > 0 {
		result.Summary.K8sVersion = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	// Kernel drift finding
	if len(kernelSet) > 1 {
		result.DriftFindings = append(result.DriftFindings, OSDriftFinding{
			Node:     "cluster-wide",
			Finding:  fmt.Sprintf("%d different kernel versions across nodes — version drift detected", len(kernelSet)),
			Severity: "medium",
			Impact:   "Kernel version drift can cause inconsistent behavior and complicate debugging",
		})
	}
	if len(runtimeSet) > 1 {
		result.DriftFindings = append(result.DriftFindings, OSDriftFinding{
			Node:     "cluster-wide",
			Finding:  fmt.Sprintf("%d different container runtimes — runtime drift detected", len(runtimeSet)),
			Severity: "medium",
			Impact:   "Mixed container runtimes (containerd/CRI-O/docker) can cause compatibility issues",
		})
	}

	// Score
	score := 100
	score -= (len(kernelSet) - 1) * 15
	score -= (len(runtimeSet) - 1) * 10
	if oldestDays > 365 {
		score -= 10
	}
	if oldestDays > 730 {
		score -= 15
	}
	for _, nf := range result.DriftFindings {
		if nf.Severity == "critical" {
			score -= 20
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	// Sort
	sort.Slice(result.NodeDetails, func(i, j int) bool {
		return result.NodeDetails[i].AgeDays > result.NodeDetails[j].AgeDays
	})
	sort.Slice(result.DriftFindings, func(i, j int) bool {
		return result.DriftFindings[i].Severity > result.DriftFindings[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Node OS health: %d/100 (grade %s) — %d nodes, K8s %s", result.HealthScore, result.Grade, result.Summary.TotalNodes, result.Summary.K8sVersion))
	if len(kernelSet) > 1 {
		kernels := make([]string, 0, len(kernelSet))
		for k := range kernelSet {
			kernels = append(kernels, k)
		}
		recs = append(recs, fmt.Sprintf("Kernel drift: %d versions (%s) — standardize with immutable node images", len(kernelSet), strings.Join(kernels, ", ")))
	}
	if len(runtimeSet) > 1 {
		recs = append(recs, fmt.Sprintf("Runtime drift: %d versions — standardize on one container runtime", len(runtimeSet)))
	}
	if oldestDays > 365 {
		recs = append(recs, fmt.Sprintf("Oldest node %d days old — implement node rotation policy", oldestDays))
	}
	if !result.Summary.HasGPU {
		recs = append(recs, "No GPU nodes detected — consider GPU pool for ML/AI workloads if needed")
	}
	if len(recs) == 1 {
		recs = append(recs, "Node OS lifecycle is healthy — uniform kernel and runtime versions")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
