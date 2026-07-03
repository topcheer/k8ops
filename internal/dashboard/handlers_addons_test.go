package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildScanCtx creates an addonScanContext with given namespaces, pods, and CRDs.
func buildScanCtx(nsList []string, pods []corev1.Pod, crdList []string) *addonScanContext {
	nsSet := make(map[string]bool)
	for _, ns := range nsList {
		nsSet[ns] = true
	}

	podMap := make(map[string][]corev1.Pod)
	for _, p := range pods {
		podMap[p.Namespace] = append(podMap[p.Namespace], p)
	}

	crdSet := make(map[string]bool)
	for _, c := range crdList {
		crdSet[c] = true
	}

	return &addonScanContext{
		pods:       podMap,
		namespaces: nsSet,
		crds:       crdSet,
	}
}

func makeAddonPod(name, ns string, ready, running bool, restarts int32) corev1.Pod {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: ready, RestartCount: restarts},
			},
		},
	}
	if !running {
		p.Status.Phase = corev1.PodPending
	}
	return p
}

// --- detectByNamespace tests ---

func TestDetectByNamespace_Healthy(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"calico-system"},
		[]corev1.Pod{
			makeAddonPod("calico-node-abc", "calico-system", true, true, 0),
			makeAddonPod("calico-node-def", "calico-system", true, true, 0),
		},
		nil,
	)

	info := detectByNamespace(sctx, "Calico", "cni", []string{"calico-system"}, []string{"calico"})
	if !info.Detected {
		t.Error("expected Calico to be detected")
	}
	if info.Status != AddonHealthy {
		t.Errorf("expected healthy, got %s", info.Status)
	}
	if info.Pods.Ready != 2 || info.Pods.Total != 2 {
		t.Errorf("expected 2/2 ready, got %d/%d", info.Pods.Ready, info.Pods.Total)
	}
}

func TestDetectByNamespace_Degraded(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"cilium-system"},
		[]corev1.Pod{
			makeAddonPod("cilium-agent-abc", "cilium-system", true, true, 0),
			makeAddonPod("cilium-agent-def", "cilium-system", false, false, 0),
		},
		nil,
	)

	info := detectByNamespace(sctx, "Cilium", "cni", []string{"cilium-system"}, []string{"cilium"})
	if !info.Detected {
		t.Error("expected Cilium to be detected")
	}
	if info.Status != AddonDegraded {
		t.Errorf("expected degraded, got %s", info.Status)
	}
	if len(info.Issues) == 0 {
		t.Error("expected issues for degraded add-on")
	}
}

func TestDetectByNamespace_NotInstalled(t *testing.T) {
	sctx := buildScanCtx([]string{"default"}, nil, nil)

	info := detectByNamespace(sctx, "Istio", "serviceMesh", []string{"istio-system"}, []string{"istiod"})
	if info.Detected {
		t.Error("expected Istio not detected")
	}
	if info.Status != AddonNotInstalled {
		t.Errorf("expected not-installed, got %s", info.Status)
	}
}

func TestDetectByNamespace_HighRestarts(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"kube-system"},
		[]corev1.Pod{
			makeAddonPod("coredns-abc", "kube-system", true, true, 10),
			makeAddonPod("coredns-def", "kube-system", true, true, 8),
		},
		nil,
	)

	info := detectByNamespace(sctx, "CoreDNS", "dns", []string{"kube-system"}, []string{"coredns"})
	if !info.Detected {
		t.Fatal("expected CoreDNS detected")
	}
	// 18 total restarts for 2 pods = high restart count
	if info.Status != AddonDegraded {
		t.Errorf("expected degraded due to high restarts, got %s", info.Status)
	}
}

func TestDetectByNamespace_FallbackNamespace(t *testing.T) {
	// First namespace doesn't exist, second does
	sctx := buildScanCtx(
		[]string{"kube-system"},
		[]corev1.Pod{
			makeAddonPod("traefik-abc", "kube-system", true, true, 0),
		},
		nil,
	)

	info := detectByNamespace(sctx, "Traefik", "ingress", []string{"traefik-system", "kube-system"}, []string{"traefik"})
	if !info.Detected {
		t.Error("expected Traefik detected in fallback namespace")
	}
	if info.Namespace != "kube-system" {
		t.Errorf("expected namespace kube-system, got %s", info.Namespace)
	}
}

// --- detectByCRD tests ---

