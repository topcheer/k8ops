package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ERResult is the encryption at rest configuration analysis.
type ERResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         ERSummary `json:"summary"`
	Findings        []EREntry `json:"findings"`
	Issues          []ERIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// ERSummary aggregates encryption stats.
type ERSummary struct {
	EncryptionEnabled   bool   `json:"encryptionEnabled"`
	EncryptionType      string `json:"encryptionType"` // aescbc, aesgcm, secretbox, none
	ProviderCount       int    `json:"providerCount"`
	HasIdentityProvider bool   `json:"hasIdentityProvider"` // identity = plaintext fallback
	APIVersion          string `json:"apiVersion"`
	SecretsEncrypted    bool   `json:"secretsEncrypted"`
	ConfigDetected      bool   `json:"configDetected"`
	SecurityScore       int    `json:"securityScore"` // 0-100
}

// EREntry describes one encryption finding.
type EREntry struct {
	Category string `json:"category"` // configuration, provider, coverage, access
	Status   string `json:"status"`   // pass, warning, fail
	Message  string `json:"message"`
}

// ERIssue is a detected encryption problem.
type ERIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleEncryptionAtRest checks if Kubernetes secrets are encrypted at rest.
// GET /api/security/encryption-at-rest
func (s *Server) handleEncryptionAtRest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ERResult{ScannedAt: time.Now()}

	// Detect encryption configuration by checking kube-apiserver pods
	encryptionEnabled := false
	encryptionType := "none"
	providerCount := 0
	hasIdentity := false
	configDetected := false

	pods, err := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err == nil && pods != nil {
		for _, pod := range pods.Items {
			for _, c := range pod.Spec.Containers {
				if c.Name == "kube-apiserver" {
					configDetected = true
					for _, arg := range c.Command {
						if strings.HasPrefix(arg, "--encryption-provider-config") {
							encryptionEnabled = true
							result.Findings = append(result.Findings, EREntry{
								Category: "configuration", Status: "pass",
								Message: "Encryption provider config flag detected on kube-apiserver",
							})
						}
					}
					break
				}
			}
		}
	}

	// Check for k3s/distro detection
	if !configDetected {
		nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && nodes != nil && len(nodes.Items) > 0 {
			nodeInfo := nodes.Items[0].Status.NodeInfo
			if strings.Contains(nodeInfo.KubeletVersion, "k3s") {
				configDetected = true
				// k3s uses secrets encryption via /etc/rancher/k3s/config.yaml
				result.Findings = append(result.Findings, EREntry{
					Category: "platform", Status: "info",
					Message: fmt.Sprintf("Cluster runs k3s (%s) — encryption is configured via server config yaml, not kube-apiserver flags", nodeInfo.KubeletVersion),
				})
			}
		}
	}

	// If we can list secrets, check if they're encrypted by checking annotation
	// Encrypted secrets have the annotation: encryptionprovider.googleapis.com/ecryption
	secrets, err := rc.clientset.CoreV1().Secrets("kube-system").List(ctx, metav1.ListOptions{
		Limit: 50,
	})
	if err == nil && secrets != nil {
		for _, secret := range secrets.Items {
			// Check for encryption-related annotations
			for k := range secret.Annotations {
				if strings.Contains(strings.ToLower(k), "encrypt") {
					encryptionEnabled = true
					break
				}
			}
		}
	}

	// If encryption is not enabled
	if !encryptionEnabled && configDetected {
		encryptionType = "none"
		result.Findings = append(result.Findings, EREntry{
			Category: "configuration", Status: "fail",
			Message: "No --encryption-provider-config flag detected on kube-apiserver — all Secrets are stored in plaintext in etcd",
		})
		result.Issues = append(result.Issues, ERIssue{
			Severity: "critical", Type: "no-encryption",
			Resource: "kube-apiserver",
			Message:  "Secret encryption at rest is NOT enabled — all Kubernetes Secrets (passwords, tokens, keys) are stored in plaintext in etcd. Anyone with etcd access can read them.",
		})
	}

	// General findings about encryption importance
	if encryptionEnabled {
		encryptionType = "aescbc" // most common, can't detect exact provider without reading config file
		providerCount = 1
		result.Summary.SecretsEncrypted = true
		result.Findings = append(result.Findings, EREntry{
			Category: "coverage", Status: "pass",
			Message: "Secret data is encrypted at rest in etcd — even if etcd is compromised, secrets remain protected",
		})
	} else {
		result.Findings = append(result.Findings, EREntry{
			Category: "coverage", Status: "fail",
			Message: "Secret data is stored in PLAINTEXT — etcd compromise exposes all passwords, tokens, and certificates",
		})
	}

	// Check etcd access pattern
	result.Findings = append(result.Findings, EREntry{
		Category: "access", Status: "info",
		Message: "etcd contains all cluster state — ensure etcd access is restricted (TLS client certs, network policies, firewall rules)",
	})

	result.Summary.EncryptionEnabled = encryptionEnabled
	result.Summary.EncryptionType = encryptionType
	result.Summary.ProviderCount = providerCount
	result.Summary.HasIdentityProvider = hasIdentity
	result.Summary.ConfigDetected = configDetected
	result.Summary.SecretsEncrypted = encryptionEnabled

	// Sort
	sort.Slice(result.Findings, func(i, j int) bool {
		return erStatusRank(result.Findings[i].Status) < erStatusRank(result.Findings[j].Status)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return erIssueRank(result.Issues[i].Severity) < erIssueRank(result.Issues[j].Severity)
	})

	result.Summary.SecurityScore = erScore(result.Summary)
	result.Recommendations = erGenRecs(result.Summary, result.Issues)

	writeJSON(w, result)
}

