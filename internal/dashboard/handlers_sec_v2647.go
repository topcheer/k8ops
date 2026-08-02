package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.47 Security: PodRunAsNonRoot, SecretTypeDist, NodeReadyCond

type PodRunAsNonRoot2647Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     PodRunAsNonRoot2647Summary `json:"summary"`
	Items       []PodRunAsNonRoot2647Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type PodRunAsNonRoot2647Summary struct {
	TotalPods   int `json:"totalPods"`
	NonRoot     int `json:"nonRoot"`
	RootDefault int `json:"rootDefault"`
}

type PodRunAsNonRoot2647Item struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	RunAsNonRoot bool   `json:"runAsNonRoot"`
}

func (s *Server) handlePodRunAsNonRoot2647(w http.ResponseWriter, r *http.Request) {
	result := PodRunAsNonRoot2647Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			nonRoot := false
			if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsNonRoot != nil {
				nonRoot = *pod.Spec.SecurityContext.RunAsNonRoot
			}
			if nonRoot {
				result.Summary.NonRoot++
			} else {
				result.Summary.RootDefault++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodRunAsNonRoot2647Item{
					Name: pod.Name, Namespace: pod.Namespace, RunAsNonRoot: nonRoot,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretTypeDist2647Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     SecretTypeDist2647Summary `json:"summary"`
	Items       []SecretTypeDist2647Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type SecretTypeDist2647Summary struct {
	TotalSecrets int `json:"totalSecrets"`
	Opaque       int `json:"opaque"`
	DockerConfig int `json:"dockerConfig"`
	TLS          int `json:"tls"`
	Other        int `json:"other"`
}

type SecretTypeDist2647Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

func (s *Server) handleSecretTypeDist2647(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeDist2647Result{ScannedAt: time.Now()}
	secs, err := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sec := range secs.Items {
			result.Summary.TotalSecrets++
			st := string(sec.Type)
			switch sec.Type {
			case corev1.SecretTypeOpaque:
				result.Summary.Opaque++
			case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
				result.Summary.DockerConfig++
			case corev1.SecretTypeTLS:
				result.Summary.TLS++
			default:
				result.Summary.Other++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SecretTypeDist2647Item{
					Name: sec.Name, Namespace: sec.Namespace, Type: st,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeReadyCond2647Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     NodeReadyCond2647Summary `json:"summary"`
	Items       []NodeReadyCond2647Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type NodeReadyCond2647Summary struct {
	TotalNodes int `json:"totalNodes"`
	ReadyNodes int `json:"readyNodes"`
	NotReady   int `json:"notReady"`
}

type NodeReadyCond2647Item struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

func (s *Server) handleNodeReadyCond2647(w http.ResponseWriter, r *http.Request) {
	result := NodeReadyCond2647Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			ready := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady {
					ready = cond.Status == corev1.ConditionTrue
					break
				}
			}
			if ready {
				result.Summary.ReadyNodes++
			} else {
				result.Summary.NotReady++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeReadyCond2647Item{
					Name: node.Name, Ready: ready,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
