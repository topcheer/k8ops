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
// v20.73 — Product Dimension (Round 32)
// 1. Pod Affinity Rule Catalog — affinity/anti-affinity usage map
// 2. Ingress TLS Coverage — TLS-enabled ingress percentage
// 3. ConfigMap Rotation Tracker — configmap update frequency
// ============================================================

type AffinityCatResult2073 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         AffinityCatSummary2073 `json:"summary"`
	WithAffinity    []AffinityCatEntry2073 `json:"withAffinity"`
	Recommendations []string               `json:"recommendations"`
}

type AffinityCatSummary2073 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithAffinity int `json:"withAffinity"`
	WithAntiAff  int `json:"withAntiAffinity"`
}

type AffinityCatEntry2073 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"affinityType"`
}

func (s *Server) handleAffinityCat2073(w http.ResponseWriter, r *http.Request) {
	result := AffinityCatResult2073{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		aff := dep.Spec.Template.Spec.Affinity
		if aff == nil {
			continue
		}

		if aff.NodeAffinity != nil {
			result.Summary.WithAffinity++
			result.WithAffinity = append(result.WithAffinity, AffinityCatEntry2073{
				Name: dep.Name, Namespace: dep.Namespace, Type: "nodeAffinity",
			})
		}
		if aff.PodAffinity != nil {
			result.Summary.WithAffinity++
			result.WithAffinity = append(result.WithAffinity, AffinityCatEntry2073{
				Name: dep.Name, Namespace: dep.Namespace, Type: "podAffinity",
			})
		}
		if aff.PodAntiAffinity != nil {
			result.Summary.WithAntiAff++
			result.WithAffinity = append(result.WithAffinity, AffinityCatEntry2073{
				Name: dep.Name, Namespace: dep.Namespace, Type: "podAntiAffinity",
			})
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WithAffinity, func(i, j int) bool { return result.WithAffinity[i].Type < result.WithAffinity[j].Type })

	if result.Summary.WithAntiAff == 0 {
		result.Recommendations = append(result.Recommendations,
			"No deployments use podAntiAffinity — add for better availability")
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Ingress TLS Coverage
// ---------------------------------------------------------------

type IngTLSResult2073 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         IngTLSSummary2073 `json:"summary"`
	NoTLS           []IngTLSEntry2073 `json:"noTLSIngresses"`
	Recommendations []string          `json:"recommendations"`
}

type IngTLSSummary2073 struct {
	TotalIngresses int `json:"totalIngresses"`
	WithTLS        int `json:"withTLS"`
	NoTLS          int `json:"noTLS"`
}

type IngTLSEntry2073 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleIngTLSCoverage(w http.ResponseWriter, r *http.Request) {
	result := IngTLSResult2073{ScannedAt: time.Now()}
	score := 100

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++

		if len(ing.Spec.TLS) > 0 {
			result.Summary.WithTLS++
		} else {
			result.Summary.NoTLS++
			result.NoTLS = append(result.NoTLS, IngTLSEntry2073{
				Name: ing.Name, Namespace: ing.Namespace,
			})
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoTLS, func(i, j int) bool { return result.NoTLS[i].Namespace < result.NoTLS[j].Namespace })

	if result.Summary.NoTLS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ingresses without TLS — add cert-manager", result.Summary.NoTLS))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. ConfigMap Rotation Tracker
// ---------------------------------------------------------------

type CMRotResult2073 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         CMRotSummary2073 `json:"summary"`
	StaleCMs        []CMRotEntry2073 `json:"staleConfigMaps"`
	Recommendations []string         `json:"recommendations"`
}

type CMRotSummary2073 struct {
	TotalCMs int `json:"totalConfigMaps"`
	StaleCMs int `json:"staleConfigMaps"`
	FreshCMs int `json:"freshConfigMaps"`
}

type CMRotEntry2073 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
}

func (s *Server) handleCMRotTracker(w http.ResponseWriter, r *http.Request) {
	result := CMRotResult2073{ScannedAt: time.Now()}
	score := 100

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		ageDays := int(now.Sub(cm.CreationTimestamp.Time).Hours() / 24)

		if ageDays > 180 {
			result.Summary.StaleCMs++
			result.StaleCMs = append(result.StaleCMs, CMRotEntry2073{
				Name: cm.Name, Namespace: cm.Namespace, AgeDays: ageDays,
			})
		} else {
			result.Summary.FreshCMs++
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.StaleCMs, func(i, j int) bool { return result.StaleCMs[i].AgeDays > result.StaleCMs[j].AgeDays })

	if result.Summary.StaleCMs > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ConfigMaps older than 180 days — review for cleanup", result.Summary.StaleCMs))
	}
	writeJSON(w, result)
}

// keep import
var _ = corev1.Pod{}
