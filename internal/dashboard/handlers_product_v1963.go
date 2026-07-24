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
// v19.63 — Product Dimension (Round 13)
// 1. Workload Insights — per-workload health score & resource efficiency summary
// 2. Storage Summary — PVC usage, storage class distribution & capacity overview
// 3. Network Topology Insights — service connectivity & traffic flow mapping
// ============================================================

// ---------------------------------------------------------------
// 1. Workload Insights
// ---------------------------------------------------------------

type WLInsightsResult1963 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         WLInsightsSummary1963 `json:"summary"`
	TopWorkloads    []WLInsightsEntry1963 `json:"topWorkloads"`
	Underperforming []WLInsightsEntry1963 `json:"underperforming"`
	Recommendations []string              `json:"recommendations"`
}

type WLInsightsSummary1963 struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	HealthyCount   int     `json:"healthyWorkloads"`
	WarningCount   int     `json:"warningWorkloads"`
	CriticalCount  int     `json:"criticalWorkloads"`
	AvgHealthScore float64 `json:"avgHealthScore"`
	TotalReplicas  int     `json:"totalReplicas"`
	ReadyReplicas  int     `json:"readyReplicas"`
	RestartTotal   int     `json:"totalRestarts"`
}

type WLInsightsEntry1963 struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	Replicas    int     `json:"replicas"`
	Ready       int     `json:"ready"`
	Restarts    int     `json:"restarts"`
	HealthScore float64 `json:"healthScore"`
	Status      string  `json:"status"`
	Age         string  `json:"age"`
}

