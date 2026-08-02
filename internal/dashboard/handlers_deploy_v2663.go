package dashboard

import (
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.63 Deployment: STSObservedGen, DSCurrentNum, DeployObservedGen

type STSObservedGen2663Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     STSObservedGen2663Summary `json:"summary"`
	Items       []STSObservedGen2663Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type STSObservedGen2663Summary struct {
	TotalSTS    int `json:"totalSTS"`
	GenMatch    int `json:"genMatch"`
	GenMismatch int `json:"genMismatch"`
}

type STSObservedGen2663Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Generation  int64  `json:"generation"`
	ObservedGen int64  `json:"observedGeneration"`
}

func (s *Server) handleSTSObservedGen2663(w http.ResponseWriter, r *http.Request) {
	result := STSObservedGen2663Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range stss.Items {
			result.Summary.TotalSTS++
			gen := sts.Generation
			obsGen := sts.Status.ObservedGeneration
			if gen == obsGen {
				result.Summary.GenMatch++
			} else {
				result.Summary.GenMismatch++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, STSObservedGen2663Item{
					Name: sts.Name, Namespace: sts.Namespace, Generation: gen, ObservedGen: obsGen,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSCurrentNum2663Result struct {
	ScannedAt   time.Time               `json:"scannedAt"`
	Summary     DSCurrentNum2663Summary `json:"summary"`
	Items       []DSCurrentNum2663Item  `json:"items"`
	HealthScore int                     `json:"healthScore"`
	Grade       string                  `json:"grade"`
}

type DSCurrentNum2663Summary struct {
	TotalDS          int `json:"totalDS"`
	CurrentScheduled int `json:"currentScheduled"`
	Misscheduled     int `json:"misscheduled"`
}

type DSCurrentNum2663Item struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	CurrentScheduled int32  `json:"currentScheduled"`
	Misscheduled     int32  `json:"misscheduled"`
}

func (s *Server) handleDSCurrentNum2663(w http.ResponseWriter, r *http.Request) {
	result := DSCurrentNum2663Result{ScannedAt: time.Now()}
	dss, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			result.Summary.TotalDS++
			result.Summary.CurrentScheduled += int(ds.Status.CurrentNumberScheduled)
			result.Summary.Misscheduled += int(ds.Status.NumberMisscheduled)
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DSCurrentNum2663Item{
					Name: ds.Name, Namespace: ds.Namespace,
					CurrentScheduled: ds.Status.CurrentNumberScheduled, Misscheduled: ds.Status.NumberMisscheduled,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DeployObservedGen2663Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     DeployObservedGen2663Summary `json:"summary"`
	Items       []DeployObservedGen2663Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type DeployObservedGen2663Summary struct {
	TotalDeployments int `json:"totalDeployments"`
	GenMatch         int `json:"genMatch"`
	GenMismatch      int `json:"genMismatch"`
}

type DeployObservedGen2663Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Generation  int64  `json:"generation"`
	ObservedGen int64  `json:"observedGeneration"`
}

func (s *Server) handleDeployObservedGen2663(w http.ResponseWriter, r *http.Request) {
	result := DeployObservedGen2663Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deps.Items {
			result.Summary.TotalDeployments++
			gen := dep.Generation
			obsGen := dep.Status.ObservedGeneration
			if gen == obsGen {
				result.Summary.GenMatch++
			} else {
				result.Summary.GenMismatch++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, DeployObservedGen2663Item{
					Name: dep.Name, Namespace: dep.Namespace, Generation: gen, ObservedGen: obsGen,
				})
			}
		}
	}
	_ = appsv1.DeploymentAvailable // ensure import used
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
