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
// v19.19 — Security Dimension (Round 6)
// 1. Secret Exposure Graph
// 2. Admission Exception Audit
// 3. Proc Mount & Tmpfs Write Risk
// ============================================================

// ---------------------------------------------------------------
// 1. Secret Exposure Graph — which secrets are over-exposed
// ---------------------------------------------------------------

type SecretExposureResult1919 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         SecretExposureSummary1919 `json:"summary"`
	OverExposed     []SecretExposureEntry1919 `json:"overExposed"`
	ByNamespace     []SecretExposureNS1919    `json:"byNamespace"`
	Recommendations []string                  `json:"recommendations"`
}

type SecretExposureSummary1919 struct {
	TotalSecrets     int `json:"totalSecrets"`
	MountedSecrets   int `json:"mountedSecrets"`
	UnusedSecrets    int `json:"unusedSecrets"`
	OverExposedCount int `json:"overExposedCount"`
	MaxMountCount    int `json:"maxMountCount"`
	DuplicateSecrets int `json:"duplicateSecrets"`
}

type SecretExposureEntry1919 struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Type       string   `json:"type"`
	MountCount int      `json:"mountCount"`
	Workloads  []string `json:"workloads"`
	RiskLevel  string   `json:"riskLevel"`
}

type SecretExposureNS1919 struct {
	Namespace    string `json:"namespace"`
	SecretCount  int    `json:"secretCount"`
	MountedCount int    `json:"mountedCount"`
	UnusedCount  int    `json:"unusedCount"`
}

func (s *Server) handleSecretExposureGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SecretExposureResult1919{ScannedAt: time.Now()}

	// Get all secrets
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	secretSet := map[string]bool{}
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		key := sec.Namespace + "/" + sec.Name
		secretSet[key] = true
		result.Summary.TotalSecrets++
	}

	// Track which secrets are mounted by deployments
	mountMap := map[string]*SecretExposureEntry1919{}
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		mountedSecrets := map[string]bool{}

		// Check volume mounts
		for _, vol := range dep.Spec.Template.Spec.Volumes {
			if vol.Secret != nil {
				key := dep.Namespace + "/" + vol.Secret.SecretName
				mountedSecrets[key] = true
			}
		}
		// Check env var refs
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					key := dep.Namespace + "/" + env.ValueFrom.SecretKeyRef.Name
					mountedSecrets[key] = true
				}
			}
			// Check envFrom
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					key := dep.Namespace + "/" + ef.SecretRef.Name
					mountedSecrets[key] = true
				}
			}
		}

		for key := range mountedSecrets {
			entry, ok := mountMap[key]
			if !ok {
				parts := strings.SplitN(key, "/", 2)
				entry = &SecretExposureEntry1919{
					Name: parts[1], Namespace: parts[0],
					MountCount: 0, Workloads: []string{},
				}
				mountMap[key] = entry
			}
			entry.MountCount++
			entry.Workloads = append(entry.Workloads, dep.Name)
			if entry.MountCount > result.Summary.MaxMountCount {
				result.Summary.MaxMountCount = entry.MountCount
			}
		}
	}

	// Build results
	nsMap := map[string]*SecretExposureNS1919{}
	for key, entry := range mountMap {
		if !secretSet[key] {
			continue
		}
		result.Summary.MountedSecrets++

		if entry.MountCount > 3 {
			entry.RiskLevel = "high"
			result.Summary.OverExposedCount++
			result.OverExposed = append(result.OverExposed, *entry)
		} else if entry.MountCount > 1 {
			entry.RiskLevel = "medium"
		} else {
			entry.RiskLevel = "low"
		}

		nsE, ok := nsMap[entry.Namespace]
		if !ok {
			nsE = &SecretExposureNS1919{Namespace: entry.Namespace}
			nsMap[entry.Namespace] = nsE
		}
		nsE.MountedCount++
	}

	// Count unused secrets
	for key := range secretSet {
		if _, mounted := mountMap[key]; !mounted {
			result.Summary.UnusedSecrets++
			parts := strings.SplitN(key, "/", 2)
			nsE, ok := nsMap[parts[0]]
			if !ok {
				nsE = &SecretExposureNS1919{Namespace: parts[0]}
				nsMap[parts[0]] = nsE
			}
			nsE.UnusedCount++
		}
	}

	for _, ns := range nsMap {
		ns.SecretCount = ns.MountedCount + ns.UnusedCount
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].UnusedCount > result.ByNamespace[j].UnusedCount
	})

	// Score: fewer over-exposed = better
	if result.Summary.TotalSecrets > 0 {
		safePct := (result.Summary.TotalSecrets - result.Summary.OverExposedCount) * 100 / result.Summary.TotalSecrets
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildSecretExposureRecs1919(&result)
	writeJSON(w, result)
}