func (s *Server) handleWorkloadInsights(w http.ResponseWriter, r *http.Request) {
	result := WLInsightsResult1963{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	// Build pod stats per workload
	type podStats struct {
		running  int
		pending  int
		failed   int
		restarts int
	}
	podMap := make(map[string]*podStats) // ns/name -> stats
	for _, pod := range podList.Items {
		key := ""
		for _, or := range pod.OwnerReferences {
			if or.Kind == "ReplicaSet" {
				// Try to match deployment name
				key = pod.Namespace + "/" + or.Name
			} else {
				key = pod.Namespace + "/" + or.Name
			}
		}
		if key == "" {
			key = pod.Namespace + "/" + pod.Name
		}
		ps, ok := podMap[key]
		if !ok {
			ps = &podStats{}
			podMap[key] = ps
		}
		switch pod.Status.Phase {
		case corev1.PodRunning:
			ps.running++
		case corev1.PodPending:
			ps.pending++
		case corev1.PodFailed:
			ps.failed++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			ps.restarts += int(cs.RestartCount)
		}
	}

	var totalScore float64
	var count int

	// Deployments
	for _, dep := range depList.Items {
		if dep.Spec.Replicas == nil || *dep.Spec.Replicas == 0 {
			continue
		}
		replicas := int(*dep.Spec.Replicas)
		ready := int(dep.Status.ReadyReplicas)
		result.Summary.TotalWorkloads++
		result.Summary.TotalReplicas += replicas
		result.Summary.ReadyReplicas += ready
		count++

		// Find restarts from pods
		restarts := 0
		for _, pod := range podList.Items {
			if pod.Namespace != dep.Namespace {
				continue
			}
			for _, or := range pod.OwnerReferences {
				if or.Kind == "ReplicaSet" {
					for _, cs := range pod.Status.ContainerStatuses {
						restarts += int(cs.RestartCount)
					}
				}
			}
		}
		result.Summary.RestartTotal += restarts

		wlScore := 100.0
		if replicas > 0 && ready < replicas {
			wlScore -= float64(replicas-ready) / float64(replicas) * 50
		}
		if restarts > 5 {
			wlScore -= 20
		}
		if restarts > 20 {
			wlScore -= 20
		}

		status := "healthy"
		if wlScore < 50 {
			status = "critical"
			result.Summary.CriticalCount++
			score -= 5
		} else if wlScore < 80 {
			status = "warning"
			result.Summary.WarningCount++
			score -= 2
		} else {
			result.Summary.HealthyCount++
		}

		entry := WLInsightsEntry1963{
			Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
			Replicas: replicas, Ready: ready, Restarts: restarts,
			HealthScore: wlScore, Status: status,
			Age: fmt.Sprintf("%.0fd", time.Since(dep.CreationTimestamp.Time).Hours()/24),
		}

		totalScore += wlScore
		if status == "healthy" {
			result.TopWorkloads = append(result.TopWorkloads, entry)
		} else {
			result.Underperforming = append(result.Underperforming, entry)
		}
	}

	// StatefulSets
	for _, sts := range stsList.Items {
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
			continue
		}
		replicas := int(*sts.Spec.Replicas)
		ready := int(sts.Status.ReadyReplicas)
		result.Summary.TotalWorkloads++
		result.Summary.TotalReplicas += replicas
		result.Summary.ReadyReplicas += ready
		count++

		wlScore := 100.0
		if replicas > 0 && ready < replicas {
			wlScore -= float64(replicas-ready) / float64(replicas) * 50
		}

		status := "healthy"
		if wlScore < 50 {
			status = "critical"
			result.Summary.CriticalCount++
		} else if wlScore < 80 {
			status = "warning"
			result.Summary.WarningCount++
		} else {
			result.Summary.HealthyCount++
		}

		entry := WLInsightsEntry1963{
			Name: sts.Name, Namespace: sts.Namespace, Kind: "StatefulSet",
			Replicas: replicas, Ready: ready,
			HealthScore: wlScore, Status: status,
			Age: fmt.Sprintf("%.0fd", time.Since(sts.CreationTimestamp.Time).Hours()/24),
		}
		totalScore += wlScore
		if status == "healthy" {
			result.TopWorkloads = append(result.TopWorkloads, entry)
		} else {
			result.Underperforming = append(result.Underperforming, entry)
		}
	}

	if count > 0 {
		result.Summary.AvgHealthScore = totalScore / float64(count)
	}

	sort.Slice(result.Underperforming, func(i, j int) bool {
		return result.Underperforming[i].HealthScore < result.Underperforming[j].HealthScore
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d workloads: %d healthy, %d warning, %d critical", result.Summary.TotalWorkloads, result.Summary.HealthyCount, result.Summary.WarningCount, result.Summary.CriticalCount))
	if len(result.Underperforming) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d underperforming workloads need attention", len(result.Underperforming)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Storage Summary
// ---------------------------------------------------------------

type StorageSummaryResult1963 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         StorageSummarySummary1963 `json:"summary"`
	StorageClasses  []StorageClassEntry1963   `json:"storageClasses"`
	LargePVCs       []PVCSummaryEntry1963     `json:"largePVCs"`
	UnboundPVCs     []PVCSummaryEntry1963     `json:"unboundPVCs"`
	Recommendations []string                  `json:"recommendations"`
}

type StorageSummarySummary1963 struct {
	TotalPVCs         int     `json:"totalPVCs"`
	BoundPVCs         int     `json:"boundPVCs"`
	UnboundPVCs       int     `json:"unboundPVCs"`
	TotalCapacityGB   float64 `json:"totalCapacityGB"`
	UsedCapacityGB    float64 `json:"usedCapacityGB"`
	StorageClassCount int     `json:"storageClassCount"`
	PVCount           int     `json:"totalPVs"`
}

type StorageClassEntry1963 struct {
	Name        string  `json:"name"`
	Provisioner string  `json:"provisioner"`
	PVCCount    int     `json:"pvcCount"`
	Capacity    float64 `json:"capacityGB"`
	IsDefault   bool    `json:"isDefault"`
}

type PVCSummaryEntry1963 struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	SizeGB       float64 `json:"sizeGB"`
	StorageClass string  `json:"storageClass"`
	Status       string  `json:"status"`
}

func (s *Server) handleStorageSummary(w http.ResponseWriter, r *http.Request) {
	result := StorageSummaryResult1963{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})

	result.Summary.PVCount = len(pvList.Items)

	// Storage class info
	scMap := make(map[string]*StorageClassEntry1963)
	defaultSC := ""
	for _, sc := range scList.Items {
		entry := &StorageClassEntry1963{
			Name: sc.Name, Provisioner: sc.Provisioner,
		}
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			entry.IsDefault = true
			defaultSC = sc.Name
		}
		scMap[sc.Name] = entry
	}
	result.Summary.StorageClassCount = len(scList.Items)

	// PVC stats
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		sizeGB := 0.0
		if sz := pvc.Spec.Resources.Requests.Storage(); sz != nil {
			sizeGB = float64(sz.Value()) / (1024 * 1024 * 1024)
		}
		result.Summary.TotalCapacityGB += sizeGB

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}

		entry := PVCSummaryEntry1963{
			Name: pvc.Name, Namespace: pvc.Namespace,
			SizeGB: sizeGB, StorageClass: scName,
		}

		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
			entry.Status = "Bound"
			// Track used capacity (approximate as requested)
			result.Summary.UsedCapacityGB += sizeGB

			// Track large PVCs (>50GB)
			if sizeGB > 50 {
				result.LargePVCs = append(result.LargePVCs, entry)
			}
		} else {
			result.Summary.UnboundPVCs++
			entry.Status = string(pvc.Status.Phase)
			result.UnboundPVCs = append(result.UnboundPVCs, entry)
			score -= 3
		}

		// Update storage class stats
		if sc, ok := scMap[scName]; ok {
			sc.PVCCount++
			sc.Capacity += sizeGB
		} else if scName != "" {
			newSC := &StorageClassEntry1963{Name: scName, PVCCount: 1, Capacity: sizeGB}
			scMap[scName] = newSC
		}
	}

	for _, sc := range scMap {
		result.StorageClasses = append(result.StorageClasses, *sc)
	}
	sort.Slice(result.StorageClasses, func(i, j int) bool {
		return result.StorageClasses[i].PVCCount > result.StorageClasses[j].PVCCount
	})
	sort.Slice(result.LargePVCs, func(i, j int) bool {
		return result.LargePVCs[i].SizeGB > result.LargePVCs[j].SizeGB
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d bound, %d unbound), %.1f GB total capacity", result.Summary.TotalPVCs, result.Summary.BoundPVCs, result.Summary.UnboundPVCs, result.Summary.TotalCapacityGB))
	if result.Summary.UnboundPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unbound PVCs — check storage class provisioning", result.Summary.UnboundPVCs))
	}
	if defaultSC != "" {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Default storage class: %s", defaultSC))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Network Topology Insights
// ---------------------------------------------------------------

type NetTopoResult1963 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NetTopoSummary1963    `json:"summary"`
	ServiceMap      []NetTopoService1963  `json:"serviceMap"`
	Endpoints       []NetTopoEndpoint1963 `json:"endpoints"`
	Recommendations []string              `json:"recommendations"`
}

