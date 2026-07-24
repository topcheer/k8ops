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
// v19.56 — Documentation Dimension (Round 12)
// 1. Annotation Coverage Report — metadata annotation governance
// 2. Pod Topology Map — node-to-pod placement documentation
// 3. Storage Attachment Inventory — PVC-to-pod-to-node mapping
// ============================================================

type AnnotationReportResult1956 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         AnnotationReportSummary1956 `json:"summary"`
	ByKind          []AnnotationReportKind1956  `json:"byKind"`
	Missing         []AnnotationReportEntry1956 `json:"missingAnnotations"`
	Recommendations []string                    `json:"recommendations"`
}

type AnnotationReportSummary1956 struct {
	TotalResources     int     `json:"totalResources"`
	WithAnnotations    int     `json:"withAnnotations"`
	WithoutAnnotations int     `json:"withoutAnnotations"`
	TopAnnotationKeys  int     `json:"topAnnotationKeys"`
	CoveragePct        float64 `json:"coveragePct"`
}

type AnnotationReportKind1956 struct {
	Kind         string `json:"kind"`
	WithAnnot    int    `json:"withAnnotations"`
	WithoutAnnot int    `json:"withoutAnnotations"`
}

type AnnotationReportEntry1956 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

func (s *Server) handleAnnotationReport(w http.ResponseWriter, r *http.Request) {
	result := AnnotationReportResult1956{ScannedAt: time.Now()}
	score := 100
	kindStats := make(map[string]*AnnotationReportKind1956)

	checkAnnot := func(name, ns, kind string, annots map[string]string) {
		if isSystemNamespace(ns) {
			return
		}
		result.Summary.TotalResources++
		ks, ok := kindStats[kind]
		if !ok {
			ks = &AnnotationReportKind1956{Kind: kind}
			kindStats[kind] = ks
		}

		if len(annots) > 0 {
			result.Summary.WithAnnotations++
			ks.WithAnnot++
		} else {
			result.Summary.WithoutAnnotations++
			ks.WithoutAnnot++
			if len(result.Missing) < 100 {
				result.Missing = append(result.Missing, AnnotationReportEntry1956{Name: name, Namespace: ns, Kind: kind})
			}
			score -= 1
		}
	}

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, d := range depList.Items {
		checkAnnot(d.Name, d.Namespace, "Deployment", d.Annotations)
	}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, sv := range svcList.Items {
		checkAnnot(sv.Name, sv.Namespace, "Service", sv.Annotations)
	}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		checkAnnot(ns.Name, "", "Namespace", ns.Annotations)
	}

	for _, ks := range kindStats {
		result.ByKind = append(result.ByKind, *ks)
	}
	if result.Summary.TotalResources > 0 {
		result.Summary.CoveragePct = float64(result.Summary.WithAnnotations) * 100 / float64(result.Summary.TotalResources)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutAnnotations > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources without annotations — add for documentation", result.Summary.WithoutAnnotations))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%.0f%% annotation coverage (%d/%d)", result.Summary.CoveragePct, result.Summary.WithAnnotations, result.Summary.TotalResources))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Topology Map
// ---------------------------------------------------------------

type TopologyMapResult1956 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         TopologyMapSummary1956 `json:"summary"`
	Nodes           []TopologyMapNode1956  `json:"nodes"`
	ByNS            []TopologyMapNS1956    `json:"byNamespace"`
	Recommendations []string               `json:"recommendations"`
}

type TopologyMapSummary1956 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	TotalZones     int     `json:"totalZones"`
	TotalArchs     int     `json:"totalArchitectures"`
}

type TopologyMapNode1956 struct {
	Node     string `json:"node"`
	Zone     string `json:"zone"`
	Arch     string `json:"architecture"`
	PodCount int    `json:"podCount"`
	CPU      string `json:"cpuCapacity"`
}

type TopologyMapNS1956 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	NodeCount int    `json:"nodeCount"`
}

