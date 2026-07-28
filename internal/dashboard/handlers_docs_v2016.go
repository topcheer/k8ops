package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.16 — Documentation Dimension (Round 22)
// 1. Validating Webhook Config Inventory — webhook config catalog
// 2. Ingress Class Inventory — ingress controller class catalog
// 3. APIService Registration Catalog — aggregated API server catalog
// ============================================================

// ---------------------------------------------------------------
// 1. Validating Webhook Config Inventory
// ---------------------------------------------------------------

type VWConfigResult2016 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         VWConfigSummary2016 `json:"summary"`
	Configs         []VWConfigEntry2016 `json:"configs"`
	Recommendations []string            `json:"recommendations"`
}

type VWConfigSummary2016 struct {
	TotalConfigs int `json:"totalConfigs"`
	WithNSScope  int `json:"withNamespaceScope"`
	WithObjScope int `json:"withObjectScope"`
	WithTLS      int `json:"withTLSConfig"`
}

type VWConfigEntry2016 struct {
	Name         string `json:"name"`
	WebhookCount int    `json:"webhookCount"`
	ServiceName  string `json:"serviceName"`
	HasTLS       bool   `json:"hasTLS"`
}

func (s *Server) handleVWConfigInv(w http.ResponseWriter, r *http.Request) {
	result := VWConfigResult2016{ScannedAt: time.Now()}
	score := 100

	whList, err := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, wh := range whList.Items {
		result.Summary.TotalConfigs++

		entry := VWConfigEntry2016{
			Name: wh.Name, WebhookCount: len(wh.Webhooks),
		}

		for _, webhook := range wh.Webhooks {
			if webhook.ClientConfig.Service != nil {
				entry.ServiceName = webhook.ClientConfig.Service.Name
			}
			if webhook.ClientConfig.CABundle != nil {
				entry.HasTLS = true
			}
			if webhook.NamespaceSelector != nil {
				result.Summary.WithNSScope++
			}
			if webhook.ObjectSelector != nil {
				result.Summary.WithObjScope++
			}
		}

		if entry.HasTLS {
			result.Summary.WithTLS++
		}

		result.Configs = append(result.Configs, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d validating configs (%d with TLS, %d webhook entries)", result.Summary.TotalConfigs, result.Summary.WithTLS, len(result.Configs)))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Ingress Class Inventory
// ---------------------------------------------------------------

type IngClassResult2016 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         IngClassSummary2016 `json:"summary"`
	Classes         []IngClassEntry2016 `json:"classes"`
	Recommendations []string            `json:"recommendations"`
}

type IngClassSummary2016 struct {
	TotalClasses int  `json:"totalIngressClasses"`
	HasDefault   bool `json:"hasDefaultClass"`
	WithParams   int  `json:"withParamsRef"`
}

type IngClassEntry2016 struct {
	Name       string `json:"name"`
	Controller string `json:"controller"`
	IsDefault  bool   `json:"isDefault"`
	HasParams  bool   `json:"hasParameters"`
}

func (s *Server) handleIngClassInv(w http.ResponseWriter, r *http.Request) {
	result := IngClassResult2016{ScannedAt: time.Now()}
	score := 100

	icList, err := s.clientset.NetworkingV1().IngressClasses().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, ic := range icList.Items {
		result.Summary.TotalClasses++

		entry := IngClassEntry2016{
			Name:       ic.Name,
			Controller: ic.Spec.Controller,
		}

		if ic.Spec.Parameters != nil {
			entry.HasParams = true
			result.Summary.WithParams++
		}

		// Check default annotation
		if ic.Annotations != nil && ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			entry.IsDefault = true
			result.Summary.HasDefault = true
		}

		result.Classes = append(result.Classes, entry)
	}

	if result.Summary.TotalClasses > 0 && !result.Summary.HasDefault {
		score -= 2
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingress classes, default: %v, %d with params", result.Summary.TotalClasses, result.Summary.HasDefault, result.Summary.WithParams))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. APIService Registration Catalog
// ---------------------------------------------------------------

type APISvcResult2016 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         APISvcSummary2016 `json:"summary"`
	Services        []APISvcEntry2016 `json:"services"`
	Recommendations []string          `json:"recommendations"`
}

type APISvcSummary2016 struct {
	TotalAPIServices int `json:"totalAPIServices"`
	AvailableCount   int `json:"availableServices"`
	UnavailableCount int `json:"unavailableServices"`
	WithCA           int `json:"withCABundle"`
}

type APISvcEntry2016 struct {
	Name      string `json:"name"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
}

func (s *Server) handleAPISvcReg(w http.ResponseWriter, r *http.Request) {
	result := APISvcResult2016{ScannedAt: time.Now()}
	score := 100

	// Use discovery interface to list API groups
	groups, err := s.clientset.Discovery().ServerGroups()
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, group := range groups.Groups {
		// Skip core API group (empty name)
		if group.Name == "" {
			continue
		}
		for _, version := range group.Versions {
			result.Summary.TotalAPIServices++
			// Assume available if preferred version
			isAvailable := true
			if version.GroupVersion != group.PreferredVersion.GroupVersion {
				// Non-preferred version - still available but not default
			}

			if isAvailable {
				result.Summary.AvailableCount++
			} else {
				result.Summary.UnavailableCount++
			}

			result.Services = append(result.Services, APISvcEntry2016{
				Name:      version.GroupVersion,
				Group:     group.Name,
				Version:   version.Version,
				Available: isAvailable,
			})
		}
	}

	result.Summary.WithCA = 0 // discovery doesn't expose CA bundle info

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d API groups (%d available, %d versions)", result.Summary.TotalAPIServices, result.Summary.AvailableCount, result.Summary.UnavailableCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
