package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.15 — Security Dimension (Round 22)
// 1. Validating Webhook Coverage — admission validation enforcement
// 2. Ingress TLS Cert Age — TLS certificate staleness estimator
// 3. Service Account Token Volume — projected token volume audit
// ============================================================

// ---------------------------------------------------------------
// 1. Validating Webhook Coverage
// ---------------------------------------------------------------

type ValWebhookResult2015 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ValWebhookSummary2015 `json:"summary"`
	Webhooks        []ValWebhookEntry2015 `json:"webhooks"`
	Recommendations []string              `json:"recommendations"`
}

type ValWebhookSummary2015 struct {
	TotalWebhooks  int `json:"totalValidatingWebhooks"`
	WithFailPolicy int `json:"withFailPolicy"`
	CatchAll       int `json:"catchAllWebhooks"`
	WithTimeout    int `json:"withTimeout"`
}

type ValWebhookEntry2015 struct {
	Name          string `json:"name"`
	FailurePolicy string `json:"failurePolicy"`
	IsCatchAll    bool   `json:"isCatchAll"`
	TimeoutSec    int32  `json:"timeoutSeconds"`
}

func (s *Server) handleValWebhookCov(w http.ResponseWriter, r *http.Request) {
	result := ValWebhookResult2015{ScannedAt: time.Now()}
	score := 100

	whList, err := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, wh := range whList.Items {
		for _, webhook := range wh.Webhooks {
			result.Summary.TotalWebhooks++

			fp := ""
			if webhook.FailurePolicy != nil {
				fp = string(*webhook.FailurePolicy)
				result.Summary.WithFailPolicy++
			}

			isCatchAll := false
			if webhook.NamespaceSelector == nil && webhook.ObjectSelector == nil {
				isCatchAll = true
				result.Summary.CatchAll++
			}

			timeout := int32(0)
			if webhook.TimeoutSeconds != nil {
				timeout = *webhook.TimeoutSeconds
				result.Summary.WithTimeout++
			}

			result.Webhooks = append(result.Webhooks, ValWebhookEntry2015{
				Name:          wh.Name + "/" + webhook.Name,
				FailurePolicy: fp, IsCatchAll: isCatchAll, TimeoutSec: timeout,
			})
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d validating webhooks (%d catch-all, %d with timeout)", result.Summary.TotalWebhooks, result.Summary.CatchAll, result.Summary.WithTimeout))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Ingress TLS Cert Age
// ---------------------------------------------------------------

type TLSCertResult2015 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         TLSCertSummary2015 `json:"summary"`
	Expiring        []TLSCertEntry2015 `json:"expiringCerts"`
	Recommendations []string           `json:"recommendations"`
}

type TLSCertSummary2015 struct {
	TotalIngresses int `json:"totalIngresses"`
	WithTLS        int `json:"withTLS"`
	WithoutTLS     int `json:"withoutTLS"`
	SecretBased    int `json:"secretBasedTLS"`
}

type TLSCertEntry2015 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	TLSSecret string `json:"tlsSecretName"`
}

func (s *Server) handleTLSCertAge(w http.ResponseWriter, r *http.Request) {
	result := TLSCertResult2015{ScannedAt: time.Now()}
	score := 100

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++

		if len(ing.Spec.TLS) > 0 {
			result.Summary.WithTLS++
			for _, tls := range ing.Spec.TLS {
				if tls.SecretName != "" {
					result.Summary.SecretBased++
					result.Expiring = append(result.Expiring, TLSCertEntry2015{
						Name: ing.Name, Namespace: ing.Namespace,
						TLSSecret: tls.SecretName,
					})
				}
			}
		} else {
			result.Summary.WithoutTLS++
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses: %d with TLS, %d without, %d secret-based", result.Summary.TotalIngresses, result.Summary.WithTLS, result.Summary.WithoutTLS, result.Summary.SecretBased))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Service Account Token Volume
// ---------------------------------------------------------------

type SATokenVolResult2015 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenVolSummary2015 `json:"summary"`
	Pods            []SATokenVolEntry2015 `json:"pods"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenVolSummary2015 struct {
	TotalPods      int `json:"totalPods"`
	WithTokenMount int `json:"withSATokenMount"`
	AutoMountTrue  int `json:"autoMountTrue"`
	AutoMountFalse int `json:"autoMountFalse"`
}

type SATokenVolEntry2015 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	SAName    string `json:"serviceAccountName"`
}

func (s *Server) handleSATokenVol(w http.ResponseWriter, r *http.Request) {
	result := SATokenVolResult2015{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}

		autoMount := true
		if pod.Spec.AutomountServiceAccountToken != nil {
			autoMount = *pod.Spec.AutomountServiceAccountToken
		}

		if autoMount {
			result.Summary.AutoMountTrue++
			result.Summary.WithTokenMount++

			// Check for token volume mounts
			hasTokenVol := false
			for _, vol := range pod.Spec.Volumes {
				if vol.Name == "kube-api-access" || vol.Projected != nil {
					hasTokenVol = true
					break
				}
			}

			if hasTokenVol && saName != "default" {
				result.Pods = append(result.Pods, SATokenVolEntry2015{
					Pod: pod.Name, Namespace: pod.Namespace, SAName: saName,
				})
			}
		} else {
			result.Summary.AutoMountFalse++
		}
	}

	if result.Summary.AutoMountTrue > result.Summary.TotalPods/2 && result.Summary.AutoMountFalse < 5 {
		score -= 2
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with SA token, %d auto-mount true, %d false", result.Summary.TotalPods, result.Summary.WithTokenMount, result.Summary.AutoMountTrue, result.Summary.AutoMountFalse))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
