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

// ============================================================
// v19.21 — Product Dimension (Round 6)
// 1. Image Lifecycle Tracker — image age, staleness, reclaimable
// 2. Volume Snapshot Readiness — PVC CSI snapshot eligibility
// 3. Idle Resource Detector — zero-traffic pods & unused services
// ============================================================

// ---------------------------------------------------------------
// 1. Image Lifecycle Tracker — tracks image freshness and reuse
// ---------------------------------------------------------------

type ImageLifecycleResult1921 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         ImageLifecycleSummary1921 `json:"summary"`
	Images          []ImageLifecycleEntry1921 `json:"images"`
	StaleImages     []ImageStaleEntry1921     `json:"staleImages"`
	Recommendations []string                  `json:"recommendations"`
}

type ImageLifecycleSummary1921 struct {
	TotalImages      int `json:"totalImages"`
	UniqueImages     int `json:"uniqueImages"`
	StaleCount       int `json:"staleCount"`
	LatestTagCount   int `json:"latestTagCount"`
	PinnedCount      int `json:"pinnedCount"`
	FloatingTagCount int `json:"floatingTagCount"`
	MaxReuse         int `json:"maxReuse"`
}

type ImageLifecycleEntry1921 struct {
	Image      string   `json:"image"`
	ReuseCount int      `json:"reuseCount"`
	Namespaces []string `json:"namespaces"`
	Workloads  []string `json:"workloads"`
	TagType    string   `json:"tagType"` // pinned (sha256), latest, versioned, floating
}

type ImageStaleEntry1921 struct {
	Image     string   `json:"image"`
	Reason    string   `json:"reason"`
	Severity  string   `json:"severity"`
	Workloads []string `json:"workloads"`
}

func (s *Server) handleImageLifecycle(w http.ResponseWriter, r *http.Request) {
	result := ImageLifecycleResult1921{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Track image usage
	type imgData struct {
		count   int
		nsSet   map[string]bool
		wlSet   map[string]bool
		tagType string
	}
	imgMap := make(map[string]*imgData)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			appName = pod.Name
		}
		for _, container := range pod.Spec.Containers {
			img := container.Image
			data, exists := imgMap[img]
			if !exists {
				tagType := "versioned"
				if strings.Contains(img, "@sha256:") || strings.HasPrefix(img, "sha256:") {
					tagType = "pinned"
				} else if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
					tagType = "floating"
				}
				data = &imgData{
					nsSet:   make(map[string]bool),
					wlSet:   make(map[string]bool),
					tagType: tagType,
				}
				imgMap[img] = data
			}
			data.count++
			data.nsSet[pod.Namespace] = true
			data.wlSet[appName] = true
		}
	}

	// Build entries
	for img, data := range imgMap {
		nsList := make([]string, 0, len(data.nsSet))
		for ns := range data.nsSet {
			nsList = append(nsList, ns)
		}
		wlList := make([]string, 0, len(data.wlSet))
		for wl := range data.wlSet {
			wlList = append(wlList, wl)
		}
		entry := ImageLifecycleEntry1921{
			Image:      img,
			ReuseCount: data.count,
			Namespaces: nsList,
			Workloads:  wlList,
			TagType:    data.tagType,
		}
		result.Images = append(result.Images, entry)

		if data.tagType == "pinned" {
			result.Summary.PinnedCount++
		} else if data.tagType == "floating" {
			result.Summary.FloatingTagCount++
			result.StaleImages = append(result.StaleImages, ImageStaleEntry1921{
				Image:     img,
				Reason:    "Using floating tag (:latest or no tag) — not reproducible",
				Severity:  "warning",
				Workloads: wlList,
			})
		}
		if data.count > result.Summary.MaxReuse {
			result.Summary.MaxReuse = data.count
		}
	}

	// Check for images with >10 reuses (blast radius)
	for _, entry := range result.Images {
		if entry.ReuseCount > 10 {
			result.StaleImages = append(result.StaleImages, ImageStaleEntry1921{
				Image:     entry.Image,
				Reason:    fmt.Sprintf("Image reused %d times — high blast radius for vulnerability", entry.ReuseCount),
				Severity:  "medium",
				Workloads: entry.Workloads,
			})
		}
	}

	sort.Slice(result.Images, func(i, j int) bool {
		return result.Images[i].ReuseCount > result.Images[j].ReuseCount
	})

	result.Summary.TotalImages = len(result.Images)
	result.Summary.UniqueImages = len(imgMap)
	result.Summary.StaleCount = len(result.StaleImages)

	// Score
	if result.Summary.FloatingTagCount > 0 {
		score -= result.Summary.FloatingTagCount * 3
	}
	if result.Summary.MaxReuse > 20 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.FloatingTagCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images use floating tags — pin to specific versions or digests", result.Summary.FloatingTagCount))
	}
	if result.Summary.MaxReuse > 10 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Most reused image appears %d times — scan for vulnerabilities regularly", result.Summary.MaxReuse))
	}
	if result.Summary.PinnedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images are pinned to digests — best practice for reproducibility", result.Summary.PinnedCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume Snapshot Readiness — PVC CSI snapshot eligibility
