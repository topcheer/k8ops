package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.12 Documentation: Service Port Name Catalog, Pod HostAlias Inventory, Node Boot ID Census
type SvcPortNameResult2312 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		NamedPorts    int            `json:"withNamedPorts"`
		ByProtocol    map[string]int `json:"byProtocol"`
	} `json:"summary"`
}

func (s *Server) handleSvcPortName2312(w http.ResponseWriter, r *http.Request) {
	result := SvcPortNameResult2312{ScannedAt: time.Now()}
	result.Summary.ByProtocol = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			result.Summary.ByProtocol[string(p.Protocol)]++
			if p.Name != "" {
				result.Summary.NamedPorts++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HostAliasResult2312 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithHostAlias int `json:"withHostAliases"`
		TotalAliases  int `json:"totalAliases"`
	} `json:"summary"`
}

func (s *Server) handleHostAlias2312(w http.ResponseWriter, r *http.Request) {
	result := HostAliasResult2312{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.HostAliases) > 0 {
			result.Summary.WithHostAlias++
			result.Summary.TotalAliases += len(pod.Spec.HostAliases)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeBootIDResult2312 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		UniqueBootIDs int `json:"uniqueBootIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeBootID2312(w http.ResponseWriter, r *http.Request) {
	result := NodeBootIDResult2312{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		seen[node.Status.NodeInfo.BootID] = true
	}
	result.Summary.UniqueBootIDs = len(seen)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
