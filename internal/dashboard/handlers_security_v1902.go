package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.02 — Security Dimension (Round 3)
// 1. Volume Encryption Audit
// 2. Admission Webhook Posture
// 3. Key Rotation Compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Volume Encryption Audit — check PVC/StorageClass encryption
// ---------------------------------------------------------------

type VolEncryptionResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         VolEncSummary    `json:"summary"`
	StorageClasses  []VolEncSCEntry  `json:"storageClasses"`
	UnencryptedPVCs []VolEncPVCEntry `json:"unencryptedPVCs"`
	SensitiveNS     []VolEncNSEntry  `json:"sensitiveNamespaces"`
	Recommendations []string         `json:"recommendations"`
}

type VolEncSummary struct {
	TotalPVCs           int `json:"totalPVCs"`
	EncryptedPVCs       int `json:"encryptedPVCs"`
	UnencryptedPVCs     int `json:"unencryptedPVCs"`
	TotalStorageClasses int `json:"totalStorageClasses"`
	EncryptedSCs        int `json:"encryptedStorageClasses"`
	TotalSensitiveData  int `json:"totalSensitiveData"`
}

type VolEncSCEntry struct {
	Name          string `json:"name"`
	Provisioner   string `json:"provisioner"`
	HasEncryption bool   `json:"hasEncryption"`
	EncryptedBy   string `json:"encryptedBy"`
	PVCCount      int    `json:"pvcCount"`
}

type VolEncPVCEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	StorageClass string `json:"storageClass"`
	SizeGB       int    `json:"sizeGB"`
	Encrypted    bool   `json:"encrypted"`
	RiskLevel    string `json:"riskLevel"`
}

type VolEncNSEntry struct {
	Namespace       string `json:"namespace"`
	HasSecrets      int    `json:"hasSecrets"`
	HasPVCs         int    `json:"hasPVCs"`
	UnencryptedPVCs int    `json:"unencryptedPVCs"`
	RiskLevel       string `json:"riskLevel"`
}

func (s *Server) handleVolEncryptionAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := VolEncryptionResult{ScannedAt: time.Now()}

	// Check StorageClasses for encryption parameters
	scList, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	scEncrypted := map[string]bool{}
	scEncMethod := map[string]string{}
	pvcCountBySC := map[string]int{}

	for _, sc := range scList.Items {
		entry := VolEncSCEntry{
			Name: sc.Name, Provisioner: sc.Provisioner,
		}
		// Check for encryption parameters
		encrypted := false
		encBy := ""
		if sc.Parameters != nil {
			// Common encryption parameter patterns
			for k, v := range sc.Parameters {
				kl := strings.ToLower(k)
				vl := strings.ToLower(v)
				if strings.Contains(kl, "encrypt") && (vl == "true" || vl == "aes256" || vl == "ssl") {
					encrypted = true
					encBy = k + "=" + v
				}
				if strings.Contains(kl, "crypt") && vl != "none" && vl != "false" {
					encrypted = true
					encBy = k + "=" + v
				}
			}
		}
		entry.HasEncryption = encrypted
		entry.EncryptedBy = encBy
		if encrypted {
			result.Summary.EncryptedSCs++
		}
		scEncrypted[sc.Name] = encrypted
		scEncMethod[sc.Name] = encBy
		result.Summary.TotalStorageClasses++
		result.StorageClasses = append(result.StorageClasses, entry)
	}

	// Analyze PVCs
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		pvcCountBySC[scName]++

		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}

		encrypted := scEncrypted[scName]
		entry := VolEncPVCEntry{
			Name: pvc.Name, Namespace: pvc.Namespace,
			StorageClass: scName, SizeGB: sizeGB, Encrypted: encrypted,
		}
		if encrypted {
			result.Summary.EncryptedPVCs++
			entry.RiskLevel = "low"
		} else {
			result.Summary.UnencryptedPVCs++
			entry.RiskLevel = "high"
			result.UnencryptedPVCs = append(result.UnencryptedPVCs, entry)
		}
	}

	// Update SC PVC counts
	for i := range result.StorageClasses {
		result.StorageClasses[i].PVCCount = pvcCountBySC[result.StorageClasses[i].Name]
	}

	// Check namespaces with sensitive data (Secrets) and unencrypted PVCs
	nsSecretCount := map[string]int{}
	nsPVCCount := map[string]int{}
	nsUnencPVC := map[string]int{}
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) || strings.HasPrefix(secret.Name, "default-token-") {
			continue
		}
		nsSecretCount[secret.Namespace]++
	}
	for _, pvc := range result.UnencryptedPVCs {
		nsPVCCount[pvc.Namespace]++
		nsUnencPVC[pvc.Namespace]++
	}
	for _, pvc := range pvcs.Items {
		if !isSystemNamespace(pvc.Namespace) {
			// Count all PVCs per NS
		}
	}

	for ns, secCount := range nsSecretCount {
		if secCount > 0 {
			result.Summary.TotalSensitiveData++
			riskLevel := "medium"
			if nsUnencPVC[ns] > 0 {
				riskLevel = "high"
			}
			result.SensitiveNS = append(result.SensitiveNS, VolEncNSEntry{
				Namespace: ns, HasSecrets: secCount,
				UnencryptedPVCs: nsUnencPVC[ns], RiskLevel: riskLevel,
			})
		}
	}
	sort.Slice(result.SensitiveNS, func(i, j int) bool {
		return result.SensitiveNS[i].RiskLevel == "high" && result.SensitiveNS[j].RiskLevel != "high"
	})

	// Score
	if result.Summary.TotalPVCs > 0 {
		result.HealthScore = result.Summary.EncryptedPVCs * 100 / result.Summary.TotalPVCs
	} else {
		result.HealthScore = 100 // No PVCs = no risk
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildVolEncRecs1902(&result)
	writeJSON(w, result)
}

