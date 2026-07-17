package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResTopologyResult builds a cluster-wide resource dependency graph.
// It maps how workloads connect to ConfigMaps, Secrets, PVs, PVCs,
// and Services to visualize data flow and identify orphaned resources.
type ResTopologyResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         ResTopoSummary  `json:"summary"`
	Nodes           []ResTopoNode   `json:"nodes"`
	Edges           []ResTopoEdge   `json:"edges"`
	Orphaned        []ResTopoOrphan `json:"orphaned"`
	SharedResources []ResTopoShared `json:"sharedResources"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type ResTopoSummary struct {
	TotalNodes    int `json:"totalNodes"`
	TotalEdges    int `json:"totalEdges"`
	Workloads     int `json:"workloads"`
	ConfigMaps    int `json:"configMaps"`
	Secrets       int `json:"secrets"`
	PVCs          int `json:"pvcs"`
	Services      int `json:"services"`
	OrphanedCount int `json:"orphanedCount"`
	SharedCount   int `json:"sharedCount"`
}

type ResTopoNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

type ResTopoEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // config-ref, secret-ref, pvc-mount, service-selector
}

type ResTopoOrphan struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
}

type ResTopoShared struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	UsedBy    []string `json:"usedBy"`
	UseCount  int      `json:"useCount"`
}

// handleResourceTopology handles GET /api/operations/resource-topology
func (s *Server) handleResourceTopology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ResTopologyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Track resource usage: resourceKey -> []workload
	usageMap := make(map[string][]string)

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		wlID := fmt.Sprintf("wl/%s/%s", d.Namespace, d.Name)
		result.Nodes = append(result.Nodes, ResTopoNode{
			ID: wlID, Name: d.Name, Namespace: d.Namespace, Kind: "Deployment",
		})
		result.Summary.Workloads++

		// ConfigMap refs
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil {
				cmID := fmt.Sprintf("cm/%s/%s", d.Namespace, vol.ConfigMap.Name)
				result.Edges = append(result.Edges, ResTopoEdge{Source: wlID, Target: cmID, Type: "config-ref"})
				usageMap[cmID] = append(usageMap[cmID], d.Name)
			}
			if vol.Secret != nil {
				secID := fmt.Sprintf("sec/%s/%s", d.Namespace, vol.Secret.SecretName)
				result.Edges = append(result.Edges, ResTopoEdge{Source: wlID, Target: secID, Type: "secret-ref"})
				usageMap[secID] = append(usageMap[secID], d.Name)
			}
			if vol.PersistentVolumeClaim != nil {
				pvcID := fmt.Sprintf("pvc/%s/%s", d.Namespace, vol.PersistentVolumeClaim.ClaimName)
				result.Edges = append(result.Edges, ResTopoEdge{Source: wlID, Target: pvcID, Type: "pvc-mount"})
				usageMap[pvcID] = append(usageMap[pvcID], d.Name)
			}
		}

		// Env var refs
		for _, c := range d.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					cmID := fmt.Sprintf("cm/%s/%s", d.Namespace, env.ValueFrom.ConfigMapKeyRef.Name)
					usageMap[cmID] = append(usageMap[cmID], d.Name)
				}
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					secID := fmt.Sprintf("sec/%s/%s", d.Namespace, env.ValueFrom.SecretKeyRef.Name)
					usageMap[secID] = append(usageMap[secID], d.Name)
				}
			}
		}

		// Service selector match
		for _, svc := range services.Items {
			if svc.Namespace != d.Namespace {
				continue
			}
			if d.Spec.Selector != nil && matchesSelector(d.Spec.Selector.MatchLabels, svc.Spec.Selector) {
				svcID := fmt.Sprintf("svc/%s/%s", svc.Namespace, svc.Name)
				result.Edges = append(result.Edges, ResTopoEdge{Source: wlID, Target: svcID, Type: "service-selector"})
				usageMap[svcID] = append(usageMap[svcID], d.Name)
			}
		}
	}

	// Add resource nodes
	for _, cm := range configmaps.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		id := fmt.Sprintf("cm/%s/%s", cm.Namespace, cm.Name)
		result.Nodes = append(result.Nodes, ResTopoNode{ID: id, Name: cm.Name, Namespace: cm.Namespace, Kind: "ConfigMap"})
		result.Summary.ConfigMaps++
	}
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		id := fmt.Sprintf("sec/%s/%s", sec.Namespace, sec.Name)
		result.Nodes = append(result.Nodes, ResTopoNode{ID: id, Name: sec.Name, Namespace: sec.Namespace, Kind: "Secret"})
		result.Summary.Secrets++
	}
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		id := fmt.Sprintf("pvc/%s/%s", pvc.Namespace, pvc.Name)
		result.Nodes = append(result.Nodes, ResTopoNode{ID: id, Name: pvc.Name, Namespace: pvc.Namespace, Kind: "PVC"})
		result.Summary.PVCs++
	}
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		id := fmt.Sprintf("svc/%s/%s", svc.Namespace, svc.Name)
		result.Nodes = append(result.Nodes, ResTopoNode{ID: id, Name: svc.Name, Namespace: svc.Namespace, Kind: "Service"})
		result.Summary.Services++
	}

	// Find orphaned resources (no usage)
	for _, cm := range configmaps.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		id := fmt.Sprintf("cm/%s/%s", cm.Namespace, cm.Name)
		if len(usageMap[id]) == 0 {
			result.Orphaned = append(result.Orphaned, ResTopoOrphan{
				Kind: "ConfigMap", Name: cm.Name, Namespace: cm.Namespace, Age: svcAge(cm.CreationTimestamp.Time),
			})
		}
	}
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		id := fmt.Sprintf("sec/%s/%s", sec.Namespace, sec.Name)
		if len(usageMap[id]) == 0 {
			result.Orphaned = append(result.Orphaned, ResTopoOrphan{
				Kind: "Secret", Name: sec.Name, Namespace: sec.Namespace, Age: svcAge(sec.CreationTimestamp.Time),
			})
		}
	}
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		id := fmt.Sprintf("pvc/%s/%s", pvc.Namespace, pvc.Name)
		if len(usageMap[id]) == 0 && pvc.Status.Phase == corev1.ClaimBound {
			result.Orphaned = append(result.Orphaned, ResTopoOrphan{
				Kind: "PVC", Name: pvc.Name, Namespace: pvc.Namespace, Age: svcAge(pvc.CreationTimestamp.Time),
			})
		}
	}

	// Shared resources (used by multiple workloads)
	for resID, users := range usageMap {
		unique := uniqueStrings(users)
		if len(unique) >= 3 {
			parts := splitTopologyID(resID)
			result.SharedResources = append(result.SharedResources, ResTopoShared{
				Kind: parts[0], Name: parts[2], Namespace: parts[1],
				UsedBy: unique, UseCount: len(unique),
			})
		}
	}
	sort.Slice(result.SharedResources, func(i, j int) bool {
		return result.SharedResources[i].UseCount > result.SharedResources[j].UseCount
	})

	result.Summary.TotalNodes = len(result.Nodes)
	result.Summary.TotalEdges = len(result.Edges)
	result.Summary.OrphanedCount = len(result.Orphaned)
	result.Summary.SharedCount = len(result.SharedResources)

	// Score based on orphan ratio
	totalRes := result.Summary.ConfigMaps + result.Summary.Secrets + result.Summary.PVCs
	if totalRes > 0 {
		result.HealthScore = (totalRes - result.Summary.OrphanedCount) * 100 / totalRes
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildTopologyRecs(&result)
	writeJSON(w, result)
}

func matchesSelector(wlSelector, svcSelector map[string]string) bool {
	if len(svcSelector) == 0 {
		return false
	}
	for k, v := range svcSelector {
		if wlSelector[k] != v {
			return false
		}
	}
	return true
}

func splitTopologyID(id string) []string {
	parts := []string{}
	current := ""
	for _, c := range id {
		if c == '/' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func buildTopologyRecs(r *ResTopologyResult) []string {
	recs := []string{}
	if r.Summary.OrphanedCount > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个孤立资源（无工作负载引用），建议清理", r.Summary.OrphanedCount))
	}
	if r.Summary.SharedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d 个共享资源被多个工作负载使用，注意变更影响面", r.Summary.SharedCount))
	}
	if len(recs) == 0 {
		recs = append(recs, "资源拓扑健康，无孤立资源")
	}
	return recs
}
