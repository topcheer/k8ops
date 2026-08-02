package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.66 Documentation: PodPriorityAudit, NodeKubeProxyVer, IngressClassName

type PodPriority2666Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     PodPriority2666Summary `json:"summary"`
	Items       []PodPriority2666Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type PodPriority2666Summary struct {
	TotalPods    int `json:"totalPods"`
	WithPriority int `json:"withPriority"`
	NoPriority   int `json:"noPriority"`
}

type PodPriority2666Item struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	PriorityClass string `json:"priorityClass"`
}

func (s *Server) handlePodPriority2666(w http.ResponseWriter, r *http.Request) {
	result := PodPriority2666Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			pc := pod.Spec.PriorityClassName
			if pc != "" {
				result.Summary.WithPriority++
			} else {
				result.Summary.NoPriority++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodPriority2666Item{
					Name: pod.Name, Namespace: pod.Namespace, PriorityClass: pc,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeKubeProxyVer2666Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     NodeKubeProxyVer2666Summary `json:"summary"`
	Items       []NodeKubeProxyVer2666Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type NodeKubeProxyVer2666Summary struct {
	TotalNodes     int `json:"totalNodes"`
	UniqueVersions int `json:"uniqueVersions"`
}

type NodeKubeProxyVer2666Item struct {
	Name         string `json:"name"`
	KubeProxyVer string `json:"kubeProxyVersion"`
}

func (s *Server) handleNodeKubeProxyVer2666(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeProxyVer2666Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		verSet := map[string]bool{}
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			kv := node.Status.NodeInfo.KubeProxyVersion
			verSet[kv] = true
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeKubeProxyVer2666Item{
					Name: node.Name, KubeProxyVer: kv,
				})
			}
		}
		result.Summary.UniqueVersions = len(verSet)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IngressClassName2666Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     IngressClassName2666Summary `json:"summary"`
	Items       []IngressClassName2666Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type IngressClassName2666Summary struct {
	TotalIngress  int `json:"totalIngress"`
	WithClassName int `json:"withClassName"`
	NoClassName   int `json:"noClassName"`
}

type IngressClassName2666Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ClassName string `json:"className"`
}

func (s *Server) handleIngressClassName2666(w http.ResponseWriter, r *http.Request) {
	result := IngressClassName2666Result{ScannedAt: time.Now()}
	ingresses, err := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			result.Summary.TotalIngress++
			cn := ""
			if ing.Spec.IngressClassName != nil {
				cn = *ing.Spec.IngressClassName
			}
			if cn != "" {
				result.Summary.WithClassName++
			} else {
				result.Summary.NoClassName++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, IngressClassName2666Item{
					Name: ing.Name, Namespace: ing.Namespace, ClassName: cn,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