func buildVolEncRecs1902(r *VolEncryptionResult) []string {
	recs := []string{fmt.Sprintf("Volume encryption: %d PVCs (%d encrypted, %d unencrypted), %d/%d SCs with encryption",
		r.Summary.TotalPVCs, r.Summary.EncryptedPVCs, r.Summary.UnencryptedPVCs,
		r.Summary.EncryptedSCs, r.Summary.TotalStorageClasses)}
	if r.Summary.UnencryptedPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d unencrypted PVCs - enable encryption at StorageClass level", r.Summary.UnencryptedPVCs))
	}
	if len(r.SensitiveNS) > 0 {
		highRisk := 0
		for _, ns := range r.SensitiveNS {
			if ns.RiskLevel == "high" {
				highRisk++
			}
		}
		if highRisk > 0 {
			recs = append(recs, fmt.Sprintf("%d namespaces with secrets AND unencrypted PVCs - prioritize encryption", highRisk))
		}
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Admission Webhook Posture — webhook config & security
// ---------------------------------------------------------------

type WebhookPostureResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         WebhookSummary        `json:"summary"`
	Webhooks        []WebhookPostureEntry `json:"webhooks"`
	Misconfigured   []WebhookPostureEntry `json:"misconfigured"`
	Recommendations []string              `json:"recommendations"`
}

type WebhookSummary struct {
	TotalWebhooks      int `json:"totalWebhooks"`
	ValidatingWebhooks int `json:"validatingWebhooks"`
	MutatingWebhooks   int `json:"mutatingWebhooks"`
	WithTLS            int `json:"withTLS"`
	WithoutTLS         int `json:"withoutTLS"`
	WithTimeout        int `json:"withTimeout"`
	WithoutTimeout     int `json:"withoutTimeout"`
	HighLatency        int `json:"highLatency"`
	FailurePolicyFail  int `json:"failurePolicyFail"`
}

type WebhookPostureEntry struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Service        string `json:"service"`
	FailurePolicy  string `json:"failurePolicy"`
	TimeoutSeconds int32  `json:"timeoutSeconds"`
	HasCABundle    bool   `json:"hasCABundle"`
	RiskLevel      string `json:"riskLevel"`
	Issue          string `json:"issue"`
}

