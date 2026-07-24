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
// v19.55 — Security Dimension (Round 12)
// 1. Pod Security Standard Violation — PSA enforcement audit
// 2. Service Account Automount Risk — token automount exposure
// 3. Image Registry Trust — allowed/denied registry analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Security Standard Violation
// ---------------------------------------------------------------

type PSAViolationResult1955 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PSAViolationSummary1955 `json:"summary"`
	Violations      []PSAViolationEntry1955 `json:"violations"`
	ByNS            []PSAViolationNS1955    `json:"byNamespace"`
	Recommendations []string                `json:"recommendations"`
}

type PSAViolationSummary1955 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	EnforcedNS      int `json:"enforcedNamespaces"`
	AuditNS         int `json:"auditOnlyNamespaces"`
	NoPSANS         int `json:"noPSANamespaces"`
	ViolationCount  int `json:"violationCount"`
	CriticalCount   int `json:"criticalCount"`
}

type PSAViolationEntry1955 struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

type PSAViolationNS1955 struct {
	Namespace  string `json:"namespace"`
	PSALevel   string `json:"psaLevel"`
	Violations int    `json:"violationCount"`
}

func (s *Server) handlePSAViolation(w http.ResponseWriter, r *http.Request) {
	result := PSAViolationResult1955{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Check PSA enforcement per namespace
	nsPSA := make(map[string]string)
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		level := "none"
		if enforce := ns.Labels["pod-security.kubernetes.io/enforce"]; enforce != "" {
			level = enforce
			result.Summary.EnforcedNS++
		} else if audit := ns.Labels["pod-security.kubernetes.io/audit"]; audit != "" {
			level = "audit:" + audit
			result.Summary.AuditNS++
		} else {
			result.Summary.NoPSANS++
		}
		nsPSA[ns.Name] = level
	}

	nsViolations := make(map[string]int)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		var violations []PSAViolationEntry1955

		// Check privileged
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				violations = append(violations, PSAViolationEntry1955{
					Namespace: pod.Namespace, PodName: pod.Name,
					Violation: "privileged container", Severity: "critical",
				})
				result.Summary.CriticalCount++
			}
		}

		// Check hostNetwork/hostPID
		if pod.Spec.HostNetwork {
			violations = append(violations, PSAViolationEntry1955{
				Namespace: pod.Namespace, PodName: pod.Name,
				Violation: "hostNetwork", Severity: "high",
			})
		}
		if pod.Spec.HostPID {
			violations = append(violations, PSAViolationEntry1955{
				Namespace: pod.Namespace, PodName: pod.Name,
				Violation: "hostPID", Severity: "high",
			})
		}

		// Check hostPath volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				violations = append(violations, PSAViolationEntry1955{
					Namespace: pod.Namespace, PodName: pod.Name,
					Violation: fmt.Sprintf("hostPath:%s", vol.HostPath.Path), Severity: "medium",
				})
			}
		}

		for _, v := range violations {
			result.Summary.ViolationCount++
			nsViolations[v.Namespace]++
			if len(result.Violations) < 100 {
				result.Violations = append(result.Violations, v)
			}
			if v.Severity == "critical" {
				score -= 5
			} else if v.Severity == "high" {
				score -= 3
			} else {
				score -= 1
			}
		}
	}

	for ns, level := range nsPSA {
		result.ByNS = append(result.ByNS, PSAViolationNS1955{
			Namespace: ns, PSALevel: level, Violations: nsViolations[ns],
		})
	}

	if result.Summary.NoPSANS > 0 {
		score -= 2
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.NoPSANS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without PSA labels — add enforce:restricted", result.Summary.NoPSANS))
	}
	if result.Summary.ViolationCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PSA violations (%d critical) — enforce Pod Security Standards", result.Summary.ViolationCount, result.Summary.CriticalCount))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Account Automount Risk
// ---------------------------------------------------------------

