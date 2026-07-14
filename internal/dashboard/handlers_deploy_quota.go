package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployQuotaResult is the deployment resource quota impact & namespace deployment capacity audit.
type DeployQuotaResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         DeployQuotaSummary  `json:"summary"`
	Namespaces      []DeployQuotaNS     `json:"namespaces"`
	Impacts         []DeployQuotaImpact `json:"impacts"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// DeployQuotaSummary aggregates quota impact statistics.
type DeployQuotaSummary struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	NSWithQuota      int `json:"nsWithQuota"`
	NSWithoutQuota   int `json:"nsWithoutQuota"`
	NSNearQuotaLimit int `json:"nsNearQuotaLimit"` // >80% quota used
	NSOverQuota      int `json:"nsOverQuota"`      // quota exceeded
	NSWithHeadroom   int `json:"nsWithHeadroom"`   // <50% used, plenty of room
	TotalDeploys     int `json:"totalDeploys"`
	DeploysBlocked   int `json:"deploysBlocked"` // not enough quota for deploy
	RiskyDeploys     int `json:"riskyDeploys"`   // deploy would push >90%
}

// DeployQuotaNS describes per-namespace quota capacity.
type DeployQuotaNS struct {
	Namespace    string  `json:"namespace"`
	HasQuota     bool    `json:"hasQuota"`
	CPURequested string  `json:"cpuRequested,omitempty"`
	CPULimited   string  `json:"cpuLimited,omitempty"`
	MemRequested string  `json:"memRequested,omitempty"`
	MemLimited   string  `json:"memLimited,omitempty"`
	CPUQuota     string  `json:"cpuQuota,omitempty"`
	MemQuota     string  `json:"memQuota,omitempty"`
	CPUUsagePct  float64 `json:"cpuUsagePct,omitempty"`
	MemUsagePct  float64 `json:"memUsagePct,omitempty"`
	DeployCount  int     `json:"deployCount"`
	HeadroomPct  float64 `json:"headroomPct,omitempty"`
	RiskLevel    string  `json:"riskLevel"`
}

// DeployQuotaImpact describes a deployment that would impact quota.
type DeployQuotaImpact struct {
	Namespace  string `json:"namespace"`
	DeployName string `json:"deployName"`
	Replicas   int    `json:"replicas"`
	CPURequest string `json:"cpuRequest,omitempty"`
	MemRequest string `json:"memRequest,omitempty"`
	TotalCPU   string `json:"totalCPU,omitempty"`
	TotalMem   string `json:"totalMem,omitempty"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"`
}

