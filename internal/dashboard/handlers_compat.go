package dashboard

import (
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MinKubernetesVersion is the minimum supported Kubernetes minor version.
// Locked at 1.25 because k8ops uses policy/v1 PDB (GA in 1.25, v1beta1 removed).
const MinKubernetesVersion = "1.25"

// ClusterCompat holds the detected cluster compatibility information.
type ClusterCompat struct {
	CloudProvider string      `json:"cloudProvider"` // aws, azure, gcp, alibaba, tencent, oracle, ibm, vsphere, bare-metal, unknown
	Distribution  string      `json:"distribution"`  // eks, gke, aks, k3s, rke2, openshift, oke, ack, tke, minikube, kind, vanilla, unknown
	Managed       bool        `json:"managed"`       // true for managed cloud Kubernetes services
	K8sVersion    string      `json:"k8sVersion"`    // e.g. "v1.28.4-eks-123456"
	MinVersion    string      `json:"minVersion"`    // minimum supported version
	Compatible    bool        `json:"compatible"`    // whether the cluster meets minimum requirements
	Warning       string      `json:"warning,omitempty"`
	Features      []string    `json:"features,omitempty"` // detected feature highlights
	NodeInfo      NodeSummary `json:"nodeInfo"`
}

// NodeSummary is a high-level summary of node-level observations.
type NodeSummary struct {
	TotalNodes      int `json:"totalNodes"`
	ControlPlane    int `json:"controlPlane"`
	Worker          int `json:"worker"`
	BareMetalNodes  int `json:"bareMetalNodes"`
	VirtualNodes    int `json:"virtualNodes"`
}

// detectCloudProvider identifies the cloud or infrastructure provider
// by examining Node.Spec.ProviderID, which follows the format:
//
//	<provider>://<instance-id>
//
// Examples:
//   - aws://i-1234567890abcdef0
//   - gce://projects/.../zones/.../instances/...
//   - azure://subscriptions/.../resourceGroups/.../providers/Microsoft.Compute/virtualMachines/...
//   - vsphere://420e7c8a-...-cnab
func detectCloudProvider(nodes []corev1.Node) string {
	for _, n := range nodes {
		providerID := n.Spec.ProviderID
		if providerID == "" {
			continue
		}
		// Extract the prefix before "://"
		prefix := providerID
		if idx := strings.Index(providerID, "://"); idx > 0 {
			prefix = providerID[:idx]
		}

		switch strings.ToLower(prefix) {
		case "aws":
			return "aws"
		case "gce":
			return "gcp"
		case "azure":
			return "azure"
		case "vsphere":
			return "vsphere"
		case "openstack":
			return "openstack"
		case "digitalocean":
			return "digitalocean"
		case "linode":
			return "linode"
		case "scaleway":
			return "scaleway"
		case "alibaba", "alicloud":
			return "alibaba"
		case "tencent":
			return "tencent"
		case "huawei", "huaweicloud":
			return "huawei"
		case "oracle", "oci":
			return "oracle"
		case "ibm", "ibmcloud":
			return "ibm"
		case "baidu", "bcc":
			return "baidu"
		case "cloudstack":
			return "cloudstack"
		case "k3s", "k3os":
			return "k3s"
		case "rke2":
			return "rke2"
		case "external":
			// Some bare-metal setups use "external://"
			if n.Labels["kubernetes.io/hostname"] != "" {
				// Could be bare-metal or any external infra
				return "bare-metal"
			}
			continue
		default:
			// Continue checking other nodes
			continue
		}
	}

	// Fallback: check node labels for known cloud provider labels
	for _, n := range nodes {
		labels := n.Labels
		if _, ok := labels["kubernetes.io/cloud-provider-aws"]; ok {
			return "aws"
		}
		if v, ok := labels["topology.kubernetes.io/zone"]; ok {
			if strings.HasPrefix(v, "aws-") {
				return "aws"
			}
		}
	}

	// If some nodes have ProviderID but none matched, it could be bare-metal
	hasProviderID := false
	for _, n := range nodes {
		if n.Spec.ProviderID != "" {
			hasProviderID = true
			break
		}
	}

	if !hasProviderID && len(nodes) > 0 {
		return "bare-metal"
	}

	return "unknown"
}

// detectDistribution identifies the Kubernetes distribution by combining
// version info (build metadata) and node labels.
func detectDistribution(gitVersion string, nodes []corev1.Node) string {
	// Check version string for known distribution markers
	v := strings.ToLower(gitVersion)

	if strings.Contains(v, "-eks-") {
		return "eks"
	}
	if strings.Contains(v, "-gke.") {
		return "gke"
	}
	if strings.Contains(v, "-aks") || strings.Contains(v, "azure") {
		return "aks"
	}
	if strings.Contains(v, "-oke-") || strings.Contains(v, "oracle") {
		return "oke"
	}
	if strings.Contains(v, "-ack") || strings.Contains(v, "aliyun") {
		return "ack"
	}
	if strings.Contains(v, "-tke") || strings.Contains(v, "tencent") {
		return "tke"
	}
	if strings.Contains(v, "-k3s") || strings.Contains(v, "k3s") {
		return "k3s"
	}
	if strings.Contains(v, "-rke2") || strings.Contains(v, "rke2") {
		return "rke2"
	}
	if strings.Contains(v, "+rke") {
		return "rke"
	}
	if strings.Contains(v, "-kk8s") {
		return "kk8s"
	}
	if strings.Contains(v, "-openshift") {
		return "openshift"
	}

	// Check node labels for distribution-specific markers
	for _, n := range nodes {
		for k, v := range n.Labels {
			kLower := strings.ToLower(k)
			vLower := strings.ToLower(v)

			if strings.Contains(kLower, "openshift") || strings.Contains(vLower, "openshift") {
				return "openshift"
			}
			if strings.Contains(kLower, "rke2") || strings.Contains(vLower, "rke2") {
				return "rke2"
			}
			if strings.Contains(kLower, "k3s") || strings.Contains(vLower, "k3s") {
				return "k3s"
			}
			// Talos OS — detect via talos.dev/* labels or os name
			if strings.Contains(kLower, "talos") || strings.Contains(vLower, "talos") {
				return "talos"
			}
			// Minikube
			if strings.Contains(kLower, "minikube") || strings.Contains(vLower, "minikube") {
				return "minikube"
			}
			// Kind
			if strings.Contains(kLower, "kind") || strings.Contains(vLower, "kind") {
				return "kind"
			}
			// MicroK8s
			if strings.Contains(vLower, "microk8s") {
				return "microk8s"
			}
			// DOKS (DigitalOcean)
			if strings.Contains(vLower, "doks") || strings.Contains(vLower, "digitalocean") {
				return "doks"
			}
		}
	}

	return "vanilla"
}

// isManagedCloud returns true if the distribution is a managed cloud Kubernetes service.
func isManagedCloud(distribution string) bool {
	switch distribution {
	case "eks", "gke", "aks", "oke", "ack", "tke", "doks", "lke":
		return true
	default:
		return false
	}
}

// checkVersionCompatibility parses a git version string and checks if it meets
// the minimum required version. Returns compatible bool and a warning message.
func checkVersionCompatibility(gitVersion string) (bool, string) {
	// Parse major.minor from strings like "v1.28.4-eks-123456"
	v := strings.TrimPrefix(gitVersion, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false, "unable to parse Kubernetes version: " + gitVersion
	}

	major := parts[0]
	minor := parts[1]

	// Strip any suffix from minor (e.g. "28-eks-123456" -> "28")
	if idx := strings.IndexAny(minor, "-+"); idx > 0 {
		minor = minor[:idx]
	}

	// Minimum version parts
	minParts := strings.SplitN(MinKubernetesVersion, ".", 2)
	minMinor := minParts[1]

	// Compare major first
	if major != minParts[0] {
		if major > minParts[0] {
			return true, ""
		}
		return false, "Kubernetes " + major + "." + minor + " is below minimum required " + MinKubernetesVersion
	}

	// Same major, compare minor
	if minor >= minMinor {
		return true, ""
	}

	return false, "Kubernetes v" + major + "." + minor + " is below minimum required v" + MinKubernetesVersion
}

// detectNodeSummary counts control plane vs worker nodes and checks for bare-metal.
func detectNodeSummary(nodes []corev1.Node) NodeSummary {
	summary := NodeSummary{TotalNodes: len(nodes)}
	for _, n := range nodes {
		isControlPlane := false
		for k := range n.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/control-plane") ||
				strings.HasPrefix(k, "node-role.kubernetes.io/master") {
				isControlPlane = true
				break
			}
		}
		if isControlPlane {
			summary.ControlPlane++
		} else {
			summary.Worker++
		}

		// Check if bare-metal (no cloud ProviderID)
		if n.Spec.ProviderID == "" {
			summary.BareMetalNodes++
		} else {
			summary.VirtualNodes++
		}
	}
	return summary
}

