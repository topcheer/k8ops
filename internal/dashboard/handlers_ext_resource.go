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

// ExtResourceResult is the extended resource & device plugin health audit.
type ExtResourceResult struct {
	Timestamp         time.Time          `json:"timestamp"`
	Score             int                `json:"score"`
	Status            string             `json:"status"`
	Summary           ExtResourceSummary `json:"summary"`
	ExtendedResources []ExtResourceInfo  `json:"extendedResources"`
	DevicePlugins     []DevicePluginInfo `json:"devicePlugins"`
	NodeAllocation    []NodeExtAlloc     `json:"nodeAllocation"`
	GpuHealth         []GpuNodeInfo      `json:"gpuHealth"`
	Issues            []ExtResourceIssue `json:"issues"`
	Recommendations   []string           `json:"recommendations"`
}

// ExtResourceSummary holds aggregate extended resource metrics.
type ExtResourceSummary struct {
	TotalExtendedResources int `json:"totalExtendedResources"`
	NodesWithDevices       int `json:"nodesWithDevices"`
	TotalDevicePlugins     int `json:"totalDevicePlugins"`
	HealthyDevicePlugins   int `json:"healthyDevicePlugins"`
	GpuNodes               int `json:"gpuNodes"`
	TotalResourceTypes     int `json:"totalResourceTypes"`
	IssueCount             int `json:"issueCount"`
}

// ExtResourceInfo describes an extended resource type.
type ExtResourceInfo struct {
	Name           string  `json:"name"`
	Capacity       int64   `json:"capacity"`
	Allocatable    int64   `json:"allocatable"`
	Allocated      int64   `json:"allocated"`
	Available      int64   `json:"available"`
	NodeCount      int     `json:"nodeCount"`
	UtilizationPct float64 `json:"utilizationPct"`
}

// DevicePluginInfo describes a device plugin DaemonSet or pod.
type DevicePluginInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Ready     bool   `json:"ready"`
	NodeName  string `json:"nodeName"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restarts"`
}

// NodeExtAlloc shows extended resource allocation per node.
type NodeExtAlloc struct {
	Node      string                    `json:"node"`
	Resources map[string]ExtAllocDetail `json:"resources"`
}

// ExtAllocDetail shows capacity/allocated for one resource on one node.
type ExtAllocDetail struct {
	Capacity  int64 `json:"capacity"`
	Allocated int64 `json:"allocated"`
	Available int64 `json:"available"`
}

// GpuNodeInfo shows GPU status per node.
type GpuNodeInfo struct {
	Node      string `json:"node"`
	GpuCount  int64  `json:"gpuCount"`
	Model     string `json:"model"`
	DriverVer string `json:"driverVersion"`
	Allocated int64  `json:"allocated"`
	Available int64  `json:"available"`
	Healthy   bool   `json:"healthy"`
}

// ExtResourceIssue identifies a device plugin or extended resource issue.
type ExtResourceIssue struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Node     string `json:"node"`
	Message  string `json:"message"`
}

func (s *Server) handleExtResourceHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("kube-system").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	result := analyzeExtResourceHealth(nodes.Items, pods.Items)
	writeJSON(w, result)
}

