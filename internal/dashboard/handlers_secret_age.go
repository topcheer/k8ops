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

// SecretAgeResult is the secret age & stale credential audit.
type SecretAgeResult struct {
	Timestamp       time.Time           `json:"timestamp"`
	Score           int                 `json:"score"`
	Status          string              `json:"status"`
	Summary         SecretAgeSummary    `json:"summary"`
	ByType          []SecretAgeTypeStat `json:"byType"`
	ByAge           []SecretAgeBucket   `json:"byAge"`
	StaleSecrets    []StaleSecret       `json:"staleSecrets"`
	OrphanedSecrets []OrphanedSecret    `json:"orphanedSecrets"`
	ByNamespace     []SecretNSStat      `json:"byNamespace"`
	Recommendations []string            `json:"recommendations"`
}

// SecretAgeSummary holds aggregate secret age metrics.
type SecretAgeSummary struct {
	TotalSecrets  int `json:"totalSecrets"`
	OlderThan90d  int `json:"olderThan90d"`
	OlderThan180d int `json:"olderThan180d"`
	OlderThan365d int `json:"olderThan365d"`
	OrphanedCount int `json:"orphanedCount"`
	TLSSecrets    int `json:"tlsSecrets"`
	DockerSecrets int `json:"dockerSecrets"`
	OpaqueSecrets int `json:"opaqueSecrets"`
	StaleCount    int `json:"staleCount"`
}

// SecretAgeTypeStat breaks down secrets by type.
type SecretAgeTypeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// SecretAgeBucket groups secrets by age range.
type SecretAgeBucket struct {
	Range string `json:"range"`
	Count int    `json:"count"`
	Risk  string `json:"risk"`
}

// StaleSecret is a secret that's too old.
type StaleSecret struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	AgeDays   int    `json:"ageDays"`
	Severity  string `json:"severity"`
}

// OrphanedSecret is a secret not referenced by any pod.
type OrphanedSecret struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	AgeDays   int    `json:"ageDays"`
}

// SecretNSStat aggregates secrets per namespace.
type SecretNSStat struct {
	Namespace  string `json:"namespace"`
	Total      int    `json:"total"`
	Stale      int    `json:"stale"`
	OldestDays int    `json:"oldestDays"`
}

func (s *Server) handleSecretAge(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list secrets: %v", err))
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	result := analyzeSecretAge(secrets.Items, pods.Items)
	writeJSON(w, result)
}

