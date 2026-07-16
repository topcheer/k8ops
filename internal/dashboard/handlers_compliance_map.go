package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComplianceMapResult maps cluster state to compliance frameworks (SOC2, PCI-DSS, CIS).
type ComplianceMapResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ComplianceMapSummary `json:"summary"`
	Controls        []ComplianceControl `json:"controls"`
	Frameworks      []FrameworkResult   `json:"frameworks"`
	FailingControls []ControlFinding    `json:"failingControls"`
	OverallScore    int                 `json:"overallScore"`
	ComplianceScore int                 `json:"complianceScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type FrameworkResult struct {
	Name           string  `json:"name"`
	PassRate       float64 `json:"passRate"`
	Passing        int     `json:"passing"`
	TotalControls  int     `json:"totalControls"`
	Status         string  `json:"status"`
}

type ControlFinding struct {
	Framework   string `json:"framework"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Remediation string `json:"remediation"`
}

type ComplianceMapSummary struct {
	SOC2Pct       float64 `json:"soc2Pct"`
	PCIPct        float64 `json:"pciDssPct"`
	CISPct        float64 `json:"cisPct"`
	TotalControls int     `json:"totalControls"`
	Passed        int     `json:"passed"`
	Failed        int     `json:"failed"`
}

type ComplianceControl struct {
	ID         string `json:"id"`
	Framework  string `json:"framework"`
	Category   string `json:"category"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
}

// handleComplianceMap maps cluster state to compliance frameworks.
// GET /api/security/compliance-map
func (s *Server) handleComplianceMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ComplianceMapResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Check various compliance controls
	checks := []struct {
		id, framework, category string
		check                  func() (bool, string)
	}{
		// SOC2
		{"SOC2-CC1", "SOC2", "Access Control", func() (bool, string) {
			for _, ns := range nsList.Items {
				if ns.Labels["pod-security.kubernetes.io/enforce"] != "" { return true, "PSA enforced on at least one namespace" }
			}
			return false, "No Pod Security Admission labels found on any namespace"
		}},
		{"SOC2-CC2", "SOC2", "Encryption", func() (bool, string) {
			for _, sec := range secrets.Items {
				if sec.Type == "kubernetes.io/tls" { return true, "TLS certificates detected" }
			}
			return false, "No TLS secrets found — data in transit may be unencrypted"
		}},
		{"SOC2-CC3", "SOC2", "Monitoring", func() (bool, string) {
			for _, pod := range pods.Items {
				img := strings.ToLower(pod.Spec.Containers[0].Image)
				if strings.Contains(img, "prometheus") || strings.Contains(img, "fluent") { return true, "Monitoring/logging detected" }
			}
			return false, "No monitoring/logging agent detected"
		}},
		// PCI-DSS
		{"PCI-1", "PCI-DSS", "Network Segmentation", func() (bool, string) {
			for _, ns := range nsList.Items {
				if strings.Contains(strings.ToLower(ns.Name), "pci") || strings.Contains(strings.ToLower(ns.Name), "payment") { return true, "PCI namespace isolated" }
			}
			return false, "No dedicated PCI namespace found"
		}},
		{"PCI-2", "PCI-DSS", "Audit Logging", func() (bool, string) {
			for _, pod := range pods.Items {
				img := strings.ToLower(pod.Spec.Containers[0].Image)
				if strings.Contains(img, "fluent") || strings.Contains(img, "vector") { return true, "Log forwarding detected" }
			}
			return false, "No log forwarding agent detected"
		}},
		// CIS
		{"CIS-1", "CIS", "Privileged Pods", func() (bool, string) {
			for _, pod := range pods.Items {
				if systemNS[pod.Namespace] { continue }
				for _, c := range pod.Spec.Containers {
					if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
						return false, "Privileged containers detected in user namespaces"
					}
				}
			}
			return true, "No privileged containers in user namespaces"
		}},
		{"CIS-2", "CIS", "Resource Limits", func() (bool, string) {
			for _, dep := range deployments.Items {
				if systemNS[dep.Namespace] { continue }
				for _, c := range dep.Spec.Template.Spec.Containers {
					if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
						return false, "Deployments without resource limits detected"
					}
				}
			}
			return true, "All deployments have resource limits"
		}},
		{"CIS-3", "CIS", "Secret Management", func() (bool, string) {
			for _, pod := range pods.Items {
				img := strings.ToLower(pod.Spec.Containers[0].Image)
				if strings.Contains(img, "external-secrets") || strings.Contains(img, "vault") { return true, "External secret management detected" }
			}
			return false, "No External Secrets or Vault detected"
		}},
	}

	soc2Pass, soc2Total, pciPass, pciTotal, cisPass, cisTotal := 0, 0, 0, 0, 0, 0
	for _, chk := range checks {
		ok, detail := chk.check()
		status := "pass"
		if !ok { status = "fail" }
		result.Controls = append(result.Controls, ComplianceControl{
			ID: chk.id, Framework: chk.framework, Category: chk.category, Status: status, Detail: detail,
		})
		result.Summary.TotalControls++
		if ok {
			result.Summary.Passed++
			switch chk.framework {
			case "SOC2": soc2Pass++
			case "PCI-DSS": pciPass++
			case "CIS": cisPass++
			}
		} else {
			result.Summary.Failed++
		}
		switch chk.framework {
		case "SOC2": soc2Total++
		case "PCI-DSS": pciTotal++
		case "CIS": cisTotal++
		}
	}

	if soc2Total > 0 { result.Summary.SOC2Pct = float64(soc2Pass) / float64(soc2Total) * 100 }
	if pciTotal > 0 { result.Summary.PCIPct = float64(pciPass) / float64(pciTotal) * 100 }
	if cisTotal > 0 { result.Summary.CISPct = float64(cisPass) / float64(cisTotal) * 100 }

	result.ComplianceScore = int(float64(result.Summary.Passed) / float64(result.Summary.TotalControls) * 100)
	result.OverallScore = result.ComplianceScore
	result.Grade = goldenScoreToGrade(result.ComplianceScore)

	// Build framework summaries
	result.Frameworks = []FrameworkResult{
		{Name: "SOC2 Type II", PassRate: result.Summary.SOC2Pct, Passing: soc2Pass, TotalControls: soc2Total, Status: fwStatus(result.Summary.SOC2Pct)},
		{Name: "PCI-DSS 4.0", PassRate: result.Summary.PCIPct, Passing: pciPass, TotalControls: pciTotal, Status: fwStatus(result.Summary.PCIPct)},
		{Name: "HIPAA", PassRate: result.Summary.CISPct, Passing: cisPass, TotalControls: cisTotal, Status: fwStatus(result.Summary.CISPct)},
	}

	// Build failing controls list
	for _, c := range result.Controls {
		if c.Status == "fail" {
			result.FailingControls = append(result.FailingControls, ControlFinding{
				Framework: c.Framework, Title: c.Category, Severity: "high", Remediation: c.Detail,
			})
		}
	}

	sort.Slice(result.Controls, func(i, j int) bool {
		return result.Controls[i].Status > result.Controls[j].Status // fail before pass
	})

	recs := generateComplianceMapRecs(ComplianceMapResult{
		Frameworks: result.Frameworks,
		FailingControls: result.FailingControls,
		OverallScore: result.ComplianceScore,
	})
	result.Recommendations = recs

	writeJSON(w, result)
}

func generateComplianceMapRecs(r ComplianceMapResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Overall compliance score: %d/100", r.OverallScore))
	for _, fw := range r.Frameworks {
		if fw.Status != "passing" {
			recs = append(recs, fmt.Sprintf("%s: %.0f%% pass rate (%d/%d controls) — %s", fw.Name, fw.PassRate, fw.Passing, fw.TotalControls, fw.Status))
		}
	}
	for _, fc := range r.FailingControls {
		recs = append(recs, fmt.Sprintf("[%s] %s — %s", fc.Severity, fc.Title, fc.Remediation))
	}
	if len(recs) == 1 { recs = append(recs, "All compliance frameworks passing") }
	return recs
}

func fwStatus(pct float64) string {
	if pct >= 100 { return "passing" }
	if pct >= 50 { return "partial" }
	return "failing"
}