// ---------------------------------------------------------------

type VolSnapshotReadyResult1921 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         VolSnapshotReadySummary1921 `json:"summary"`
	Volumes         []VolSnapshotEntry1921      `json:"volumes"`
	NotReady        []VolSnapshotNotReady1921   `json:"notReady"`
	StorageClasses  []VolSnapshotSCEntry1921    `json:"storageClasses"`
	Recommendations []string                    `json:"recommendations"`
}

type VolSnapshotReadySummary1921 struct {
	TotalPVCs          int `json:"totalPVCs"`
	ReadyForSnapshot   int `json:"readyForSnapshot"`
	NotReadyCount      int `json:"notReadyCount"`
	BoundPVCs          int `json:"boundPVCs"`
	UnboundPVCs        int `json:"unboundPVCs"`
	SnapshotCapableSCs int `json:"snapshotCapableSCs"`
	TotalSCs           int `json:"totalStorageClasses"`
}

type VolSnapshotEntry1921 struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	StorageClass  string `json:"storageClass"`
	Size          string `json:"size"`
	AccessMode    string `json:"accessMode"`
	Bound         bool   `json:"bound"`
	SnapshotReady bool   `json:"snapshotReady"`
}

type VolSnapshotNotReady1921 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}

type VolSnapshotSCEntry1921 struct {
	Name            string `json:"name"`
	Provisioner     string `json:"provisioner"`
	SnapshotSupport bool   `json:"snapshotSupport"`
	PVCCount        int    `json:"pvcCount"`
}

