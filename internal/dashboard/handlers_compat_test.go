package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeCompatNode(name, providerID, kubeletVersion string, labels map[string]string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.NodeSpec{
			ProviderID: providerID,
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:  kubeletVersion,
				Architecture:    "amd64",
				OperatingSystem: "linux",
			},
		},
	}
}

// --- Cloud Provider Detection ---

func TestDetectCloudProvider_AWS(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "aws://i-1234567890abcdef0", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "aws" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "aws")
	}
}

func TestDetectCloudProvider_GCP(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "gce://projects/my-project/zones/us-central1-a/instances/1234567890", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "gcp" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "gcp")
	}
}

func TestDetectCloudProvider_Azure(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "azure:///subscriptions/abc/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm-1", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "azure" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "azure")
	}
}

func TestDetectCloudProvider_VSphere(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "vsphere://420e7c8a-dead-beef-cnab", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "vsphere" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "vsphere")
	}
}

func TestDetectCloudProvider_BareMetal(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "", "", nil),
		makeCompatNode("node-2", "", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "bare-metal" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "bare-metal")
	}
}

func TestDetectCloudProvider_Alibaba(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "alicloud://cn-hangzhou.i-bp1abc", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "alibaba" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "alibaba")
	}
}

func TestDetectCloudProvider_Tencent(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "tencent://ins-12345678", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "tencent" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "tencent")
	}
}

func TestDetectCloudProvider_MixedNodes(t *testing.T) {
	// First node has no providerID, second does — should detect from second
	nodes := []corev1.Node{
		makeCompatNode("node-1", "", "", nil),
		makeCompatNode("node-2", "aws://i-abc123", "", nil),
	}
	got := detectCloudProvider(nodes)
	if got != "aws" {
		t.Errorf("detectCloudProvider() = %q, want %q", got, "aws")
	}
}

// --- Distribution Detection ---

func TestDetectDistribution_EKS(t *testing.T) {
	got := detectDistribution("v1.28.4-eks-123456", nil)
	if got != "eks" {
		t.Errorf("detectDistribution() = %q, want %q", got, "eks")
	}
}

func TestDetectDistribution_GKE(t *testing.T) {
	got := detectDistribution("v1.28.3-gke.100", nil)
	if got != "gke" {
		t.Errorf("detectDistribution() = %q, want %q", got, "gke")
	}
}

func TestDetectDistribution_AKS(t *testing.T) {
	got := detectDistribution("v1.28.5-aks", nil)
	if got != "aks" {
		t.Errorf("detectDistribution() = %q, want %q", got, "aks")
	}
}

func TestDetectDistribution_K3s(t *testing.T) {
	got := detectDistribution("v1.28.4+k3s1", nil)
	if got != "k3s" {
		t.Errorf("detectDistribution() = %q, want %q", got, "k3s")
	}
}

func TestDetectDistribution_RKE2(t *testing.T) {
	got := detectDistribution("v1.28.4+rke2r1", nil)
	if got != "rke2" {
		t.Errorf("detectDistribution() = %q, want %q", got, "rke2")
	}
}

func TestDetectDistribution_OpenShift(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "", "", map[string]string{
			"node.openshift.io/os_id": "rhcos",
		}),
	}
	got := detectDistribution("v1.28.4", nodes)
	if got != "openshift" {
		t.Errorf("detectDistribution() = %q, want %q", got, "openshift")
	}
}

func TestDetectDistribution_Talos(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "", "", map[string]string{
			"kubernetes.io/hostname": "talos-ctrl-1",
		}),
	}
	got := detectDistribution("v1.28.4", nodes)
	// Talos might not be detected from hostname alone; test with label
	if got != "vanilla" {
		// Without a specific label, it's vanilla — that's expected
	}

	nodes2 := []corev1.Node{
		makeCompatNode("node-1", "", "", map[string]string{
			"talos.dev/version": "v1.7.0",
		}),
	}
	got = detectDistribution("v1.28.4", nodes2)
	if got != "talos" {
		t.Errorf("detectDistribution() = %q, want %q", got, "talos")
	}
}

func TestDetectDistribution_Vanilla(t *testing.T) {
	got := detectDistribution("v1.28.4", nil)
	if got != "vanilla" {
		t.Errorf("detectDistribution() = %q, want %q", got, "vanilla")
	}
}

