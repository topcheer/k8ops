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
// v20.27 — Security Dimension (Round 24)
// 1. Secret Mount Exposure — secrets mounted in pods and accessible env vars
// 2. NetworkPolicy Bare Namespace — namespaces without any NetworkPolicy
// 3. Privileged Escalation Path — pods with privileged or CAP_SYS_ADMIN
// ============================================================

// ---------------------------------------------------------------
// 1. Secret Mount Exposure
// ---------------------------------------------------------------

type SecretMountResult2027 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SecretMountSummary2027 `json:"summary"`
	ExposedSecrets  []SecretMountEntry2027 `json:"exposedSecrets"`
	Recommendations []string               `json:"recommendations"`
}

type SecretMountSummary2027 struct {
	TotalPods       int `json:"totalPods"`
	PodsWithSecrets int `json:"podsWithSecrets"`
	EnvVarSecrets   int `json:"envVarSecrets"`
	VolumeSecrets   int `json:"volumeSecrets"`
}

type SecretMountEntry2027 struct {
	Pod          string `json:"pod"`
	Namespace    string `json:"namespace"`
	SecretName   string `json:"secretName"`
	ExposureType string `json:"exposureType"` // env-var or volume
	Container    string `json:"container"`
}

func (s *Server) handleSecretMountExposure(w http.ResponseWriter, r *http.Request) {
	result := SecretMountResult2027{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		hasSecret := false

		for _, c := range pod.Spec.Containers {
			// Check env var secrets
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					result.Summary.EnvVarSecrets++
					hasSecret = true
					result.ExposedSecrets = append(result.ExposedSecrets, SecretMountEntry2027{
						Pod: pod.Name, Namespace: pod.Namespace,
						SecretName:   env.ValueFrom.SecretKeyRef.Name,
						ExposureType: "env-var", Container: c.Name,
					})
				}
			}
			// Check envFrom secrets
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					result.Summary.EnvVarSecrets++
					hasSecret = true
					result.ExposedSecrets = append(result.ExposedSecrets, SecretMountEntry2027{
						Pod: pod.Name, Namespace: pod.Namespace,
						SecretName:   ef.SecretRef.Name,
						ExposureType: "envFrom", Container: c.Name,
					})
				}
			}
		}

		// Check volume-mounted secrets
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				result.Summary.VolumeSecrets++
				hasSecret = true
				result.ExposedSecrets = append(result.ExposedSecrets, SecretMountEntry2027{
					Pod: pod.Name, Namespace: pod.Namespace,
					SecretName:   vol.Secret.SecretName,
					ExposureType: "volume",
				})
			}
		}

		if hasSecret {
			result.Summary.PodsWithSecrets++
		}
	}

	// Score: each env-var exposure is riskier than volume
	score -= result.Summary.EnvVarSecrets * 2
	score -= result.Summary.VolumeSecrets
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.ExposedSecrets, func(i, j int) bool {
		return result.ExposedSecrets[i].Namespace < result.ExposedSecrets[j].Namespace
	})

	if score < 70 {
		result.Recommendations = append(result.Recommendations,
			"Prefer volume-mounted secrets over env vars to reduce exposure surface")
	}
	if result.Summary.EnvVarSecrets > 10 {
		result.Recommendations = append(result.Recommendations,
			"High number of env-var secrets detected — consider using a secrets manager (Vault, Sealed Secrets)")
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. NetworkPolicy Bare Namespace
// ---------------------------------------------------------------

type BareNSResult2027 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         BareNSSummary2027 `json:"summary"`
	BareNamespaces  []BareNSEntry2027 `json:"bareNamespaces"`
	Recommendations []string          `json:"recommendations"`
}

type BareNSSummary2027 struct {
	TotalNamespaces      int `json:"totalNamespaces"`
	NamespacesWithNetPol int `json:"namespacesWithNetPol"`
	BareNamespaces       int `json:"bareNamespaces"`
}