func (s *Server) handleVolumeSnapshotReadiness(w http.ResponseWriter, r *http.Request) {
	result := VolSnapshotReadyResult1921{
		ScannedAt: time.Now(),
	}
	score := 100

	// Get storage classes and check snapshot support
	scList, err := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		csiProvisioners := map[string]bool{
			"driver.longhorn.io": true, "rancher.io/local-path": true,
			"disk.csi.azure.com": true, "gp.csi.aws.com": true,
			"pd.csi.storage.gke.io": true, "ebs.csi.aws.com": true,
			"csi.trident.netapp.io": true, "rook-ceph.rbd.csi.ceph.com": true,
			"rook-ceph.cephfs.csi.ceph.com": true, "sigs.k8s.io/nfs-subdir-external-provisioner": false,
		}
		for _, sc := range scList.Items {
			supportsSnap := csiProvisioners[sc.Provisioner]
			result.StorageClasses = append(result.StorageClasses, VolSnapshotSCEntry1921{
				Name:            sc.Name,
				Provisioner:     sc.Provisioner,
				SnapshotSupport: supportsSnap,
			})
			result.Summary.TotalSCs++
			if supportsSnap {
				result.Summary.SnapshotCapableSCs++
			}
		}
	}

	// Check PVCs
	scSnapMap := make(map[string]bool)
	for _, sc := range result.StorageClasses {
		scSnapMap[sc.Name] = sc.SnapshotSupport
	}

	pvcList, err := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pvc := range pvcList.Items {
			if isSystemNamespace(pvc.Namespace) {
				continue
			}
			bound := pvc.Status.Phase == corev1.ClaimBound
			scName := *pvc.Spec.StorageClassName
			if scName == "" {
				scName = "default"
			}
			snapReady := bound && scSnapMap[scName]
			accessMode := ""
			if len(pvc.Spec.AccessModes) > 0 {
				accessMode = string(pvc.Spec.AccessModes[0])
			}
			entry := VolSnapshotEntry1921{
				Name:          pvc.Name,
				Namespace:     pvc.Namespace,
				StorageClass:  scName,
				Size:          pvc.Spec.Resources.Requests.Storage().String(),
				AccessMode:    accessMode,
				Bound:         bound,
				SnapshotReady: snapReady,
			}
			result.Volumes = append(result.Volumes, entry)
			result.Summary.TotalPVCs++
			if bound {
				result.Summary.BoundPVCs++
			} else {
				result.Summary.UnboundPVCs++
			}
			if snapReady {
				result.Summary.ReadyForSnapshot++
			} else {
				result.Summary.NotReadyCount++
				reason := "PVC not bound"
				if !bound {
					reason = "PVC is not in Bound phase"
				} else if !scSnapMap[scName] {
					reason = fmt.Sprintf("StorageClass %s does not support CSI snapshots", scName)
				}
				result.NotReady = append(result.NotReady, VolSnapshotNotReady1921{
					Name:      pvc.Name,
					Namespace: pvc.Namespace,
					Reason:    reason,
				})
			}
		}
	}

	// Score
	if result.Summary.TotalPVCs > 0 {
		ratio := result.Summary.ReadyForSnapshot * 100 / result.Summary.TotalPVCs
		if ratio < 50 {
			score -= (50 - ratio)
		}
	}
	if result.Summary.SnapshotCapableSCs == 0 && result.Summary.TotalSCs > 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.NotReadyCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs not snapshot-ready — migrate to CSI-supported StorageClass", result.Summary.NotReadyCount))
	}
	if result.Summary.SnapshotCapableSCs == 0 && result.Summary.TotalSCs > 0 {
		result.Recommendations = append(result.Recommendations, "No StorageClass supports CSI snapshots — install a CSI driver with VolumeSnapshot capability")
	}
	if result.Summary.UnboundPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unbound PVCs — investigate provisioning failures", result.Summary.UnboundPVCs))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Idle Resource Detector — finds zero-traffic pods & unused services
// ---------------------------------------------------------------

type IdleResourceResult1921 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         IdleResourceSummary1921   `json:"summary"`
	IdlePods        []IdlePodEntry1921        `json:"idlePods"`
	UnusedServices  []IdleServiceEntry1921    `json:"unusedServices"`
	WastedResources []WastedResourceEntry1921 `json:"wastedResources"`
	Recommendations []string                  `json:"recommendations"`
}

type IdleResourceSummary1921 struct {
	TotalPods          int     `json:"totalPods"`
	IdlePodCount       int     `json:"idlePodCount"`
	TotalServices      int     `json:"totalServices"`
	UnusedServiceCount int     `json:"unusedServiceCount"`
	EstWastedCPUCores  float64 `json:"estWastedCPUCores"`
	EstWastedMemMB     int     `json:"estWastedMemMB"`
	EstMonthlyCostUSD  float64 `json:"estMonthlyCostUSD"`
}

type IdlePodEntry1921 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Workload   string `json:"workload"`
	Age        string `json:"age"`
	CPURequest string `json:"cpuRequest"`
	MemRequest string `json:"memRequest"`
	Reason     string `json:"reason"`
}

type IdleServiceEntry1921 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Age       string `json:"age"`
	Reason    string `json:"reason"`
}

type WastedResourceEntry1921 struct {
	Resource  string  `json:"resource"`
	Namespace string  `json:"namespace"`
	Detail    string  `json:"detail"`
	CostUSD   float64 `json:"estCostUSD"`
}