func (s *Server) handleWebhookPosture(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := WebhookPostureResult{ScannedAt: time.Now()}

	// Check ValidatingWebhookConfigurations
	vwList, _ := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	for _, vwc := range vwList.Items {
		if strings.HasPrefix(vwc.Name, "kube-") {
			continue
		}
		for _, wh := range vwc.Webhooks {
			result.Summary.TotalWebhooks++
			result.Summary.ValidatingWebhooks++
			entry := WebhookPostureEntry{
				Name: vwc.Name, Type: "validating",
				FailurePolicy: webhookFailurePolicyStr1902(wh.FailurePolicy),
			}
			if wh.TimeoutSeconds != nil {
				entry.TimeoutSeconds = *wh.TimeoutSeconds
				result.Summary.WithTimeout++
				if *wh.TimeoutSeconds >= 30 {
					result.Summary.HighLatency++
					entry.RiskLevel = "medium"
					entry.Issue = fmt.Sprintf("high timeout: %ds", *wh.TimeoutSeconds)
				}
			} else {
				result.Summary.WithoutTimeout++
			}
			if wh.ClientConfig.CABundle != nil && len(wh.ClientConfig.CABundle) > 0 {
				entry.HasCABundle = true
				result.Summary.WithTLS++
			} else {
				result.Summary.WithoutTLS++
				entry.RiskLevel = "high"
				entry.Issue = "missing CA bundle"
			}
			if wh.ClientConfig.Service != nil {
				entry.Service = wh.ClientConfig.Service.Name
			}
			if wh.FailurePolicy != nil && *wh.FailurePolicy == admissionregv1.Fail {
				result.Summary.FailurePolicyFail++
			}
			if entry.RiskLevel != "" {
				result.Misconfigured = append(result.Misconfigured, entry)
			}
			result.Webhooks = append(result.Webhooks, entry)
		}
	}

	// Check MutatingWebhookConfigurations
	mwList, _ := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	for _, mwc := range mwList.Items {
		if strings.HasPrefix(mwc.Name, "kube-") {
			continue
		}
		for _, wh := range mwc.Webhooks {
			result.Summary.TotalWebhooks++
			result.Summary.MutatingWebhooks++
			entry := WebhookPostureEntry{
				Name: mwc.Name, Type: "mutating",
				FailurePolicy: webhookFailurePolicyStr1902(wh.FailurePolicy),
			}
			if wh.TimeoutSeconds != nil {
				entry.TimeoutSeconds = *wh.TimeoutSeconds
				result.Summary.WithTimeout++
			} else {
				result.Summary.WithoutTimeout++
			}
			if wh.ClientConfig.CABundle != nil && len(wh.ClientConfig.CABundle) > 0 {
				entry.HasCABundle = true
				result.Summary.WithTLS++
			} else {
				result.Summary.WithoutTLS++
				entry.RiskLevel = "high"
				entry.Issue = "missing CA bundle"
			}
			if wh.ClientConfig.Service != nil {
				entry.Service = wh.ClientConfig.Service.Name
			}
			if entry.RiskLevel != "" {
				result.Misconfigured = append(result.Misconfigured, entry)
			}
			result.Webhooks = append(result.Webhooks, entry)
		}
	}

	// Score
	if result.Summary.TotalWebhooks > 0 {
		healthyCount := result.Summary.TotalWebhooks - len(result.Misconfigured)
		result.HealthScore = healthyCount * 100 / result.Summary.TotalWebhooks
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildWebhookRecs1902(&result)
	writeJSON(w, result)
}

func buildWebhookRecs1902(r *WebhookPostureResult) []string {
	recs := []string{fmt.Sprintf("Admission webhook posture: %d webhooks (%d validating, %d mutating), %d misconfigured",
		r.Summary.TotalWebhooks, r.Summary.ValidatingWebhooks, r.Summary.MutatingWebhooks, len(r.Misconfigured))}
	if r.Summary.WithoutTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks without CA bundle - TLS verification disabled", r.Summary.WithoutTLS))
	}
	if r.Summary.HighLatency > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks with timeout >= 30s - may slow down API operations", r.Summary.HighLatency))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Key Rotation Compliance — secret/token age & rotation status
// ---------------------------------------------------------------

type KeyRotationResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         KeyRotationSummary `json:"summary"`
	OverdueSecrets  []KeyRotationEntry `json:"overdueSecrets"`
	ByType          map[string]int     `json:"byType"`
	Recommendations []string           `json:"recommendations"`
}

