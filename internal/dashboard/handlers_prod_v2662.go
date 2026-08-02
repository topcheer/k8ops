package dashboard

import (
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.62 Product: PodHostIPC, DeploymentAvailable, ServiceTypeDist

type PodHostIPC2662Result struct {
	ScannedAt   time.Time             `json:"scannedAt"`
	Summary     PodHostIPC2662Summary `json:"summary"`
	Items       []PodHostIPC2662Item  `json:"items"`
	HealthScore int                   `json:"healthScore"`
	Grade       string                `json:"grade"`
}

type PodHostIPC2662Summary struct {
	TotalPods   int `json:"totalPods"`
	HostIPCPods int `json:"hostIpcPods"`
	NormalPods  int `json:"normalPods"`
}

type PodHostIPC2662Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HostIPC   bool   `json:"hostIpc"`
}

func (s *Server) handlePodHostIPC2662(w http.ResponseWriter, r *http.Request) {
	result := PodHostIPC2662Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			if pod.Spec.HostIPC {
				result.Summary.HostIPCPods++
			} else {
				result.Summary.NormalPods++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodHostIPC2662Item{
					Name: pod.Name, Namespace: pod.Namespace, HostIPC: pod.Spec.HostIPC,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployAvailable2662Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     DeployAvailable2662Summary `json:"summary"`
	Items       []DeployAvailable2662Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type DeployAvailable2662Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	Available        int `json:"available"`
	Unavailable      int `json:"unavailable"`
}

type DeployAvailable2662Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Available bool   `json:"available"`
}

func (s *Server) handleDeployAvailable2662(w http.ResponseWriter, r *http.Request) {
	result := DeployAvailable2662Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			avail := false
			for _, cond := range dep.Status.Conditions {
				if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
					avail = true
					break
				}
			}
			if avail {
				result.Summary.Available++
			} else {
				result.Summary.Unavailable++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployAvailable2662Item{
					Name: dep.Name, Namespace: dep.Namespace, Available: avail,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcTypeDist2662Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     SvcTypeDist2662Summary `json:"summary"`
	Items       []SvcTypeDist2662Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type SvcTypeDist2662Summary struct {
	TotalServices int `json:"totalServices"`
	ClusterIP     int `json:"clusterIp"`
	NodePort      int `json:"nodePort"`
	LoadBalancer  int `json:"loadBalancer"`
	ExternalName  int `json:"externalName"`
}

type SvcTypeDist2662Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

func (s *Server) handleSvcTypeDist2662(w http.ResponseWriter, r *http.Request) {
	result := SvcTypeDist2662Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			st := string(svc.Spec.Type)
			switch svc.Spec.Type {
			case corev1.ServiceTypeClusterIP:
				result.Summary.ClusterIP++
			case corev1.ServiceTypeNodePort:
				result.Summary.NodePort++
			case corev1.ServiceTypeLoadBalancer:
				result.Summary.LoadBalancer++
			case corev1.ServiceTypeExternalName:
				result.Summary.ExternalName++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcTypeDist2662Item{
					Name: svc.Name, Namespace: svc.Namespace, Type: st,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