type BareNSEntry2027 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleBareNamespaceNetPol(w http.ResponseWriter, r *http.Request) {
	result := BareNSResult2027{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	netpolList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build set of namespaces with NetworkPolicies
	nsWithNetPol := make(map[string]bool)
	for _, np := range netpolList.Items {
		nsWithNetPol[np.Namespace] = true
	}

	// Count pods per namespace
	podCountPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCountPerNS[pod.Namespace]++
		}
	}

	// Check system namespaces to skip
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
		"k8ops-system": true,
	}

	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNamespaces++
		if nsWithNetPol[ns.Name] {
			result.Summary.NamespacesWithNetPol++
		} else {
			result.Summary.BareNamespaces++
			result.BareNamespaces = append(result.BareNamespaces, BareNSEntry2027{
				Namespace: ns.Name,
				PodCount:  podCountPerNS[ns.Name],
			})
			// Higher risk if namespace has many pods
			pods := podCountPerNS[ns.Name]
			if pods > 5 {
				score -= 5
			} else if pods > 0 {
				score -= 3
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.BareNamespaces, func(i, j int) bool {
		return result.BareNamespaces[i].PodCount > result.BareNamespaces[j].PodCount
	})

	if result.Summary.BareNamespaces > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have no NetworkPolicy — all traffic is unrestricted", result.Summary.BareNamespaces))
	}
	if score < 70 {
		result.Recommendations = append(result.Recommendations,
			"Apply default-deny NetworkPolicies to namespaces with workloads")
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Privileged Escalation Path
// ---------------------------------------------------------------

type PrivEscResult2027 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PrivEscSummary2027 `json:"summary"`
	PrivilegedPods  []PrivEscEntry2027 `json:"privilegedPods"`
	Recommendations []string           `json:"recommendations"`
}

type PrivEscSummary2027 struct {
	TotalContainers int `json:"totalContainers"`
	Privileged      int `json:"privileged"`
	WithSysAdmin    int `json:"withSysAdmin"`
	WithHostPID     int `json:"withHostPID"`
	WithHostNetwork int `json:"withHostNetwork"`
}

type PrivEscEntry2027 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
}

func (s *Server) handlePrivEscalationPath(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult2027{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// HostPID / HostNetwork at pod level
		if pod.Spec.HostPID {
			result.Summary.WithHostPID++
			result.PrivilegedPods = append(result.PrivilegedPods, PrivEscEntry2027{
				Pod: pod.Name, Namespace: pod.Namespace,
				Issue: "hostPID",
			})
			score -= 5
		}
		if pod.Spec.HostNetwork {
			result.Summary.WithHostNetwork++
			result.PrivilegedPods = append(result.PrivilegedPods, PrivEscEntry2027{
				Pod: pod.Name, Namespace: pod.Namespace,
				Issue: "hostNetwork",
			})
			score -= 3
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			sc := c.SecurityContext
			if sc == nil {
				continue
			}

			// Privileged container
			if sc.Privileged != nil && *sc.Privileged {
				result.Summary.Privileged++
				result.PrivilegedPods = append(result.PrivilegedPods, PrivEscEntry2027{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, Issue: "privileged",
				})
				score -= 5
			}

			// CAP_SYS_ADMIN
			if sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Add {
					if cap == "SYS_ADMIN" {
						result.Summary.WithSysAdmin++
						result.PrivilegedPods = append(result.PrivilegedPods, PrivEscEntry2027{
							Pod: pod.Name, Namespace: pod.Namespace,
							Container: c.Name, Issue: "CAP_SYS_ADMIN",
						})
						score -= 4
					}
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.PrivilegedPods, func(i, j int) bool {
		return result.PrivilegedPods[i].Namespace < result.PrivilegedPods[j].Namespace
	})

	if result.Summary.Privileged > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d privileged containers detected — remove privileged flag unless absolutely required", result.Summary.Privileged))
	}
	if result.Summary.WithSysAdmin > 0 {
		result.Recommendations = append(result.Recommendations,
			"CAP_SYS_ADMIN grants near-root access — replace with specific capabilities")
	}
	if score < 70 {
		result.Recommendations = append(result.Recommendations,
			"Enforce Pod Security Admission 'restricted' level to block privileged containers")
	}

	writeJSON(w, result)
}
