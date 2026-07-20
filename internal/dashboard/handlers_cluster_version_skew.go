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

// ClusterVersionSkewResult detects Kubernetes version skew between control plane and nodes.
type ClusterVersionSkewResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         VersionSkewSummary     `json:"summary"`
	Nodes           []VersionSkewNodeEntry `json:"nodes"`
	SkewRiskNodes   []VersionSkewNodeEntry `json:"skewRiskNodes"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type VersionSkewSummary struct {
	ControlPlaneVersion string `json:"controlPlaneVersion"`
	TotalNodes          int    `json:"totalNodes"`
	MatchingNodes       int    `json:"matchingNodes"`
	MinorSkewNodes      int    `json:"minorSkewNodes"` // 1 minor behind
	MajorSkewNodes      int    `json:"majorSkewNodes"` // 2+ minor behind
	OldestNodeVersion   string `json:"oldestNodeVersion"`
	SkewSupported       bool   `json:"skewSupported"` // within k8s supported skew policy (N-2)
}

type VersionSkewNodeEntry struct {
	NodeName    string   `json:"nodeName"`
	KubeletVer  string   `json:"kubeletVersion"`
	SkewLevel   string   `json:"skewLevel"` // none, minor, major
	SkewDelta   string   `json:"skewDelta"`
	Ready       bool     `json:"ready"`
	RiskFactors []string `json:"riskFactors"`
}

// handleClusterVersionSkew handles GET /api/operations/cluster-version-skew
func (s *Server) handleClusterVersionSkew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ClusterVersionSkewResult{ScannedAt: time.Now()}

	// Get control plane version
	versionInfo, err := rc.clientset.Discovery().ServerVersion()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "failed to get server version")
		return
	}
	cpVer := versionInfo.GitVersion
	result.Summary.ControlPlaneVersion = cpVer
	cpMinor := extractMinorVer1869(cpVer)

	// Get nodes
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	var oldestMinor = cpMinor
	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		entry := VersionSkewNodeEntry{
			NodeName:   node.Name,
			KubeletVer: node.Status.NodeInfo.KubeletVersion,
		}
		entry.Ready = isNodeReady1869(&node)
		nodeMinor := extractMinorVer1869(entry.KubeletVer)
		if nodeMinor >= 0 {
			if nodeMinor < oldestMinor {
				oldestMinor = nodeMinor
				result.Summary.OldestNodeVersion = entry.KubeletVer
			}
			delta := cpMinor - nodeMinor
			entry.SkewDelta = fmt.Sprintf("%d minor", delta)
			var risks []string
			switch {
			case delta == 0:
				entry.SkewLevel = "none"
				result.Summary.MatchingNodes++
			case delta == 1:
				entry.SkewLevel = "minor"
				result.Summary.MinorSkewNodes++
				risks = append(risks, "1-minor-skew")
			case delta >= 2:
				entry.SkewLevel = "major"
				result.Summary.MajorSkewNodes++
				risks = append(risks, fmt.Sprintf("%d-minor-skew", delta))
				risks = append(risks, "unsupported-skew-policy")
			}
			if !entry.Ready {
				risks = append(risks, "node-not-ready")
			}
			entry.RiskFactors = risks
		}

		if entry.SkewLevel == "minor" || entry.SkewLevel == "major" {
			result.SkewRiskNodes = append(result.SkewRiskNodes, entry)
		}
		result.Nodes = append(result.Nodes, entry)
	}

	// Kubernetes supports N-2 skew (2 minor versions)
	result.Summary.SkewSupported = (cpMinor - oldestMinor) <= 2

	sort.Slice(result.Nodes, func(i, j int) bool {
		rank := map[string]int{"major": 0, "minor": 1, "none": 2}
		return rank[result.Nodes[i].SkewLevel] < rank[result.Nodes[j].SkewLevel]
	})

	// Score: penalize for skew
	if result.Summary.TotalNodes > 0 {
		matchRatio := float64(result.Summary.MatchingNodes) / float64(result.Summary.TotalNodes)
		result.HealthScore = int(matchRatio * 100)
		if result.Summary.MajorSkewNodes > 0 {
			result.HealthScore -= 20
		}
		if !result.Summary.SkewSupported {
			result.HealthScore -= 15
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("版本偏差: 控制面 %s, %d 节点匹配, %d 小偏差(1 minor), %d 大偏差(2+ minor)",
			cpVer, result.Summary.MatchingNodes, result.Summary.MinorSkewNodes, result.Summary.MajorSkewNodes),
	}
	if !result.Summary.SkewSupported {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("偏差超过 k8s 支持策略(N-2), 最旧节点: %s", result.Summary.OldestNodeVersion))
	}
	if result.Summary.MajorSkewNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个节点偏差 2+ minor, 建议立即升级 kubelet", result.Summary.MajorSkewNodes))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 制定节点滚动升级计划, 保持 kubelet 版本与控制面一致")
	}
	writeJSON(w, result)
}

func extractMinorVer1869(ver string) int {
	// Parse versions like v1.28.4, 1.29.2
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		return -1
	}
	s := strings.TrimPrefix(parts[1], "v")
	var minor int
	fmt.Sscanf(s, "%d", &minor)
	return minor
}

func isNodeReady1869(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