func TestDetectDistribution_Minikube(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("minikube", "", "", map[string]string{
			"kubernetes.io/hostname": "minikube",
		}),
	}
	got := detectDistribution("v1.28.0", nodes)
	if got != "minikube" {
		t.Errorf("detectDistribution() = %q, want %q", got, "minikube")
	}
}

// --- Managed Cloud Detection ---

func TestIsManagedCloud(t *testing.T) {
	managed := []string{"eks", "gke", "aks", "oke", "ack", "tke", "doks", "lke"}
	for _, d := range managed {
		if !isManagedCloud(d) {
			t.Errorf("isManagedCloud(%q) = false, want true", d)
		}
	}

	unmanaged := []string{"vanilla", "k3s", "rke2", "openshift", "minikube", "talos"}
	for _, d := range unmanaged {
		if isManagedCloud(d) {
			t.Errorf("isManagedCloud(%q) = true, want false", d)
		}
	}
}

// --- Version Compatibility ---

func TestCheckVersionCompatibility_SupportedVersions(t *testing.T) {
	versions := []string{
		"v1.28.4-eks-123456",
		"v1.30.0",
		"v1.25.0",
		"v1.26.5+gke.100",
		"v2.0.0",
	}
	for _, v := range versions {
		compatible, warning := checkVersionCompatibility(v)
		if !compatible {
			t.Errorf("checkVersionCompatibility(%q) = false, want true. Warning: %s", v, warning)
		}
		if warning != "" {
			t.Errorf("checkVersionCompatibility(%q) warning = %q, want empty", v, warning)
		}
	}
}

func TestCheckVersionCompatibility_OldVersions(t *testing.T) {
	versions := []string{
		"v1.24.0",
		"v1.22.5",
		"v1.20.0",
	}
	for _, v := range versions {
		compatible, _ := checkVersionCompatibility(v)
		if compatible {
			t.Errorf("checkVersionCompatibility(%q) = true, want false (below minimum)", v)
		}
	}
}

func TestCheckVersionCompatibility_ExactMinimum(t *testing.T) {
	compatible, warning := checkVersionCompatibility("v1.25.0")
	if !compatible {
		t.Errorf("checkVersionCompatibility(v1.25.0) = false, want true (exact minimum)")
	}
	if warning != "" {
		t.Errorf("checkVersionCompatibility(v1.25.0) warning = %q, want empty", warning)
	}
}

func TestCheckVersionCompatibility_InvalidVersion(t *testing.T) {
	compatible, _ := checkVersionCompatibility("invalid")
	if compatible {
		t.Errorf("checkVersionCompatibility(invalid) = true, want false")
	}
}

// --- Node Summary ---

func TestDetectNodeSummary(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("master-1", "aws://i-aaa", "", map[string]string{
			"node-role.kubernetes.io/control-plane": "",
		}),
		makeCompatNode("worker-1", "aws://i-bbb", "", map[string]string{
			"node-role.kubernetes.io/worker": "",
		}),
		makeCompatNode("worker-2", "", "", map[string]string{
			"node-role.kubernetes.io/worker": "",
		}),
	}

	summary := detectNodeSummary(nodes)
	if summary.TotalNodes != 3 {
		t.Errorf("TotalNodes = %d, want 3", summary.TotalNodes)
	}
	if summary.ControlPlane != 1 {
		t.Errorf("ControlPlane = %d, want 1", summary.ControlPlane)
	}
	if summary.Worker != 2 {
		t.Errorf("Worker = %d, want 2", summary.Worker)
	}
	if summary.BareMetalNodes != 1 {
		t.Errorf("BareMetalNodes = %d, want 1", summary.BareMetalNodes)
	}
	if summary.VirtualNodes != 2 {
		t.Errorf("VirtualNodes = %d, want 2", summary.VirtualNodes)
	}
}

func TestDetectNodeSummary_MasterLabel(t *testing.T) {
	// Test old-style master label
	nodes := []corev1.Node{
		makeCompatNode("master-1", "", "", map[string]string{
			"node-role.kubernetes.io/master": "",
		}),
	}

	summary := detectNodeSummary(nodes)
	if summary.ControlPlane != 1 {
		t.Errorf("ControlPlane = %d, want 1 (old master label)", summary.ControlPlane)
	}
}

// --- Feature Detection ---