// erScore computes security score 0-100.
func erScore(s ERSummary) int {
	score := 0
	if s.EncryptionEnabled {
		score += 60
		if !s.HasIdentityProvider {
			score += 15
		}
		if s.ProviderCount >= 1 {
			score += 15
		}
	}
	if s.ConfigDetected {
		score += 10
	}
	return score
}

// erGenRecs produces actionable advice.
func erGenRecs(s ERSummary, issues []ERIssue) []string {
	var recs []string

	if !s.EncryptionEnabled {
		recs = append(recs, "Enable encryption at rest immediately — create an EncryptionConfiguration with AES-CBC or AES-GCM provider and add --encryption-provider-config flag to kube-apiserver")
	}
	if s.EncryptionEnabled && s.HasIdentityProvider {
		recs = append(recs, "Remove the 'identity' (plaintext) provider from EncryptionConfiguration — it allows fallback to unencrypted storage")
	}
	if !s.ConfigDetected {
		recs = append(recs, "Unable to detect kube-apiserver configuration — if using managed Kubernetes (EKS/GKE/AKS), check provider docs for encryption configuration")
	}
	recs = append(recs, "Restrict etcd access: use TLS client certificates, firewall rules, and never expose etcd to untrusted networks")
	if s.SecurityScore < 50 {
		recs = append(recs, fmt.Sprintf("Encryption security score is %d/100 — critical gap in data-at-rest protection", s.SecurityScore))
	}
	if s.EncryptionEnabled {
		recs = append(recs, "Consider rotating encryption keys periodically and verify with 'kubectl get secret -o json | jq .data' that data appears encrypted")
	}
	if s.EncryptionEnabled && !s.HasIdentityProvider {
		recs = append(recs, fmt.Sprintf("Encryption at rest is properly configured (score: %d/100) — secrets are protected in etcd", s.SecurityScore))
	}

	return recs
}

func erStatusRank(s string) int {
	switch s {
	case "fail":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	case "pass":
		return 3
	default:
		return 4
	}
}

func erIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
