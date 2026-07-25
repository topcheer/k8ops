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
// v19.79 — Security Dimension (Round 16)
// 1. SA Privilege Scope — service account permission breadth analysis
// 2. Token Audit Trail — SA token mount & usage pattern tracker
// 3. Secret Volume Exposure — secrets mounted as volumes vs env vars
// ============================================================

// ---------------------------------------------------------------
// 1. SA Privilege Scope
// ---------------------------------------------------------------

type SAPrivScopeResult1979 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SAPrivScopeSummary1979 `json:"summary"`
	BroadSA         []SAPrivScopeEntry1979 `json:"broadScopeSAs"`
	Recommendations []string               `json:"recommendations"`
}

type SAPrivScopeSummary1979 struct {
	TotalSAs      int `json:"totalServiceAccounts"`
	BroadScope    int `json:"broadScopeSAs"`
	ClusterScoped int `json:"clusterScopedBindings"`
	WithWildcard  int `json:"withWildcardPermissions"`
	DefaultSAs    int `json:"defaultSAsInUse"`
}

type SAPrivScopeEntry1979 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Bindings  int    `json:"bindingCount"`
	IsCluster bool   `json:"isClusterScoped"`
}

func (s *Server) handleSAPrivScope(w http.ResponseWriter, r *http.Request) {
	result := SAPrivScopeResult1979{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	// Build SA binding counts
	saBindings := make(map[string]int) // ns/name -> count
	saClusterScoped := make(map[string]bool)

	for _, rb := range rbList.Items {
		for _, sub := range rb.Subjects {
			if sub.Kind == "ServiceAccount" {
				key := sub.Namespace + "/" + sub.Name
				saBindings[key]++
			}
		}
	}
	for _, crb := range crbList.Items {
		for _, sub := range crb.Subjects {
			if sub.Kind == "ServiceAccount" {
				key := sub.Namespace + "/" + sub.Name
				saBindings[key]++
				saClusterScoped[key] = true
			}
		}
	}

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++

		key := sa.Namespace + "/" + sa.Name
		bindCount := saBindings[key]
		isCluster := saClusterScoped[key]

		if sa.Name == "default" && bindCount > 0 {
			result.Summary.DefaultSAs++
		}

		if isCluster {
			result.Summary.ClusterScoped++
			result.BroadSA = append(result.BroadSA, SAPrivScopeEntry1979{
				Name: sa.Name, Namespace: sa.Namespace,
				Bindings: bindCount, IsCluster: true,
			})
			score -= 3
		} else if bindCount > 3 {
			result.Summary.BroadScope++
			result.BroadSA = append(result.BroadSA, SAPrivScopeEntry1979{
				Name: sa.Name, Namespace: sa.Namespace,
				Bindings: bindCount, IsCluster: false,
			})
			score -= 1
		}
	}

	sort.Slice(result.BroadSA, func(i, j int) bool {
		return result.BroadSA[i].Bindings > result.BroadSA[j].Bindings
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs: %d broad scope, %d cluster-scoped, %d default SAs in use", result.Summary.TotalSAs, result.Summary.BroadScope, result.Summary.ClusterScoped, result.Summary.DefaultSAs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Token Audit Trail
// ---------------------------------------------------------------

type TokenAuditResult1979 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         TokenAuditSummary1979 `json:"summary"`
	PodsWithTokens  []TokenAuditEntry1979 `json:"podsWithTokens"`
	Recommendations []string              `json:"recommendations"`
}

type TokenAuditSummary1979 struct {
	TotalPods       int `json:"totalPods"`
	WithAutoMount   int `json:"withAutomountToken"`
	ExplicitDisable int `json:"explicitlyDisabled"`
	UsingDefaultSA  int `json:"usingDefaultSAToken"`
}

type TokenAuditEntry1979 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	SAName    string `json:"serviceAccount"`
	Automount bool   `json:"automount"`
	Issue     string `json:"issue"`
}

func (s *Server) handleTokenAuditTrail(w http.ResponseWriter, r *http.Request) {
	result := TokenAuditResult1979{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		automount := true
		if pod.Spec.AutomountServiceAccountToken != nil {
			automount = *pod.Spec.AutomountServiceAccountToken
		}

		if automount {
			result.Summary.WithAutoMount++
		} else {
			result.Summary.ExplicitDisable++
		}

		saName := pod.Spec.ServiceAccountName
		if saName == "" || saName == "default" {
			result.Summary.UsingDefaultSA++

			entry := TokenAuditEntry1979{
				Pod: pod.Name, Namespace: pod.Namespace,
				SAName: saName, Automount: automount,
			}
			if automount {
				entry.Issue = "Default SA token automounted — broad permissions"
				score -= 2
			} else {
				entry.Issue = "Using default SA but token disabled (good)"
			}
			result.PodsWithTokens = append(result.PodsWithTokens, entry)
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d automount, %d disabled, %d using default SA", result.Summary.TotalPods, result.Summary.WithAutoMount, result.Summary.ExplicitDisable, result.Summary.UsingDefaultSA))
	if result.Summary.UsingDefaultSA > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods using default SA — create dedicated service accounts", result.Summary.UsingDefaultSA))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Secret Volume Exposure
// ---------------------------------------------------------------

type SecretVolResult1979 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SecretVolSummary1979 `json:"summary"`
	ExposedSecrets  []SecretVolEntry1979 `json:"exposedSecrets"`
	Recommendations []string             `json:"recommendations"`
}

type SecretVolSummary1979 struct {
	TotalPods      int `json:"totalPods"`
	VolumeMounts   int `json:"secretVolumeMounts"`
	EnvVarRefs     int `json:"secretEnvVarRefs"`
	AllKeysMounted int `json:"allKeysMountedCount"`
	WritableMount  int `json:"writableMounts"`
}

type SecretVolEntry1979 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Secret    string `json:"secretName"`
	MountType string `json:"mountType"`
	ReadOnly  bool   `json:"readOnly"`
}

func (s *Server) handleSecretVolExposure(w http.ResponseWriter, r *http.Request) {
	result := SecretVolResult1979{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				result.Summary.VolumeMounts++
				readOnly := true
				if vol.Secret.Optional != nil && !*vol.Secret.Optional {
					// required secret
				}

				entry := SecretVolEntry1979{
					Pod: pod.Name, Namespace: pod.Namespace,
					Secret: vol.Secret.SecretName, MountType: "volume",
					ReadOnly: readOnly,
				}

				// Check if secret items are specified (not all keys)
				if len(vol.Secret.Items) == 0 {
					result.Summary.AllKeysMounted++
					entry.ReadOnly = false
				}

				result.ExposedSecrets = append(result.ExposedSecrets, entry)
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						result.Summary.VolumeMounts++
						result.ExposedSecrets = append(result.ExposedSecrets, SecretVolEntry1979{
							Pod: pod.Name, Namespace: pod.Namespace,
							Secret: src.Secret.Name, MountType: "projected-volume",
							ReadOnly: true,
						})
					}
				}
			}
		}

		// Check env var refs
		for _, c := range pod.Spec.Containers {
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					result.Summary.EnvVarRefs++
					result.ExposedSecrets = append(result.ExposedSecrets, SecretVolEntry1979{
						Pod: pod.Name, Namespace: pod.Namespace,
						Secret: ef.SecretRef.Name, MountType: "envFrom",
						ReadOnly: true,
					})
				}
			}
		}
	}

	if result.Summary.AllKeysMounted > 10 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d volume mounts, %d env refs, %d all-keys mounted", result.Summary.TotalPods, result.Summary.VolumeMounts, result.Summary.EnvVarRefs, result.Summary.AllKeysMounted))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
