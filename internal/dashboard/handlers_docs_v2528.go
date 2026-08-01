package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.28 Documentation: Node NodeInfo OSImage vs KernelVersion, Pod Spec HostAliases Detail, Namespace Label Key Distribution
type NodeInfoCompareResult2528 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOSImage  map[string]int `json:"byOSImage"`
	} `json:"summary"`
}

func (s *Server) handleNodeInfoCompare2528(w http.ResponseWriter, r *http.Request) {
	result := NodeInfoCompareResult2528{ScannedAt: time.Now()}
	result.Summary.ByOSImage = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		osImg := node.Status.NodeInfo.OSImage
		if osImg == "" {
			osImg = "<unknown>"
		}
		result.Summary.ByOSImage[osImg]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HostAliasesDetailResult2528 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByAliasHost map[string]int `json:"byAliasHost"`
	} `json:"summary"`
}

func (s *Server) handleHostAliasesDetail2528(w http.ResponseWriter, r *http.Request) {
	result := HostAliasesDetailResult2528{ScannedAt: time.Now()}
	result.Summary.ByAliasHost = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, ha := range pod.Spec.HostAliases {
			for _, h := range ha.Hostnames {
				result.Summary.ByAliasHost[h]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSLabelKeyResult2528 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNamespaces"`
		ByLabel map[string]int `json:"byLabelKey"`
	} `json:"summary"`
}

func (s *Server) handleNSLabelKey2528(w http.ResponseWriter, r *http.Request) {
	result := NSLabelKeyResult2528{ScannedAt: time.Now()}
	result.Summary.ByLabel = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		for k := range ns.Labels {
			result.Summary.ByLabel[k]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
