package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SvcMeshReadinessResult evaluates service mesh adoption readiness
// based on traffic patterns, protocol compatibility, and observability.
type SvcMeshReadinessResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         SvcMeshReadinessSummary `json:"summary"`
	ByService       []SvcMeshEntry          `json:"byService"`
	ReadyServices   []SvcMeshEntry          `json:"readyServices"`
	ReadinessScore  int                     `json:"readinessScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type SvcMeshReadinessSummary struct {
	TotalServices  int `json:"totalServices"`
	MeshReady      int `json:"meshReady"`
	HasProbes      int `json:"hasProbes"`
	HasMetrics     int `json:"hasMetrics"`
	HTTPTraffic    int `json:"httpTraffic"`
	TCPTraffic     int `json:"tcpTraffic"`
	UDPTraffic     int `json:"udpTraffic"`
	BlockingIssues int `json:"blockingIssues"`
}

type SvcMeshEntry struct {
	ServiceName  string   `json:"serviceName"`
	Namespace    string   `json:"namespace"`
	Ports        int      `json:"portCount"`
	Protocols    []string `json:"protocols"`
	HasProbe     bool     `json:"hasProbe"`
	BackingPods  int      `json:"backingPods"`
	MeshReady    bool     `json:"meshReady"`
	Blockers     []string `json:"blockers"`
	ReadinessPct int      `json:"readinessPct"`
}

// handleSvcMeshReadiness handles GET /api/product/svc-mesh-readiness
func (s *Server) handleSvcMeshReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SvcMeshReadinessResult{ScannedAt: time.Now()}
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	depHasProbe := make(map[string]bool)
	for _, d := range deployments.Items {
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				depHasProbe[d.Namespace+"/"+d.Name] = true
				break
			}
		}
	}

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		if svc.Spec.ClusterIP == "None" && svc.Spec.ClusterIPs == nil {
			continue
		}

		result.Summary.TotalServices++
		entry := SvcMeshEntry{ServiceName: svc.Name, Namespace: svc.Namespace, Ports: len(svc.Spec.Ports)}

		for _, port := range svc.Spec.Ports {
			entry.Protocols = append(entry.Protocols, string(port.Protocol))
			switch port.Protocol {
			case corev1.ProtocolTCP:
				result.Summary.HTTPTraffic++ // TCP is mesh-compatible
			case corev1.ProtocolUDP:
				result.Summary.UDPTraffic++
			}
		}

		// Check backing pods
		for _, pod := range pods.Items {
			if pod.Namespace != svc.Namespace || pod.Status.Phase != corev1.PodRunning {
				continue
			}
			if svc.Spec.Selector == nil {
				continue
			}
			match := true
			for k, v := range svc.Spec.Selector {
				if pod.Labels[k] != v {
					match = false
					break
				}
			}
			if match {
				entry.BackingPods++
			}
		}

		// Check probes
		entry.HasProbe = depHasProbe[svc.Namespace+"/"+svc.Name]
		if entry.HasProbe {
			result.Summary.HasProbes++
		}

		// Mesh readiness checks
		checks := 4
		passed := 0
		if entry.HasProbe {
			passed++
		} else {
			entry.Blockers = append(entry.Blockers, "no-readiness-probe")
		}
		if entry.BackingPods >= 2 {
			passed++
		} else {
			entry.Blockers = append(entry.Blockers, "single-replica")
		}
		// TCP is mesh-friendly
		hasTCP := false
		for _, p := range entry.Protocols {
			if p == "TCP" {
				hasTCP = true
			}
		}
		if hasTCP {
			passed++
		} else {
			entry.Blockers = append(entry.Blockers, "non-tcp-protocol")
		}
		// Has selector (mesh needs selector for service discovery)
		if len(svc.Spec.Selector) > 0 {
			passed++
		} else {
			entry.Blockers = append(entry.Blockers, "no-selector")
		}

		entry.ReadinessPct = passed * 100 / checks
		entry.MeshReady = len(entry.Blockers) == 0
		if entry.MeshReady {
			result.Summary.MeshReady++
		} else {
			result.Summary.BlockingIssues++
		}

		result.ByService = append(result.ByService, entry)
	}

	sort.Slice(result.ByService, func(i, j int) bool {
		return result.ByService[i].ReadinessPct < result.ByService[j].ReadinessPct
	})

	for _, e := range result.ByService {
		if e.MeshReady {
			result.ReadyServices = append(result.ReadyServices, e)
		}
	}

	if result.Summary.TotalServices > 0 {
		result.ReadinessScore = result.Summary.MeshReady * 100 / result.Summary.TotalServices
	}
	gradeFromScore(&result.Grade, result.ReadinessScore)

	result.Recommendations = []string{
		fmt.Sprintf("Mesh 就绪: %d/%d 服务 (%d%%), %d 阻塞项", result.Summary.MeshReady, result.Summary.TotalServices, result.ReadinessScore, result.Summary.BlockingIssues),
	}
	if result.Summary.UDPTraffic > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 UDP 端口不完全兼容 L7 Mesh", result.Summary.UDPTraffic))
	}
	if result.ReadinessScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 为所有服务添加 readiness probe, 确保多副本")
	}
	writeJSON(w, result)
}