func buildSecretExposureRecs1919(r *SecretExposureResult1919) []string {
	recs := []string{fmt.Sprintf("Secret exposure: %d secrets, %d mounted, %d unused, %d over-exposed (max %d mounts)",
		r.Summary.TotalSecrets, r.Summary.MountedSecrets, r.Summary.UnusedSecrets,
		r.Summary.OverExposedCount, r.Summary.MaxMountCount)}
	if r.Summary.UnusedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d unused secrets - clean up to reduce attack surface", r.Summary.UnusedSecrets))
	}
	if r.Summary.OverExposedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d secrets mounted by >3 workloads - consider namespace-scoped secrets", r.Summary.OverExposedCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Admission Exception Audit — PSA exceptions & webhook gaps
// ---------------------------------------------------------------

type AdmissionExceptionResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         AdmissionExcSummary   `json:"summary"`
	Exceptions      []AdmissionException  `json:"exceptions"`
	WebhookGaps     []AdmissionWebhookGap `json:"webhookGaps"`
	Recommendations []string              `json:"recommendations"`
}

type AdmissionExcSummary struct {
	TotalNamespaces    int  `json:"totalNamespaces"`
	WithPSAEnforce     int  `json:"withPSAEnforce"`
	WithPSAExcept      int  `json:"withPSAExceptions"`
	WithoutPSA         int  `json:"withoutPSA"`
	WebhookCount       int  `json:"webhookCount"`
	MutatingWebhooks   int  `json:"mutatingWebhooks"`
	ValidatingWebhooks int  `json:"validatingWebhooks"`
	MissingGatekeeper  bool `json:"missingGatekeeper"`
}

type AdmissionException struct {
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Detail    string `json:"detail"`
	RiskLevel string `json:"riskLevel"`
}

type AdmissionWebhookGap struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Issue string `json:"issue"`
}