func (s *Server) handleTopologyMap(w http.ResponseWriter, r *http.Request) {
	result := TopologyMapResult1956{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nodeInfo := make(map[string]*TopologyMapNode1956)
	zoneSet := make(map[string]bool)
	archSet := make(map[string]bool)

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		zone := node.Labels["topology.kubernetes.io/zone"]
		arch := node.Status.NodeInfo.Architecture
		if zone == "" {
			zone = "unknown"
		}
		zoneSet[zone] = true
		archSet[arch] = true
		nodeInfo[node.Name] = &TopologyMapNode1956{
			Node: node.Name, Zone: zone, Arch: arch,
			CPU: node.Status.Capacity.Cpu().String(),
		}
	}

	nsNodes := make(map[string]map[string]bool)
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		result.Summary.TotalPods++
		if ni, ok := nodeInfo[pod.Spec.NodeName]; ok {
			ni.PodCount++
		}
		if !isSystemNamespace(pod.Namespace) {
			nsPods[pod.Namespace]++
			if nsNodes[pod.Namespace] == nil {
				nsNodes[pod.Namespace] = make(map[string]bool)
			}
			nsNodes[pod.Namespace][pod.Spec.NodeName] = true
		}
	}

	for _, ni := range nodeInfo {
		result.Nodes = append(result.Nodes, *ni)
	}
	for ns, pods := range nsPods {
		result.ByNS = append(result.ByNS, TopologyMapNS1956{Namespace: ns, PodCount: pods, NodeCount: len(nsNodes[ns])})
	}
	result.Summary.TotalZones = len(zoneSet)
	result.Summary.TotalArchs = len(archSet)
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = float64(result.Summary.TotalPods) / float64(result.Summary.TotalNodes)
	}

	if result.Summary.TotalZones <= 1 && result.Summary.TotalNodes > 1 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations,
		fmt.Sprintf("%d nodes (%d zones, %d archs), %d pods, %.0f pods/node",
			result.Summary.TotalNodes, result.Summary.TotalZones, result.Summary.TotalArchs,
			result.Summary.TotalPods, result.Summary.AvgPodsPerNode))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Storage Attachment Inventory
// ---------------------------------------------------------------

type StorageInvResult1956 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         StorageInvSummary1956      `json:"summary"`
	Attachments     []StorageInvEntry1956      `json:"attachments"`
	Unattached      []StorageInvUnattached1956 `json:"unattached"`
	Recommendations []string                   `json:"recommendations"`
}

type StorageInvSummary1956 struct {
	TotalPVCs      int `json:"totalPVCs"`
	MountedPVCs    int `json:"mountedPVCs"`
	UnattachedPVCs int `json:"unattachedPVCs"`
	TotalMounts    int `json:"totalMounts"`
	ReadOnlyMounts int `json:"readOnlyMounts"`
}

type StorageInvEntry1956 struct {
	PVCName   string `json:"pvcName"`
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	NodeName  string `json:"nodeName"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type StorageInvUnattached1956 struct {
	PVCName   string `json:"pvcName"`
	Namespace string `json:"namespace"`
	Size      string `json:"size"`
	Age       string `json:"age"`
}

func (s *Server) handleStorageAttachmentInv(w http.ResponseWriter, r *http.Request) {
	result := StorageInvResult1956{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build PVC -> pod mapping
	pvcMounted := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				pvcMounted[key] = true
				result.Summary.TotalMounts++

				// Find mount path and readonly
				for _, c := range pod.Spec.Containers {
					for _, vm := range c.VolumeMounts {
						if vm.Name == vol.Name {
							entry := StorageInvEntry1956{
								PVCName:   vol.PersistentVolumeClaim.ClaimName,
								Namespace: pod.Namespace, PodName: pod.Name,
								NodeName: pod.Spec.NodeName, MountPath: vm.MountPath,
							}
							if len(result.Attachments) < 100 {
								result.Attachments = append(result.Attachments, entry)
							}
							if vm.ReadOnly {
								result.Summary.ReadOnlyMounts++
								entry.ReadOnly = true
							}
						}
					}
				}
			}
		}
	}

	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		if pvcMounted[key] {
			result.Summary.MountedPVCs++
		} else {
			result.Summary.UnattachedPVCs++
			result.Unattached = append(result.Unattached, StorageInvUnattached1956{
				PVCName: pvc.Name, Namespace: pvc.Namespace,
				Size: pvc.Spec.Resources.Requests.Storage().String(),
				Age:  fmt.Sprintf("%.0fd", time.Since(pvc.CreationTimestamp.Time).Hours()/24),
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.UnattachedPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unattached PVCs — clean up unused storage", result.Summary.UnattachedPVCs))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d mounted, %d unattached, %d total mounts)",
		result.Summary.TotalPVCs, result.Summary.MountedPVCs, result.Summary.UnattachedPVCs, result.Summary.TotalMounts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
