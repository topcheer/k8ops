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

// ImagePullAuditResult is the image pull policy & secret management audit.
type ImagePullAuditResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         ImagePullSummary       `json:"summary"`
	ByNamespace     []ImagePullNSStat      `json:"byNamespace"`
	PolicyIssues    []ImagePullPolicyIssue `json:"policyIssues"`
	SecretIssues    []ImagePullSecretIssue `json:"secretIssues"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// ImagePullSummary aggregates image pull statistics.
type ImagePullSummary struct {
	TotalPods           int `json:"totalPods"`
	TotalContainers     int `json:"totalContainers"`
	AlwaysPull          int `json:"alwaysPull"`    // imagePullPolicy: Always
	IfNotPresent        int `json:"ifNotPresent"`  // imagePullPolicy: IfNotPresent
	NeverPull           int `json:"neverPull"`     // imagePullPolicy: Never
	MissingPolicy       int `json:"missingPolicy"` // no imagePullPolicy set
	PodsWithPullSecrets int `json:"podsWithPullSecrets"`
	PodsNoPullSecrets   int `json:"podsNoPullSecrets"` // pods using private images but no secrets
	StaleSecrets        int `json:"staleSecrets"`      // secrets not referenced by any pod
	DuplicateSecrets    int `json:"duplicateSecrets"`  // same dockerconfigjson in multiple secrets
}

// ImagePullNSStat shows per-namespace image pull stats.
type ImagePullNSStat struct {
	Namespace      string `json:"namespace"`
	PodCount       int    `json:"podCount"`
	AlwaysPull     int    `json:"alwaysPull"`
	IfNotPresent   int    `json:"ifNotPresent"`
	NeverPull      int    `json:"neverPull"`
	MissingPolicy  int    `json:"missingPolicy"`
	HasPullSecrets bool   `json:"hasPullSecrets"`
	RiskLevel      string `json:"riskLevel"`
}

// ImagePullPolicyIssue describes an image pull policy issue.
type ImagePullPolicyIssue struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Policy    string `json:"currentPolicy"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// ImagePullSecretIssue describes an image pull secret issue.
type ImagePullSecretIssue struct {
	Namespace  string `json:"namespace"`
	SecretName string `json:"secretName"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"`
}