type NetTopoSummary1963 struct {
	TotalServices    int `json:"totalServices"`
	ClusterIPSvc     int `json:"clusterIPServices"`
	NodePortSvc      int `json:"nodePortServices"`
	LoadBalancerSvc  int `json:"loadBalancerServices"`
	ExternalNameSvc  int `json:"externalNameServices"`
	WithEndpoints    int `json:"servicesWithEndpoints"`
	WithoutEndpoints int `json:"servicesWithoutEndpoints"`
	TotalIngress     int `json:"totalIngressRules"`
}

type NetTopoService1963 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"clusterIP"`
	HasPorts  int    `json:"portCount"`
}

type NetTopoEndpoint1963 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Addresses int    `json:"endpointCount"`
	Ready     bool   `json:"hasReadyEndpoints"`
}

func (s *Server) handleNetworkTopology(w http.ResponseWriter, r *http.Request) {
	result := NetTopoResult1963{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalIngress = len(ingList.Items)

	// Build endpoint map
	epMap := make(map[string]bool) // ns/svc -> has ready endpoints
	for _, ep := range epList.Items {
		hasReady := false
		for _, sub := range ep.Subsets {
			if len(sub.Addresses) > 0 {
				hasReady = true
			}
		}
		key := ep.Namespace + "/" + ep.Name
		epMap[key] = hasReady

		result.Endpoints = append(result.Endpoints, NetTopoEndpoint1963{
			Service: ep.Name, Namespace: ep.Namespace,
			Addresses: len(subCountAddr(ep)), Ready: hasReady,
		})
	}

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		entry := NetTopoService1963{
			Name: svc.Name, Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			HasPorts:  len(svc.Spec.Ports),
		}

		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			result.Summary.ClusterIPSvc++
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePortSvc++
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancerSvc++
		case corev1.ServiceTypeExternalName:
			result.Summary.ExternalNameSvc++
		}

		// Check endpoint health
		key := svc.Namespace + "/" + svc.Name
		if ready, ok := epMap[key]; ok {
			if ready {
				result.Summary.WithEndpoints++
			} else {
				result.Summary.WithoutEndpoints++
				score -= 2
			}
		} else {
			// No endpoint object — might be headless or ExternalName
			if svc.Spec.Type != corev1.ServiceTypeExternalName {
				result.Summary.WithoutEndpoints++
				score -= 2
			}
		}

		result.ServiceMap = append(result.ServiceMap, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services (%d ClusterIP, %d NodePort, %d LB), %d with healthy endpoints", result.Summary.TotalServices, result.Summary.ClusterIPSvc, result.Summary.NodePortSvc, result.Summary.LoadBalancerSvc, result.Summary.WithEndpoints))
	if result.Summary.WithoutEndpoints > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services without ready endpoints — check pod health", result.Summary.WithoutEndpoints))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// subCountAddr counts total addresses across all subsets
func subCountAddr(ep corev1.Endpoints) []corev1.EndpointAddress {
	var addrs []corev1.EndpointAddress
	for _, sub := range ep.Subsets {
		addrs = append(addrs, sub.Addresses...)
	}
	return addrs
}
