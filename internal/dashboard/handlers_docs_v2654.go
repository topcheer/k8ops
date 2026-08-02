package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.54 Documentation: PodAffinityCount, NodeArchDist, EndpointSliceCount

type PodAffinityCount2654Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     PodAffinityCount2654Summary `json:"summary"`
	Items       []PodAffinityCount2654Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type PodAffinityCount2654Summary struct {
	TotalPods    int `json:"totalPods"`
	WithAffinity int `json:"withAffinity"`
	NoAffinity   int `json:"noAffinity"`
}

type PodAffinityCount2654Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	AffinityRules int    `json:"affinityRules"`
}

func (s *Server) handlePodAffinityCount2654(w http.ResponseWriter, r *http.Request) {
	result := PodAffinityCount2654Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			rules := 0
			if pod.Spec.Affinity != nil {
				if pod.Spec.Affinity.PodAffinity != nil {
					rules += len(pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
					rules += len(pod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
				}
				if pod.Spec.Affinity.PodAntiAffinity != nil {
					rules += len(pod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
					rules += len(pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
				}
			}
			if rules > 0 {
				result.Summary.WithAffinity++
			} else {
				result.Summary.NoAffinity++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodAffinityCount2654Item{
					Name: pod.Name, Namespace: pod.Namespace, AffinityRules: rules,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeArchDist2654Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     NodeArchDist2654Summary `json:"summary"`
	Items       []NodeArchDist2654Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type NodeArchDist2654Summary struct {
	TotalNodes int `json:"totalNodes"`
	Amd64Nodes int `json:"amd64Nodes"`
	Arm64Nodes int `json:"arm64Nodes"`
	OtherArch  int `json:"otherArch"`
}

type NodeArchDist2654Item struct {
	Name string `json:"name"`
	Arch string `json:"arch"`
}

func (s *Server) handleNodeArchDist2654(w http.ResponseWriter, r *http.Request) {
	result := NodeArchDist2654Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			arch := node.Status.NodeInfo.Architecture
			switch arch {
			case "amd64":
				result.Summary.Amd64Nodes++
			case "arm64":
				result.Summary.Arm64Nodes++
			default:
				result.Summary.OtherArch++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeArchDist2654Item{
					Name: node.Name, Arch: arch,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EndpointSliceCount2654Result struct {
	ScannedAt   time.Time                     `json:"scannedAt"`
	Summary     EndpointSliceCount2654Summary `json:"summary"`
	Items       []EndpointSliceCount2654Item  `json:"items"`
	HealthScore int                           `json:"healthScore"`
	Grade       string                        `json:"grade"`
}

type EndpointSliceCount2654Summary struct {
	TotalSlices    int `json:"totalSlices"`
	TotalEndpoints int `json:"totalEndpoints"`
}

type EndpointSliceCount2654Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	EndpointCount int    `json:"endpointCount"`
}

func (s *Server) handleEndpointSliceCount2654(w http.ResponseWriter, r *http.Request) {
	result := EndpointSliceCount2654Result{ScannedAt: time.Now()}
	slices, err := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sl := range slices.Items {
			result.Summary.TotalSlices++
			result.Summary.TotalEndpoints += len(sl.Endpoints)
			if len(result.Items) < 50 {
				result.Items = append(result.Items, EndpointSliceCount2654Item{
					Name: sl.Name, Namespace: sl.Namespace, EndpointCount: len(sl.Endpoints),
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
