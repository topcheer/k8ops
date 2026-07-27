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
// v20.13 — Deployment Dimension (Round 22)
// 1. Pod SetHostname Domain — hostname & subdomain config compliance
// 2. Container TC Egress Mark — traffic shaping & QoS annotation audit
// 3. Pod NodeSelector Validation — node selector key format compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Pod SetHostname Domain
// ---------------------------------------------------------------

type HostnameResult2013 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HostnameSummary2013 `json:"summary"`
	WithConfig      []HostnameEntry2013 `json:"withHostnameConfig"`
	Recommendations []string            `json:"recommendations"`
}

type HostnameSummary2013 struct {
	TotalPods             int `json:"totalPods"`
	WithHostname          int `json:"withHostname"`
	WithSubdomain         int `json:"withSubdomain"`
	WithSetHostnameAsFQDN int `json:"withSetHostnameAsFQDN"`
}

type HostnameEntry2013 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Hostname  string `json:"hostname"`
	Subdomain string `json:"subdomain"`
}

func (s *Server) handleHostnameDomain(w http.ResponseWriter, r *http.Request) {
	result := HostnameResult2013{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasConfig := false
		entry := HostnameEntry2013{
			Pod: pod.Name, Namespace: pod.Namespace,
		}

		if pod.Spec.Hostname != "" {
			result.Summary.WithHostname++
			entry.Hostname = pod.Spec.Hostname
			hasConfig = true
		}
		if pod.Spec.Subdomain != "" {
			result.Summary.WithSubdomain++
			entry.Subdomain = pod.Spec.Subdomain
			hasConfig = true
		}
		if pod.Spec.SetHostnameAsFQDN != nil && *pod.Spec.SetHostnameAsFQDN {
			result.Summary.WithSetHostnameAsFQDN++
			hasConfig = true
		}

		if hasConfig {
			result.WithConfig = append(result.WithConfig, entry)
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with hostname, %d with subdomain, %d FQDN", result.Summary.TotalPods, result.Summary.WithHostname, result.Summary.WithSubdomain, result.Summary.WithSetHostnameAsFQDN))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container TC Egress Mark
// ---------------------------------------------------------------

type TCEgressResult2013 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TCEgressSummary2013 `json:"summary"`
	WithMark        []TCEgressEntry2013 `json:"withTrafficMark"`
	Recommendations []string            `json:"recommendations"`
}

type TCEgressSummary2013 struct {
	TotalPods     int `json:"totalPods"`
	WithBandwidth int `json:"withBandwidthAnnotation"`
	WithTCMark    int `json:"withTCMark"`
}

type TCEgressEntry2013 struct {
	Pod        string `json:"pod"`
	Namespace  string `json:"namespace"`
	Annotation string `json:"annotation"`
	Value      string `json:"value"`
}

func (s *Server) handleTCEgressMark(w http.ResponseWriter, r *http.Request) {
	result := TCEgressResult2013{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	tcAnnotations := []string{
		"kubernetes.io/ingress-bandwidth",
		"kubernetes.io/egress-bandwidth",
		"kubernetes.io/trusted",
		"tce.mark",
		"traffic.k8s.io/mark",
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for k, v := range pod.Annotations {
			for _, ann := range tcAnnotations {
				if k == ann {
					result.Summary.WithTCMark++
					result.WithMark = append(result.WithMark, TCEgressEntry2013{
						Pod: pod.Name, Namespace: pod.Namespace,
						Annotation: k, Value: v,
					})
				}
			}
			if k == "kubernetes.io/ingress-bandwidth" || k == "kubernetes.io/egress-bandwidth" {
				result.Summary.WithBandwidth++
			}
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with bandwidth, %d with TC mark", result.Summary.TotalPods, result.Summary.WithBandwidth, result.Summary.WithTCMark))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod NodeSelector Validation
// ---------------------------------------------------------------

type NSValidResult2013 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         NSValidSummary2013 `json:"summary"`
	Selectors       []NSValidEntry2013 `json:"selectors"`
	Recommendations []string           `json:"recommendations"`
}

type NSValidSummary2013 struct {
	TotalPods    int `json:"totalPods"`
	WithSelector int `json:"withNodeSelector"`
	ValidKeys    int `json:"withValidKeys"`
	Suspicious   int `json:"suspiciousKeys"`
}

type NSValidEntry2013 struct {
	Pod       string            `json:"pod"`
	Namespace string            `json:"namespace"`
	Selectors map[string]string `json:"nodeSelector"`
}

func (s *Server) handleNodeSelValid(w http.ResponseWriter, r *http.Request) {
	result := NSValidResult2013{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Known valid label prefixes
	validPrefixes := []string{
		"kubernetes.io/", "k8s.io/", "node.kubernetes.io/",
		"beta.kubernetes.io/", "topology.kubernetes.io/",
		"node-role.", "k3s.io/", "projectcalico.org/",
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if len(pod.Spec.NodeSelector) == 0 {
			continue
		}
		result.Summary.WithSelector++

		entry := NSValidEntry2013{
			Pod: pod.Name, Namespace: pod.Namespace,
			Selectors: pod.Spec.NodeSelector,
		}

		allValid := true
		for k := range pod.Spec.NodeSelector {
			isValid := false
			for _, prefix := range validPrefixes {
				if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
					isValid = true
					break
				}
			}
			if !isValid {
				// Custom key - not necessarily wrong but worth tracking
				allValid = false
			}
		}

		if allValid {
			result.Summary.ValidKeys++
		} else {
			result.Summary.Suspicious++
		}

		result.Selectors = append(result.Selectors, entry)
	}

	if len(result.Selectors) > 30 {
		result.Selectors = result.Selectors[:30]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with selector (%d valid, %d suspicious)", result.Summary.TotalPods, result.Summary.WithSelector, result.Summary.ValidKeys, result.Summary.Suspicious))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
