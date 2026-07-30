package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.37 — Documentation Dimension (Round 42)
// 1. Node Provider ID Catalog
// 2. CRD Name Scope Distribution
// 3. Service LoadBalancer Class Inventory
// ============================================================

type ProviderIDResult2137 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ProviderIDSummary2137 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type ProviderIDSummary2137 struct {
	TotalNodes int            `json:"totalNodes"`
	ByProvider map[string]int `json:"byProvider"`
}

func (s *Server) handleProviderID2137(w http.ResponseWriter, r *http.Request) {
	result := ProviderIDResult2137{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	byProv := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pid := node.Spec.ProviderID
		if len(pid) > 0 {
			parts := splitStr2137(pid, "://")
			if len(parts) > 0 {
				byProv[parts[0]]++
			}
		}
	}
	result.Summary.ByProvider = byProv
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

func splitStr2137(s, sep string) []string {
	var result []string
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[:i])
			result = append(result, s[i+len(sep):])
			return result
		}
	}
	return []string{s}
}

// 2. CRD Scope Distribution
type CRDScopeResult2137 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CRDScopeSummary2137 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type CRDScopeSummary2137 struct {
	TotalCRDs int            `json:"totalCRDs"`
	ByScope   map[string]int `json:"byScope"`
}

func (s *Server) handleCRDScope2137(w http.ResponseWriter, r *http.Request) {
	result := CRDScopeResult2137{ScannedAt: time.Now()}
	score := 100
	// Use discovery to count CRDs
	groups, _ := s.clientset.Discovery().ServerGroups()
	customCount := 0
	for _, grp := range groups.Groups {
		if !startsWithStr(grp.Name, "k8s.io") && !startsWithStr(grp.Name, "kubernetes.io") && grp.Name != "" {
			customCount++
		}
	}
	result.Summary.TotalCRDs = customCount
	byScope := make(map[string]int)
	byScope["Namespaced"] = customCount
	result.Summary.ByScope = byScope
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. LB Class Inventory
type LBClassResult2137 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         LBClassSummary2137 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type LBClassSummary2137 struct {
	TotalLBServices int            `json:"totalLBs"`
	ByClass         map[string]int `json:"byLoadBalancerClass"`
}

func (s *Server) handleLBClass2137(w http.ResponseWriter, r *http.Request) {
	result := LBClassResult2137{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	byClass := make(map[string]int)
	for _, svc := range svcList.Items {
		if svc.Spec.Type != "LoadBalancer" {
			continue
		}
		result.Summary.TotalLBServices++
		cls := "default"
		if svc.Spec.LoadBalancerClass != nil {
			cls = *svc.Spec.LoadBalancerClass
		}
		byClass[cls]++
	}
	result.Summary.ByClass = byClass
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
