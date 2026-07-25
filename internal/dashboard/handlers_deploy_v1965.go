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
// v19.65 — Deployment Dimension (Round 14)
// 1. DNS Policy Audit — dnsPolicy & dnsConfig compliance
// 2. Pod Priority Preemption — priority class usage & preemption risk
// 3. Secret Env Reference — env var secret exposure & key mapping
// ============================================================

// ---------------------------------------------------------------
// 1. DNS Policy Audit
// ---------------------------------------------------------------

type DNSPolicyResult1965 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSPolicySummary1965 `json:"summary"`
	NonDefault      []DNSPolicyEntry1965 `json:"nonDefaultPods"`
	WithCustomDNS   []DNSPolicyEntry1965 `json:"customDNSPods"`
	Recommendations []string             `json:"recommendations"`
}

type DNSPolicySummary1965 struct {
	TotalPods       int `json:"totalPods"`
	DefaultPolicy   int `json:"clusterFirstPolicy"`
	ClusterFirst    int `json:"clusterFirstWithHostDNS"`
	DefaultNone     int `json:"nonePolicy"`
	WithCustomDNS   int `json:"withCustomDNSConfig"`
	WithNameservers int `json:"withCustomNameservers"`
}

type DNSPolicyEntry1965 struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Policy      string   `json:"dnsPolicy"`
	HasCustom   bool     `json:"hasCustomDNSConfig"`
	Nameservers []string `json:"nameservers,omitempty"`
}

