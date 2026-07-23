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
// v19.31 — Security Dimension (Round 8)
// 1. Volume Mount Audit — sensitive path exposure
// 2. Privilege Escalation Risk — setuid/runAsUser analysis
// 3. Image Base Layer Scan — base image vulnerability indicators
// ============================================================

// ---------------------------------------------------------------
// 1. Volume Mount Audit — sensitive path exposure
// ---------------------------------------------------------------

type VolumeMountResult1931 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         VolumeMountSummary1931 `json:"summary"`
	RiskyMounts     []RiskyMountEntry1931  `json:"riskyMounts"`
	VolumeTypes     []VolumeTypeStat1931   `json:"volumeTypes"`
	Recommendations []string               `json:"recommendations"`
}

type VolumeMountSummary1931 struct {
	TotalPods       int `json:"totalPods"`
	WithHostPath    int `json:"withHostPath"`
	WithHostNet     int `json:"withHostNetwork"`
	WithHostPID     int `json:"withHostPID"`
	WithHostIPC     int `json:"withHostIPC"`
	ReadOnlyRootFS  int `json:"readOnlyRootFS"`
	RiskyMountCount int `json:"riskyMountCount"`
}

type RiskyMountEntry1931 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	MountPath string `json:"mountPath"`
	HostPath  string `json:"hostPath"`
	ReadOnly  bool   `json:"readOnly"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type VolumeTypeStat1931 struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

var sensitiveHostPaths = []string{
	"/etc", "/var/run/docker.sock", "/root", "/var/lib/kubelet",
	"/proc", "/sys", "/dev", "/var/lib/etcd", "/var/lib/cni",
	"/etc/kubernetes", "/home", "/var/log",
}

func (s *Server) handleVolumeMountAudit(w http.ResponseWriter, r *http.Request) {
	result := VolumeMountResult1931{
		ScannedAt: time.Now(),
	}
	score := 100
	volTypes := make(map[string]int)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		// Host namespaces
		if pod.Spec.HostNetwork {
			result.Summary.WithHostNet++
			result.RiskyMounts = append(result.RiskyMounts, RiskyMountEntry1931{
				PodName: pod.Name, Namespace: pod.Namespace,
				RiskType: "hostNetwork", Severity: "high",
				Detail: "Pod uses hostNetwork — bypasses network isolation",
			})
			score -= 5
		}
		if pod.Spec.HostPID {
			result.Summary.WithHostPID++
			score -= 5
		}
		if pod.Spec.HostIPC {
			result.Summary.WithHostIPC++
			score -= 3
		}

		// Check volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				result.Summary.WithHostPath++
				volTypes["hostPath"]++
				hp := vol.HostPath.Path
				isSensitive := false
				for _, sp := range sensitiveHostPaths {
					if strings.HasPrefix(hp, sp) {
						isSensitive = true
						break
					}
				}
				ro := false
				if vol.HostPath.Type != nil && *vol.HostPath.Type == "ReadOnly" {
					ro = true
				}
				severity := "medium"
				if isSensitive && !ro {
					severity = "critical"
				} else if isSensitive {
					severity = "high"
				}
				result.RiskyMounts = append(result.RiskyMounts, RiskyMountEntry1931{
					PodName: pod.Name, Namespace: pod.Namespace,
					HostPath: hp, ReadOnly: ro,
					RiskType: "hostPath", Severity: severity,
				})
				if isSensitive {
					score -= 5
				} else {
					score -= 1
				}
			}
			if vol.EmptyDir != nil {
				volTypes["emptyDir"]++
			}
			if vol.ConfigMap != nil {
				volTypes["configMap"]++
			}
			if vol.Secret != nil {
				volTypes["secret"]++
			}
			if vol.PersistentVolumeClaim != nil {
				volTypes["pvc"]++
			}
			if vol.Projected != nil {
				volTypes["projected"]++
			}
		}

		// Check readonly rootfs
		hasReadOnlyRoot := true
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
				hasReadOnlyRoot = false
				break
			}
		}
		if hasReadOnlyRoot && len(pod.Spec.Containers) > 0 {
			result.Summary.ReadOnlyRootFS++
		}
	}

	for t, c := range volTypes {
		result.VolumeTypes = append(result.VolumeTypes, VolumeTypeStat1931{Type: t, Count: c})
	}
	result.Summary.RiskyMountCount = len(result.RiskyMounts)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithHostPath > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with hostPath volumes — use PVCs for isolation", result.Summary.WithHostPath))
	}
	if result.Summary.WithHostNet > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with hostNetwork — restrict to system pods only", result.Summary.WithHostNet))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Privilege Escalation Risk — setuid/runAsUser analysis
// ---------------------------------------------------------------

type PrivEscResult1931 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PrivEscSummary1931 `json:"summary"`
	Risks           []PrivEscEntry1931 `json:"risks"`
	Recommendations []string           `json:"recommendations"`
}

type PrivEscSummary1931 struct {
	TotalContainers int `json:"totalContainers"`
	RunAsRoot       int `json:"runAsRoot"`
	RunAsNonRoot    int `json:"runAsNonRoot"`
	AllowPrivEsc    int `json:"allowPrivilegeEscalation"`
	PrivilegedMode  int `json:"privilegedMode"`
	RunAsUserID0    int `json:"runAsUserID0"`
	NoSCContext     int `json:"noSecurityContext"`
}

type PrivEscEntry1931 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handlePrivEscRisk(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult1931{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Pod-level security context
		podSC := pod.Spec.SecurityContext
		podRunAsNonRoot := false
		podRunAsUser := int64(0)
		if podSC != nil {
			if podSC.RunAsNonRoot != nil && *podSC.RunAsNonRoot {
				podRunAsNonRoot = true
			}
			if podSC.RunAsUser != nil {
				podRunAsUser = *podSC.RunAsUser
			}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			sc := c.SecurityContext

			if sc == nil {
				result.Summary.NoSCContext++
				if !podRunAsNonRoot {
					result.Risks = append(result.Risks, PrivEscEntry1931{
						PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
						RiskType: "no-security-context", Severity: "medium",
						Detail: "No security context — defaults to root",
					})
					score -= 2
				}
				continue
			}

			// Privileged mode
			if sc.Privileged != nil && *sc.Privileged {
				result.Summary.PrivilegedMode++
				result.Risks = append(result.Risks, PrivEscEntry1931{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					RiskType: "privileged", Severity: "critical",
					Detail: "Container runs in privileged mode — full host access",
				})
				score -= 10
			}

			// AllowPrivilegeEscalation
			if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
				result.Summary.AllowPrivEsc++
				result.Risks = append(result.Risks, PrivEscEntry1931{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					RiskType: "allow-priv-esc", Severity: "high",
					Detail: "AllowPrivilegeEscalation=true — can gain more privileges",
				})
				score -= 3
			}

			// RunAsUser = 0 (root)
			runAsUser := podRunAsUser
			if sc.RunAsUser != nil {
				runAsUser = *sc.RunAsUser
			}
			if runAsUser == 0 {
				result.Summary.RunAsUserID0++
				result.Summary.RunAsRoot++
				result.Risks = append(result.Risks, PrivEscEntry1931{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					RiskType: "run-as-root", Severity: "medium",
					Detail: "Container runs as root (UID 0)",
				})
				score -= 2
			} else if runAsUser != 0 {
				result.Summary.RunAsNonRoot++
			}

			// RunAsNonRoot check
			if sc.RunAsNonRoot != nil && *sc.RunAsNonRoot {
				result.Summary.RunAsNonRoot++
			} else if podRunAsNonRoot {
				result.Summary.RunAsNonRoot++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PrivilegedMode > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d privileged containers — remove privileged flag", result.Summary.PrivilegedMode))
	}
	if result.Summary.AllowPrivEsc > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers allow privilege escalation — set to false", result.Summary.AllowPrivEsc))
	}
	if result.Summary.RunAsRoot > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers run as root — set runAsNonRoot: true", result.Summary.RunAsRoot))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Image Base Layer Scan — base image vulnerability indicators
// ---------------------------------------------------------------

type ImageBaseScanResult1931 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ImageBaseScanSummary1931 `json:"summary"`
	Images          []ImageBaseEntry1931     `json:"images"`
	Risks           []ImageBaseRisk1931      `json:"risks"`
	Recommendations []string                 `json:"recommendations"`
}

type ImageBaseScanSummary1931 struct {
	TotalImages     int `json:"totalImages"`
	DistrolessCount int `json:"distrolessCount"`
	AlpineCount     int `json:"alpineCount"`
	DebianCount     int `json:"debianCount"`
	UbuntuCount     int `json:"ubuntuCount"`
	UnknownBase     int `json:"unknownBase"`
	LatestTagCount  int `json:"latestTagCount"`
	SlimCount       int `json:"slimCount"`
}

type ImageBaseEntry1931 struct {
	Image     string `json:"image"`
	BaseImage string `json:"baseImage"`
	IsLatest  bool   `json:"isLatest"`
	IsSlim    bool   `json:"isSlim"`
}

type ImageBaseRisk1931 struct {
	Image    string `json:"image"`
	RiskType string `json:"riskType"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleImageBaseScan(w http.ResponseWriter, r *http.Request) {
	result := ImageBaseScanResult1931{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	imgSet := make(map[string]bool)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			if imgSet[img] {
				continue
			}
			imgSet[img] = true

			imgLower := strings.ToLower(img)
			baseImg := "unknown"
			isLatest := strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":")
			isSlim := strings.Contains(imgLower, "slim") || strings.Contains(imgLower, "alpine") ||
				strings.Contains(imgLower, "distroless") || strings.Contains(imgLower, "scratch")

			// Detect base image from image name
			if strings.Contains(imgLower, "distroless") {
				baseImg = "distroless"
				result.Summary.DistrolessCount++
			} else if strings.Contains(imgLower, "alpine") {
				baseImg = "alpine"
				result.Summary.AlpineCount++
			} else if strings.Contains(imgLower, "ubuntu") {
				baseImg = "ubuntu"
				result.Summary.UbuntuCount++
			} else if strings.Contains(imgLower, "debian") || strings.Contains(imgLower, "bookworm") ||
				strings.Contains(imgLower, "bullseye") || strings.Contains(imgLower, "buster") {
				baseImg = "debian"
				result.Summary.DebianCount++
			} else if strings.Contains(imgLower, "slim") {
				baseImg = "slim"
				result.Summary.SlimCount++
			} else {
				result.Summary.UnknownBase++
			}

			if isLatest {
				result.Summary.LatestTagCount++
			}
			if isSlim {
				result.Summary.SlimCount++
			}

			entry := ImageBaseEntry1931{
				Image: img, BaseImage: baseImg, IsLatest: isLatest, IsSlim: isSlim,
			}
			result.Images = append(result.Images, entry)
			result.Summary.TotalImages++

			// Risks
			if isLatest {
				result.Risks = append(result.Risks, ImageBaseRisk1931{
					Image: img, RiskType: "latest-tag", Severity: "medium",
					Detail: "Uses :latest tag — base image may change unpredictably",
				})
				score -= 2
			}
			if baseImg == "unknown" {
				result.Risks = append(result.Risks, ImageBaseRisk1931{
					Image: img, RiskType: "unknown-base", Severity: "low",
					Detail: "Base image unknown — scan with trivy/grype for CVEs",
				})
			}
			if baseImg == "debian" || baseImg == "ubuntu" {
				if !isSlim {
					result.Risks = append(result.Risks, ImageBaseRisk1931{
						Image: img, RiskType: "large-base", Severity: "low",
						Detail: fmt.Sprintf("Full %s base — use %s-slim to reduce attack surface", baseImg, baseImg),
					})
					score -= 1
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DistrolessCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d distroless images — best practice for minimal attack surface", result.Summary.DistrolessCount))
	}
	if result.Summary.LatestTagCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images use :latest — pin to specific versions", result.Summary.LatestTagCount))
	}
	if result.Summary.UnknownBase > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images with unknown base — scan for vulnerabilities", result.Summary.UnknownBase))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
