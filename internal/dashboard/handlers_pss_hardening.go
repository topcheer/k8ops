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

// PSSHardeningResult is the pod security standards enforcement gap & workload hardening audit.
type PSSHardeningResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          PSSHardeningSummary `json:"summary"`
	PrivilegedPods   []PSSPodIssue       `json:"privilegedPods"`
	NoSeccompPods    []PSSPodIssue       `json:"noSeccompPods"`
	NoAppArmorPods   []PSSPodIssue       `json:"noAppArmorPods"`
	NoReadOnlyFs     []PSSPodIssue       `json:"noReadOnlyFs"`
	HostNamespace    []PSSPodIssue       `json:"hostNamespacePods"`
	CapabilityIssues []PSSPodIssue       `json:"capabilityIssues"`
	Recommendations  []string            `json:"recommendations"`
	HealthScore      int                 `json:"healthScore"`
}

// PSSHardeningSummary aggregates PSS hardening statistics.
type PSSHardeningSummary struct {
	TotalPods             int `json:"totalPods"`
	TotalContainers       int `json:"totalContainers"`
	PrivilegedContainers  int `json:"privilegedContainers"`
	PodsNoSeccomp         int `json:"podsNoSeccomp"`
	PodsNoAppArmor        int `json:"podsNoAppArmor"`
	PodsNoReadOnlyRootFS  int `json:"podsNoReadOnlyRootFS"`
	PodsWithHostPID       int `json:"podsWithHostPID"`
	PodsWithHostNetwork   int `json:"podsWithHostNetwork"`
	PodsWithHostIPC       int `json:"podsWithHostIPC"`
	ContainersWithAddCaps int `json:"containersWithAddCaps"`
	ContainersWithAllCaps int `json:"containersWithAllCaps"`
	PodsNoDropAllCaps     int `json:"podsNoDropAllCaps"`
	PrivEscContainers     int `json:"privEscContainers"`
	HardeningScore        int `json:"hardeningScore"`
}