func analyzeExtResourceHealth(nodes []corev1.Node, kubeSystemPods []corev1.Pod) ExtResourceResult {
	now := time.Now()

	// Collect all extended resources across nodes
	resourceMap := make(map[string]*ExtResourceInfo) // resource name -> aggregate
	resourceNodeCount := make(map[string]int)
	var nodeAllocs []NodeExtAlloc
	var gpuNodes []GpuNodeInfo
	var issues []ExtResourceIssue
	nodesWithDevices := 0

	// Device plugin pods
	var devicePlugins []DevicePluginInfo

	for _, pod := range kubeSystemPods {
		nameLower := strings.ToLower(pod.Name)
		if strings.Contains(nameLower, "device-plugin") || strings.Contains(nameLower, "gpu") ||
			strings.Contains(nameLower, "nvidia") || strings.Contains(nameLower, "amd") ||
			strings.Contains(nameLower, "intel") || strings.Contains(nameLower, "habana") {
			dp := DevicePluginInfo{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Kind:      "Pod",
				NodeName:  pod.Spec.NodeName,
			}
			dp.Ready = pod.Status.Phase == corev1.PodRunning
			if dp.Ready {
				dp.Status = "Running"
			} else {
				dp.Status = string(pod.Status.Phase)
			}
			for _, cs := range pod.Status.ContainerStatuses {
				dp.Restarts = cs.RestartCount
			}
			devicePlugins = append(devicePlugins, dp)

			if !dp.Ready {
				issues = append(issues, ExtResourceIssue{
					Type:     "DevicePluginNotRunning",
					Severity: "high",
					Node:     dp.NodeName,
					Message:  fmt.Sprintf("Device plugin %s on node %s is %s", dp.Name, dp.NodeName, dp.Status),
				})
			}
			if dp.Restarts > 3 {
				issues = append(issues, ExtResourceIssue{
					Type:     "DevicePluginCrashLoop",
					Severity: "medium",
					Node:     dp.NodeName,
					Message:  fmt.Sprintf("Device plugin %s has restarted %d times", dp.Name, dp.Restarts),
				})
			}
		}
	}

	for _, node := range nodes {
		alloc := NodeExtAlloc{Node: node.Name, Resources: make(map[string]ExtAllocDetail)}
		hasExtResource := false

		// Check capacity for extended resources
		for resName, capacity := range node.Status.Capacity {
			if isExtendedResource(string(resName)) {
				hasExtResource = true
				allocatable := node.Status.Allocatable[resName]
				allocated := capacity.DeepCopy()
				allocated.Sub(allocatable.DeepCopy())

				capInt := capacity.Value()
				allocInt := allocatable.Value()
				allocDetail := ExtAllocDetail{
					Capacity:  capInt,
					Available: allocInt,
					Allocated: capInt - allocInt,
				}
				alloc.Resources[string(resName)] = allocDetail

				// Aggregate
				if ri, ok := resourceMap[string(resName)]; ok {
					ri.Capacity += capInt
					ri.Allocatable += allocInt
					ri.Allocated += capInt - allocInt
					ri.Available += allocInt
					ri.NodeCount++
				} else {
					resourceMap[string(resName)] = &ExtResourceInfo{
						Name:        string(resName),
						Capacity:    capInt,
						Allocatable: allocInt,
						Allocated:   capInt - allocInt,
						Available:   allocInt,
						NodeCount:   1,
					}
				}
				resourceNodeCount[string(resName)]++

				// GPU specific info
				if strings.Contains(string(resName), "nvidia.com/gpu") || strings.Contains(string(resName), "amd.com/gpu") {
					gpuInfo := GpuNodeInfo{
						Node:      node.Name,
						GpuCount:  capInt,
						Allocated: capInt - allocInt,
						Available: allocInt,
						Healthy:   true,
					}
					// Try to get GPU model from labels
					if model, ok := node.Labels["nvidia.com/gpu.product"]; ok {
						gpuInfo.Model = model
					}
					if drv, ok := node.Labels["nvidia.com/gpu.driver_version"]; ok {
						gpuInfo.DriverVer = drv
					}
					if allocInt == 0 && capInt > 0 {
						gpuInfo.Healthy = false
						issues = append(issues, ExtResourceIssue{
							Type:     "GPUFullyAllocated",
							Severity: "medium",
							Node:     node.Name,
							Message:  fmt.Sprintf("All %d GPU(s) on node %s are allocated; no GPU capacity available", capInt, node.Name),
						})
					}
					gpuNodes = append(gpuNodes, gpuInfo)
				}
			}
		}

		if hasExtResource {
			nodesWithDevices++
		}
		nodeAllocs = append(nodeAllocs, alloc)
	}

	// Calculate utilization for each resource
	var extResources []ExtResourceInfo
	for _, ri := range resourceMap {
		if ri.Capacity > 0 {
			ri.UtilizationPct = float64(ri.Allocated) / float64(ri.Capacity) * 100
		}
		extResources = append(extResources, *ri)
	}
	sort.Slice(extResources, func(i, j int) bool {
		return extResources[i].UtilizationPct > extResources[j].UtilizationPct
	})

	// Summary
	healthyDPs := 0
	for _, dp := range devicePlugins {
		if dp.Ready {
			healthyDPs++
		}
	}

	summary := ExtResourceSummary{
		TotalExtendedResources: len(extResources),
		NodesWithDevices:       nodesWithDevices,
		TotalDevicePlugins:     len(devicePlugins),
		HealthyDevicePlugins:   healthyDPs,
		GpuNodes:               len(gpuNodes),
		TotalResourceTypes:     len(extResources),
		IssueCount:             len(issues),
	}

	// Score
	score := 100
	if len(devicePlugins) > 0 {
		unhealthyRatio := float64(len(devicePlugins)-healthyDPs) / float64(len(devicePlugins))
		score -= int(unhealthyRatio * 30)
	}
	score -= len(issues) * 5
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if len(extResources) == 0 {
		recs = append(recs, "No extended resources detected; cluster has no GPU or specialized hardware registered")
	} else {
		for _, r := range extResources {
			if r.UtilizationPct > 80 {
				recs = append(recs, fmt.Sprintf("%s is %.0f%% allocated (%d/%d); consider adding more nodes with this resource", r.Name, r.UtilizationPct, r.Allocated, r.Capacity))
			}
		}
	}
	if len(devicePlugins) > healthyDPs {
		recs = append(recs, fmt.Sprintf("%d/%d device plugins are unhealthy; check pod logs and node socket directories", len(devicePlugins)-healthyDPs, len(devicePlugins)))
	}
	if len(gpuNodes) > 0 {
		fullyAllocatedGPUs := 0
		for _, g := range gpuNodes {
			if g.Available == 0 {
				fullyAllocatedGPUs++
			}
		}
		if fullyAllocatedGPUs > 0 {
			recs = append(recs, fmt.Sprintf("%d GPU node(s) fully allocated; schedule GPU workloads on other nodes or add capacity", fullyAllocatedGPUs))
		}
	}
	if len(recs) == 0 {
		recs = append(recs, "Extended resource health and device plugin status look good")
	}

	return ExtResourceResult{
		Timestamp:         now,
		Score:             score,
		Status:            status,
		Summary:           summary,
		ExtendedResources: extResources,
		DevicePlugins:     devicePlugins,
		NodeAllocation:    nodeAllocs,
		GpuHealth:         gpuNodes,
		Issues:            issues,
		Recommendations:   recs,
	}
}

// isExtendedResource checks if a resource name is an extended resource (not cpu/memory/pod/ephemeral-storage).
func isExtendedResource(name string) bool {
	switch name {
	case "cpu", "memory", "pods", "ephemeral-storage", "hugepages-2Mi", "hugepages-1Gi":
		return false
	default:
		// Extended resources typically have a domain prefix (e.g. nvidia.com/gpu)
		return strings.Contains(name, "/")
	}
}