func (s *Server) handleIdleResource(w http.ResponseWriter, r *http.Request) {
	result := IdleResourceResult1921{
		ScannedAt: time.Now(),
	}
	score := 100
	var wastedCPU float64
	var wastedMemMB int

	// List services to check for unused ones
	svcList, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	svcRefCount := make(map[string]int)
	for _, svc := range svcList.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		svcRefCount[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = 0
	}

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	result.Summary.TotalPods = len(podList.Items)

	// Build service reference map from pod env vars
	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			for _, env := range container.Env {
				for svcKey := range svcRefCount {
					parts := strings.Split(svcKey, "/")
					if len(parts) == 2 && strings.Contains(env.Value, parts[1]) {
						svcRefCount[svcKey]++
					}
				}
			}
		}
	}

	// Detect idle pods (very low resource usage indicators)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			continue
		}

		// Check restart count as idle indicator
		restartCount := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restartCount += int(cs.RestartCount)
		}

		// Check for pods that completed their work (Jobs marked as completed)
		isJob := false
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "Job" {
				isJob = true
			}
		}

		// Pods with 0 restarts, running > 7 days, and no service reference
		ageHours := time.Since(pod.CreationTimestamp.Time).Hours()
		hasServiceRef := false
		for _, container := range pod.Spec.Containers {
			for _, env := range container.Env {
				for svcKey := range svcRefCount {
					parts := strings.Split(svcKey, "/")
					if len(parts) == 2 && strings.Contains(env.Value, parts[1]) {
						hasServiceRef = true
					}
				}
			}
		}

		if ageHours > 168 && !hasServiceRef && !isJob {
			cpuReq := "0"
			memReq := "0"
			for _, container := range pod.Spec.Containers {
				if !container.Resources.Requests.Cpu().IsZero() {
					cpuReq = container.Resources.Requests.Cpu().String()
					wastedCPU += container.Resources.Requests.Cpu().AsApproximateFloat64()
				}
				if !container.Resources.Requests.Memory().IsZero() {
					memReq = container.Resources.Requests.Memory().String()
					wastedMemMB += int(container.Resources.Requests.Memory().Value() / (1024 * 1024))
				}
			}
			result.IdlePods = append(result.IdlePods, IdlePodEntry1921{
				Name:       pod.Name,
				Namespace:  pod.Namespace,
				Workload:   appName,
				Age:        fmt.Sprintf("%.0fd", ageHours/24),
				CPURequest: cpuReq,
				MemRequest: memReq,
				Reason:     "Pod running >7 days with no service references",
			})
			result.Summary.IdlePodCount++
		}
	}

	// Detect unused services (no references found)
	for _, svc := range svcList.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		refCount := svcRefCount[svcKey]
		if refCount == 0 && svc.Spec.Type != corev1.ServiceTypeExternalName {
			ageDays := time.Since(svc.CreationTimestamp.Time).Hours() / 24
			if ageDays > 1 { // Only flag services older than 1 day
				result.UnusedServices = append(result.UnusedServices, IdleServiceEntry1921{
					Name:      svc.Name,
					Namespace: svc.Namespace,
					Type:      string(svc.Spec.Type),
					Age:       fmt.Sprintf("%.0fd", ageDays),
					Reason:    "No pod references this service",
				})
				result.Summary.UnusedServiceCount++
			}
		}
	}
	result.Summary.TotalServices = len(svcList.Items)

	// Calculate wasted cost (same pricing as cost overview: $28/core + $3.5/GB)
	result.Summary.EstWastedCPUCores = wastedCPU
	result.Summary.EstWastedMemMB = wastedMemMB
	result.Summary.EstMonthlyCostUSD = wastedCPU*28 + float64(wastedMemMB)/1024*3.5

	// Wasted resource entries
	if wastedCPU > 0 {
		result.WastedResources = append(result.WastedResources, WastedResourceEntry1921{
			Resource: "CPU",
			Detail:   fmt.Sprintf("%.2f cores from idle pods", wastedCPU),
			CostUSD:  wastedCPU * 28,
		})
	}
	if wastedMemMB > 0 {
		result.WastedResources = append(result.WastedResources, WastedResourceEntry1921{
			Resource: "Memory",
			Detail:   fmt.Sprintf("%dMB from idle pods", wastedMemMB),
			CostUSD:  float64(wastedMemMB) / 1024 * 3.5,
		})
	}

	// Score
	if result.Summary.IdlePodCount > 5 {
		score -= 10
	}
	if result.Summary.UnusedServiceCount > 10 {
		score -= 10
	}
	if result.Summary.EstMonthlyCostUSD > 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.IdlePodCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d idle pods detected — review for termination", result.Summary.IdlePodCount))
	}
	if result.Summary.UnusedServiceCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unused services — clean up to reduce clutter", result.Summary.UnusedServiceCount))
	}
	if result.Summary.EstMonthlyCostUSD > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Estimated $%.2f/month wasted on idle resources", result.Summary.EstMonthlyCostUSD))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