type KeyRotationSummary struct {
	TotalSecrets   int `json:"totalSecrets"`
	StaleSecrets   int `json:"staleSecrets"`
	Overdue90Days  int `json:"overdue90Days"`
	Overdue180Days int `json:"overdue180Days"`
	Overdue365Days int `json:"overdue365Days"`
	FreshSecrets   int `json:"freshSecrets"`
	NeverRotated   int `json:"neverRotated"`
}

type KeyRotationEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	AgeDays   int    `json:"ageDays"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

func (s *Server) handleKeyRotationCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := KeyRotationResult{
		ScannedAt: time.Now(),
		ByType:    map[string]int{},
	}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) || strings.HasPrefix(secret.Name, "default-token-") {
			continue
		}
		// Skip non-sensitive secret types
		if secret.Type != corev1.SecretTypeOpaque &&
			secret.Type != corev1.SecretTypeDockerConfigJson &&
			secret.Type != corev1.SecretTypeTLS &&
			!strings.HasPrefix(string(secret.Type), "kubernetes.io/") {
			continue
		}

		result.Summary.TotalSecrets++
		typeStr := string(secret.Type)
		if typeStr == "" {
			typeStr = "Opaque"
		}
		result.ByType[typeStr]++

		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)
		entry := KeyRotationEntry{
			Name: secret.Name, Namespace: secret.Namespace,
			Type: typeStr, AgeDays: ageDays,
		}

		switch {
		case ageDays >= 365:
			entry.RiskLevel = "critical"
			entry.Issue = fmt.Sprintf("not rotated in %d days (>1 year)", ageDays)
			result.Summary.Overdue365Days++
			result.Summary.Overdue90Days++
			result.Summary.Overdue180Days++
			result.OverdueSecrets = append(result.OverdueSecrets, entry)
		case ageDays >= 180:
			entry.RiskLevel = "high"
			entry.Issue = fmt.Sprintf("not rotated in %d days (>6 months)", ageDays)
			result.Summary.Overdue180Days++
			result.Summary.Overdue90Days++
			result.OverdueSecrets = append(result.OverdueSecrets, entry)
		case ageDays >= 90:
			entry.RiskLevel = "medium"
			entry.Issue = fmt.Sprintf("not rotated in %d days (>3 months)", ageDays)
			result.Summary.Overdue90Days++
			result.OverdueSecrets = append(result.OverdueSecrets, entry)
		case ageDays >= 30:
			entry.RiskLevel = "low"
			result.Summary.StaleSecrets++
		default:
			result.Summary.FreshSecrets++
		}
	}

	// Score
	if result.Summary.TotalSecrets > 0 {
		freshPct := result.Summary.FreshSecrets * 100 / result.Summary.TotalSecrets
		overduePenalty := result.Summary.Overdue90Days * 3
		result.HealthScore = freshPct - overduePenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildKeyRotationRecs1902(&result)
	writeJSON(w, result)
}

func buildKeyRotationRecs1902(r *KeyRotationResult) []string {
	recs := []string{fmt.Sprintf("Key rotation: %d secrets, %d fresh, %d overdue (>90d), %d critical (>365d)",
		r.Summary.TotalSecrets, r.Summary.FreshSecrets,
		r.Summary.Overdue90Days, r.Summary.Overdue365Days)}
	if r.Summary.Overdue90Days > 0 {
		recs = append(recs, fmt.Sprintf("%d secrets not rotated in 90+ days - implement rotation policy", r.Summary.Overdue90Days))
	}
	if r.Summary.Overdue365Days > 0 {
		recs = append(recs, fmt.Sprintf("%d secrets not rotated in 1+ year - immediate rotation required", r.Summary.Overdue365Days))
	}
	return recs
}

func webhookFailurePolicyStr1902(fp *admissionregv1.FailurePolicyType) string {
	if fp == nil {
		return ""
	}
	return string(*fp)
}
