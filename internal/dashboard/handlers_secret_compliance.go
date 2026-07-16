package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretComplianceResult evaluates secret rotation compliance across the cluster.
// Unlike secret-lifecycle (which tracks existence/types), this engine focuses
// on rotation freshness: are secrets being rotated regularly?
type SecretComplianceResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SecretCompSummary   `json:"summary"`
	ByType          []SecretCompType    `json:"byType"`
	ByNamespace     []SecretCompNS      `json:"byNamespace"`
	StaleSecrets    []SecretCompStale   `json:"staleSecrets"`
	RotationPolicy  SecretCompPolicy    `json:"rotationPolicy"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type SecretCompSummary struct {
	TotalSecrets      int     `json:"totalSecrets"`
	CompliantCount    int     `json:"compliantCount"`
	StaleCount        int     `json:"staleCount"`
	NeverRotated      int     `json:"neverRotated"`
	AvgAgeDays        float64 `json:"avgAgeDays"`
	OldestAgeDays     int     `json:"oldestAgeDays"`
	MaxRecommendedAge int     `json:"maxRecommendedAgeDays"`
	CompliancePct     float64 `json:"compliancePct"`
}

type SecretCompType struct {
	Type       string  `json:"type"`
	Count      int     `json:"count"`
	StaleCount int     `json:"staleCount"`
	AvgAgeDays float64 `json:"avgAgeDays"`
}

type SecretCompNS struct {
	Namespace   string `json:"namespace"`
	SecretCount int    `json:"secretCount"`
	StaleCount  int    `json:"staleCount"`
	OldestDays  int    `json:"oldestDays"`
}

type SecretCompStale struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Type       string `json:"type"`
	AgeDays    int    `json:"ageDays"`
	LastUpdate string `json:"lastUpdate"`
	Severity   string `json:"severity"`
	UsedBy     int    `json:"usedBy"`
}

type SecretCompPolicy struct {
	TLSCertMaxDays      int `json:"tlsCertMaxDays"`
	OpaqueMaxDays       int `json:"opaqueMaxDays"`
	DockerConfigMaxDays int `json:"dockerConfigMaxDays"`
	DefaultMaxDays      int `json:"defaultMaxDays"`
}

// handleSecretCompliance handles GET /api/security/secret-rotation
func (s *Server) handleSecretCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecretComplianceResult{
		ScannedAt: time.Now(),
		RotationPolicy: SecretCompPolicy{
			TLSCertMaxDays: 90, OpaqueMaxDays: 180,
			DockerConfigMaxDays: 90, DefaultMaxDays: 365,
		},
	}
	result.Summary.MaxRecommendedAge = 365

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	now := time.Now()

	// Build secret usage map
	secretUsage := map[string]int{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				secretUsage[pod.Namespace+"/"+vol.Secret.SecretName]++
			}
		}
	}

	typeStats := map[string]*SecretCompType{}
	nsStats := map[string]*SecretCompNS{}
	totalAge := 0

	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) || secret.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}
		result.Summary.TotalSecrets++

		lastUpdate := secret.CreationTimestamp.Time
		updateStatus := "never"
		if secret.Annotations != nil {
			if rotTime, ok := secret.Annotations["last-rotated"]; ok {
				if t, err := time.Parse(time.RFC3339, rotTime); err == nil {
					lastUpdate = t
					updateStatus = "rotated"
				}
			}
		}

		ageDays := int(now.Sub(lastUpdate).Hours() / 24)
		totalAge += ageDays

		maxAge := result.RotationPolicy.DefaultMaxDays
		switch secret.Type {
		case corev1.SecretTypeTLS:
			maxAge = result.RotationPolicy.TLSCertMaxDays
		case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
			maxAge = result.RotationPolicy.DockerConfigMaxDays
		case corev1.SecretTypeOpaque:
			maxAge = result.RotationPolicy.OpaqueMaxDays
		}

		isStale := ageDays > maxAge
		if isStale {
			result.Summary.StaleCount++
			severity := "warning"
			if ageDays > maxAge*2 {
				severity = "critical"
			}
			key := secret.Namespace + "/" + secret.Name
			result.StaleSecrets = append(result.StaleSecrets, SecretCompStale{
				Name: secret.Name, Namespace: secret.Namespace,
				Type: string(secret.Type), AgeDays: ageDays,
				LastUpdate: updateStatus, Severity: severity,
				UsedBy: secretUsage[key],
			})
		} else {
			result.Summary.CompliantCount++
		}

		if ageDays > result.Summary.OldestAgeDays {
			result.Summary.OldestAgeDays = ageDays
		}

		typeName := string(secret.Type)
		if typeStats[typeName] == nil {
			typeStats[typeName] = &SecretCompType{Type: typeName}
		}
		typeStats[typeName].Count++
		if isStale {
			typeStats[typeName].StaleCount++
		}

		if nsStats[secret.Namespace] == nil {
			nsStats[secret.Namespace] = &SecretCompNS{Namespace: secret.Namespace}
		}
		nsStats[secret.Namespace].SecretCount++
		if isStale {
			nsStats[secret.Namespace].StaleCount++
		}
		if ageDays > nsStats[secret.Namespace].OldestDays {
			nsStats[secret.Namespace].OldestDays = ageDays
		}
	}

	if result.Summary.TotalSecrets > 0 {
		result.Summary.AvgAgeDays = float64(totalAge) / float64(result.Summary.TotalSecrets)
		result.Summary.CompliancePct = float64(result.Summary.CompliantCount) / float64(result.Summary.TotalSecrets) * 100
	}

	for _, ts := range typeStats {
		result.ByType = append(result.ByType, *ts)
	}
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	sort.Slice(result.StaleSecrets, func(i, j int) bool {
		if result.StaleSecrets[i].Severity != result.StaleSecrets[j].Severity {
			return result.StaleSecrets[i].Severity == "critical"
		}
		return result.StaleSecrets[i].AgeDays > result.StaleSecrets[j].AgeDays
	})
	if len(result.StaleSecrets) > 30 {
		result.StaleSecrets = result.StaleSecrets[:30]
	}

	result.HealthScore = computeSecretCompScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generateSecretCompRecs(result)

	writeJSON(w, result)
}

func computeSecretCompScore(s SecretCompSummary) int {
	score := 100
	if s.TotalSecrets == 0 {
		return score
	}
	staleRatio := float64(s.StaleCount) / float64(s.TotalSecrets)
	score -= int(staleRatio * 60)
	if s.OldestAgeDays > s.MaxRecommendedAge*2 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

func generateSecretCompRecs(r SecretComplianceResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Secret rotation: %.1f%% compliant (%d/%d), %d stale (score %d/100)",
		r.Summary.CompliancePct, r.Summary.CompliantCount, r.Summary.TotalSecrets,
		r.Summary.StaleCount, r.HealthScore))

	critical := 0
	for _, s := range r.StaleSecrets {
		if s.Severity == "critical" {
			critical++
		}
	}
	if critical > 0 {
		recs = append(recs, fmt.Sprintf("%d critically stale secret(s) (>2x max age) — rotate immediately", critical))
	}
	if r.Summary.OldestAgeDays > 365 {
		recs = append(recs, fmt.Sprintf("Oldest secret is %d days old — implement automated rotation", r.Summary.OldestAgeDays))
	}
	return recs
}