// handleDeployQuota audits deployment resource quota impact & namespace deployment capacity.
// GET /api/deployment/quota-impact
func (s *Server) handleDeployQuota(w http.ResponseWriter, r *http.Request) {
	result := DeployQuotaResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// 1. Get all ResourceQuotas
	quotas, err := rc.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsQuotaMap := make(map[string]*DeployQuotaNS)

		for _, rq := range quotas.Items {
			if systemNamespaces[rq.Namespace] {
				continue
			}

			nsStat, ok := nsQuotaMap[rq.Namespace]
			if !ok {
				nsStat = &DeployQuotaNS{Namespace: rq.Namespace, HasQuota: true}
				nsQuotaMap[rq.Namespace] = nsStat
				result.Summary.NSWithQuota++
			}

			for key, val := range rq.Status.Hard {
				switch key {
				case corev1.ResourceCPU:
					nsStat.CPUQuota = val.String()
				case corev1.ResourceMemory:
					nsStat.MemQuota = val.String()
				}
			}

			// Get used values
			for key, val := range rq.Status.Used {
				switch key {
				case corev1.ResourceCPU:
					nsStat.CPURequested = val.String()
				case corev1.ResourceMemory:
					nsStat.MemRequested = val.String()
				}
			}

			// Calculate usage percentages
			if nsStat.CPUQuota != "" && nsStat.CPURequested != "" {
				quota := resource.MustParse(nsStat.CPUQuota)
				used := resource.MustParse(nsStat.CPURequested)
				if quota.MilliValue() > 0 {
					nsStat.CPUUsagePct = float64(used.MilliValue()) / float64(quota.MilliValue()) * 100
				}
			}
			if nsStat.MemQuota != "" && nsStat.MemRequested != "" {
				quota := resource.MustParse(nsStat.MemQuota)
				used := resource.MustParse(nsStat.MemRequested)
				if quota.Value() > 0 {
					nsStat.MemUsagePct = float64(used.Value()) / float64(quota.Value()) * 100
				}
			}

			nsStat.HeadroomPct = 100 - max(nsStat.CPUUsagePct, nsStat.MemUsagePct)
			nsStat.RiskLevel = "low"
			if nsStat.CPUUsagePct > 90 || nsStat.MemUsagePct > 90 {
				nsStat.RiskLevel = "critical"
				result.Summary.NSOverQuota++
			} else if nsStat.CPUUsagePct > 80 || nsStat.MemUsagePct > 80 {
				nsStat.RiskLevel = "high"
				result.Summary.NSNearQuotaLimit++
			} else if nsStat.CPUUsagePct < 50 && nsStat.MemUsagePct < 50 {
				nsStat.RiskLevel = "safe"
				result.Summary.NSWithHeadroom++
			}
		}

		// 2. Get deployments and calculate impact
		deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
		if err == nil {
			for _, dep := range deployments.Items {
				if systemNamespaces[dep.Namespace] {
					continue
				}
				result.Summary.TotalDeploys++

				nsStat, ok := nsQuotaMap[dep.Namespace]
				if !ok {
					nsStat = &DeployQuotaNS{Namespace: dep.Namespace, HasQuota: false, RiskLevel: "info"}
					nsQuotaMap[dep.Namespace] = nsStat
					result.Summary.NSWithoutQuota++
				}
				nsStat.DeployCount++

				replicas := 1
				if dep.Spec.Replicas != nil {
					replicas = int(*dep.Spec.Replicas)
				}

				// Calculate total resource request for this deployment
				totalCPU := resource.Quantity{}
				totalMem := resource.Quantity{}
				for _, c := range dep.Spec.Template.Spec.Containers {
					if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						scaled := cpu
						scaled.Mul(int64(replicas))
						totalCPU.Add(scaled)
					}
					if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						scaled := mem
						scaled.Mul(int64(replicas))
						totalMem.Add(scaled)
					}
				}

				// Check if deployment would push namespace over quota
				if nsStat.HasQuota && nsStat.CPUQuota != "" {
					quota := resource.MustParse(nsStat.CPUQuota)
					used := resource.MustParse(nsStat.CPURequested)
					projected := used
					projected.Add(totalCPU)
					pct := float64(projected.MilliValue()) / float64(quota.MilliValue()) * 100

					if pct > 100 {
						result.Summary.DeploysBlocked++
						result.Impacts = append(result.Impacts, DeployQuotaImpact{
							Namespace:  dep.Namespace,
							DeployName: dep.Name,
							Replicas:   replicas,
							TotalCPU:   totalCPU.String(),
							Issue:      fmt.Sprintf("Deployment would exceed CPU quota (%.1f%% projected)", pct),
							Severity:   "critical",
						})
					} else if pct > 90 {
						result.Summary.RiskyDeploys++
						result.Impacts = append(result.Impacts, DeployQuotaImpact{
							Namespace:  dep.Namespace,
							DeployName: dep.Name,
							Replicas:   replicas,
							TotalCPU:   totalCPU.String(),
							Issue:      fmt.Sprintf("Deployment would push CPU usage to %.1f%% of quota", pct),
							Severity:   "high",
						})
					}
				}
			}
		}

		// Build namespace list
		for _, ns := range nsQuotaMap {
			result.Namespaces = append(result.Namespaces, *ns)
		}
		result.Summary.TotalNamespaces = len(result.Namespaces)

		sort.Slice(result.Namespaces, func(i, j int) bool {
			return result.Namespaces[i].RiskLevel > result.Namespaces[j].RiskLevel
		})
		sort.Slice(result.Impacts, func(i, j int) bool {
			return result.Impacts[i].Severity > result.Impacts[j].Severity
		})
	}

	// Recommendations
	if result.Summary.NSOverQuota > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have exceeded quota — increase quota or reduce workloads", result.Summary.NSOverQuota))
	}
	if result.Summary.NSNearQuotaLimit > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces are near quota limit (>80%%) — plan capacity expansion", result.Summary.NSNearQuotaLimit))
	}
	if result.Summary.DeploysBlocked > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments would be blocked by quota — scale down or increase quota", result.Summary.DeploysBlocked))
	}
	if result.Summary.NSWithoutQuota > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have no ResourceQuota — add quotas to prevent resource monopolization", result.Summary.NSWithoutQuota))
	}

	// Health score
	score := 100
	score -= result.Summary.NSOverQuota * 15
	score -= result.Summary.NSNearQuotaLimit * 5
	score -= result.Summary.DeploysBlocked * 10
	score -= result.Summary.NSWithoutQuota * 2
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
