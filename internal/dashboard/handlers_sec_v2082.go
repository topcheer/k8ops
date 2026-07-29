package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.82 — Security Dimension (Round 33)
// 1. NetworkPolicy Egress Audit — egress rule coverage
// 2. Audit Log Configuration — API audit policy check
// 3. Certificate Age Tracker — cert-manager cert age
// ============================================================

type EgressAuditResult2082 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         EgressAuditSummary2082 `json:"summary"`
	OpenEgress      []EgressAuditEntry2082 `json:"openEgressNS"`
	Recommendations []string               `json:"recommendations"`
}

type EgressAuditSummary2082 struct {
	TotalNS    int `json:"totalNamespaces"`
	WithEgress int `json:"withEgressPolicy"`
	OpenEgress int `json:"openEgress"`
}

type EgressAuditEntry2082 struct {
	Namespace string `json:"namespace"`
}

func (s *Server) handleEgressAudit2082(w http.ResponseWriter, r *http.Request) {
	result := EgressAuditResult2082{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	nsEgress := make(map[string]bool)
	for _, np := range npList.Items {
		for _, policy := range np.Spec.PolicyTypes {
			if policy == "Egress" {
				nsEgress[np.Namespace] = true
			}
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsEgress[ns.Name] {
			result.Summary.WithEgress++
		} else {
			result.Summary.OpenEgress++
			result.OpenEgress = append(result.OpenEgress, EgressAuditEntry2082{Namespace: ns.Name})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.OpenEgress > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces without egress NetworkPolicy", result.Summary.OpenEgress))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Audit Log Configuration
// ---------------------------------------------------------------

type AuditLogResult2082 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AuditLogSummary2082 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type AuditLogSummary2082 struct {
	PodCount     int  `json:"auditPods"`
	AuditEnabled bool `json:"auditEnabled"`
}

func (s *Server) handleAuditLogConfig2082(w http.ResponseWriter, r *http.Request) {
	result := AuditLogResult2082{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("kube-system").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, cmd := range c.Command {
				if len(cmd) > 5 && cmd[:5] == "kube-" {
					for _, arg := range c.Args {
						if containsStr2039(arg, "audit-log") {
							result.Summary.AuditEnabled = true
						}
					}
				}
			}
		}
		if containsStr2039(pod.Name, "audit") {
			result.Summary.PodCount++
		}
	}

	if !result.Summary.AuditEnabled {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if !result.Summary.AuditEnabled {
		result.Recommendations = append(result.Recommendations,
			"API server audit logging may not be configured — enable for compliance")
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Certificate Age Tracker
// ---------------------------------------------------------------

type CertAgeResult2082 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CertAgeSummary2082 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type CertAgeSummary2082 struct {
	TotalSecrets int `json:"totalCertSecrets"`
	OldCerts     int `json:"oldCerts"`
}

func (s *Server) handleCertAge2082(w http.ResponseWriter, r *http.Request) {
	result := CertAgeResult2082{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, secret := range secretList.Items {
		if secret.Type != "kubernetes.io/tls" && secret.Type != "cert-manager.io/v1" {
			continue
		}
		result.Summary.TotalSecrets++
		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)
		if ageDays > 90 {
			result.Summary.OldCerts++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.OldCerts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d TLS certificates older than 90 days — set up rotation", result.Summary.OldCerts))
	}
	writeJSON(w, result)
}
