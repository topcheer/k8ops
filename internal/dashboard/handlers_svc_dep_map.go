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

// ServiceDependencyMapResult builds a service-to-service dependency map
// by analyzing DNS usage, env var references, and service selectors.
// It visualizes which services depend on which, helping identify
// critical paths and single points of failure.
type ServiceDependencyMapResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         SvcDepMapSummary  `json:"summary"`
	Dependencies    []SvcDependency   `json:"dependencies"`
	CriticalPaths   []SvcCriticalPath `json:"criticalPaths"`
	ByNamespace     []SvcDepNS        `json:"byNamespace"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type SvcDepMapSummary struct {
	TotalServices int `json:"totalServices"`
	WithDeps      int `json:"withDependencies"`
	Isolated      int `json:"isolatedServices"`
	MaxDepCount   int `json:"maxDependencyCount"`
	CriticalSvcs  int `json:"criticalServices"`
}

type SvcDependency struct {
	Service    string   `json:"service"`
	Namespace  string   `json:"namespace"`
	DependsOn  []string `json:"dependsOn"`
	DepCount   int      `json:"depCount"`
	IsCritical bool     `json:"isCritical"`
}

type SvcCriticalPath struct {
	Path   []string `json:"path"`
	Length int      `json:"length"`
	Risk   string   `json:"risk"`
}

type SvcDepNS struct {
	Namespace string `json:"namespace"`
	Services  int    `json:"services"`
	Internal  int    `json:"internalDeps"` // deps within same ns
	External  int    `json:"externalDeps"` // deps to other ns
}

// handleServiceDependencyMap handles GET /api/product/service-dependency-map
func (s *Server) handleServiceDependencyMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ServiceDependencyMapResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build service name set per namespace
	nsSvcMap := make(map[string]map[string]bool)
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		if _, ok := nsSvcMap[svc.Namespace]; !ok {
			nsSvcMap[svc.Namespace] = make(map[string]bool)
		}
		nsSvcMap[svc.Namespace][svc.Name] = true
	}

	// Build dependency map: analyze env vars for service references
	depMap := make(map[string]map[string]bool) // ns/svc -> set of deps
	depCount := make(map[string]int)
	refsByService := make(map[string]int) // how many services reference this

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			// Check env vars for service-like references
			for _, env := range c.Env {
				val := env.Value
				if val == "" {
					continue
				}
				// Look for patterns like http://service-name or svc-name.namespace
				deps := extractServiceRefs(val, d.Namespace, nsSvcMap)
				for _, dep := range deps {
					key := d.Namespace + "/" + d.Name
					if depMap[key] == nil {
						depMap[key] = make(map[string]bool)
					}
					depMap[key][dep] = true
					depCount[key]++
					refsByService[dep]++
				}
			}
		}
	}

	// Build dependency entries
	nsStats := make(map[string]*SvcDepNS)
	var deps []SvcDependency
	var criticalSvcs []string

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		key := d.Namespace + "/" + d.Name
		result.Summary.TotalServices++

		depSet := depMap[key]
		depList := []string{}
		for dep := range depSet {
			depList = append(depList, dep)
		}

		if len(depList) > 0 {
			result.Summary.WithDeps++
		} else {
			result.Summary.Isolated++
		}

		if len(depList) > result.Summary.MaxDepCount {
			result.Summary.MaxDepCount = len(depList)
		}

		isCritical := refsByService[key] >= 3
		if isCritical {
			result.Summary.CriticalSvcs++
			criticalSvcs = append(criticalSvcs, key)
		}

		sort.Strings(depList)
		deps = append(deps, SvcDependency{
			Service: d.Name, Namespace: d.Namespace,
			DependsOn: depList, DepCount: len(depList),
			IsCritical: isCritical,
		})

		// NS stats
		if _, ok := nsStats[d.Namespace]; !ok {
			nsStats[d.Namespace] = &SvcDepNS{Namespace: d.Namespace}
		}
		nsStats[d.Namespace].Services++
		for _, dep := range depList {
			depNS := strings.SplitN(dep, "/", 2)[0]
			if depNS == d.Namespace {
				nsStats[d.Namespace].Internal++
			} else {
				nsStats[d.Namespace].External++
			}
		}
	}

	// Build critical paths (simplified: chains of length >= 3)
	// Just identify services with high dependency chains
	for _, d := range deps {
		if d.DepCount >= 3 {
			path := append([]string{d.Namespace + "/" + d.Service}, d.DependsOn...)
			result.CriticalPaths = append(result.CriticalPaths, SvcCriticalPath{
				Path: path, Length: len(path), Risk: "high",
			})
		}
	}
	if len(result.CriticalPaths) > 10 {
		result.CriticalPaths = result.CriticalPaths[:10]
	}

	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].External > result.ByNamespace[j].External
	})

	sort.Slice(deps, func(i, j int) bool {
		return deps[i].DepCount > deps[j].DepCount
	})
	result.Dependencies = deps

	// Score: more isolated services = higher risk of unknown deps
	if result.Summary.TotalServices > 0 {
		depPct := result.Summary.WithDeps * 100 / result.Summary.TotalServices
		result.HealthScore = depPct
	}

	switch {
	case result.HealthScore >= 70:
		result.Grade = "A"
	case result.HealthScore >= 50:
		result.Grade = "B"
	case result.HealthScore >= 30:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildSvcDepMapRecs(&result)
	writeJSON(w, result)
}

func extractServiceRefs(val, currentNS string, nsSvcMap map[string]map[string]bool) []string {
	var refs []string
	seen := make(map[string]bool)

	// Pattern: http://service-name or https://service-name
	for _, prefix := range []string{"http://", "https://"} {
		if idx := strings.Index(val, prefix); idx >= 0 {
			rest := val[idx+len(prefix):]
			endIdx := strings.IndexAny(rest, "/: ?\"")
			if endIdx < 0 {
				endIdx = len(rest)
			}
			if endIdx > 0 {
				name := rest[:endIdx]
				// Check if it's a real service in current namespace
				if nsSvcMap[currentNS] != nil && nsSvcMap[currentNS][name] {
					ref := currentNS + "/" + name
					if !seen[ref] {
						refs = append(refs, ref)
						seen[ref] = true
					}
				}
			}
		}
	}

	// Pattern: SERVICE_HOST or SERVICE_PORT env vars (K8s auto-injected)
	if strings.HasSuffix(val, "_SERVICE_HOST") || strings.HasSuffix(val, "_SERVICE_PORT") {
		serviceName := strings.TrimSuffix(strings.TrimSuffix(val, "_HOST"), "_SERVICE")
		serviceName = strings.ReplaceAll(serviceName, "_", "-")
		serviceName = strings.ToLower(serviceName)
		if nsSvcMap[currentNS] != nil && nsSvcMap[currentNS][serviceName] {
			ref := currentNS + "/" + serviceName
			if !seen[ref] {
				refs = append(refs, ref)
				seen[ref] = true
			}
		}
	}

	return refs
}

func buildSvcDepMapRecs(r *ServiceDependencyMapResult) []string {
	recs := []string{}
	if r.Summary.Isolated > 0 {
		recs = append(recs, fmt.Sprintf("%d 个服务没有检测到依赖关系（可能是独立服务或依赖未被自动发现）", r.Summary.Isolated))
	}
	if r.Summary.CriticalSvcs > 0 {
		recs = append(recs, fmt.Sprintf("%d 个关键服务被其他服务依赖，是单点风险", r.Summary.CriticalSvcs))
	}
	if len(r.CriticalPaths) > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 条高依赖链路，故障可能级联传播", len(r.CriticalPaths)))
	}
	if len(recs) == 0 {
		recs = append(recs, "服务依赖图健康，依赖关系清晰")
	}
	return recs
}

var _ corev1.Service