func TestDetectByCRD_Found(t *testing.T) {
	sctx := buildScanCtx(nil, nil, []string{"backups.velero.io", "schedules.velero.io"})

	info := detectByCRD(sctx, "Velero", "backup", []string{"backups.velero.io"})
	if !info.Detected {
		t.Error("expected Velero detected via CRD")
	}
	if info.Status != AddonHealthy {
		t.Errorf("expected healthy, got %s", info.Status)
	}
}

func TestDetectByCRD_NotFound(t *testing.T) {
	sctx := buildScanCtx(nil, nil, []string{"other.crd.io"})

	info := detectByCRD(sctx, "Velero", "backup", []string{"backups.velero.io"})
	if info.Detected {
		t.Error("expected Velero not detected")
	}
}

func TestDetectByCRD_WithPods(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"velero", "velero-system"},
		[]corev1.Pod{
			makeAddonPod("velero-abc", "velero", true, true, 0),
		},
		[]string{"backups.velero.io"},
	)

	info := detectByCRD(sctx, "Velero", "backup", []string{"backups.velero.io"})
	if !info.Detected {
		t.Error("expected Velero detected")
	}
	if info.Pods == nil {
		t.Error("expected pod info for Velero")
	}
}

// --- Category scan tests ---

func TestScanCNI_DetectCalico(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"calico-system"},
		[]corev1.Pod{
			makeAddonPod("calico-node-xxx", "calico-system", true, true, 0),
		},
		nil,
	)

	addons := scanCNI(sctx)
	calico := findAddon(addons, "Calico")
	if calico == nil {
		t.Fatal("Calico not in scan results")
	}
	if !calico.Detected {
		t.Error("expected Calico detected")
	}
}

func TestScanCNI_NoneInstalled(t *testing.T) {
	sctx := buildScanCtx([]string{"default"}, nil, nil)

	addons := scanCNI(sctx)
	for _, a := range addons {
		if a.Detected {
			t.Errorf("expected no CNI detected, but found %s", a.Name)
		}
	}
}

func TestScanIngress_DetectMultiple(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"ingress-nginx", "traefik-system"},
		[]corev1.Pod{
			makeAddonPod("nginx-ingress-controller-abc", "ingress-nginx", true, true, 0),
			makeAddonPod("traefik-xyz", "traefik-system", true, true, 0),
		},
		nil,
	)

	addons := scanIngress(sctx)
	nginx := findAddon(addons, "NGINX Ingress")
	traefik := findAddon(addons, "Traefik")
	if nginx == nil || !nginx.Detected {
		t.Error("expected NGINX Ingress detected")
	}
	if traefik == nil || !traefik.Detected {
		t.Error("expected Traefik detected")
	}
}

func TestScanServiceMesh_NoneInstalled(t *testing.T) {
	sctx := buildScanCtx([]string{"default"}, nil, nil)

	addons := scanServiceMesh(sctx)
	for _, a := range addons {
		if a.Detected {
			t.Errorf("expected no mesh detected, found %s", a.Name)
		}
	}
}

func TestScanBackup_DetectVelero(t *testing.T) {
	sctx := buildScanCtx(nil, nil, []string{"backups.velero.io", "schedules.velero.io"})

	addons := scanBackup(sctx)
	velero := findAddon(addons, "Velero")
	if velero == nil || !velero.Detected {
		t.Error("expected Velero detected via CRD")
	}
}

func TestScanMonitoring_DetectPrometheus(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"monitoring"},
		[]corev1.Pod{
			makeAddonPod("prometheus-main-0", "monitoring", true, true, 0),
			makeAddonPod("grafana-abc", "monitoring", true, true, 0),
		},
		nil,
	)

	addons := scanMonitoring(sctx)
	prom := findAddon(addons, "Prometheus")
	grafana := findAddon(addons, "Grafana")
	if prom == nil || !prom.Detected {
		t.Error("expected Prometheus detected")
	}
	if grafana == nil || !grafana.Detected {
		t.Error("expected Grafana detected")
	}
}

func TestScanGitOps_DetectArgoCD(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"argocd"},
		[]corev1.Pod{
			makeAddonPod("argocd-server-abc", "argocd", true, true, 0),
			makeAddonPod("argocd-application-controller-xyz", "argocd", true, true, 0),
		},
		nil,
	)

	addons := scanGitOps(sctx)
	argocd := findAddon(addons, "Argo CD")
	if argocd == nil || !argocd.Detected {
		t.Error("expected Argo CD detected")
	}
}

// --- Full scan integration ---

