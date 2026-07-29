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
// v20.52 — Security Dimension (Round 28)
// 1. Secret Data Volume Audit — secrets mounted as env vs volume
// 2. NetworkPolicy Default Deny Check — namespaces missing default deny
// 3. Admission Webhook Risk — webhook configurations with failurePolicy Fail
// ============================================================

// ---------------------------------------------------------------
// 1. Secret Data Volume Audit
// ---------------------------------------------------------------

type SecDataVolResult2052 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SecDataVolSummary2052 `json:"summary"`
	EnvSecrets      []SecDataVolEntry2052 `json:"envSecrets"`
	Recommendations []string              `json:"recommendations"`
}

type SecDataVolSummary2052 struct {
	TotalSecrets  int `json:"totalSecrets"`
	VolumeSecrets int `json:"volumeMounted"`
	EnvSecrets    int `json:"envVarExposed"`
}

type SecDataVolEntry2052 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSecDataVolAudit(w http.ResponseWriter, r *http.Request) {
	result := SecDataVolResult2052{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalSecrets = len(secretList.Items)
	volSecretSet := make(map[string]bool)
	envSecretSet := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				key := pod.Namespace + "/" + vol.Secret.SecretName
				volSecretSet[key] = true
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					key := pod.Namespace + "/" + env.ValueFrom.SecretKeyRef.Name
					envSecretSet[key] = true
				}
			}
		}
	}

	result.Summary.VolumeSecrets = len(volSecretSet)
	result.Summary.EnvSecrets = len(envSecretSet)
	score -= result.Summary.EnvSecrets

	for k := range envSecretSet {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 {
			result.EnvSecrets = append(result.EnvSecrets, SecDataVolEntry2052{Name: parts[1], Namespace: parts[0]})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.EnvSecrets, func(i, j int) bool {
		return result.EnvSecrets[i].Namespace < result.EnvSecrets[j].Namespace
	})

	if result.Summary.EnvSecrets > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d secrets exposed via env vars — prefer volume mounts", result.Summary.EnvSecrets))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. NetworkPolicy Default Deny Check
// ---------------------------------------------------------------

type DefaultDenyResult2052 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         DefaultDenySummary2052 `json:"summary"`
	UnprotectedNS   []DefaultDenyEntry2052 `json:"unprotectedNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

type DefaultDenySummary2052 struct {
	TotalNS         int `json:"totalNamespaces"`
	WithDefaultDeny int `json:"withDefaultDeny"`
	Unprotected     int `json:"unprotected"`
}

type DefaultDenyEntry2052 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleDefaultDenyCheck(w http.ResponseWriter, r *http.Request) {
	result := DefaultDenyResult2052{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Check which namespaces have default deny ingress
	nsDefaultDeny := make(map[string]bool)
	for _, np := range npList.Items {
		if np.Spec.PodSelector.Size() == 0 { // empty selector = all pods
			for _, policy := range np.Spec.PolicyTypes {
				if policy == "Ingress" && len(np.Spec.Ingress) == 0 {
					nsDefaultDeny[np.Namespace] = true
				}
			}
		}
	}

	podCountNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCountNS[pod.Namespace]++
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsDefaultDeny[ns.Name] {
			result.Summary.WithDefaultDeny++
		} else {
			result.Summary.Unprotected++
			result.UnprotectedNS = append(result.UnprotectedNS, DefaultDenyEntry2052{
				Namespace: ns.Name, PodCount: podCountNS[ns.Name],
			})
			if podCountNS[ns.Name] > 0 {
				score -= 3
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.UnprotectedNS, func(i, j int) bool {
		return result.UnprotectedNS[i].PodCount > result.UnprotectedNS[j].PodCount
	})

	if result.Summary.Unprotected > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces without default deny NetworkPolicy", result.Summary.Unprotected))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Admission Webhook Risk
// ---------------------------------------------------------------

type WebhookRiskResult2052 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         WebhookRiskSummary2052 `json:"summary"`
	FailWebhooks    []WebhookRiskEntry2052 `json:"failWebhooks"`
	Recommendations []string               `json:"recommendations"`
}

type WebhookRiskSummary2052 struct {
	TotalWebhooks   int `json:"totalWebhooks"`
	MutatingCount   int `json:"mutatingCount"`
	ValidatingCount int `json:"validatingCount"`
	FailPolicyCount int `json:"failPolicyCount"`
}

type WebhookRiskEntry2052 struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	FailurePolicy string `json:"failurePolicy"`
}

func (s *Server) handleWebhookRiskAudit(w http.ResponseWriter, r *http.Request) {
	result := WebhookRiskResult2052{ScannedAt: time.Now()}
	score := 100

	mwcList, _ := s.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	vwcList, _ := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})

	result.Summary.MutatingCount = len(mwcList.Items)
	result.Summary.ValidatingCount = len(vwcList.Items)
	result.Summary.TotalWebhooks = result.Summary.MutatingCount + result.Summary.ValidatingCount

	for _, mwc := range mwcList.Items {
		for _, wh := range mwc.Webhooks {
			policy := string(*wh.FailurePolicy)
			if policy == "Fail" {
				result.Summary.FailPolicyCount++
				result.FailWebhooks = append(result.FailWebhooks, WebhookRiskEntry2052{
					Name: wh.Name, Type: "mutating", FailurePolicy: "Fail",
				})
				score -= 2
			}
		}
	}

	for _, vwc := range vwcList.Items {
		for _, wh := range vwc.Webhooks {
			policy := string(*wh.FailurePolicy)
			if policy == "Fail" {
				result.Summary.FailPolicyCount++
				result.FailWebhooks = append(result.FailWebhooks, WebhookRiskEntry2052{
					Name: wh.Name, Type: "validating", FailurePolicy: "Fail",
				})
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.FailWebhooks, func(i, j int) bool {
		return result.FailWebhooks[i].Type < result.FailWebhooks[j].Type
	})

	if result.Summary.FailPolicyCount > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d webhooks with failurePolicy=Fail — API server blocked if webhook is down", result.Summary.FailPolicyCount))
	}

	writeJSON(w, result)
}
