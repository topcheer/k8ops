package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.50 Product: PodDNSPolicy, DeploymentPausedAudit, ServiceLoadBalancerClass

type PodDNSPolicy2650Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     PodDNSPolicy2650Summary `json:"summary"`
	Items       []PodDNSPolicy2650Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type PodDNSPolicy2650Summary struct {
	TotalPods    int `json:"totalPods"`
	ClusterFirst int `json:"clusterFirst"`
	Default      int `json:"default"`
	None         int `json:"none"`
}

type PodDNSPolicy2650Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	DNSPolicy string `json:"dnsPolicy"`
}

func (s *Server) handlePodDNSPolicy2650(w http.ResponseWriter, r *http.Request) {
	result := PodDNSPolicy2650Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			dp := string(pod.Spec.DNSPolicy)
			switch dp {
			case "ClusterFirst":
				result.Summary.ClusterFirst++
			case "Default":
				result.Summary.Default++
			case "None":
				result.Summary.None++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodDNSPolicy2650Item{
					Name: pod.Name, Namespace: pod.Namespace, DNSPolicy: dp,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployPaused2650Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     DeployPaused2650Summary `json:"summary"`
	Items       []DeployPaused2650Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type DeployPaused2650Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	PausedCount      int `json:"pausedCount"`
	ActiveCount      int `json:"activeCount"`
}

type DeployPaused2650Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Paused    bool   `json:"paused"`
}

func (s *Server) handleDeployPaused2650(w http.ResponseWriter, r *http.Request) {
	result := DeployPaused2650Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			if dep.Spec.Paused {
				result.Summary.PausedCount++
			} else {
				result.Summary.ActiveCount++
			}
			if dep.Spec.Paused && len(result.Items) < 50 {
				result.Items = append(result.Items, DeployPaused2650Item{
					Name: dep.Name, Namespace: dep.Namespace, Paused: true,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcLBClass2650Result struct {
	ScannedAt   time.Time             `json:"scannedAt"`
	Summary     SvcLBClass2650Summary `json:"summary"`
	Items       []SvcLBClass2650Item  `json:"items"`
	HealthScore int                   `json:"healthScore"`
	Grade       string                `json:"grade"`
}

type SvcLBClass2650Summary struct {
	TotalServices  int `json:"totalServices"`
	WithLBClass    int `json:"withLBClass"`
	WithoutLBClass int `json:"withoutLBClass"`
}

type SvcLBClass2650Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	LBClass   string `json:"lbClass"`
}

func (s *Server) handleSvcLBClass2650(w http.ResponseWriter, r *http.Request) {
	result := SvcLBClass2650Result{ScannedAt: time.Now()}
	svcs, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, svc := range svcs.Items {
			result.Summary.TotalServices++
			lbClass := ""
			if svc.Spec.LoadBalancerClass != nil {
				lbClass = *svc.Spec.LoadBalancerClass
				result.Summary.WithLBClass++
			} else {
				result.Summary.WithoutLBClass++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, SvcLBClass2650Item{
					Name: svc.Name, Namespace: svc.Namespace, LBClass: lbClass,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
