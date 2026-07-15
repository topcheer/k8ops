package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PortExposureResult is the container port exposure & named port consistency audit.
type PortExposureResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PortExposureSummary `json:"summary"`
	ByWorkload      []PortWorkloadEntry `json:"byWorkload"`
	Conflicts       []PortConflict      `json:"conflicts"`
	Risks           []PortRisk          `json:"risks"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// PortExposureSummary aggregates port exposure metrics.
type PortExposureSummary struct {
	TotalContainers   int `json:"totalContainers"`
	WithNamedPorts    int `json:"withNamedPorts"`   // containers using named ports
	WithUnnamedPorts  int `json:"withUnnamedPorts"` // containers with unnamed ports
	WithHostPort      int `json:"withHostPort"`     // containers exposing host ports
	WithHostIP        int `json:"withHostIP"`       // hostPort bound to specific IP
	TotalPortMappings int `json:"totalPortMappings"`
	Conflicts         int `json:"conflicts"`
	HostPortConflicts int `json:"hostPortConflicts"`
	SvcPortMismatch   int `json:"svcPortMismatch"` // service port not matching container port
}

// PortWorkloadEntry per-workload port info.
type PortWorkloadEntry struct {
	Name       string          `json:"name"`
	Namespace  string          `json:"namespace"`
	Kind       string          `json:"kind"`
	Containers []PortContainer `json:"containers"`
	HasIssue   bool            `json:"hasIssue"`
}

// PortContainer per-container port info.
type PortContainer struct {
	Name  string     `json:"name"`
	Ports []PortInfo `json:"ports"`
}

// PortInfo describes a single port.
type PortInfo struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
	HostPort      int32  `json:"hostPort,omitempty"`
	Protocol      string `json:"protocol"`
	HasName       bool   `json:"hasName"`
}

// PortConflict describes a port conflict.
type PortConflict struct {
	Workload1 string `json:"workload1,omitempty"`
	Workload2 string `json:"workload2,omitempty"`
	Port      int32  `json:"port"`
	Type      string `json:"type"` // host-port-conflict, same-port-different-protocol
	Severity  string `json:"severity"`
}

// PortRisk describes a port-related risk.
type PortRisk struct {
	Workload  string `json:"workload,omitempty"`
	Container string `json:"container,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handlePortExposure audits container port exposure & named port consistency.
// GET /api/product/port-exposure
func (s *Server) handlePortExposure(w http.ResponseWriter, r *http.Request) {
	result := PortExposureResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get pods
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// 2. Get services for port mismatch detection
	services, _ := rc.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	// Build service port map: namespace/serviceName → containerPort set
	svcPortMap := map[string]map[int32]bool{}
	if services != nil {
		for _, svc := range services.Items {
			key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
			svcPortMap[key] = map[int32]bool{}
			for _, p := range svc.Spec.Ports {
				if p.TargetPort.IntVal > 0 {
					svcPortMap[key][p.TargetPort.IntVal] = true
				} else if p.TargetPort.StrVal != "" {
					// Named target port — we'll match by name later
					svcPortMap[key][-1] = true // sentinel for named port
				}
			}
		}
	}

	// Track host ports for conflict detection
	hostPortMap := map[int32][]string{} // hostPort → list of workload names

	// 3. Analyze each pod's containers
	seenWorkloads := map[string]*PortWorkloadEntry{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName == "" {
			continue // pending pod
		}

		// Determine workload name from owner references
		wlName := pod.Name
		wlKind := "Pod"
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" {
				wlKind = "Deployment"
				// Strip ReplicaSet hash
				wlName = ref.Name
				if idx := strings.LastIndex(wlName, "-"); idx > 0 {
					lastSeg := wlName[idx+1:]
					if len(lastSeg) >= 5 && isAllHex(lastSeg) {
						wlName = wlName[:idx]
					}
				}
			} else if ref.Kind != "" {
				wlKind = ref.Kind
				wlName = ref.Name
			}
		}

		wlKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, wlKind, wlName)
		if seenWorkloads[wlKey] == nil {
			seenWorkloads[wlKey] = &PortWorkloadEntry{
				Name: wlName, Namespace: pod.Namespace, Kind: wlKind,
			}
		}
		entry := seenWorkloads[wlKey]

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			cEntry := PortContainer{Name: c.Name}

			hasNamedPort := false
			for _, p := range c.Ports {
				pInfo := PortInfo{
					ContainerPort: p.ContainerPort,
					Protocol:      string(p.Protocol),
					HasName:       p.Name != "",
				}
				if p.Name != "" {
					pInfo.Name = p.Name
					hasNamedPort = true
				}
				if p.HostPort > 0 {
					pInfo.HostPort = p.HostPort
					result.Summary.WithHostPort++
					hostPortMap[p.HostPort] = append(hostPortMap[p.HostPort], fmt.Sprintf("%s/%s", pod.Namespace, wlName))
				}
				result.Summary.TotalPortMappings++
				cEntry.Ports = append(cEntry.Ports, pInfo)
			}

			if hasNamedPort {
				result.Summary.WithNamedPorts++
			} else if len(c.Ports) > 0 {
				result.Summary.WithUnnamedPorts++
				entry.HasIssue = true
				result.Risks = append(result.Risks, PortRisk{
					Workload:  fmt.Sprintf("%s/%s", pod.Namespace, wlName),
					Container: c.Name,
					Issue:     "Container has unnamed ports — use named ports for better service configuration",
					Severity:  "low",
				})
			}

			entry.Containers = append(entry.Containers, cEntry)
		}
	}

	// 4. Detect host port conflicts
	for port, workloads := range hostPortMap {
		if len(workloads) > 1 {
			result.Summary.HostPortConflicts++
			result.Summary.Conflicts++
			result.Conflicts = append(result.Conflicts, PortConflict{
				Workload1: workloads[0],
				Workload2: workloads[1],
				Port:      port,
				Type:      "host-port-conflict",
				Severity:  "critical",
			})
			result.Risks = append(result.Risks, PortRisk{
				Issue:    fmt.Sprintf("Host port %d is used by multiple workloads: %s and %s", port, workloads[0], workloads[1]),
				Severity: "critical",
			})
		}
	}

	// 5. Build workload entries slice
	for _, entry := range seenWorkloads {
		result.ByWorkload = append(result.ByWorkload, *entry)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		if result.ByWorkload[i].HasIssue != result.ByWorkload[j].HasIssue {
			return result.ByWorkload[i].HasIssue
		}
		return result.ByWorkload[i].Name < result.ByWorkload[j].Name
	})

	// 6. Calculate health score
	score := 100
	if result.Summary.HostPortConflicts > 0 {
		score -= 30
	}
	if result.Summary.WithHostPort > 0 {
		score -= min(15, result.Summary.WithHostPort*3)
	}
	if result.Summary.WithUnnamedPorts > 0 {
		score -= min(10, result.Summary.WithUnnamedPorts)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	if result.Summary.HostPortConflicts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d host port conflict(s) detected — use ClusterIP/NodePort services instead of hostPort", result.Summary.HostPortConflicts))
	}
	if result.Summary.WithHostPort > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d container(s) use hostPort — avoid hostPort for portability, use NodePort or LoadBalancer service", result.Summary.WithHostPort))
	}
	if result.Summary.WithUnnamedPorts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d container(s) have unnamed ports — name ports for better service targetPort referencing", result.Summary.WithUnnamedPorts))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Container port configuration is clean — no conflicts, all ports named, no hostPort usage")
	}

	writeJSON(w, result)
}

// isAllHex checks if a string is all hex characters.
func isAllHex(s string) bool {
	if len(s) < 5 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
