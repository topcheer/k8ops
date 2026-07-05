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

// VolSecResult is the volume security & mount risk analysis.
type VolSecResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         VolSecSummary  `json:"summary"`
	DangerousMounts []VolSecEntry  `json:"dangerousMounts"`
	HostPathVolumes []VolSecEntry  `json:"hostPathVolumes"`
	SATokenVolumes  []VolSecEntry  `json:"saTokenVolumes"`
	ByNamespace     []VolSecNSStat `json:"byNamespace"`
	Issues          []VolSecIssue  `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// VolSecSummary aggregates volume security statistics.
type VolSecSummary struct {
	TotalPods          int `json:"totalPods"`
	PodsWithHostPath   int `json:"podsWithHostPath"`
	PodsWithHostNet    int `json:"podsWithHostNet"`
	PodsWithHostPID    int `json:"podsWithHostPID"`
	PodsWithHostIPC    int `json:"podsWithHostIPC"`
	PodsWithPrivileged int `json:"podsWithPrivileged"`
	CriticalMounts     int `json:"criticalMounts"`   // docker.sock, /proc, /sys, /
	ReadWriteHostPath  int `json:"readOnlyHostPath"` // hostPath with RW
	SATokenAutoMount   int `json:"saTokenAutoMount"` // default SA token mounted
	TotalHostPathVol   int `json:"totalHostPathVol"`
	TotalSATokenVol    int `json:"totalSATokenVol"`
	SecurityScore      int `json:"securityScore"` // 0-100 (higher = safer)
}

// VolSecEntry describes a pod with a risky volume mount.
type VolSecEntry struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	VolumeName    string `json:"volumeName"`
	VolumeType    string `json:"volumeType"` // hostPath / projected / secret / configMap
	MountPath     string `json:"mountPath"`
	HostPath      string `json:"hostPath,omitempty"`
	ReadOnly      bool   `json:"readOnly"`
	DangerLevel   string `json:"dangerLevel"` // critical / high / medium / low
	DangerReason  string `json:"dangerReason"`
	ContainerName string `json:"containerName"`
}

// VolSecNSStat per-namespace stats.
type VolSecNSStat struct {
	Namespace      string `json:"namespace"`
	PodCount       int    `json:"podCount"`
	HostPathPods   int    `json:"hostPathPods"`
	PrivilegedPods int    `json:"privilegedPods"`
	CriticalCount  int    `json:"criticalCount"`
	RiskLevel      string `json:"riskLevel"`
}

// VolSecIssue is a detected security problem.
type VolSecIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// Dangerous mount paths that enable container escape.
var dangerousPaths = map[string]string{
	"/":                    "root filesystem mount — full host access",
	"/var/run/docker.sock": "Docker socket — container escape, full host control",
	"/run/docker.sock":     "Docker socket — container escape, full host control",
	"/var/run/containerd":  "containerd socket — runtime access",
	"/var/run/cri-dockerd": "CRI socket — runtime access",
	"/proc":                "proc filesystem — host process info, kernel access",
	"/sys":                 "sys filesystem — kernel module manipulation",
	"/dev":                 "device filesystem — hardware access",
	"/etc/kubernetes":      "Kubernetes config — cluster credential theft",
	"/root/.kube":          "kubeconfig — cluster admin credential theft",
	"/var/lib/kubelet":     "kubelet data — pod manifest injection, credential theft",
	"/var/lib/etcd":        "etcd data — full cluster state and secrets",
	"/var/lib/docker":      "Docker data — image manipulation",
	"/var/lib/containerd":  "containerd data — image manipulation",
}

// handleVolumeSecurity audits volume mounts for security risks.
// GET /api/security/volume-mounts
func (s *Server) handleVolumeSecurity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := VolSecResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*VolSecNSStat)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		result.Summary.TotalPods++
		nsStat := vsGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.PodCount++

		// Host namespace sharing
		if pod.Spec.HostNetwork {
			result.Summary.PodsWithHostNet++
		}
		if pod.Spec.HostPID {
			result.Summary.PodsWithHostPID++
		}
		if pod.Spec.HostIPC {
			result.Summary.PodsWithHostIPC++
		}

		// Check each container for privileged + volume mounts
		allContainers := append([]corev1.Container{}, pod.Spec.Containers...)
		allContainers = append(allContainers, pod.Spec.InitContainers...)

		podHasHostPath := false
		for _, c := range allContainers {
			// Privileged container
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.PodsWithPrivileged++
				nsStat.PrivilegedPods++
				result.Issues = append(result.Issues, VolSecIssue{
					Severity: "critical", Type: "privileged-container",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in pod %s/%s is privileged — full host access", c.Name, pod.Namespace, pod.Name),
				})
			}

			// Volume mounts
			for _, vm := range c.VolumeMounts {
				vol := findVolume(pod.Spec.Volumes, vm.Name)
				if vol == nil {
					continue
				}

				entry := VolSecEntry{
					PodName:       pod.Name,
					Namespace:     pod.Namespace,
					VolumeName:    vm.Name,
					MountPath:     vm.MountPath,
					ReadOnly:      vm.ReadOnly,
					ContainerName: c.Name,
				}

				// HostPath analysis
				if vol.HostPath != nil {
					entry.VolumeType = "hostPath"
					entry.HostPath = vol.HostPath.Path
					podHasHostPath = true
					result.Summary.TotalHostPathVol++

					entry.DangerLevel, entry.DangerReason = vsAssessHostPath(vol.HostPath.Path, entry.ReadOnly)

					result.HostPathVolumes = append(result.HostPathVolumes, entry)

					if !entry.ReadOnly {
						result.Summary.ReadWriteHostPath++
					}

					if entry.DangerLevel == "critical" {
						result.Summary.CriticalMounts++
						nsStat.CriticalCount++
						result.DangerousMounts = append(result.DangerousMounts, entry)
						result.Issues = append(result.Issues, VolSecIssue{
							Severity: "critical", Type: "dangerous-hostpath",
							Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
							Message:  fmt.Sprintf("Pod %s/%s mounts hostPath %s at %s — %s", pod.Namespace, pod.Name, vol.HostPath.Path, vm.MountPath, entry.DangerReason),
						})
					} else if entry.DangerLevel == "high" {
						result.DangerousMounts = append(result.DangerousMounts, entry)
						result.Issues = append(result.Issues, VolSecIssue{
							Severity: "warning", Type: "hostpath-mount",
							Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
							Message:  fmt.Sprintf("Pod %s/%s mounts hostPath %s — host filesystem access", pod.Namespace, pod.Name, vol.HostPath.Path),
						})
					}
				}

				// Dangerous mount path detection (even for non-hostPath)
				if reason, isDangerous := dangerousPaths[vm.MountPath]; isDangerous {
					if entry.DangerLevel == "" {
						entry.DangerLevel = "critical"
						entry.DangerReason = reason
						entry.VolumeType = vsVolumeType(*vol)
						result.DangerousMounts = append(result.DangerousMounts, entry)
						result.Summary.CriticalMounts++
						nsStat.CriticalCount++
						result.Issues = append(result.Issues, VolSecIssue{
							Severity: "critical", Type: "dangerous-mount-path",
							Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
							Message:  fmt.Sprintf("Pod %s/%s mounts dangerous path %s — %s", pod.Namespace, pod.Name, vm.MountPath, reason),
						})
					}
				}
			}
		}

		// ServiceAccount token volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ServiceAccountToken != nil {
						result.Summary.TotalSATokenVol++
						result.SATokenVolumes = append(result.SATokenVolumes, VolSecEntry{
							PodName:      pod.Name,
							Namespace:    pod.Namespace,
							VolumeName:   vol.Name,
							VolumeType:   "projected-SA-token",
							DangerLevel:  "info",
							DangerReason: fmt.Sprintf("ServiceAccount %s token projected", pod.Spec.ServiceAccountName),
						})
					}
				}
			}
			if vol.Secret != nil && strings.Contains(strings.ToLower(vol.Secret.SecretName), "default-token") {
				result.Summary.SATokenAutoMount++
			}
		}

		if podHasHostPath {
			result.Summary.PodsWithHostPath++
			nsStat.HostPathPods++
		}
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		nsStat.RiskLevel = vsNSRisk(nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.DangerousMounts, func(i, j int) bool {
		return vsRiskRank(result.DangerousMounts[i].DangerLevel) < vsRiskRank(result.DangerousMounts[j].DangerLevel)
	})
	sort.Slice(result.HostPathVolumes, func(i, j int) bool {
		return vsRiskRank(result.HostPathVolumes[i].DangerLevel) < vsRiskRank(result.HostPathVolumes[j].DangerLevel)
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CriticalCount > result.ByNamespace[j].CriticalCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return vsIssueRank(result.Issues[i].Severity) < vsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.SecurityScore = vsScore(result.Summary)
	result.Recommendations = vsRecs(result.Summary, result.DangerousMounts)

	writeJSON(w, result)
}

// vsAssessHostPath evaluates hostPath danger level.
func vsAssessHostPath(path string, readOnly bool) (level, reason string) {
	// Check against known dangerous paths
	if r, ok := dangerousPaths[path]; ok {
		if readOnly {
			return "high", r + " (read-only)"
		}
		return "critical", r
	}

	// Docker socket variants
	if strings.Contains(path, "docker.sock") || strings.Contains(path, "containerd") {
		if readOnly {
			return "high", "container runtime socket access (read-only)"
		}
		return "critical", "container runtime socket — full host control"
	}

	// Kubernetes paths
	if strings.Contains(path, "kubernetes") || strings.Contains(path, "kubelet") || strings.Contains(path, "etcd") {
		if readOnly {
			return "high", "Kubernetes internals access (read-only)"
		}
		return "critical", "Kubernetes internals — credential theft risk"
	}

	// Root or broad paths
	if path == "/" || path == "/var" || path == "/etc" {
		return "critical", "broad host filesystem access"
	}

	// Generic hostPath
	if readOnly {
		return "low", "read-only hostPath access"
	}
	return "medium", "read-write hostPath — host filesystem modification"
}

// vsVolumeType returns a human-readable volume type.
func vsVolumeType(vol corev1.Volume) string {
	if vol.HostPath != nil {
		return "hostPath"
	}
	if vol.Secret != nil {
		return "secret"
	}
	if vol.ConfigMap != nil {
		return "configMap"
	}
	if vol.Projected != nil {
		return "projected"
	}
	if vol.PersistentVolumeClaim != nil {
		return "pvc"
	}
	if vol.EmptyDir != nil {
		return "emptyDir"
	}
	return "other"
}

// findVolume finds a volume by name.
func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

// vsNSRisk determines namespace risk level.
func vsNSRisk(ns *VolSecNSStat) string {
	if ns.CriticalCount > 0 {
		return "critical"
	}
	if ns.PrivilegedPods > 0 || ns.HostPathPods > 0 {
		return "high"
	}
	return "low"
}

// vsScore computes 0-100 (higher = safer).
func vsScore(s VolSecSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.CriticalMounts * 12
	score -= s.PodsWithPrivileged * 15
	score -= s.PodsWithHostNet * 5
	score -= s.PodsWithHostPID * 5
	score -= s.PodsWithHostIPC * 3
	score -= s.ReadWriteHostPath * 4
	if score < 0 {
		score = 0
	}
	return score
}

// vsRecs produces actionable advice.
func vsRecs(s VolSecSummary, dangerous []VolSecEntry) []string {
	var recs []string

	if s.PodsWithPrivileged > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged container(s) — remove privileged flag, use specific capabilities instead", s.PodsWithPrivileged))
	}
	if s.CriticalMounts > 0 {
		top := ""
		if len(dangerous) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s → %s)", dangerous[0].Namespace, dangerous[0].PodName, dangerous[0].MountPath)
		}
		recs = append(recs, fmt.Sprintf("%d critical volume mount(s)%s — remove dangerous hostPath mounts immediately", s.CriticalMounts, top))
	}
	if s.ReadWriteHostPath > 0 {
		recs = append(recs, fmt.Sprintf("%d read-write hostPath volume(s) — set readOnly:true or use PVC", s.ReadWriteHostPath))
	}
	if s.PodsWithHostNet > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with hostNetwork — restrict network namespace sharing", s.PodsWithHostNet))
	}
	if s.PodsWithHostPID > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with hostPID — restrict PID namespace sharing", s.PodsWithHostPID))
	}
	if s.SecurityScore < 60 {
		recs = append(recs, fmt.Sprintf("Volume security score is %d/100 — multiple high-risk mount configurations detected", s.SecurityScore))
	}
	if s.CriticalMounts == 0 && s.PodsWithPrivileged == 0 && s.PodsWithHostPath == 0 {
		recs = append(recs, "No dangerous volume mounts detected — volume security posture is healthy")
	}

	return recs
}

func vsGetOrCreateNS(m map[string]*VolSecNSStat, ns string) *VolSecNSStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &VolSecNSStat{Namespace: ns}
	m[ns] = e
	return e
}

func vsRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

func vsIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