// handleImagePullAudit audits image pull policy & secret management.
// GET /api/deployment/image-pull-audit
func (s *Server) handleImagePullAudit(w http.ResponseWriter, r *http.Request) {
	result := ImagePullAuditResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// Known public registries that don't need pull secrets
	publicRegistries := map[string]bool{
		"registry.k8s.io": true,
		"k8s.gcr.io":      true,
		"gcr.io":          true,
		"docker.io":       true,
		"quay.io":         true,
		"ghcr.io":         true,
		"public.ecr.aws":  true,
	}

	// 1. Scan all pods for imagePullPolicy and imagePullSecrets
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsStats := make(map[string]*ImagePullNSStat)

		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}

			nsStat, ok := nsStats[pod.Namespace]
			if !ok {
				nsStat = &ImagePullNSStat{Namespace: pod.Namespace}
				nsStats[pod.Namespace] = nsStat
			}
			nsStat.PodCount++
			result.Summary.TotalPods++

			// Check imagePullSecrets
			hasPullSecrets := len(pod.Spec.ImagePullSecrets) > 0
			if hasPullSecrets {
				result.Summary.PodsWithPullSecrets++
				nsStat.HasPullSecrets = true
			} else {
				result.Summary.PodsNoPullSecrets++
			}

			// Check each container's imagePullPolicy
			allContainers := append([]corev1.Container{}, pod.Spec.Containers...)
			allContainers = append(allContainers, pod.Spec.InitContainers...)

			for _, c := range allContainers {
				result.Summary.TotalContainers++
				policy := string(c.ImagePullPolicy)
				isPrivateImage := true

				// Check if image is from a public registry
				for reg := range publicRegistries {
					if strings.HasPrefix(strings.ToLower(c.Image), reg) || strings.Contains(c.Image, reg+"/") {
						isPrivateImage = false
						break
					}
				}
				if !strings.Contains(c.Image, "/") {
					// Official Docker Hub library images (e.g. "nginx:1.21")
					isPrivateImage = false
				}

				switch policy {
				case string(corev1.PullAlways):
					result.Summary.AlwaysPull++
					nsStat.AlwaysPull++
				case string(corev1.PullIfNotPresent):
					result.Summary.IfNotPresent++
					nsStat.IfNotPresent++
				case string(corev1.PullNever):
					result.Summary.NeverPull++
					nsStat.NeverPull++
					// Never pull is risky
					result.PolicyIssues = append(result.PolicyIssues, ImagePullPolicyIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						Container: c.Name,
						Image:     c.Image,
						Policy:    policy,
						Issue:     "imagePullPolicy: Never prevents pulling updated images",
						Severity:  "high",
					})
				default:
					result.Summary.MissingPolicy++
					nsStat.MissingPolicy++
					// Missing policy defaults to Always for :latest, IfNotPresent otherwise
					if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":") {
						result.PolicyIssues = append(result.PolicyIssues, ImagePullPolicyIssue{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							Container: c.Name,
							Image:     c.Image,
							Policy:    "(default: Always)",
							Issue:     "No explicit imagePullPolicy set; defaults to Always for :latest tags",
							Severity:  "low",
						})
					}
				}

				// Check if private image without pull secrets
				if isPrivateImage && !hasPullSecrets {
					result.SecretIssues = append(result.SecretIssues, ImagePullSecretIssue{
						Namespace:  pod.Namespace,
						SecretName: "",
						Issue:      fmt.Sprintf("Private image %s used without imagePullSecrets", c.Image),
						Severity:   "high",
					})
				}

				// Always pull on production is wasteful
				if policy == string(corev1.PullAlways) && !strings.HasSuffix(c.Image, ":latest") {
					result.PolicyIssues = append(result.PolicyIssues, ImagePullPolicyIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						Container: c.Name,
						Image:     c.Image,
						Policy:    policy,
						Issue:     "imagePullPolicy: Always on pinned image wastes bandwidth; consider IfNotPresent",
						Severity:  "low",
					})
				}
			}
		}

		// Build namespace stats
		for _, nsStat := range nsStats {
			nsStat.RiskLevel = "low"
			if nsStat.MissingPolicy > 0 || nsStat.NeverPull > 0 {
				nsStat.RiskLevel = "high"
			} else if nsStat.AlwaysPull > nsStat.IfNotPresent {
				nsStat.RiskLevel = "medium"
			}
			result.ByNamespace = append(result.ByNamespace, *nsStat)
		}
		sort.Slice(result.ByNamespace, func(i, j int) bool {
			return result.ByNamespace[i].RiskLevel > result.ByNamespace[j].RiskLevel
		})
	}

	// 2. Scan dockerconfigjson secrets for stale/duplicate
	secrets, err := rc.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		// Track which secrets are referenced by pods
		referencedSecrets := make(map[string]bool)
		if pods != nil {
			for _, pod := range pods.Items {
				for _, ips := range pod.Spec.ImagePullSecrets {
					key := fmt.Sprintf("%s/%s", pod.Namespace, ips.Name)
					referencedSecrets[key] = true
				}
			}
		}

		// Track dockerconfigjson content for duplicate detection
		secretContentMap := make(map[string][]string) // content -> []secret names

		for _, secret := range secrets.Items {
			if secret.Type != corev1.SecretTypeDockerConfigJson {
				continue
			}
			if systemNamespaces[secret.Namespace] {
				continue
			}

			key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
			if !referencedSecrets[key] {
				result.Summary.StaleSecrets++
				result.SecretIssues = append(result.SecretIssues, ImagePullSecretIssue{
					Namespace:  secret.Namespace,
					SecretName: secret.Name,
					Issue:      "Docker config secret not referenced by any pod",
					Severity:   "low",
				})
			}

			// Check for duplicates
			content := string(secret.Data[corev1.DockerConfigJsonKey])
			if content != "" {
				secretContentMap[content] = append(secretContentMap[content], key)
			}
		}

		for content, names := range secretContentMap {
			if len(names) > 1 {
				result.Summary.DuplicateSecrets++
				result.SecretIssues = append(result.SecretIssues, ImagePullSecretIssue{
					Namespace:  "",
					SecretName: strings.Join(names, ", "),
					Issue:      fmt.Sprintf("Duplicate dockerconfigjson found in %d secrets", len(names)),
					Severity:   "low",
				})
			}
			_ = content
		}
	}

	// Sort issues
	sort.Slice(result.PolicyIssues, func(i, j int) bool {
		return result.PolicyIssues[i].Severity > result.PolicyIssues[j].Severity
	})
	sort.Slice(result.SecretIssues, func(i, j int) bool {
		return result.SecretIssues[i].Severity > result.SecretIssues[j].Severity
	})

	// Recommendations
	if result.Summary.NeverPull > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Fix %d containers with imagePullPolicy: Never (prevents image updates)", result.Summary.NeverPull))
	}
	if result.Summary.MissingPolicy > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Set explicit imagePullPolicy on %d containers", result.Summary.MissingPolicy))
	}
	if result.Summary.PodsNoPullSecrets > 0 && result.Summary.TotalPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods may need imagePullSecrets for private registry access", result.Summary.PodsNoPullSecrets))
	}
	if result.Summary.StaleSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Clean up %d unreferenced dockerconfigjson secrets", result.Summary.StaleSecrets))
	}
	if result.Summary.DuplicateSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Consolidate %d duplicate dockerconfigjson secrets", result.Summary.DuplicateSecrets))
	}

	// Health score
	score := 100
	score -= result.Summary.NeverPull * 10
	score -= result.Summary.MissingPolicy * 2
	score -= result.Summary.StaleSecrets * 2
	score -= result.Summary.DuplicateSecrets * 1
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
