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

// SecretLifecycleResult analyzes secret management lifecycle:
// secret age, rotation policy compliance, plaintext detection,
// key management integration, and secret sprawl analysis.
type SecretLifecycleResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         SecretLifecycleSummary `json:"summary"`
	AgedSecrets     []AgedSecret          `json:"agedSecrets"`
	PlaintextRisk   []PlaintextRisk       `json:"plaintextRisks"`
	SecretSprawl    []SecretSprawl        `json:"secretSprawl"`
	LifecycleScore  int                   `json:"lifecycleScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type SecretLifecycleSummary struct {
	TotalSecrets      int     `json:"totalSecrets"`
	DockerconfigJSON  int     `json:"dockerconfigJson"`
	OPAQUE            int     `json:"opaque"`
	TLS               int     `json:"tls"`
	OlderThan90Days   int     `json:"olderThan90Days"`
	OlderThan365Days  int     `json:"olderThan365Days"`
	DuplicateCount    int     `json:"duplicateCount"`
	UnusedCount       int     `json:"unusedCount"`
}

type AgedSecret struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Age       string `json:"age"`
	DaysOld   int    `json:"daysOld"`
	Severity  string `json:"severity"`
}

type PlaintextRisk struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	ValueLen  int    `json:"valueLen"`
	Risk      string `json:"risk"`
}

type SecretSprawl struct {
	Key       string   `json:"key"`
	Count     int      `json:"count"`
	Namespaces []string `json:"namespaces"`
	Risk      string   `json:"risk"`
}

// handleSecretLifecycle analyzes secret management lifecycle and rotation.
// GET /api/security/secret-lifecycle
func (s *Server) handleSecretLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecretLifecycleResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build map of which secrets are actually used by pods
	usedSecrets := map[string]bool{}
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				usedSecrets[pod.Namespace+"/"+vol.Secret.SecretName] = true
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					usedSecrets[pod.Namespace+"/"+env.ValueFrom.SecretKeyRef.Name] = true
				}
			}
			for _, from := range c.EnvFrom {
				if from.SecretRef != nil {
					usedSecrets[pod.Namespace+"/"+from.SecretRef.Name] = true
				}
			}
		}
		for _, is := range pod.Spec.ImagePullSecrets {
			usedSecrets[pod.Namespace+"/"+is.Name] = true
		}
	}

	// Track duplicate secret keys across namespaces
	keyLocations := map[string][]string{} // key -> [ns/secret,...]

	now := time.Now()
	ninetyDaysAgo := now.AddDate(0, -3, 0)
	oneYearAgo := now.AddDate(-1, 0, 0)

	for _, sec := range secrets.Items {
		if systemNS[sec.Namespace] {
			continue
		}
		// Skip Helm release secrets
		if sec.Type == "helm.sh/release.v1" {
			continue
		}
		// Skip service-account-token auto-managed
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}

		result.Summary.TotalSecrets++

		// Classify type
		switch sec.Type {
		case corev1.SecretTypeDockerConfigJson:
			result.Summary.DockerconfigJSON++
		case corev1.SecretTypeTLS:
			result.Summary.TLS++
		default:
			result.Summary.OPAQUE++
		}

		// Check age
		ageDays := int(now.Sub(sec.CreationTimestamp.Time).Hours() / 24)
		if sec.CreationTimestamp.Time.Before(ninetyDaysAgo) {
			result.Summary.OlderThan90Days++
			severity := "medium"
			if sec.CreationTimestamp.Time.Before(oneYearAgo) {
				result.Summary.OlderThan365Days++
				severity = "high"
			}
			secType := string(sec.Type)
			result.AgedSecrets = append(result.AgedSecrets, AgedSecret{
				Name:      sec.Name,
				Namespace: sec.Namespace,
				Type:      secType,
				Age:       fmt.Sprintf("%dd", ageDays),
				DaysOld:   ageDays,
				Severity:  severity,
			})
		}

		// Check for plaintext-sensitive keys
		sensitiveKeys := map[string]bool{
			"password": true, "passwd": true, "secret": true, "token": true,
			"apikey": true, "api_key": true, "key": true, "credential": true,
		}
		for k, v := range sec.Data {
			kl := strings.ToLower(k)
			for sk := range sensitiveKeys {
				if strings.Contains(kl, sk) {
					// Check if value looks like plaintext (not base64 encoded TLS cert etc.)
					if len(v) > 0 && len(v) < 200 && !strings.HasPrefix(string(v), "-----BEGIN") {
						result.PlaintextRisk = append(result.PlaintextRisk, PlaintextRisk{
							Name:      sec.Name,
							Namespace: sec.Namespace,
							Key:       k,
							ValueLen:  len(v),
							Risk:      fmt.Sprintf("Potential plaintext secret key '%s' in %s/%s", k, sec.Namespace, sec.Name),
						})
					}
				}
			}
			// Track key for sprawl analysis
			keyLocations[k] = append(keyLocations[k], sec.Namespace+"/"+sec.Name)
		}

		// Check if unused
		key := sec.Namespace + "/" + sec.Name
		if !usedSecrets[key] {
			result.Summary.UnusedCount++
		}
	}

	// Secret sprawl: same key name across many namespaces
	for key, locations := range keyLocations {
		nsSet := map[string]bool{}
		for _, loc := range locations {
			parts := strings.SplitN(loc, "/", 2)
			if len(parts) > 0 {
				nsSet[parts[0]] = true
			}
		}
		if len(nsSet) > 3 {
			result.Summary.DuplicateCount++
			var nsList []string
			for ns := range nsSet {
				nsList = append(nsList, ns)
			}
			sort.Strings(nsList)
			result.SecretSprawl = append(result.SecretSprawl, SecretSprawl{
				Key:        key,
				Count:      len(locations),
				Namespaces: nsList,
				Risk:       fmt.Sprintf("Key '%s' duplicated across %d namespaces", key, len(nsSet)),
			})
		}
	}

	// Score
	score := 100
	score -= result.Summary.OlderThan90Days * 2
	score -= result.Summary.OlderThan365Days * 3
	score -= result.Summary.UnusedCount
	score -= result.Summary.DuplicateCount * 5
	score -= len(result.PlaintextRisk) * 3
	if score < 0 {
		score = 0
	}
	result.LifecycleScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.LifecycleScore)

	// Sort
	sort.Slice(result.AgedSecrets, func(i, j int) bool {
		return result.AgedSecrets[i].DaysOld > result.AgedSecrets[j].DaysOld
	})
	sort.Slice(result.SecretSprawl, func(i, j int) bool {
		return result.SecretSprawl[i].Count > result.SecretSprawl[j].Count
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Secret lifecycle score: %d/100 (grade %s) — %d total secrets", result.LifecycleScore, result.Grade, result.Summary.TotalSecrets))
	if result.Summary.OlderThan90Days > 0 {
		recs = append(recs, fmt.Sprintf("%d secrets older than 90 days — implement rotation policy", result.Summary.OlderThan90Days))
	}
	if result.Summary.OlderThan365Days > 0 {
		recs = append(recs, fmt.Sprintf("%d secrets older than 1 year — critical rotation needed", result.Summary.OlderThan365Days))
	}
	if result.Summary.UnusedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d unused secrets not referenced by any pod — clean up", result.Summary.UnusedCount))
	}
	if len(result.PlaintextRisk) > 0 {
		recs = append(recs, fmt.Sprintf("%d potential plaintext secret values detected — use External Secrets or Vault", len(result.PlaintextRisk)))
	}
	if result.Summary.DuplicateCount > 0 {
		recs = append(recs, fmt.Sprintf("%d duplicate secret keys across namespaces — centralize with External Secrets Operator", result.Summary.DuplicateCount))
	}
	if len(recs) == 1 {
		recs = append(recs, "Secret lifecycle management is healthy — maintain current rotation policies")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
