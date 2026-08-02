package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.43 Scalability: HPAResourceType, TopWorkloadsByMemory, NSNetworkPolicyCount

type HPAResourceType2643Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     HPAResourceType2643Summary `json:"summary"`
	Items       []HPAResourceType2643Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type HPAResourceType2643Summary struct {
	TotalHPA    int `json:"totalHPA"`
	CPUBased    int `json:"cpuBased"`
	MemoryBased int `json:"memoryBased"`
}

type HPAResourceType2643Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
}

func (s *Server) handleHPAResourceType2643(w http.ResponseWriter, r *http.Request) {
	result := HPAResourceType2643Result{ScannedAt: time.Now()}
	hpas, err := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			result.Summary.TotalHPA++
			resType := "none"
			if hpa.Spec.Metrics != nil {
				for _, m := range hpa.Spec.Metrics {
					if m.Type == "Resource" && m.Resource != nil && m.Resource.Name != "" {
						resType = string(m.Resource.Name)
						if resType == "cpu" {
							result.Summary.CPUBased++
						} else if resType == "memory" {
							result.Summary.MemoryBased++
						}
						break
					}
				}
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, HPAResourceType2643Item{
					Name: hpa.Name, Namespace: hpa.Namespace, Resource: resType,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopWorkloadsByMem2643Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     TopWorkloadsByMem2643Summary `json:"summary"`
	Items       []TopWorkloadsByMem2643Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type TopWorkloadsByMem2643Summary struct {
	TotalPods       int `json:"totalPods"`
	HighMemRequests int `json:"highMemRequests"`
}

type TopWorkloadsByMem2643Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	MemReqMB  int64  `json:"memReqMB"`
}

func (s *Server) handleTopWorkloadsByMem2643(w http.ResponseWriter, r *http.Request) {
	result := TopWorkloadsByMem2643Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			var totalMem int64
			for _, c := range pod.Spec.Containers {
				if c.Resources.Requests != nil {
					if q := c.Resources.Requests.Memory(); q != nil {
						totalMem += q.Value()
					}
				}
			}
			memMB := totalMem / (1024 * 1024)
			if memMB > 512 {
				result.Summary.HighMemRequests++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, TopWorkloadsByMem2643Item{
					Name: pod.Name, Namespace: pod.Namespace, MemReqMB: memMB,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSNetPolicyCount2643Result struct {
	ScannedAt   time.Time                   `json:"scannedAt"`
	Summary     NSNetPolicyCount2643Summary `json:"summary"`
	Items       []NSNetPolicyCount2643Item  `json:"items"`
	HealthScore int                         `json:"healthScore"`
	Grade       string                      `json:"grade"`
}

type NSNetPolicyCount2643Summary struct {
	TotalNamespaces    int `json:"totalNamespaces"`
	WithNetPolicies    int `json:"withNetPolicies"`
	WithoutNetPolicies int `json:"withoutNetPolicies"`
}

type NSNetPolicyCount2643Item struct {
	Namespace   string `json:"namespace"`
	PolicyCount int    `json:"policyCount"`
}

func (s *Server) handleNSNetPolicyCount2643(w http.ResponseWriter, r *http.Request) {
	result := NSNetPolicyCount2643Result{ScannedAt: time.Now()}
	nss, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ns := range nss.Items {
			result.Summary.TotalNamespaces++
			nps, _ := s.clientset.NetworkingV1().NetworkPolicies(ns.Name).List(r.Context(), metav1.ListOptions{})
			if len(nps.Items) > 0 {
				result.Summary.WithNetPolicies++
				if len(result.Items) < 50 {
					result.Items = append(result.Items, NSNetPolicyCount2643Item{
						Namespace: ns.Name, PolicyCount: len(nps.Items),
					})
				}
			} else {
				result.Summary.WithoutNetPolicies++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
