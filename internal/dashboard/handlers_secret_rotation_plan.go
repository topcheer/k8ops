package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretRotationPlanResult generates a prioritized secret rotation plan.
// It identifies old secrets, service account tokens, TLS certs, and
// produces a rotation schedule with commands.
type SecretRotationPlanResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         SecretRotSummary `json:"summary"`
	Plan            []SecretRotEntry `json:"plan"`
	ByType          []SecretRotType  `json:"byType"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type SecretRotSummary struct {
	TotalSecrets  int `json:"totalSecrets"`
	NeedsRotation int `json:"needsRotation"`
	Critical      int `json:"critical"`
	High          int `json:"high"`
	Medium        int `json:"medium"`
	OpaqueCount   int `json:"opaqueCount"`
	TLSCount      int `json:"tlsCount"`
	SATokenCount  int `json:"saTokenCount"`
}

type SecretRotEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Age       string `json:"age"`
	AgeDays   int    `json:"ageDays"`
	Priority  string `json:"priority"`
	Reason    string `json:"reason"`
	Action    string `json:"action"`
	Command   string `json:"command"`
}

type SecretRotType struct {
	Type       string `json:"type"`
	Count      int    `json:"count"`
	OldestDays int    `json:"oldestDays"`
}

// handleSecretRotationPlan handles GET /api/security/secret-rotation-plan
func (s *Server) handleSecretRotationPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecretRotationPlanResult{ScannedAt: time.Now()}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	typeMap := make(map[string]*SecretRotType)
	var plan []SecretRotEntry

	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		result.Summary.TotalSecrets++

		typeStr := string(sec.Type)
		ageDays := int(now.Sub(sec.CreationTimestamp.Time).Hours() / 24)

		if _, ok := typeMap[typeStr]; !ok {
			typeMap[typeStr] = &SecretRotType{Type: typeStr}
		}
		typeMap[typeStr].Count++
		if ageDays > typeMap[typeStr].OldestDays {
			typeMap[typeStr].OldestDays = ageDays
		}

		// Classify by type
		switch sec.Type {
		case corev1.SecretTypeServiceAccountToken:
			result.Summary.SATokenCount++
			continue // SA tokens auto-rotate
		case corev1.SecretTypeTLS:
			result.Summary.TLSCount++
		case corev1.SecretTypeOpaque:
			result.Summary.OpaqueCount++
		default:
			continue
		}

		// Rotation priority based on age
		if ageDays < 30 {
			continue // Fresh enough
		}

		priority := "medium"
		reason := fmt.Sprintf("Secret age %d days", ageDays)
		action := "Rotate secret value"
		command := fmt.Sprintf("kubectl create secret generic %s --from-literal=key=newvalue -n %s --dry-run=client -o yaml | kubectl apply -f -", sec.Name, sec.Namespace)

		if typeStr == string(corev1.SecretTypeTLS) {
			if ageDays > 365 {
				priority = "critical"
				reason = fmt.Sprintf("TLS cert age %d days, likely expired", ageDays)
				action = "Renew TLS certificate"
				command = fmt.Sprintf("# Use cert-manager: kubectl renew certificate %s -n %s", sec.Name, sec.Namespace)
				result.Summary.Critical++
			} else if ageDays > 90 {
				priority = "high"
				reason = fmt.Sprintf("TLS cert age %d days", ageDays)
				result.Summary.High++
			}
		} else if ageDays > 180 {
			priority = "high"
			reason = fmt.Sprintf("Opaque secret age %d days", ageDays)
			result.Summary.High++
		} else {
			result.Summary.Medium++
		}

		result.Summary.NeedsRotation++

		plan = append(plan, SecretRotEntry{
			Name: sec.Name, Namespace: sec.Namespace, Type: typeStr,
			Age: fmt.Sprintf("%dd", ageDays), AgeDays: ageDays,
			Priority: priority, Reason: reason, Action: action, Command: command,
		})
	}

	sort.Slice(plan, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return sevOrder[plan[i].Priority] < sevOrder[plan[j].Priority]
	})
	result.Plan = plan

	for _, t := range typeMap {
		result.ByType = append(result.ByType, *t)
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// Score
	if result.Summary.TotalSecrets > 0 {
		fresh := result.Summary.TotalSecrets - result.Summary.NeedsRotation - result.Summary.SATokenCount
		result.HealthScore = fresh * 100 / result.Summary.TotalSecrets
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildSecretRotRecs(&result)
	writeJSON(w, result)
}

func buildSecretRotRecs(r *SecretRotationPlanResult) []string {
	recs := []string{}
	if r.Summary.NeedsRotation == 0 {
		recs = append(recs, "所有 Secret 都在有效期内")
		return recs
	}
	recs = append(recs, fmt.Sprintf("%d/%d 个 Secret 需要轮换", r.Summary.NeedsRotation, r.Summary.TotalSecrets))
	if r.Summary.Critical > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 TLS 证书可能已过期", r.Summary.Critical))
	}
	if r.Summary.High > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Secret 超过 90 天未轮换", r.Summary.High))
	}
	recs = append(recs, "建议使用 cert-manager 自动轮换 TLS 证书")
	return recs
}
