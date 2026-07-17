package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceCatalogResult provides a comprehensive catalog of all Services in
// the cluster: type, ports, backends, health, exposure, and discovery status.
// Useful for understanding what's running and how services connect.
type ServiceCatalogResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SvcCatalogSummary   `json:"summary"`
	Services        []SvcCatalogEntry   `json:"services"`
	ByType          []SvcCatalogType    `json:"byType"`
	ExposedExternal []SvcCatalogExposed `json:"exposedExternal"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type SvcCatalogSummary struct {
	TotalServices int `json:"totalServices"`
	ClusterIP     int `json:"clusterIP"`
	NodePort      int `json:"nodePort"`
	LoadBalancer  int `json:"loadBalancer"`
	ExternalName  int `json:"externalName"`
	Headless      int `json:"headless"`
	WithEndpoints int `json:"withEndpoints"`
	NoEndpoints   int `json:"noEndpoints"`
	WithSelector  int `json:"withSelector"`
	NoSelector    int `json:"noSelector"`
	MultiPort     int `json:"multiPort"`
}

type SvcCatalogEntry struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	Type          string    `json:"type"`
	ClusterIP     string    `json:"clusterIP"`
	ExternalIPs   []string  `json:"externalIPs"`
	Ports         []SvcPort `json:"ports"`
	HasSelector   bool      `json:"hasSelector"`
	HasEndpoints  bool      `json:"hasEndpoints"`
	EndpointCount int       `json:"endpointCount"`
	Age           string    `json:"age"`
	Healthy       bool      `json:"healthy"`
}

type SvcPort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"nodePort"`
}

type SvcCatalogType struct {
	Type    string `json:"type"`
	Count   int    `json:"count"`
	Healthy int    `json:"healthy"`
}

type SvcCatalogExposed struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	IPAddress string `json:"ipAddress"`
	Ports     string `json:"ports"`
	Risk      string `json:"risk"`
}

// handleServiceCatalog handles GET /api/product/service-catalog
func (s *Server) handleServiceCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ServiceCatalogResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})

	// Build endpoints map: namespace/name -> ready count
	epMap := make(map[string]int)
	for _, ep := range endpoints.Items {
		ready := 0
		for _, sub := range ep.Subsets {
			ready += len(sub.Addresses)
		}
		epMap[ep.Namespace+"/"+ep.Name] = ready
	}

	typeMap := make(map[string]*SvcCatalogType)

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}

		entry := SvcCatalogEntry{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			Age:       svcAge(svc.CreationTimestamp.Time),
		}

		if svc.Spec.ClusterIP == "None" {
			entry.Type = "Headless"
		}

		entry.HasSelector = len(svc.Spec.Selector) > 0
		epCount := epMap[svc.Namespace+"/"+svc.Name]
		entry.EndpointCount = epCount
		entry.HasEndpoints = epCount > 0
		entry.Healthy = !entry.HasSelector || epCount > 0 // no selector = external/managed, ok

		for _, p := range svc.Spec.Ports {
			entry.Ports = append(entry.Ports, SvcPort{
				Name: p.Name, Port: p.Port,
				TargetPort: p.TargetPort.String(),
				Protocol:   string(p.Protocol), NodePort: p.NodePort,
			})
		}

		// Summary
		result.Summary.TotalServices++
		switch entry.Type {
		case "ClusterIP":
			result.Summary.ClusterIP++
		case "NodePort":
			result.Summary.NodePort++
		case "LoadBalancer":
			result.Summary.LoadBalancer++
		case "ExternalName":
			result.Summary.ExternalName++
		case "Headless":
			result.Summary.Headless++
		}
		if entry.HasEndpoints {
			result.Summary.WithEndpoints++
		} else {
			result.Summary.NoEndpoints++
		}
		if entry.HasSelector {
			result.Summary.WithSelector++
		} else {
			result.Summary.NoSelector++
		}
		if len(svc.Spec.Ports) > 1 {
			result.Summary.MultiPort++
		}

		// External exposure
		if entry.Type == "NodePort" || entry.Type == "LoadBalancer" || len(svc.Spec.ExternalIPs) > 0 {
			ip := svc.Spec.ClusterIP
			if entry.Type == "LoadBalancer" && len(svc.Status.LoadBalancer.Ingress) > 0 {
				if svc.Status.LoadBalancer.Ingress[0].IP != "" {
					ip = svc.Status.LoadBalancer.Ingress[0].IP
				} else {
					ip = svc.Status.LoadBalancer.Ingress[0].Hostname
				}
			}
			portStrs := []string{}
			for _, p := range entry.Ports {
				if p.NodePort > 0 {
					portStrs = append(portStrs, fmt.Sprintf("%d:%d", p.Port, p.NodePort))
				} else {
					portStrs = append(portStrs, fmt.Sprintf("%d", p.Port))
				}
			}
			risk := "low"
			if entry.Type == "LoadBalancer" {
				risk = "high"
			} else if entry.Type == "NodePort" {
				risk = "medium"
			}
			result.ExposedExternal = append(result.ExposedExternal, SvcCatalogExposed{
				Name: svc.Name, Namespace: svc.Namespace,
				Type: entry.Type, IPAddress: ip,
				Ports: joinStrs(portStrs, ", "), Risk: risk,
			})
		}

		// Type map
		if _, ok := typeMap[entry.Type]; !ok {
			typeMap[entry.Type] = &SvcCatalogType{Type: entry.Type}
		}
		typeMap[entry.Type].Count++
		if entry.Healthy {
			typeMap[entry.Type].Healthy++
		}

		result.Services = append(result.Services, entry)
	}

	for _, t := range typeMap {
		result.ByType = append(result.ByType, *t)
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// Score
	if result.Summary.TotalServices > 0 {
		unhealthy := result.Summary.NoEndpoints - result.Summary.NoSelector
		result.HealthScore = (result.Summary.TotalServices - unhealthy) * 100 / result.Summary.TotalServices
	}
	switch {
	case result.HealthScore >= 85:
		result.Grade = "A"
	case result.HealthScore >= 70:
		result.Grade = "B"
	case result.HealthScore >= 55:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildSvcCatalogRecs(&result)
	sort.Slice(result.Services, func(i, j int) bool {
		return !result.Services[i].Healthy && result.Services[j].Healthy
	})

	writeJSON(w, result)
}

func buildSvcCatalogRecs(r *ServiceCatalogResult) []string {
	recs := []string{}
	if r.Summary.NoEndpoints > 0 && r.Summary.WithSelector > 0 {
		recs = append(recs, fmt.Sprintf("%d 个有 selector 的 Service 没有就绪端点", r.Summary.NoEndpoints))
	}
	if r.Summary.LoadBalancer > 5 {
		recs = append(recs, fmt.Sprintf("%d 个 LoadBalancer 可能产生额外费用", r.Summary.LoadBalancer))
	}
	if r.Summary.NoSelector > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Service 没有 selector（手动管理端点）", r.Summary.NoSelector))
	}
	if len(recs) == 0 {
		recs = append(recs, "服务目录健康，所有有 selector 的 Service 都有就绪端点")
	}
	return recs
}

func svcAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	if d > 720*time.Hour {
		return fmt.Sprintf("%.0fd", d.Hours()/24)
	}
	if d > 24*time.Hour {
		return fmt.Sprintf("%.0fd", d.Hours()/24)
	}
	return fmt.Sprintf("%.0fh", d.Hours())
}

func joinStrs(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}

var _ corev1.ServiceType = ""