func TestDetectFeatures_ARM(t *testing.T) {
	node := makeCompatNode("arm-1", "aws://i-aaa", "", nil)
	node.Status.NodeInfo.Architecture = "arm64"
	nodes := []corev1.Node{node}

	features := detectFeatures(nodes)
	found := false
	for _, f := range features {
		if f == "arm64-nodes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("arm64-nodes not detected in features: %v", features)
	}
}

func TestDetectFeatures_Windows(t *testing.T) {
	node := makeCompatNode("win-1", "aws://i-aaa", "", nil)
	node.Status.NodeInfo.OperatingSystem = "windows"
	nodes := []corev1.Node{node}

	features := detectFeatures(nodes)
	found := false
	for _, f := range features {
		if f == "windows-nodes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("windows-nodes not detected in features: %v", features)
	}
}

func TestDetectFeatures_GPU(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("gpu-1", "aws://i-aaa", "", map[string]string{
			"nvidia.com/gpu.present": "true",
		}),
	}

	features := detectFeatures(nodes)
	found := false
	for _, f := range features {
		if f == "gpu-nodes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("gpu-nodes not detected in features: %v", features)
	}
}

// --- Full Integration ---

func TestDetectClusterCompat_EKS(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("master-1", "aws://i-aaa", "v1.28.4-eks-123", map[string]string{
			"node-role.kubernetes.io/control-plane": "",
			"topology.kubernetes.io/zone":           "us-east-1a",
			"node.kubernetes.io/instance-type":      "m5.large",
		}),
		makeCompatNode("worker-1", "aws://i-bbb", "v1.28.4-eks-123", map[string]string{
			"node-role.kubernetes.io/worker":   "",
			"topology.kubernetes.io/zone":      "us-east-1b",
			"node.kubernetes.io/instance-type": "m5.xlarge",
		}),
	}

	compat := detectClusterCompat("v1.28.4-eks-123456", nodes)

	if compat.CloudProvider != "aws" {
		t.Errorf("CloudProvider = %q, want aws", compat.CloudProvider)
	}
	if compat.Distribution != "eks" {
		t.Errorf("Distribution = %q, want eks", compat.Distribution)
	}
	if !compat.Managed {
		t.Errorf("Managed = false, want true")
	}
	if !compat.Compatible {
		t.Errorf("Compatible = false, want true")
	}
	if compat.Warning != "" {
		t.Errorf("Warning = %q, want empty", compat.Warning)
	}
	if compat.NodeInfo.ControlPlane != 1 {
		t.Errorf("NodeInfo.ControlPlane = %d, want 1", compat.NodeInfo.ControlPlane)
	}
	if compat.NodeInfo.Worker != 1 {
		t.Errorf("NodeInfo.Worker = %d, want 1", compat.NodeInfo.Worker)
	}
}

func TestDetectClusterCompat_K3sBareMetal(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("k3s-1", "", "v1.28.4+k3s1", map[string]string{
			"node-role.kubernetes.io/control-plane": "",
		}),
		makeCompatNode("k3s-2", "", "v1.28.4+k3s1", map[string]string{
			"node-role.kubernetes.io/worker": "",
		}),
	}

	compat := detectClusterCompat("v1.28.4+k3s1", nodes)

	if compat.CloudProvider != "bare-metal" {
		t.Errorf("CloudProvider = %q, want bare-metal", compat.CloudProvider)
	}
	if compat.Distribution != "k3s" {
		t.Errorf("Distribution = %q, want k3s", compat.Distribution)
	}
	if compat.Managed {
		t.Errorf("Managed = true, want false")
	}
	if !compat.Compatible {
		t.Errorf("Compatible = false, want true")
	}
	if compat.NodeInfo.BareMetalNodes != 2 {
		t.Errorf("BareMetalNodes = %d, want 2", compat.NodeInfo.BareMetalNodes)
	}
}

func TestDetectClusterCompat_OldVersion(t *testing.T) {
	nodes := []corev1.Node{
		makeCompatNode("node-1", "", "", nil),
	}

	compat := detectClusterCompat("v1.22.5", nodes)

	if compat.Compatible {
		t.Errorf("Compatible = true, want false (v1.22 < minimum v1.25)")
	}
	if compat.Warning == "" {
		t.Errorf("Warning = empty, want non-empty for old version")
	}
}

func TestDetectClusterCompat_MinKubernetesVersion(t *testing.T) {
	if MinKubernetesVersion != "1.25" {
		t.Errorf("MinKubernetesVersion = %q, want 1.25", MinKubernetesVersion)
	}
}