// detectClusterCompat is the main entry point that gathers all compatibility info.
func detectClusterCompat(gitVersion string, nodes []corev1.Node) ClusterCompat {
	compat := ClusterCompat{
		K8sVersion: gitVersion,
		MinVersion: MinKubernetesVersion,
		NodeInfo:   detectNodeSummary(nodes),
	}

	// Cloud provider detection
	cloud := detectCloudProvider(nodes)
	compat.CloudProvider = cloud

	// Distribution detection
	dist := detectDistribution(gitVersion, nodes)
	compat.Distribution = dist
	compat.Managed = isManagedCloud(dist)

	// Version compatibility check
	compatible, warning := checkVersionCompatibility(gitVersion)
	compat.Compatible = compatible
	compat.Warning = warning

	// Feature highlights
	compat.Features = detectFeatures(nodes)

	return compat
}

// detectFeatures identifies interesting cluster features from node labels.
func detectFeatures(nodes []corev1.Node) []string {
	features := make([]string, 0)
	seen := make(map[string]bool)

	checkFeature := func(label, feature string) {
		if seen[feature] {
			return
		}
		for _, n := range nodes {
			if _, ok := n.Labels[label]; ok {
				features = append(features, feature)
				seen[feature] = true
				break
			}
		}
	}

	checkFeature("nvidia.com/gpu.present", "gpu-nodes")
	checkFeature("intel.com/sgx", "sgx-nodes")
	checkFeature("node.kubernetes.io/instance-type", "cloud-provider-managed")
	checkFeature("topology.kubernetes.io/zone", "multi-zone")
	checkFeature("topology.kubernetes.io/region", "multi-region-capable")

	// Check for ARM nodes
	for _, n := range nodes {
		if n.Status.NodeInfo.Architecture == "arm64" {
			if !seen["arm64-nodes"] {
				features = append(features, "arm64-nodes")
				seen["arm64-nodes"] = true
			}
		}
	}

	// Check for Windows nodes
	for _, n := range nodes {
		if n.Status.NodeInfo.OperatingSystem == "windows" {
			if !seen["windows-nodes"] {
				features = append(features, "windows-nodes")
				seen["windows-nodes"] = true
			}
			break
		}
	}

	return features
}

// handleCompatibility returns detailed cluster compatibility and distribution info.
// GET /api/compatibility
func (s *Server) handleCompatibility(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)

	// Get server version
	info, err := rc.clientset.Discovery().ServerVersion()
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get nodes for detection
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	compat := detectClusterCompat(info.GitVersion, nodes.Items)

	// Add supported API versions map
	compat.Features = append(compat.Features,
		"policy/v1 (PDB)",
		"autoscaling/v2 (HPA)",
		"networking.k8s.io/v1 (Ingress)",
		"batch/v1 (CronJob)",
	)

	writeJSON(w, compat)
}
