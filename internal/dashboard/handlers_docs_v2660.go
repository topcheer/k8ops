package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.60 Documentation: PodVolumeCount, NodeKubeletVersion, IngressTLSAudit

type PodVolumeCount2660Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     PodVolumeCount2660Summary `json:"summary"`
	Items       []PodVolumeCount2660Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type PodVolumeCount2660Summary struct {
	TotalPods   int `json:"totalPods"`
	MultiVolume int `json:"multiVolume"`
	NoVolume    int `json:"noVolume"`
}

type PodVolumeCount2660Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	VolumeCount int    `json:"volumeCount"`
}

func (s *Server) handlePodVolumeCount2660(w http.ResponseWriter, r *http.Request) {
	result := PodVolumeCount2660Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			cnt := len(pod.Spec.Volumes)
			if cnt > 1 {
				result.Summary.MultiVolume++
			} else {
				result.Summary.NoVolume++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodVolumeCount2660Item{
					Name: pod.Name, Namespace: pod.Namespace, VolumeCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeKubeletVersion2660Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     NodeKubeletVersion2660Summary `json:"summary"`
	Items       []NodeKubeletVersion2660Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type NodeKubeletVersion2660Summary struct {
	TotalNodes     int `json:"totalNodes"`
	UniqueVersions int `json:"uniqueVersions"`
}

type NodeKubeletVersion2660Item struct {
	Name           string `json:"name"`
	KubeletVersion string `json:"kubeletVersion"`
}

func (s *Server) handleNodeKubeletVersion2660(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeletVersion2660Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		verSet := map[string]bool{}
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			kv := node.Status.NodeInfo.KubeletVersion
			if !verSet[kv] {
				verSet[kv] = true
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeKubeletVersion2660Item{
					Name: node.Name, KubeletVersion: kv,
				})
			}
		}
		result.Summary.UniqueVersions = len(verSet)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IngressTLSAudit2660Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     IngressTLSAudit2660Summary `json:"summary"`
	Items       []IngressTLSAudit2660Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type IngressTLSAudit2660Summary struct {
	TotalIngress int `json:"totalIngress"`
	WithTLS      int `json:"withTLS"`
	WithoutTLS   int `json:"withoutTLS"`
}

type IngressTLSAudit2660Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HasTLS    bool   `json:"hasTLS"`
}

func (s *Server) handleIngressTLSAudit2660(w http.ResponseWriter, r *http.Request) {
	result := IngressTLSAudit2660Result{ScannedAt: time.Now()}
	ingresses, err := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			result.Summary.TotalIngress++
			hasTLS := len(ing.Spec.TLS) > 0
			if hasTLS {
				result.Summary.WithTLS++
			} else {
				result.Summary.WithoutTLS++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, IngressTLSAudit2660Item{
					Name: ing.Name, Namespace: ing.Namespace, HasTLS: hasTLS,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