func (s *Server) handleDNSPolicyAudit(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyResult1965{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		policy := string(pod.Spec.DNSPolicy)
		hasCustom := pod.Spec.DNSConfig != nil

		switch pod.Spec.DNSPolicy {
		case corev1.DNSClusterFirst:
			result.Summary.ClusterFirst++
		case corev1.DNSDefault:
			result.Summary.DefaultPolicy++
		case corev1.DNSNone:
			result.Summary.DefaultNone++
		}

		var nsList []string
		if hasCustom {
			result.Summary.WithCustomDNS++
			nsList = pod.Spec.DNSConfig.Nameservers
			if len(nsList) > 0 {
				result.Summary.WithNameservers++
			}

			entry := DNSPolicyEntry1965{
				Name: pod.Name, Namespace: pod.Namespace,
				Policy: policy, HasCustom: true,
				Nameservers: nsList,
			}
			result.WithCustomDNS = append(result.WithCustomDNS, entry)

			// Custom DNS can break service discovery
			if pod.Spec.DNSPolicy == corev1.DNSNone {
				score -= 2
			}
		}

		// Non-default policy
		if pod.Spec.DNSPolicy != corev1.DNSClusterFirst && pod.Spec.DNSPolicy != "" {
			result.NonDefault = append(result.NonDefault, DNSPolicyEntry1965{
				Name: pod.Name, Namespace: pod.Namespace,
				Policy: policy, HasCustom: hasCustom,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d ClusterFirst, %d Default, %d None", result.Summary.TotalPods, result.Summary.ClusterFirst, result.Summary.DefaultPolicy, result.Summary.DefaultNone))
	if result.Summary.WithCustomDNS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with custom DNS config — verify service discovery still works", result.Summary.WithCustomDNS))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Priority Preemption
// ---------------------------------------------------------------

type PodPriorityResult1965 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         PodPrioritySummary1965   `json:"summary"`
	PriorityClasses []PriorityClassEntry1965 `json:"priorityClasses"`
	UnassignedPods  []UnassignedPodEntry1965 `json:"unassignedPods"`
	Recommendations []string                 `json:"recommendations"`
}

type PodPrioritySummary1965 struct {
	TotalPods         int `json:"totalPods"`
	WithPriorityClass int `json:"withPriorityClass"`
	WithoutPriority   int `json:"withoutPriorityClass"`
	HighPriorityPods  int `json:"highPriorityPods"`
	SystemCritical    int `json:"systemCriticalPods"`
}

type PriorityClassEntry1965 struct {
	Name      string `json:"name"`
	Value     int32  `json:"value"`
	IsDefault bool   `json:"isDefault"`
	PodCount  int    `json:"podCount"`
}

type UnassignedPodEntry1965 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePodPriorityPreempt(w http.ResponseWriter, r *http.Request) {
	result := PodPriorityResult1965{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	pcList, _ := s.clientset.SchedulingV1().PriorityClasses().List(r.Context(), metav1.ListOptions{})

	// Build priority class map
	pcMap := make(map[string]*PriorityClassEntry1965)
	for _, pc := range pcList.Items {
		entry := &PriorityClassEntry1965{
			Name: pc.Name, Value: pc.Value,
			IsDefault: pc.GlobalDefault,
		}
		pcMap[pc.Name] = entry
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		result.Summary.TotalPods++

		pcn := pod.Spec.PriorityClassName
		if pcn != "" {
			result.Summary.WithPriorityClass++
			if entry, ok := pcMap[pcn]; ok {
				entry.PodCount++
				if entry.Value >= 1000000 {
					result.Summary.HighPriorityPods++
				}
				if strings.HasPrefix(pcn, "system-") {
					result.Summary.SystemCritical++
				}
			}
		} else {
			result.Summary.WithoutPriority++
			result.UnassignedPods = append(result.UnassignedPods, UnassignedPodEntry1965{
				Name: pod.Name, Namespace: pod.Namespace,
			})
		}
	}

	for _, pc := range pcMap {
		result.PriorityClasses = append(result.PriorityClasses, *pc)
	}
	sort.Slice(result.PriorityClasses, func(i, j int) bool {
		return result.PriorityClasses[i].Value > result.PriorityClasses[j].Value
	})

	// Score deduction for missing priority on important workloads
	if result.Summary.WithoutPriority > result.Summary.TotalPods/2 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d priority classes, %d/%d pods with priority class", len(pcMap), result.Summary.WithPriorityClass, result.Summary.TotalPods))
	if result.Summary.WithoutPriority > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods without priority class — assign for scheduling predictability", result.Summary.WithoutPriority))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Secret Env Reference
// ---------------------------------------------------------------

type SecretEnvResult1965 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SecretEnvSummary1965 `json:"summary"`
	ExposedSecrets  []SecretEnvEntry1965 `json:"exposedSecrets"`
	Recommendations []string             `json:"recommendations"`
}

type SecretEnvSummary1965 struct {
	TotalPods         int `json:"totalPods"`
	PodsWithSecretEnv int `json:"podsWithSecretEnv"`
	TotalSecretRefs   int `json:"totalSecretRefs"`
	AllKeysExposed    int `json:"allKeysExposed"`
	MissingSecrets    int `json:"missingSecretRefs"`
}

type SecretEnvEntry1965 struct {
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Secret    string   `json:"secretName"`
	Keys      []string `json:"keys"`
	AllKeys   bool     `json:"allKeysExposed"`
}

func (s *Server) handleSecretEnvRef(w http.ResponseWriter, r *http.Request) {
	result := SecretEnvResult1965{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	// Build secret existence map
	secretExists := make(map[string]bool)
	for _, sec := range secretList.Items {
		secretExists[sec.Namespace+"/"+sec.Name] = true
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasSecretEnv := false
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					hasSecretEnv = true
					result.Summary.TotalSecretRefs++
					secName := env.ValueFrom.SecretKeyRef.Name
					entry := SecretEnvEntry1965{
						Pod: pod.Name, Namespace: pod.Namespace,
						Secret: secName, Keys: []string{env.ValueFrom.SecretKeyRef.Key},
					}
					result.ExposedSecrets = append(result.ExposedSecrets, entry)

					// Check if secret exists
					if !secretExists[pod.Namespace+"/"+secName] {
						result.Summary.MissingSecrets++
						score -= 3
					}
				}
			}
			// Check envFrom
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					hasSecretEnv = true
					result.Summary.TotalSecretRefs++
					result.Summary.AllKeysExposed++
					entry := SecretEnvEntry1965{
						Pod: pod.Name, Namespace: pod.Namespace,
						Secret: ef.SecretRef.Name, AllKeys: true,
					}
					result.ExposedSecrets = append(result.ExposedSecrets, entry)

					if !secretExists[pod.Namespace+"/"+ef.SecretRef.Name] {
						result.Summary.MissingSecrets++
						score -= 3
					}
				}
			}
		}
		if hasSecretEnv {
			result.Summary.PodsWithSecretEnv++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with secret env refs, %d total refs (%d all-keys exposed)", result.Summary.PodsWithSecretEnv, result.Summary.TotalSecretRefs, result.Summary.AllKeysExposed))
	if result.Summary.MissingSecrets > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d missing secret references — pods will fail to start", result.Summary.MissingSecrets))
	}
	if result.Summary.AllKeysExposed > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers exposing ALL secret keys via envFrom — use specific keys for least privilege", result.Summary.AllKeysExposed))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
