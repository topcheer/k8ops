package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MultiClusterConnResult analyzes multi-cluster connectivity and federation health.
type MultiClusterConnResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         MultiClusterSummary `json:"summary"`
	Connections     []ClusterConnection `json:"connections"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type MultiClusterSummary struct {
	TotalClusters     int  `json:"totalClusters"`
	RemoteClusters    int  `json:"remoteClusters"`
	HealthyClusters   int  `json:"healthyClusters"`
	UnhealthyClusters int  `json:"unhealthyClusters"`
	HasClusterAPI     bool `json:"hasClusterAPI"`
	HasArgoFleet      bool `json:"hasArgoFleet"`
	HasKarmada        bool `json:"hasKarmada"`
}

type ClusterConnection struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
}

// handleMultiClusterConn analyzes multi-cluster connectivity.
// GET /api/scalability/multi-cluster-conn
func (s *Server) handleMultiClusterConn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MultiClusterConnResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Detect multi-cluster tools
	mcKeywords := map[string]string{
		"cluster-api": "ClusterAPI", "capi": "ClusterAPI",
		"argocd": "ArgoCD Fleet", "karmada": "Karmada",
		"kubefed": "KubeFed", "submariner": "Submariner",
		"cilium": "Cilium (cluster mesh)",
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, tool := range mcKeywords {
				if strings.Contains(imgLower, kw) {
					if tool == "ClusterAPI" {
						result.Summary.HasClusterAPI = true
					}
					if tool == "ArgoCD Fleet" {
						result.Summary.HasArgoFleet = true
					}
					if tool == "Karmada" {
						result.Summary.HasKarmada = true
					}
					result.Connections = append(result.Connections, ClusterConnection{
						Name: pod.Name, Type: tool, Status: "active", Severity: "low",
					})
				}
			}
		}
	}

	// Count namespaces that look like remote clusters
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalClusters++
		// Check for cluster-like namespaces
		if strings.Contains(ns.Name, "cluster") || strings.Contains(ns.Name, "remote") || strings.Contains(ns.Name, "fleet") {
			result.Summary.RemoteClusters++
		}
	}

	// Node count as cluster indicator
	nodeCount := len(nodes.Items)
	_ = nodeCount

	// If no multi-cluster tools detected
	if len(result.Connections) == 0 {
		result.Connections = append(result.Connections, ClusterConnection{
			Name: "none", Type: "standalone",
			Status: "single-cluster-only", Severity: "info",
		})
		result.Summary.HealthyClusters = 1 // current cluster
	} else {
		result.Summary.HealthyClusters = len(result.Connections)
	}

	// Score
	score := 50 // base for single cluster
	if result.Summary.HasClusterAPI {
		score += 15
	}
	if result.Summary.HasArgoFleet {
		score += 15
	}
	if result.Summary.HasKarmada {
		score += 20
	}
	if result.Summary.RemoteClusters > 0 {
		score += 10
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.Connections, func(i, j int) bool {
		return result.Connections[i].Status > result.Connections[j].Status
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Multi-cluster readiness: %d/100 (grade %s) — %d clusters, %d remote", result.HealthScore, result.Grade, result.Summary.TotalClusters, result.Summary.RemoteClusters))
	if !result.Summary.HasClusterAPI && !result.Summary.HasArgoFleet && !result.Summary.HasKarmada {
		recs = append(recs, "No multi-cluster management tool detected — consider Cluster API or Karmada for fleet management")
	}
	if result.Summary.HasArgoFleet {
		recs = append(recs, "ArgoCD fleet management detected — ensure application sets span all clusters")
	}
	if result.Summary.HasKarmada {
		recs = append(recs, "Karmada federation active — verify propagation policies cover critical workloads")
	}
	if len(recs) == 1 {
		recs = append(recs, "Multi-cluster setup is healthy")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
