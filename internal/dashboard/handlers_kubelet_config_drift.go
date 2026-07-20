package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeletConfigDriftResult detects kubelet configuration inconsistencies across nodes.
type KubeletConfigDriftResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         KubeletDriftSummary `json:"summary"`
	Nodes           []KubeletDriftEntry `json:"nodes"`
	DriftedNodes    []KubeletDriftEntry `json:"driftedNodes"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type KubeletDriftSummary struct {
	TotalNodes    int `json:"totalNodes"`
	Consistent    int `json:"consistentNodes"`
	DriftedCount  int `json:"driftedNodes"`
	VersionGroups int `json:"versionGroups"`
	RuntimeGroups int `json:"runtimeGroups"`
	ArchGroups    int `json:"archGroups"`
}

type KubeletDriftEntry struct {
	Name             string   `json:"name"`
	KubeletVersion   string   `json:"kubeletVersion"`
	ContainerRuntime string   `json:"containerRuntime"`
	OSImage          string   `json:"osImage"`
	Arch             string   `json:"architecture"`
	KernelVersion    string   `json:"kernelVersion"`
	IsDrifted        bool     `json:"isDrifted"`
	DriftReasons     []string `json:"driftReasons"`
	RiskLevel        string   `json:"riskLevel"`
}

// handleKubeletConfigDrift handles GET /api/operations/kubelet-config-drift
func (s *Server) handleKubeletConfigDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := KubeletConfigDriftResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Collect all values to find the "baseline" (most common)
	versionCount := make(map[string]int)
	runtimeCount := make(map[string]int)
	archCount := make(map[string]int)
	osCount := make(map[string]int)

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		versionCount[node.Status.NodeInfo.KubeletVersion]++
		runtimeCount[node.Status.NodeInfo.ContainerRuntimeVersion]++
		archCount[node.Status.NodeInfo.Architecture]++
		osCount[node.Status.NodeInfo.OSImage]++
	}

	result.Summary.VersionGroups = len(versionCount)
	result.Summary.RuntimeGroups = len(runtimeCount)
	result.Summary.ArchGroups = len(archCount)

	// Find baseline (most common)
	baselineVer := mostCommonKey(versionCount)
	baselineRuntime := mostCommonKey(runtimeCount)
	baselineOS := mostCommonKey(osCount)

	for _, node := range nodes.Items {
		entry := KubeletDriftEntry{
			Name:             node.Name,
			KubeletVersion:   node.Status.NodeInfo.KubeletVersion,
			ContainerRuntime: node.Status.NodeInfo.ContainerRuntimeVersion,
			OSImage:          node.Status.NodeInfo.OSImage,
			Arch:             node.Status.NodeInfo.Architecture,
			KernelVersion:    node.Status.NodeInfo.KernelVersion,
		}

		var drifts []string
		if entry.KubeletVersion != baselineVer {
			drifts = append(drifts, fmt.Sprintf("kubelet %s vs baseline %s", entry.KubeletVersion, baselineVer))
		}
		if entry.ContainerRuntime != baselineRuntime {
			drifts = append(drifts, fmt.Sprintf("runtime %s vs baseline %s", entry.ContainerRuntime, baselineRuntime))
		}
		if entry.OSImage != baselineOS {
			drifts = append(drifts, fmt.Sprintf("OS %s vs baseline %s", entry.OSImage, baselineOS))
		}

		entry.DriftReasons = drifts
		entry.IsDrifted = len(drifts) > 0

		switch {
		case len(drifts) >= 2:
			entry.RiskLevel = "high"
		case len(drifts) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.IsDrifted {
			result.Summary.DriftedCount++
			result.DriftedNodes = append(result.DriftedNodes, entry)
		} else {
			result.Summary.Consistent++
		}

		result.Nodes = append(result.Nodes, entry)
	}

	sort.Slice(result.DriftedNodes, func(i, j int) bool {
		return len(result.DriftedNodes[i].DriftReasons) > len(result.DriftedNodes[j].DriftReasons)
	})

	if result.Summary.TotalNodes > 0 {
		result.HealthScore = result.Summary.Consistent * 100 / result.Summary.TotalNodes
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Kubelet 配置漂移: %d 节点, %d 一致, %d 漂移, %d 版本组, %d 运行时组",
			result.Summary.TotalNodes, result.Summary.Consistent,
			result.Summary.DriftedCount, result.Summary.VersionGroups,
			result.Summary.RuntimeGroups),
	}
	if result.Summary.DriftedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个节点配置不一致, 建议统一升级", result.Summary.DriftedCount))
	}
	writeJSON(w, result)
}

func mostCommonKey(m map[string]int) string {
	max := 0
	result := ""
	for k, v := range m {
		if v > max {
			max = v
			result = k
		}
	}
	return result
}

var _ corev1.NodeSystemInfo
