package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MTLSTrustDomainResult audits mTLS configuration across the cluster,
// checking Service Mesh sidecar injection, certificate trust domains,
// and identity-based authorization policies.
type MTLSTrustDomainResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         MTLSTrustSummary `json:"summary"`
	ByNamespace     []MTLSNsEntry    `json:"byNamespace"`
	MeshStatus      MeshStatus       `json:"meshStatus"`
	TrustScore      int              `json:"trustScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type MTLSTrustSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	MeshInjected    int `json:"meshInjected"`
	StrictMtls      int `json:"strictMtls"`
	PermissiveMtls  int `json:"permissiveMtls"`
	MtlsDisabled    int `json:"mtlsDisabled"`
	TrustAnchors    int `json:"trustAnchors"`
	CASecrets       int `json:"caSecrets"`
}

type MTLSNsEntry struct {
	Namespace     string `json:"namespace"`
	MeshInjected  bool   `json:"meshInjected"`
	MeshType      string `json:"meshType"`
	MtlsMode      string `json:"mtlsMode"`
	SidecarCount  int    `json:"sidecarCount"`
	PodCount      int    `json:"podCount"`
	HasAuthPolicy bool   `json:"hasAuthPolicy"`
	TrustDomain   string `json:"trustDomain"`
	RiskLevel     string `json:"riskLevel"`
}

type MeshStatus struct {
	Detected     string   `json:"detected"`
	Namespace    string   `json:"namespace"`
	Version      string   `json:"version"`
	TrustDomain  string   `json:"trustDomain"`
	ControlPlane bool     `json:"controlPlaneHealthy"`
	StrictMode   bool     `json:"strictModeGlobal"`
	MeshPods     int      `json:"meshPods"`
	Issues       []string `json:"issues"`
}

// handleMTLSTrustDomain handles GET /api/security/mtls-trust-domain
func (s *Server) handleMTLSTrustDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MTLSTrustDomainResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Detect mesh type
	meshStatus := MeshStatus{
		Detected:    "none",
		TrustDomain: "cluster.local",
	}

	// Check for Istio
	for _, ns := range namespaces.Items {
		if ns.Name == "istio-system" {
			meshStatus.Detected = "istio"
			meshStatus.Namespace = "istio-system"
			break
		}
	}
	// Check for Linkerd
	for _, ns := range namespaces.Items {
		if ns.Name == "linkerd" || ns.Name == "linkerd-system" {
			meshStatus.Detected = "linkerd"
			meshStatus.Namespace = ns.Name
			break
		}
	}
	// Check for Consul Connect
	for _, ns := range namespaces.Items {
		if ns.Name == "consul" {
			meshStatus.Detected = "consul"
			meshStatus.Namespace = "consul"
			break
		}
	}

	// Count CA secrets
	caSecrets := 0
	for _, sec := range secrets.Items {
		lowerName := strings.ToLower(sec.Name)
		if sec.Type == corev1.SecretTypeTLS && (strings.Contains(lowerName, "ca") || strings.Contains(lowerName, "root") || strings.Contains(lowerName, "cert")) {
			caSecrets++
		}
	}
	result.Summary.CASecrets = caSecrets

	// Analyze namespaces
	nsMap := make(map[string]*MTLSNsEntry)
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) || ns.Status.Phase != corev1.NamespaceActive {
			continue
		}

		entry := &MTLSNsEntry{
			Namespace:   ns.Name,
			TrustDomain: "cluster.local",
		}

		// Check for mesh injection labels
		if meshStatus.Detected == "istio" {
			if ns.Labels["istio-injection"] == "enabled" {
				entry.MeshInjected = true
				entry.MeshType = "istio"
			}
			if rev, ok := ns.Labels["istio.io/rev"]; ok && rev != "" {
				entry.MeshInjected = true
				entry.MeshType = "istio"
			}
		}
		if meshStatus.Detected == "linkerd" {
			if ns.Labels["linkerd.io/inject"] == "enabled" {
				entry.MeshInjected = true
				entry.MeshType = "linkerd"
			}
		}

		// Check annotations for mTLS mode
		for k, v := range ns.Annotations {
			if strings.Contains(k, "mtls") {
				if v == "strict" || v == "STRICT" {
					entry.MtlsMode = "strict"
				} else if v == "permissive" || v == "PERMISSIVE" {
					entry.MtlsMode = "permissive"
				}
			}
		}

		nsMap[ns.Name] = entry
		result.Summary.TotalNamespaces++
	}

	// Analyze pods for sidecar presence
	meshPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry, ok := nsMap[pod.Namespace]
		if !ok {
			continue
		}
		entry.PodCount++

		hasSidecar := false
		for _, c := range pod.Spec.Containers {
			cname := strings.ToLower(c.Name)
			if strings.Contains(cname, "istio-proxy") || strings.Contains(cname, "envoy") || strings.Contains(cname, "linkerd-proxy") || strings.Contains(cname, "sidecar") {
				hasSidecar = true
				meshPods++
				break
			}
		}
		if hasSidecar {
			entry.SidecarCount++
		}

		// Check for auth policy annotations
		for k := range pod.Annotations {
			if strings.Contains(k, "auth-policy") || strings.Contains(k, "authorization") {
				entry.HasAuthPolicy = true
				break
			}
		}
	}
	meshStatus.MeshPods = meshPods

	// Build entries and classify
	var entries []MTLSNsEntry
	for _, e := range nsMap {
		if e.MeshInjected {
			result.Summary.MeshInjected++
			if e.MtlsMode == "strict" {
				result.Summary.StrictMtls++
			} else if e.MtlsMode == "permissive" {
				result.Summary.PermissiveMtls++
			}
		} else {
			result.Summary.MtlsDisabled++
		}

		// Risk level
		if !e.MeshInjected {
			e.RiskLevel = "high"
		} else if e.MtlsMode != "strict" {
			e.RiskLevel = "medium"
		} else if !e.HasAuthPolicy {
			e.RiskLevel = "low"
		} else {
			e.RiskLevel = "none"
		}

		entries = append(entries, *e)
	}

	// Sort by risk level
	riskRank := map[string]int{"high": 0, "medium": 1, "low": 2, "none": 3}
	sort.Slice(entries, func(i, j int) bool {
		return riskRank[entries[i].RiskLevel] < riskRank[entries[j].RiskLevel]
	})
	result.ByNamespace = entries

	// Mesh issues
	if meshStatus.Detected == "none" {
		meshStatus.Issues = append(meshStatus.Issues, "No service mesh detected - all traffic is plaintext")
	}
	if result.Summary.MeshInjected > 0 && result.Summary.StrictMtls == 0 {
		meshStatus.Issues = append(meshStatus.Issues, "Mesh injected but no namespace uses STRICT mTLS mode")
	}
	if meshPods > 0 && result.Summary.MeshInjected > result.Summary.StrictMtls {
		meshStatus.Issues = append(meshStatus.Issues, fmt.Sprintf("%d namespaces have mesh injection but not strict mTLS", result.Summary.MeshInjected-result.Summary.StrictMtls))
	}

	result.MeshStatus = meshStatus

	// Trust score
	if result.Summary.TotalNamespaces > 0 {
		strictRatio := float64(result.Summary.StrictMtls) / float64(result.Summary.TotalNamespaces)
		injectedRatio := float64(result.Summary.MeshInjected) / float64(result.Summary.TotalNamespaces)
		result.TrustScore = int((strictRatio*0.6 + injectedRatio*0.4) * 100)
	} else {
		result.TrustScore = 0
	}

	switch {
	case result.TrustScore >= 80:
		result.Grade = "A"
	case result.TrustScore >= 60:
		result.Grade = "B"
	case result.TrustScore >= 40:
		result.Grade = "C"
	case result.TrustScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildMTLSTrustRecs(&result)
	writeJSON(w, result)
}

func buildMTLSTrustRecs(r *MTLSTrustDomainResult) []string {
	recs := []string{
		fmt.Sprintf("mTLS 信任域: %s, %d 命名空间, %d 已注入, %d STRICT", r.MeshStatus.Detected, r.Summary.TotalNamespaces, r.Summary.MeshInjected, r.Summary.StrictMtls),
	}
	if r.MeshStatus.Detected == "none" {
		recs = append(recs, "警告: 未检测到 Service Mesh, 所有流量为明文传输")
	}
	if r.Summary.MtlsDisabled > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间未启用 Mesh 注入", r.Summary.MtlsDisabled))
	}
	if r.Summary.PermissiveMtls > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间使用 PERMISSIVE 模式, 建议迁移到 STRICT", r.Summary.PermissiveMtls))
	}
	if r.MeshStatus.MeshPods > 0 && r.Summary.StrictMtls == 0 {
		recs = append(recs, "建议: 设置全局 STRICT mTLS 策略, 确保所有网格流量加密")
	}
	return recs
}