func (s *Server) handleAdmissionException(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := AdmissionExceptionResult{ScannedAt: time.Now()}

	// Check namespace PSA labels
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		enforceLevel := ""
		if ns.Labels != nil {
			enforceLevel = ns.Labels["pod-security.kubernetes.io/enforce"]
		}

		if enforceLevel != "" {
			result.Summary.WithPSAEnforce++
		} else {
			result.Summary.WithoutPSA++
			result.Exceptions = append(result.Exceptions, AdmissionException{
				Namespace: ns.Name,
				Type:      "no-psa-enforce",
				Detail:    "no pod-security.kubernetes.io/enforce label",
				RiskLevel: "medium",
			})
		}

		// Check for PSA exceptions
		if ns.Labels != nil {
			if v, ok := ns.Labels["pod-security.kubernetes.io/enforce"]; ok && v != "" {
				if _, hasExcept := ns.Labels["pod-security.kubernetes.io/enforce-audit"]; hasExcept {
					result.Summary.WithPSAExcept++
					result.Exceptions = append(result.Exceptions, AdmissionException{
						Namespace: ns.Name,
						Type:      "psa-exception",
						Detail:    "PSA audit exception configured - privileged pods allowed",
						RiskLevel: "high",
					})
				}
			}
		}
	}

	// Check admission webhooks
	mutatingWebhooks, _ := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	validatingWebhooks, _ := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})

	hasGatekeeper := false
	hasKyverno := false
	hasCosign := false

	for _, wh := range mutatingWebhooks.Items {
		result.Summary.MutatingWebhooks++
		result.Summary.WebhookCount++
		whName := strings.ToLower(wh.Name)
		if strings.Contains(whName, "gatekeeper") {
			hasGatekeeper = true
		}
		if strings.Contains(whName, "kyverno") {
			hasKyverno = true
		}
		if strings.Contains(whName, "cosign") || strings.Contains(whName, "sigstore") {
			hasCosign = true
		}
	}
	for _, wh := range validatingWebhooks.Items {
		result.Summary.ValidatingWebhooks++
		result.Summary.WebhookCount++
		whName := strings.ToLower(wh.Name)
		if strings.Contains(whName, "gatekeeper") {
			hasGatekeeper = true
		}
		if strings.Contains(whName, "kyverno") {
			hasKyverno = true
		}
	}

	if !hasGatekeeper && !hasKyverno {
		result.Summary.MissingGatekeeper = true
		result.WebhookGaps = append(result.WebhookGaps, AdmissionWebhookGap{
			Name: "policy-engine", Type: "missing",
			Issue: "No Gatekeeper or Kyverno policy engine detected",
		})
	}
	if !hasCosign {
		result.WebhookGaps = append(result.WebhookGaps, AdmissionWebhookGap{
			Name: "image-signing", Type: "missing",
			Issue: "No Cosign/Sigstore image verification webhook detected",
		})
	}

	// Score
	if result.Summary.TotalNamespaces > 0 {
		enforcedPct := result.Summary.WithPSAEnforce * 100 / result.Summary.TotalNamespaces
		result.HealthScore = enforcedPct
	} else {
		result.HealthScore = 100
	}
	if result.Summary.MissingGatekeeper {
		result.HealthScore -= 15
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildAdmissionExcRecs1919(&result)
	writeJSON(w, result)
}

func buildAdmissionExcRecs1919(r *AdmissionExceptionResult) []string {
	recs := []string{fmt.Sprintf("Admission audit: %d namespaces (%d PSA enforced, %d without), %d webhooks, gatekeeper: %v",
		r.Summary.TotalNamespaces, r.Summary.WithPSAEnforce, r.Summary.WithoutPSA,
		r.Summary.WebhookCount, !r.Summary.MissingGatekeeper)}
	if r.Summary.WithoutPSA > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces without PSA enforcement - add pod-security labels", r.Summary.WithoutPSA))
	}
	if r.Summary.MissingGatekeeper {
		recs = append(recs, "No policy engine (Gatekeeper/Kyverno) - install for admission control")
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Proc Mount & Tmpfs Write Risk
// ---------------------------------------------------------------

type ProcMountResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         ProcMountSummary `json:"summary"`
	Violations      []ProcMountEntry `json:"violations"`
	ByNamespace     []ProcMountNS    `json:"byNamespace"`
	Recommendations []string         `json:"recommendations"`
}

type ProcMountSummary struct {
	TotalContainers   int `json:"totalContainers"`
	DefaultProcMount  int `json:"defaultProcMount"`
	UnmaskedProcMount int `json:"unmaskedProcMount"`
	WritableTmpfs     int `json:"writableTmpfs"`
	HostPathWritable  int `json:"hostPathWritable"`
	HighRiskCount     int `json:"highRiskCount"`
}

type ProcMountEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
	RiskLevel string `json:"riskLevel"`
}

type ProcMountNS struct {
	Namespace  string `json:"namespace"`
	Violations int    `json:"violations"`
}