func analyzeSecretAge(secrets []corev1.Secret, pods []corev1.Pod) SecretAgeResult {
	now := time.Now()

	// Build set of referenced secret keys
	referenced := make(map[string]bool) // "ns/name"
	for _, pod := range pods {
		for _, ref := range pod.Spec.ImagePullSecrets {
			referenced[pod.Namespace+"/"+ref.Name] = true
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				referenced[pod.Namespace+"/"+vol.Secret.SecretName] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						referenced[pod.Namespace+"/"+src.Secret.Name] = true
					}
				}
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					referenced[pod.Namespace+"/"+env.ValueFrom.SecretKeyRef.Name] = true
				}
			}
			for _, envFrom := range c.EnvFrom {
				if envFrom.SecretRef != nil {
					referenced[pod.Namespace+"/"+envFrom.SecretRef.Name] = true
				}
			}
		}
		for _, c := range pod.Spec.InitContainers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					referenced[pod.Namespace+"/"+env.ValueFrom.SecretKeyRef.Name] = true
				}
			}
		}
	}

	summary := SecretAgeSummary{}
	typeCount := make(map[string]int)
	var staleSecrets []StaleSecret
	var orphanedSecrets []OrphanedSecret
	nsStats := make(map[string]*SecretNSStat)
	ageBuckets := map[string]int{
		"<7d":      0,
		"7-30d":    0,
		"30-90d":   0,
		"90-180d":  0,
		"180-365d": 0,
		">365d":    0,
	}

	for _, sec := range secrets {
		// Skip system namespaces
		if sec.Namespace == "kube-system" || sec.Namespace == "k8ops-system" {
			continue
		}

		age := now.Sub(sec.CreationTimestamp.Time)
		ageDays := int(age.Hours() / 24)
		typeStr := string(sec.Type)
		typeCount[typeStr]++
		summary.TotalSecrets++

		switch sec.Type {
		case corev1.SecretTypeTLS:
			summary.TLSSecrets++
		case corev1.SecretTypeDockerConfigJson:
			summary.DockerSecrets++
		case corev1.SecretTypeOpaque:
			summary.OpaqueSecrets++
		}

		// Age buckets
		switch {
		case ageDays < 7:
			ageBuckets["<7d"]++
		case ageDays < 30:
			ageBuckets["7-30d"]++
		case ageDays < 90:
			ageBuckets["30-90d"]++
		case ageDays < 180:
			ageBuckets["90-180d"]++
		case ageDays < 365:
			ageBuckets["180-365d"]++
		default:
			ageBuckets[">365d"]++
		}

		// Stale check
		severity := ""
		if ageDays >= 365 {
			summary.OlderThan365d++
			summary.StaleCount++
			severity = "critical"
		} else if ageDays >= 180 {
			summary.OlderThan180d++
			summary.StaleCount++
			severity = "high"
		} else if ageDays >= 90 {
			summary.OlderThan90d++
			summary.StaleCount++
			severity = "medium"
		}

		if severity != "" {
			staleSecrets = append(staleSecrets, StaleSecret{
				Namespace: sec.Namespace,
				Name:      sec.Name,
				Type:      typeStr,
				AgeDays:   ageDays,
				Severity:  severity,
			})
		}

		// Orphaned check
		key := sec.Namespace + "/" + sec.Name
		if !referenced[key] {
			summary.OrphanedCount++
			orphanedSecrets = append(orphanedSecrets, OrphanedSecret{
				Namespace: sec.Namespace,
				Name:      sec.Name,
				Type:      typeStr,
				AgeDays:   ageDays,
			})
		}

		// Namespace stats
		ns, ok := nsStats[sec.Namespace]
		if !ok {
			ns = &SecretNSStat{Namespace: sec.Namespace}
			nsStats[sec.Namespace] = ns
		}
		ns.Total++
		if severity != "" {
			ns.Stale++
		}
		if ageDays > ns.OldestDays {
			ns.OldestDays = ageDays
		}
	}

	// Build type stats
	var byType []SecretAgeTypeStat
	for t, c := range typeCount {
		byType = append(byType, SecretAgeTypeStat{Type: t, Count: c})
	}
	sort.Slice(byType, func(i, j int) bool { return byType[i].Count > byType[j].Count })

	// Build age buckets
	riskByBucket := map[string]string{"<7d": "low", "7-30d": "low", "30-90d": "low", "90-180d": "medium", "180-365d": "high", ">365d": "critical"}
	var byAge []SecretAgeBucket
	for _, k := range []string{"<7d", "7-30d", "30-90d", "90-180d", "180-365d", ">365d"} {
		byAge = append(byAge, SecretAgeBucket{Range: k, Count: ageBuckets[k], Risk: riskByBucket[k]})
	}

	// Build namespace stats
	var nsList []SecretNSStat
	for _, ns := range nsStats {
		nsList = append(nsList, *ns)
	}
	sort.Slice(nsList, func(i, j int) bool { return nsList[i].Stale > nsList[j].Stale })

	// Sort stale secrets by age descending
	sort.Slice(staleSecrets, func(i, j int) bool { return staleSecrets[i].AgeDays > staleSecrets[j].AgeDays })

	// Score
	score := 100
	score -= summary.OlderThan365d * 5
	score -= summary.OlderThan180d * 3
	score -= summary.OlderThan90d * 1
	score -= summary.OrphanedCount * 2
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if summary.OlderThan365d > 0 {
		recs = append(recs, fmt.Sprintf("%d secret(s) are over 1 year old; rotate immediately for security compliance", summary.OlderThan365d))
	}
	if summary.OlderThan180d > 0 {
		recs = append(recs, fmt.Sprintf("%d secret(s) are over 6 months old; schedule rotation", summary.OlderThan180d))
	}
	if summary.OrphanedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned secret(s) not referenced by any pod; clean up to reduce attack surface", summary.OrphanedCount))
	}
	if summary.TLSSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d TLS secret(s) detected; verify certificate validity and expiry dates", summary.TLSSecrets))
	}
	if len(recs) == 0 {
		recs = append(recs, "Secret age distribution looks healthy; no stale credentials detected")
	}

	return SecretAgeResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		ByType:          byType,
		ByAge:           byAge,
		StaleSecrets:    staleSecrets,
		OrphanedSecrets: orphanedSecrets,
		ByNamespace:     nsList,
		Recommendations: recs,
	}
}

// formatSecretType returns a human-readable secret type.
func formatSecretType(t corev1.SecretType) string {
	switch t {
	case corev1.SecretTypeOpaque:
		return "Opaque"
	case corev1.SecretTypeTLS:
		return "TLS"
	case corev1.SecretTypeDockerConfigJson:
		return "Docker Config"
	default:
		return strings.Title(string(t))
	}
}
