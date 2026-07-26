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
// v19.94 — Scalability & HA Dimension (Round 18 Final)
// 1. Control Plane Load — API server & controller pressure estimator
// 2. Volume Attachment Density — per-node PVC attachment load
// 3. Namespace Quota Utilization — resource quota consumption analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Control Plane Load
// ---------------------------------------------------------------

type CPLoadResult1994 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CPLoadSummary1994   `json:"summary"`
	PerNS           []CPLoadNSEntry1994 `json:"perNamespace"`
	Recommendations []string            `json:"recommendations"`
}

type CPLoadSummary1994 struct {
	TotalObjects  int     `json:"totalObjects"`
	TotalPods     int     `json:"totalPods"`
	TotalServices int     `json:"totalServices"`
	EstAPIQPS     float64 `json:"estAPIQPS"`
	LoadLevel     string  `json:"loadLevel"`
}

type CPLoadNSEntry1994 struct {
	Namespace   string `json:"namespace"`
	ObjectCount int    `json:"objectCount"`
}

func (s *Server) handleControlPlaneLoad(w http.ResponseWriter, r *http.Request) {
	result := CPLoadResult1994{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalPods = len(podList.Items)
	result.Summary.TotalServices = len(svcList.Items)
	result.Summary.TotalObjects = result.Summary.TotalPods + result.Summary.TotalServices

	// Estimate API QPS from object count
	// ~0.5 QPS per pod (watch + periodic), ~2 QPS per controller
	result.Summary.EstAPIQPS = float64(result.Summary.TotalPods)*0.5 + float64(result.Summary.TotalServices)*0.3 + 10

	nsStats := make(map[string]int)
	for _, pod := range podList.Items {
		nsStats[pod.Namespace]++
	}
	for _, svc := range svcList.Items {
		nsStats[svc.Namespace]++
	}
	for _, ns := range nsList.Items {
		if nsStats[ns.Name] > 0 {
			result.PerNS = append(result.PerNS, CPLoadNSEntry1994{
				Namespace: ns.Name, ObjectCount: nsStats[ns.Name],
			})
		}
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].ObjectCount > result.PerNS[j].ObjectCount
	})

	// Load level
	if result.Summary.EstAPIQPS > 500 {
		result.Summary.LoadLevel = "critical"
		score -= 10
	} else if result.Summary.EstAPIQPS > 200 {
		result.Summary.LoadLevel = "high"
		score -= 5
	} else if result.Summary.EstAPIQPS > 100 {
		result.Summary.LoadLevel = "medium"
	} else {
		result.Summary.LoadLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d objects (%d pods, %d services), est %.0f API QPS, load: %s", result.Summary.TotalObjects, result.Summary.TotalPods, result.Summary.TotalServices, result.Summary.EstAPIQPS, result.Summary.LoadLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume Attachment Density
// ---------------------------------------------------------------

type VolAttachResult1994 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         VolAttachSummary1994     `json:"summary"`
	PerNode         []VolAttachNodeEntry1994 `json:"perNode"`
	Recommendations []string                 `json:"recommendations"`
}

type VolAttachSummary1994 struct {
	TotalPVCs    int     `json:"totalPVCs"`
	BoundPVCs    int     `json:"boundPVCs"`
	AvgPerNode   float64 `json:"avgAttachPerNode"`
	MaxPerNode   int     `json:"maxAttachPerNode"`
	DensityLevel string  `json:"densityLevel"`
}

type VolAttachNodeEntry1994 struct {
	Node        string `json:"node"`
	AttachCount int    `json:"attachCount"`
}