// PSSPodIssue describes a pod with a security hardening issue.
type PSSPodIssue struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	OwnerKind string `json:"ownerKind"`
	OwnerName string `json:"ownerName"`
	Container string `json:"container,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handlePSSHardening audits pod security standards enforcement gaps & workload hardening.
// GET /api/security/pss-hardening
func (s *Server) handlePSSHardening(w http.ResponseWriter, r *http.Request) {
	result := PSSHardeningResult{
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

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
				continue
			}
			result.Summary.TotalPods++

			ownerKind := getOwnerKind(pod.OwnerReferences)
			ownerName := getOwnerName(pod.OwnerReferences)

			// Check pod-level security context
			psc := pod.Spec.SecurityContext
			hasSeccomp := false
			hasAppArmor := false

			if psc != nil {
				if psc.SeccompProfile != nil {
					hasSeccomp = true
				}
				if psc.AppArmorProfile != nil {
					hasAppArmor = true
				}
			}

			// Check host namespaces (on PodSpec, not SecurityContext)
			if pod.Spec.HostPID {
				result.Summary.PodsWithHostPID++
				result.HostNamespace = append(result.HostNamespace, PSSPodIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Issue:     "hostPID: true — pod can see host processes",
					Severity:  "high",
				})
			}
			if pod.Spec.HostNetwork {
				result.Summary.PodsWithHostNetwork++
				result.HostNamespace = append(result.HostNamespace, PSSPodIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Issue:     "hostNetwork: true — pod uses host network namespace",
					Severity:  "high",
				})
			}
			if pod.Spec.HostIPC {
				result.Summary.PodsWithHostIPC++
				result.HostNamespace = append(result.HostNamespace, PSSPodIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Issue:     "hostIPC: true — pod uses host IPC namespace",
					Severity:  "high",
				})
			}

			// Check each container's security context
			allContainers := append([]corev1.Container{}, pod.Spec.Containers...)
			allContainers = append(allContainers, pod.Spec.InitContainers...)

			podHasNoSeccomp := !hasSeccomp
			podHasNoAppArmor := !hasAppArmor
			podDroppedAllCaps := false

			for _, c := range allContainers {
				result.Summary.TotalContainers++
				sc := c.SecurityContext

				if sc == nil {
					// No security context at all
					result.Summary.PodsNoReadOnlyRootFS++
					result.NoReadOnlyFs = append(result.NoReadOnlyFs, PSSPodIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						OwnerKind: ownerKind,
						OwnerName: ownerName,
						Container: c.Name,
						Issue:     "No security context — readOnlyRootFilesystem not set",
						Severity:  "low",
					})
					if podHasNoSeccomp {
						result.Summary.PodsNoSeccomp++
						result.NoSeccompPods = append(result.NoSeccompPods, PSSPodIssue{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							OwnerKind: ownerKind,
							OwnerName: ownerName,
							Container: c.Name,
							Issue:     "No seccomp profile set — default to Unconfined",
							Severity:  "medium",
						})
						podHasNoSeccomp = false // avoid duplicates per pod
					}
					continue
				}

				// Check privileged
				if sc.Privileged != nil && *sc.Privileged {
					result.Summary.PrivilegedContainers++
					result.PrivilegedPods = append(result.PrivilegedPods, PSSPodIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						OwnerKind: ownerKind,
						OwnerName: ownerName,
						Container: c.Name,
						Issue:     "Container runs in privileged mode",
						Severity:  "critical",
					})
				}

				// Check allowPrivilegeEscalation
				if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
					result.Summary.PrivEscContainers++
					result.PrivilegedPods = append(result.PrivilegedPods, PSSPodIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						OwnerKind: ownerKind,
						OwnerName: ownerName,
						Container: c.Name,
						Issue:     "allowPrivilegeEscalation: true",
						Severity:  "high",
					})
				}

				// Check readOnlyRootFilesystem
				if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
					result.Summary.PodsNoReadOnlyRootFS++
					result.NoReadOnlyFs = append(result.NoReadOnlyFs, PSSPodIssue{
						Namespace: pod.Namespace,
						PodName:   pod.Name,
						OwnerKind: ownerKind,
						OwnerName: ownerName,
						Container: c.Name,
						Issue:     "readOnlyRootFilesystem not set — writable root filesystem",
						Severity:  "low",
					})
				}

				// Check seccomp at container level
				if sc.SeccompProfile != nil {
					podHasNoSeccomp = false
				}

				// Check capabilities
				if sc.Capabilities != nil {
					if len(sc.Capabilities.Add) > 0 {
						result.Summary.ContainersWithAddCaps++
						addCaps := make([]string, len(sc.Capabilities.Add))
						for i, cap := range sc.Capabilities.Add {
							addCaps[i] = string(cap)
						}
						result.CapabilityIssues = append(result.CapabilityIssues, PSSPodIssue{
							Namespace: pod.Namespace,
							PodName:   pod.Name,
							OwnerKind: ownerKind,
							OwnerName: ownerName,
							Container: c.Name,
							Issue:     fmt.Sprintf("Added capabilities: %s", strings.Join(addCaps, ",")),
							Severity:  "medium",
						})
					}
					// Check for ALL caps
					for _, cap := range sc.Capabilities.Add {
						if cap == "ALL" {
							result.Summary.ContainersWithAllCaps++
							result.CapabilityIssues = append(result.CapabilityIssues, PSSPodIssue{
								Namespace: pod.Namespace,
								PodName:   pod.Name,
								OwnerKind: ownerKind,
								OwnerName: ownerName,
								Container: c.Name,
								Issue:     "CAP_SYS_ADMIN (ALL capabilities added)",
								Severity:  "critical",
							})
						}
					}
					// Check if dropped ALL
					for _, cap := range sc.Capabilities.Drop {
						if cap == "ALL" {
							podDroppedAllCaps = true
						}
					}
				}
			}

			// Record missing seccomp/apparmor
			if podHasNoSeccomp {
				result.Summary.PodsNoSeccomp++
				result.NoSeccompPods = append(result.NoSeccompPods, PSSPodIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Issue:     "No seccomp profile set at pod or container level",
					Severity:  "medium",
				})
			}
			if podHasNoAppArmor {
				result.Summary.PodsNoAppArmor++
				result.NoAppArmorPods = append(result.NoAppArmorPods, PSSPodIssue{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Issue:     "No AppArmor profile set",
					Severity:  "low",
				})
			}

			if !podDroppedAllCaps {
				result.Summary.PodsNoDropAllCaps++
			}
		}
	}

	// Sort results
	sortAll := func(issues []PSSPodIssue) {
		sort.Slice(issues, func(i, j int) bool {
			return issues[i].Severity > issues[j].Severity
		})
	}
	sortAll(result.PrivilegedPods)
	sortAll(result.NoSeccompPods)
	sortAll(result.HostNamespace)
	sortAll(result.CapabilityIssues)

	// Recommendations
	if result.Summary.PrivilegedContainers > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Remove privileged mode from %d containers — use specific capabilities instead", result.Summary.PrivilegedContainers))
	}
	if result.Summary.PodsNoSeccomp > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Add seccomp profiles (RuntimeDefault) to %d pods for syscall filtering", result.Summary.PodsNoSeccomp))
	}
	if result.Summary.PodsNoReadOnlyRootFS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Set readOnlyRootFilesystem on %d containers for filesystem isolation", result.Summary.PodsNoReadOnlyRootFS))
	}
	if result.Summary.ContainersWithAddCaps > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Remove unnecessary capabilities from %d containers (drop ALL, add only what's needed)", result.Summary.ContainersWithAddCaps))
	}
	if result.Summary.PrivEscContainers > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Set allowPrivilegeEscalation: false on %d containers", result.Summary.PrivEscContainers))
	}

	// Health score
	score := 100
	score -= result.Summary.PrivilegedContainers * 15
	score -= result.Summary.PrivEscContainers * 10
	score -= result.Summary.ContainersWithAllCaps * 15
	score -= result.Summary.PodsWithHostPID * 5
	score -= result.Summary.PodsWithHostNetwork * 5
	score -= result.Summary.PodsWithHostIPC * 5
	score -= result.Summary.PodsNoSeccomp * 3
	score -= result.Summary.PodsNoReadOnlyRootFS * 1
	score -= result.Summary.PodsNoDropAllCaps * 1
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Summary.HardeningScore = score

	writeJSON(w, result)
}