type AutoMountResult1955 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         AutoMountSummary1955 `json:"summary"`
	AtRiskPods      []AutoMountEntry1955 `json:"atRiskPods"`
	ByNS            []AutoMountNS1955    `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type AutoMountSummary1955 struct {
	TotalPods        int `json:"totalPods"`
	WithAutoMount    int `json:"withAutomount"`
	WithoutAutoMount int `json:"withoutAutomount"`
	ExplicitFalse    int `json:"explicitAutomountFalse"`
	UsingDefaultSA   int `json:"usingDefaultSA"`
}

type AutoMountEntry1955 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	SAName    string `json:"serviceAccount"`
	IsDefault bool   `json:"isDefaultSA"`
}

type AutoMountNS1955 struct {
	Namespace   string `json:"namespace"`
	AtRiskCount int    `json:"atRiskCount"`
}

func (s *Server) handleAutoMountRisk(w http.ResponseWriter, r *http.Request) {
	result := AutoMountResult1955{ScannedAt: time.Now()}
	score := 100
	nsRisk := make(map[string]int)

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		automount := true
		if pod.Spec.AutomountServiceAccountToken != nil {
			automount = *pod.Spec.AutomountServiceAccountToken
			if !automount {
				result.Summary.ExplicitFalse++
			}
		}

		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		isDefault := saName == "default"

		if automount {
			result.Summary.WithAutoMount++
			if isDefault {
				result.Summary.UsingDefaultSA++
				result.AtRiskPods = append(result.AtRiskPods, AutoMountEntry1955{
					PodName: pod.Name, Namespace: pod.Namespace,
					SAName: saName, IsDefault: true,
				})
				nsRisk[pod.Namespace]++
				score -= 1
			}
		} else {
			result.Summary.WithoutAutoMount++
		}
	}

	for ns, c := range nsRisk {
		result.ByNS = append(result.ByNS, AutoMountNS1955{Namespace: ns, AtRiskCount: c})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.UsingDefaultSA > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods using default SA with automount — create dedicated SAs", result.Summary.UsingDefaultSA))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d pods with automount enabled (%d explicit false)", result.Summary.WithAutoMount, result.Summary.TotalPods, result.Summary.ExplicitFalse))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Image Registry Trust
// ---------------------------------------------------------------

type RegistryTrustResult1955 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         RegistryTrustSummary1955     `json:"summary"`
	Registries      []RegistryTrustEntry1955     `json:"registries"`
	Untrusted       []RegistryTrustUntrusted1955 `json:"untrusted"`
	Recommendations []string                     `json:"recommendations"`
}

type RegistryTrustSummary1955 struct {
	TotalImages      int `json:"totalImages"`
	UniqueRegistries int `json:"uniqueRegistries"`
	TrustedCount     int `json:"trustedRegistryCount"`
	UntrustedCount   int `json:"untrustedRegistryCount"`
	DockerHubCount   int `json:"dockerHubCount"`
	PrivateCount     int `json:"privateRegistryCount"`
}

type RegistryTrustEntry1955 struct {
	Registry   string `json:"registry"`
	ImageCount int    `json:"imageCount"`
	IsTrusted  bool   `json:"isTrusted"`
}

type RegistryTrustUntrusted1955 struct {
	Image    string `json:"image"`
	Registry string `json:"registry"`
}

func (s *Server) handleRegistryTrust(w http.ResponseWriter, r *http.Request) {
	result := RegistryTrustResult1955{ScannedAt: time.Now()}
	score := 100

	trustedRegistries := map[string]bool{
		"docker.io": true, "ghcr.io": true, "gcr.io": true,
		"registry.k8s.io": true, "k8s.gcr.io": true,
		"quay.io": true, "mcr.microsoft.com": true,
		"public.ecr.aws": true, "registry.iot2.win": true,
	}

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	registryImages := make(map[string]map[string]bool)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			img := c.Image

			registry := "docker.io"
			if strings.Contains(img, "/") {
				parts := strings.SplitN(img, "/", 2)
				if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
					registry = parts[0]
				}
			}

			if registryImages[registry] == nil {
				registryImages[registry] = make(map[string]bool)
			}
			registryImages[registry][img] = true
		}
	}

	for registry, images := range registryImages {
		result.Summary.UniqueRegistries++
		isTrusted := trustedRegistries[registry]
		imgCount := len(images)

		if isTrusted {
			result.Summary.TrustedCount++
		} else {
			result.Summary.UntrustedCount++
			for img := range images {
				if len(result.Untrusted) < 50 {
					result.Untrusted = append(result.Untrusted, RegistryTrustUntrusted1955{
						Image: img, Registry: registry,
					})
				}
			}
			score -= 3
		}

		if registry == "docker.io" {
			result.Summary.DockerHubCount++
		}
		if strings.Contains(registry, ".") && !isTrusted {
			result.Summary.PrivateCount++
		}

		result.Registries = append(result.Registries, RegistryTrustEntry1955{
			Registry: registry, ImageCount: imgCount, IsTrusted: isTrusted,
		})
	}

	sort.Slice(result.Registries, func(i, j int) bool { return result.Registries[i].ImageCount > result.Registries[j].ImageCount })

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.UntrustedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d untrusted registries — add admission policy to restrict", result.Summary.UntrustedCount))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d registries (%d trusted, %d untrusted), %d total images",
		result.Summary.UniqueRegistries, result.Summary.TrustedCount, result.Summary.UntrustedCount, result.Summary.TotalImages))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
