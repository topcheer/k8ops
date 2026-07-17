package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetpolGeneratorResult generates default-deny NetworkPolicy manifests for
// namespaces that lack network isolation. Provides ready-to-apply YAML
// and kubectl commands for immediate network segmentation.
type NetpolGeneratorResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         NetpolGenSummary `json:"summary"`
	Generated       []NetpolManifest `json:"generated"`
	BatchApply      []string         `json:"batchApply"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type NetpolGenSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithNetpol      int `json:"withNetpol"`
	MissingNetpol   int `json:"missingNetpol"`
}

type NetpolManifest struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	PolicyType   string `json:"policyType"` // default-deny-ingress, default-deny-egress, allow-dns
	ManifestYAML string `json:"manifestYAML"`
	ApplyCommand string `json:"applyCommand"`
}

// handleNetpolGenerator handles GET /api/security/netpol-generator
func (s *Server) handleNetpolGenerator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NetpolGeneratorResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	// Build NS coverage map
	netpolNS := make(map[string]bool)
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			netpolNS[np.Namespace] = true
		}
	}

	var manifests []NetpolManifest
	var batchCmds []string

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		if ns.Status.Phase != corev1.NamespaceActive {
			continue
		}

		result.Summary.TotalNamespaces++

		if netpolNS[ns.Name] {
			result.Summary.WithNetpol++
			continue
		}

		result.Summary.MissingNetpol++

		// Generate default-deny-ingress policy
		name := ns.Name + "-default-deny-ingress"
		yaml := generateNetpolYAML(name, ns.Name, "ingress")
		cmd := fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", yaml)
		manifests = append(manifests, NetpolManifest{
			Name: name, Namespace: ns.Name, PolicyType: "default-deny-ingress",
			ManifestYAML: yaml, ApplyCommand: cmd,
		})
		batchCmds = append(batchCmds, cmd)

		// Generate allow-dns policy (kube-dns access)
		dnsName := ns.Name + "-allow-dns"
		dnsYAML := generateAllowDNSYAML(dnsName, ns.Name)
		dnsCmd := fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", dnsYAML)
		manifests = append(manifests, NetpolManifest{
			Name: dnsName, Namespace: ns.Name, PolicyType: "allow-dns",
			ManifestYAML: dnsYAML, ApplyCommand: dnsCmd,
		})
		batchCmds = append(batchCmds, dnsCmd)
	}

	// Score
	if result.Summary.TotalNamespaces > 0 {
		result.HealthScore = result.Summary.WithNetpol * 100 / result.Summary.TotalNamespaces
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 50:
		result.Grade = "B"
	case result.HealthScore >= 25:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Generated = manifests
	result.BatchApply = batchCmds
	result.Recommendations = buildNetpolGenRecs(&result)

	writeJSON(w, result)
}

func generateNetpolYAML(name, ns, direction string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector: {}
  policyTypes:
  - %s`, name, ns, strings.Title(direction))
}

func generateAllowDNSYAML(name, ns string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector: {}
  policyTypes:
  - egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53`, name, ns)
}

func buildNetpolGenRecs(r *NetpolGeneratorResult) []string {
	recs := []string{}
	if r.Summary.MissingNetpol == 0 {
		recs = append(recs, "所有命名空间都有 NetworkPolicy 保护")
		return recs
	}
	recs = append(recs, fmt.Sprintf("%d/%d 个命名空间缺少 NetworkPolicy", r.Summary.MissingNetpol, r.Summary.TotalNamespaces))
	recs = append(recs, fmt.Sprintf("已生成 %d 个策略 YAML（default-deny + allow-dns）", len(r.Generated)))
	recs = append(recs, "注意: 应用 default-deny 后会阻断所有入站流量，需要为每个服务创建 allow 策略")
	return recs
}
