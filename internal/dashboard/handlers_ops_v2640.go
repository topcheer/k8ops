package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.40 Operations: PodQOSClass, NodeKernelVersion, NamespaceResourceQuotaCount

type PodQOSClass2640Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     PodQOSClass2640Summary `json:"summary"`
	Items       []PodQOSClass2640Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type PodQOSClass2640Summary struct {
	TotalPods  int `json:"totalPods"`
	Guaranteed int `json:"guaranteed"`
	Burstable  int `json:"burstable"`
	BestEffort int `json:"bestEffort"`
}

type PodQOSClass2640Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	QOSClass  string `json:"qosClass"`
}

func (s *Server) handlePodQOSClass2640(w http.ResponseWriter, r *http.Request) {
	result := PodQOSClass2640Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			result.Summary.TotalPods++
			qos := string(pod.Status.QOSClass)
			switch qos {
			case "Guaranteed":
				result.Summary.Guaranteed++
			case "Burstable":
				result.Summary.Burstable++
			case "BestEffort":
				result.Summary.BestEffort++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PodQOSClass2640Item{
					Name: pod.Name, Namespace: pod.Namespace, QOSClass: qos,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeKernelVersion2640Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     NodeKernelVersion2640Summary `json:"summary"`
	Items       []NodeKernelVersion2640Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type NodeKernelVersion2640Summary struct {
	TotalNodes    int `json:"totalNodes"`
	UniqueKernels int `json:"uniqueKernels"`
}

type NodeKernelVersion2640Item struct {
	Name          string `json:"name"`
	KernelVersion string `json:"kernelVersion"`
	OSImage       string `json:"osImage"`
}

func (s *Server) handleNodeKernelVersion2640(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelVersion2640Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		kernelSet := map[string]bool{}
		for _, node := range nodes.Items {
			result.Summary.TotalNodes++
			kv := node.Status.NodeInfo.KernelVersion
			if !kernelSet[kv] {
				kernelSet[kv] = true
				result.Summary.UniqueKernels++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, NodeKernelVersion2640Item{
					Name: node.Name, KernelVersion: kv, OSImage: node.Status.NodeInfo.OSImage,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSResourceQuotaCount2640Result struct {
	ScannedAt   time.Time                       `json:"scannedAt"`
	Summary     NSResourceQuotaCount2640Summary `json:"summary"`
	Items       []NSResourceQuotaCount2640Item  `json:"items"`
	HealthScore int                             `json:"healthScore"`
	Grade       string                          `json:"grade"`
}

type NSResourceQuotaCount2640Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuotas      int `json:"withQuotas"`
	WithoutQuotas   int `json:"withoutQuotas"`
}

type NSResourceQuotaCount2640Item struct {
	Namespace  string `json:"namespace"`
	QuotaCount int    `json:"quotaCount"`
}

func (s *Server) handleNSResourceQuotaCount2640(w http.ResponseWriter, r *http.Request) {
	result := NSResourceQuotaCount2640Result{ScannedAt: time.Now()}
	nss, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ns := range nss.Items {
			result.Summary.TotalNamespaces++
			qts, _ := s.clientset.CoreV1().ResourceQuotas(ns.Name).List(r.Context(), metav1.ListOptions{})
			if len(qts.Items) > 0 {
				result.Summary.WithQuotas++
				if len(result.Items) < 50 {
					result.Items = append(result.Items, NSResourceQuotaCount2640Item{
						Namespace: ns.Name, QuotaCount: len(qts.Items),
					})
				}
			} else {
				result.Summary.WithoutQuotas++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