func (s *Server) handleVolAttachDensity(w http.ResponseWriter, r *http.Request) {
	result := VolAttachResult1994{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalPVCs = len(pvcList.Items)
	for _, pvc := range pvcList.Items {
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		}
	}

	// Count volume attachments per node
	attachPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				attachPerNode[pod.Spec.NodeName]++
			}
		}
	}

	maxAttach := 0
	for _, node := range nodeList.Items {
		count := attachPerNode[node.Name]
		entry := VolAttachNodeEntry1994{Node: node.Name, AttachCount: count}
		result.PerNode = append(result.PerNode, entry)
		if count > maxAttach {
			maxAttach = count
		}
	}

	result.Summary.MaxPerNode = maxAttach
	if len(nodeList.Items) > 0 {
		totalAttach := 0
		for _, c := range attachPerNode {
			totalAttach += c
		}
		result.Summary.AvgPerNode = float64(totalAttach) / float64(len(nodeList.Items))
	}

	// Density level (typical limit is 256 per node)
	if maxAttach > 200 {
		result.Summary.DensityLevel = "critical"
		score -= 10
	} else if maxAttach > 100 {
		result.Summary.DensityLevel = "high"
		score -= 5
	} else if maxAttach > 50 {
		result.Summary.DensityLevel = "medium"
	} else {
		result.Summary.DensityLevel = "low"
	}

	sort.Slice(result.PerNode, func(i, j int) bool {
		return result.PerNode[i].AttachCount > result.PerNode[j].AttachCount
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d bound), max %d/node, avg %.1f/node, density: %s", result.Summary.TotalPVCs, result.Summary.BoundPVCs, maxAttach, result.Summary.AvgPerNode, result.Summary.DensityLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Quota Utilization
// ---------------------------------------------------------------

type QuotaUtilResult1994 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         QuotaUtilSummary1994 `json:"summary"`
	Quotas          []QuotaUtilEntry1994 `json:"quotas"`
	Recommendations []string             `json:"recommendations"`
}

type QuotaUtilSummary1994 struct {
	TotalQuotas     int `json:"totalResourceQuotas"`
	NearLimitNS     int `json:"namespacesNearLimit"`
	ExceededNS      int `json:"namespacesExceeded"`
	TotalNamespaces int `json:"totalNamespaces"`
}

type QuotaUtilEntry1994 struct {
	Namespace  string            `json:"namespace"`
	Hard       map[string]string `json:"hard"`
	Used       map[string]string `json:"used"`
	MaxUtilPct float64           `json:"maxUtilizationPct"`
}

func (s *Server) handleNSQuotaUtilization(w http.ResponseWriter, r *http.Request) {
	result := QuotaUtilResult1994{ScannedAt: time.Now()}
	score := 100

	quotaList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNamespaces = len(nsList.Items)

	for _, rq := range quotaList.Items {
		result.Summary.TotalQuotas++

		entry := QuotaUtilEntry1994{
			Namespace: rq.Namespace,
			Hard:      make(map[string]string),
			Used:      make(map[string]string),
		}

		maxUtil := 0.0
		for key, hard := range rq.Status.Hard {
			entry.Hard[string(key)] = hard.String()
			if used, ok := rq.Status.Used[key]; ok {
				entry.Used[string(key)] = used.String()
				// Calculate utilization percentage
				hardQty := hard.Value()
				usedQty := used.Value()
				if hardQty > 0 && usedQty > 0 {
					util := float64(usedQty) / float64(hardQty) * 100
					if util > maxUtil {
						maxUtil = util
					}
				}
			}
		}
		entry.MaxUtilPct = maxUtil

		if maxUtil > 90 {
			result.Summary.NearLimitNS++
			score -= 3
		}
		if maxUtil >= 100 {
			result.Summary.ExceededNS++
			score -= 5
		}

		result.Quotas = append(result.Quotas, entry)
	}

	sort.Slice(result.Quotas, func(i, j int) bool {
		return result.Quotas[i].MaxUtilPct > result.Quotas[j].MaxUtilPct
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d quotas across %d namespaces: %d near limit, %d exceeded", result.Summary.TotalQuotas, result.Summary.TotalNamespaces, result.Summary.NearLimitNS, result.Summary.ExceededNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
