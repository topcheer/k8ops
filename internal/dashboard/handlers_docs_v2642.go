package dashboard

import (
	"net/http"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.42 Documentation: PodTolerations, NodeTaintsCount, CRDVersions

type PodTolerations2642Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     PodTolerations2642Summary `json:"summary"`
	Items       []PodTolerations2642Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type PodTolerations2642Summary struct {
	TotalPods       int `json:"totalPods"`
	WithTolerations int `json:"withTolerations"`
	NoTolerations   int `json:"noTolerations"`
}

type PodTolerations2642Item struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	TolerationCount int    `json:"tolerationCount"`
}

func (s *Server) handlePodTolerations2642(w http.ResponseWriter, r *http.Request) {
	result := PodTolerations2642Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			cnt := len(pod.Spec.Tolerations)
			if cnt > 0 {
				result.Summary.WithTolerations++
			} else {
				result.Summary.NoTolerations++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodTolerations2642Item{
					Name: pod.Name, Namespace: pod.Namespace, TolerationCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeTaintsCount2642Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     NodeTaintsCount2642Summary `json:"summary"`
	Items       []NodeTaintsCount2642Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type NodeTaintsCount2642Summary struct {
	TotalNodes     int `json:"totalNodes"`
	WithTaints     int `json:"withTaints"`
	TaintFreeNodes int `json:"taintFreeNodes"`
}

type NodeTaintsCount2642Item struct {
	Name       string `json:"name"`
	TaintCount int    `json:"taintCount"`
}

func (s *Server) handleNodeTaintsCount2642(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintsCount2642Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			cnt := len(node.Spec.Taints)
			if cnt > 0 {
				result.Summary.WithTaints++
			} else {
				result.Summary.TaintFreeNodes++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeTaintsCount2642Item{
					Name: node.Name, TaintCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRDVersions2642Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     CRDVersions2642Summary `json:"summary"`
	Items       []CRDVersions2642Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type CRDVersions2642Summary struct {
	TotalCRDs       int `json:"totalCRDs"`
	WithMultipleVer int `json:"withMultipleVersions"`
}

type CRDVersions2642Item struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

func (s *Server) handleCRDVersions2642(w http.ResponseWriter, r *http.Request) {
	result := CRDVersions2642Result{ScannedAt: time.Now()}
	if s.k8sClient != nil {
		crdList := &apiextensionsv1.CustomResourceDefinitionList{}
		if err := s.k8sClient.List(r.Context(), crdList); err == nil {
			for _, crd := range crdList.Items {
				result.Summary.TotalCRDs++
				vers := make([]string, 0, len(crd.Spec.Versions))
				for _, v := range crd.Spec.Versions {
					vers = append(vers, v.Name)
				}
				if len(vers) > 1 {
					result.Summary.WithMultipleVer++
				}
				if len(result.Items) < 50 {
					result.Items = append(result.Items, CRDVersions2642Item{
						Name: crd.Name, Versions: vers,
					})
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