func TestScanAllAddons_MixedCluster(t *testing.T) {
	sctx := buildScanCtx(
		[]string{"kube-system", "cert-manager", "metallb-system", "monitoring"},
		[]corev1.Pod{
			makeAddonPod("coredns-abc", "kube-system", true, true, 0),
			makeAddonPod("traefik-xyz", "kube-system", true, true, 0),
			makeAddonPod("cert-manager-abc", "cert-manager", true, true, 0),
			makeAddonPod("cert-manager-webhook-xyz", "cert-manager", true, true, 0),
			makeAddonPod("controller-abc", "metallb-system", true, true, 0),
			makeAddonPod("speaker-def", "metallb-system", true, true, 0),
			makeAddonPod("prometheus-0", "monitoring", true, true, 0),
			makeAddonPod("grafana-abc", "monitoring", true, true, 0),
		},
		[]string{
			"certificates.cert-manager.io",
			"ipaddresspools.metallb.io",
			"backups.velero.io",
		},
	)

	result := scanAllAddons(sctx)

	if result.Summary.TotalDetected == 0 {
		t.Error("expected at least some add-ons detected")
	}
	if result.Summary.Healthy == 0 {
		t.Error("expected at least some healthy add-ons")
	}

	// Verify categories exist
	expectedCats := []string{"cni", "dns", "ingress", "certManager", "loadBalancer", "serviceMesh", "backup", "monitoring", "policy", "storage", "gitops", "virtualMachine"}
	for _, cat := range expectedCats {
		if _, ok := result.Categories[cat]; !ok {
			t.Errorf("category %s missing from results", cat)
		}
	}
}

func TestScanAllAddons_EmptyCluster(t *testing.T) {
	sctx := buildScanCtx([]string{"default"}, nil, nil)

	result := scanAllAddons(sctx)

	if result.Summary.TotalDetected != 0 {
		t.Errorf("expected 0 detected, got %d", result.Summary.TotalDetected)
	}
	if result.Summary.NotInstalled == 0 {
		t.Error("expected some not-installed count")
	}
}

// --- Utility function tests ---

func TestSummarizePods_AllRunning(t *testing.T) {
	pods := []corev1.Pod{
		makeAddonPod("a-1", "ns", true, true, 0),
		makeAddonPod("a-2", "ns", true, true, 0),
	}
	ready, total, restarts := summarizePods(pods)
	if ready != 2 || total != 2 || restarts != 0 {
		t.Errorf("got %d/%d ready, %d restarts; want 2/2, 0", ready, total, restarts)
	}
}

func TestSummarizePods_PartialFailure(t *testing.T) {
	pods := []corev1.Pod{
		makeAddonPod("a-1", "ns", true, true, 1),
		makeAddonPod("a-2", "ns", false, false, 3),
	}
	ready, total, restarts := summarizePods(pods)
	if ready != 1 || total != 2 {
		t.Errorf("got %d/%d ready; want 1/2", ready, total)
	}
	if restarts != 4 {
		t.Errorf("got %d restarts; want 4", restarts)
	}
}

func TestBuildPodMap(t *testing.T) {
	pods := []corev1.Pod{
		makeAddonPod("a", "ns1", true, true, 0),
		makeAddonPod("b", "ns1", true, true, 0),
		makeAddonPod("c", "ns2", true, true, 0),
	}
	m := buildPodMap(pods)
	if len(m["ns1"]) != 2 {
		t.Errorf("ns1: expected 2 pods, got %d", len(m["ns1"]))
	}
	if len(m["ns2"]) != 1 {
		t.Errorf("ns2: expected 1 pod, got %d", len(m["ns2"]))
	}
}

func TestBuildNamespaceSet(t *testing.T) {
	nss := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
	}
	set := buildNamespaceSet(nss)
	if !set["default"] || !set["kube-system"] {
		t.Error("expected both namespaces in set")
	}
	if set["nonexistent"] {
		t.Error("nonexistent namespace should not be in set")
	}
}

func TestExtractCRDNames(t *testing.T) {
	lists := []*metav1.APIResourceList{
		{
			GroupVersion: "cert-manager.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "certificates", Namespaced: true, Kind: "Certificate"},
				{Name: "issuers", Namespaced: true, Kind: "Issuer"},
			},
		},
		{
			GroupVersion: "velero.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "backups", Namespaced: true, Kind: "Backup"},
			},
		},
	}

	crds := extractCRDNames(lists)
	if !crds["certificates.cert-manager.io"] {
		t.Error("expected certificates.cert-manager.io")
	}
	if !crds["backups.velero.io"] {
		t.Error("expected backups.velero.io")
	}
}

// --- Helper ---

func findAddon(addons []AddonInfo, name string) *AddonInfo {
	for i := range addons {
		if addons[i].Name == name {
			return &addons[i]
		}
	}
	return nil
}
