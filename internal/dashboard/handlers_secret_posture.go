package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretPostureResult is the secret management posture & external secret integration audit.
type SecretPostureResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         SecretPostureSummary  `json:"summary"`
	Integration     SecretIntegration     `json:"integration"`
	ByNamespace     []SecretPostureNSStat `json:"byNamespace"`
	Risks           []SecretPostureRisk   `json:"risks"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// SecretPostureSummary aggregates secret management posture metrics.
type SecretPostureSummary struct {
	TotalSecrets        int `json:"totalSecrets"`
	ExternalSecrets     int `json:"externalSecrets"`     // ExternalSecret CRs
	SealedSecrets       int `json:"sealedSecrets"`       // SealedSecret CRs
	SOPSEncrypted       int `json:"sopsEncrypted"`       // SOPS-encrypted annotations
	PlaintextSecrets    int `json:"plaintextSecrets"`    // no encryption
	ManagedSecrets      int `json:"managedSecrets"`      // managed by external/sealed
	UnmanagedSecrets    int `json:"unmanagedSecrets"`    // manually created
	CrossNSRefs         int `json:"crossNSRefs"`         // cross-namespace secret references
	Dockerconfigjson    int `json:"dockerconfigjson"`    // image pull secrets
	TLSSecrets          int `json:"tlsSecrets"`          // kubernetes.io/tls type
	CASecrets           int `json:"caSecrets"`           // kubernetes.io/ca.crt
	ServiceAccountToken int `json:"serviceAccountToken"` // kubernetes.io/service-account-token
	EmptySecrets        int `json:"emptySecrets"`        // secrets with no data
	LargeSecrets        int `json:"largeSecrets"`        // secrets > 1MB
}

// SecretIntegration detects external secret management tools.
type SecretIntegration struct {
	ExternalSecretsOperator bool   `json:"externalSecretsOperator"`
	SealedSecretsController bool   `json:"sealedSecretsController"`
	SOPS                    bool   `json:"sops"`
	Vault                   bool   `json:"vault"`
	ESOVersion              string `json:"esoVersion,omitempty"`
	Status                  string `json:"status"` // integrated, partial, missing
}

// SecretPostureNSStat per-namespace secret posture.
type SecretPostureNSStat struct {
	Namespace    string `json:"namespace"`
	TotalSecrets int    `json:"totalSecrets"`
	Managed      int    `json:"managed"`
	Unmanaged    int    `json:"unmanaged"`
	Plaintext    int    `json:"plaintext"`
	CrossNSRefs  int    `json:"crossNSRefs"`
	RiskLevel    string `json:"riskLevel"` // low, medium, high, critical
}

// SecretPostureRisk describes a secret management risk.
type SecretPostureRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Secret    string `json:"secret,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleSecretPosture audits secret management posture & external secret integration.
// GET /api/security/secret-posture
func (s *Server) handleSecretPosture(w http.ResponseWriter, r *http.Request) {
	result := SecretPostureResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Detect external secret management tools
	secrets, err := rc.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list secrets: %v", err))
		return
	}

	pods, podErr := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	_ = podErr

	// Detect External Secrets Operator, Sealed Secrets, SOPS, Vault
	esoDetected := false
	sealedDetected := false
	sopsDetected := false
	vaultDetected := false
	esoVersion := ""

	if pods != nil {
		for _, pod := range pods.Items {
			podLower := strings.ToLower(pod.Name)
			nsLower := strings.ToLower(pod.Namespace)
			for _, c := range pod.Spec.Containers {
				imgLower := strings.ToLower(c.Image)

				// External Secrets Operator
				if strings.Contains(podLower, "external-secrets") || strings.Contains(imgLower, "external-secrets") {
					esoDetected = true
					if esoVersion == "" && strings.Contains(imgLower, "external-secrets") {
						esoVersion = extractVersionFromImage(c.Image)
					}
				}
				// Sealed Secrets
				if strings.Contains(podLower, "sealed-secrets") || strings.Contains(imgLower, "sealed-secrets") || strings.Contains(imgLower, "sealedsecret") {
					sealedDetected = true
				}
				// SOPS (usually runs as a job or sidecar)
				if strings.Contains(podLower, "sops") || strings.Contains(imgLower, "sops") {
					sopsDetected = true
				}
				// Vault
				if strings.Contains(podLower, "vault") || strings.Contains(imgLower, "vault") {
					vaultDetected = true
				}
			}
			// Also check namespace names
			if strings.Contains(nsLower, "external-secrets") {
				esoDetected = true
			}
			if strings.Contains(nsLower, "sealed-secrets") {
				sealedDetected = true
			}
			_ = nsLower
		}
	}

	// Set integration status
	integrationStatus := "missing"
	detectedTools := 0
	if esoDetected {
		detectedTools++
	}
	if sealedDetected {
		detectedTools++
	}
	if sopsDetected {
		detectedTools++
	}
	if vaultDetected {
		detectedTools++
	}
	if detectedTools >= 1 {
		integrationStatus = "partial"
	}
	if detectedTools >= 2 {
		integrationStatus = "integrated"
	}

	result.Integration = SecretIntegration{
		ExternalSecretsOperator: esoDetected,
		SealedSecretsController: sealedDetected,
		SOPS:                    sopsDetected,
		Vault:                   vaultDetected,
		ESOVersion:              esoVersion,
		Status:                  integrationStatus,
	}

	// 2. Analyze secrets
	nsStats := map[string]*SecretPostureNSStat{}
	sopsAnnotationKey := "sops.example.com/encrypted"

	for _, secret := range secrets.Items {
		result.Summary.TotalSecrets++

		ns := secret.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &SecretPostureNSStat{Namespace: ns, RiskLevel: "low"}
		}
		nsStats[ns].TotalSecrets++

		// Check type
		isManaged := false

		// Check for SOPS encryption annotations
		for k, v := range secret.Annotations {
			if strings.Contains(strings.ToLower(k), "sops") || strings.Contains(strings.ToLower(k), "encrypted") {
				sopsDetected = true
				result.Summary.SOPSEncrypted++
				isManaged = true
				_ = v
			}
			// Check for external-secrets managed annotation
			if strings.Contains(strings.ToLower(k), "external-secrets") {
				isManaged = true
				result.Summary.ExternalSecrets++
			}
			// Check for sealed-secrets managed annotation
			if strings.Contains(strings.ToLower(k), "sealed-secrets") {
				isManaged = true
				result.Summary.SealedSecrets++
			}
		}

		// Check labels for managed secrets
		for k, v := range secret.Labels {
			labelLower := strings.ToLower(k + "=" + v)
			if strings.Contains(labelLower, "external-secrets") {
				isManaged = true
				result.Summary.ExternalSecrets++
			}
			if strings.Contains(labelLower, "sealed-secrets") || strings.Contains(labelLower, "sealedsecret") {
				isManaged = true
				result.Summary.SealedSecrets++
			}
		}

		// Check by secret type
		switch string(secret.Type) {
		case "kubernetes.io/dockerconfigjson":
			result.Summary.Dockerconfigjson++
		case "kubernetes.io/tls":
			result.Summary.TLSSecrets++
		case "kubernetes.io/service-account-token":
			result.Summary.ServiceAccountToken++
		case "kubernetes.io/ca.crt":
			result.Summary.CASecrets++
		}

		// Check data size
		if len(secret.Data) == 0 {
			result.Summary.EmptySecrets++
		} else {
			totalSize := 0
			for _, v := range secret.Data {
				totalSize += len(v)
			}
			if totalSize > 1024*1024 {
				result.Summary.LargeSecrets++
			}
		}

		// Track managed vs unmanaged
		if isManaged {
			result.Summary.ManagedSecrets++
			nsStats[ns].Managed++
		} else {
			result.Summary.UnmanagedSecrets++
			nsStats[ns].Unmanaged++
			// Only flag as plaintext if it's not a service-account token
			if string(secret.Type) != "kubernetes.io/service-account-token" {
				result.Summary.PlaintextSecrets++
				nsStats[ns].Plaintext++
				if len(secret.Data) > 0 {
					result.Risks = append(result.Risks, SecretPostureRisk{
						Namespace: ns,
						Secret:    secret.Name,
						Issue:     "Unmanaged plaintext secret — not encrypted or managed by external secret tool",
						Severity:  "warning",
					})
				}
			}
		}

		_ = sopsAnnotationKey
	}

	// 3. Assess namespace risk levels
	for _, stat := range nsStats {
		if stat.Plaintext > 5 {
			stat.RiskLevel = "critical"
		} else if stat.Plaintext > 2 {
			stat.RiskLevel = "high"
		} else if stat.Plaintext > 0 {
			stat.RiskLevel = "medium"
		}
		if stat.Unmanaged > 10 {
			if stat.RiskLevel == "low" {
				stat.RiskLevel = "medium"
			}
		}
	}

	// 4. Build namespace stats slice
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalSecrets > result.ByNamespace[j].TotalSecrets
	})

	// 5. Calculate health score
	score := 100
	if result.Integration.Status == "missing" {
		score -= 20
	}
	if result.Integration.Status == "partial" {
		score -= 5
	}
	// Plaintext secrets penalty
	if result.Summary.PlaintextSecrets > 0 {
		score -= min(30, result.Summary.PlaintextSecrets*3)
	}
	// Empty secrets
	if result.Summary.EmptySecrets > 0 {
		score -= min(10, result.Summary.EmptySecrets)
	}
	// Large secrets
	if result.Summary.LargeSecrets > 0 {
		score -= min(10, result.Summary.LargeSecrets*2)
	}
	// No external secret management
	if !esoDetected && !sealedDetected && !vaultDetected {
		result.Risks = append(result.Risks, SecretPostureRisk{
			Issue:    "No external secret management tool detected (External Secrets Operator, Sealed Secrets, Vault)",
			Severity: "high",
		})
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 6. Recommendations
	if result.Integration.Status == "missing" {
		result.Recommendations = append(result.Recommendations,
			"Deploy an external secret management tool (External Secrets Operator, Sealed Secrets, or HashiCorp Vault)")
	}
	if result.Summary.PlaintextSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unmanaged plaintext secrets detected — migrate to encrypted/managed secret storage", result.Summary.PlaintextSecrets))
	}
	if result.Summary.EmptySecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d empty secrets detected — clean up unused secrets", result.Summary.EmptySecrets))
	}
	if result.Summary.LargeSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d large secrets (>1MB) detected — consider storing large data in external systems", result.Summary.LargeSecrets))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Secret management posture is healthy — all secrets are managed by external tools or encrypted")
	}

	writeJSON(w, result)
}

// extractVersionFromImage extracts the version tag from a container image string.
func extractVersionFromImage(image string) string {
	// image format: registry/image:tag or registry/image@sha256:...
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		v := image[idx+1:]
		// Skip sha256 digests
		if strings.Contains(v, "sha256") {
			return ""
		}
		return v
	}
	return ""
}