func (s *Server) handleProcMountRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ProcMountResult{ScannedAt: time.Now()}

	nsMap := map[string]*ProcMountNS{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}

		// Check volumes for writable emptyDir (tmpfs) and hostPath
		hasWritableTmpfs := false
		hasHostPathWritable := false
		for _, vol := range dep.Spec.Template.Spec.Volumes {
			if vol.EmptyDir != nil && vol.EmptyDir.Medium == corev1.StorageMediumMemory {
				hasWritableTmpfs = true
			}
			if vol.HostPath != nil {
				typeStr := ""
				if vol.HostPath.Type != nil {
					typeStr = string(*vol.HostPath.Type)
				}
				if typeStr != string(corev1.HostPathFile) && typeStr != string(corev1.HostPathDirectory) {
					hasHostPathWritable = true
				} else if typeStr == string(corev1.HostPathDirectoryOrCreate) || typeStr == string(corev1.HostPathFileOrCreate) {
					hasHostPathWritable = true
				}
			}
		}

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			// Check procMount
			if c.SecurityContext != nil && c.SecurityContext.ProcMount != nil {
				if string(*c.SecurityContext.ProcMount) == "Unmasked" {
					result.Summary.UnmaskedProcMount++
					result.Summary.HighRiskCount++
					entry := ProcMountEntry{
						Name: dep.Name, Namespace: dep.Namespace,
						Container: c.Name,
						Issue:     "Unmasked proc mount - /proc not masked, exposes kernel info",
						RiskLevel: "critical",
					}
					result.Violations = append(result.Violations, entry)
					nsE, ok := nsMap[dep.Namespace]
					if !ok {
						nsE = &ProcMountNS{Namespace: dep.Namespace}
						nsMap[dep.Namespace] = nsE
					}
					nsE.Violations++
				}
			} else {
				result.Summary.DefaultProcMount++
			}

			// Check writable tmpfs
			if hasWritableTmpfs {
				result.Summary.WritableTmpfs++
				entry := ProcMountEntry{
					Name: dep.Name, Namespace: dep.Namespace,
					Container: c.Name,
					Issue:     "writable tmpfs (emptyDir with Memory medium) - can consume node memory",
					RiskLevel: "medium",
				}
				result.Violations = append(result.Violations, entry)
				nsE, ok := nsMap[dep.Namespace]
				if !ok {
					nsE = &ProcMountNS{Namespace: dep.Namespace}
					nsMap[dep.Namespace] = nsE
				}
				nsE.Violations++
			}

			// Check hostPath writable
			if hasHostPathWritable {
				result.Summary.HostPathWritable++
				result.Summary.HighRiskCount++
				entry := ProcMountEntry{
					Name: dep.Name, Namespace: dep.Namespace,
					Container: c.Name,
					Issue:     "writable hostPath volume - can modify host filesystem",
					RiskLevel: "high",
				}
				result.Violations = append(result.Violations, entry)
			}
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	// Score
	if result.Summary.TotalContainers > 0 {
		safePct := (result.Summary.TotalContainers - result.Summary.HighRiskCount) * 100 / result.Summary.TotalContainers
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildProcMountRecs1919(&result)
	writeJSON(w, result)
}

func buildProcMountRecs1919(r *ProcMountResult) []string {
	recs := []string{fmt.Sprintf("ProcMount risk: %d containers, %d unmasked proc, %d writable tmpfs, %d writable hostPath",
		r.Summary.TotalContainers, r.Summary.UnmaskedProcMount,
		r.Summary.WritableTmpfs, r.Summary.HostPathWritable)}
	if r.Summary.HostPathWritable > 0 {
		recs = append(recs, fmt.Sprintf("%d writable hostPath volumes - restrict to read-only or remove", r.Summary.HostPathWritable))
	}
	if r.Summary.UnmaskedProcMount > 0 {
		recs = append(recs, fmt.Sprintf("%d containers with unmasked /proc - use default masked proc mount", r.Summary.UnmaskedProcMount))
	}
	return recs
}
