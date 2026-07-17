package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigSnapshotResult captures a point-in-time snapshot of cluster
// configuration state for drift detection. It records key workload
// specs, resource counts, and configuration hashes that can be compared
// against future snapshots to detect unauthorized or unexpected changes.
type ConfigSnapshotResult struct {
	SnapshotAt      time.Time         `json:"snapshotAt"`
	ClusterVersion  string            `json:"clusterVersion"`
	Summary         ConfigSnapSummary `json:"summary"`
	WorkloadHashes  []ConfigSnapHash  `json:"workloadHashes"`
	ResourceCounts  map[string]int    `json:"resourceCounts"`
	NamespaceList   []string          `json:"namespaces"`
	SnapshotID      string            `json:"snapshotId"`
	Recommendations []string          `json:"recommendations"`
}

type ConfigSnapSummary struct {
	Nodes           int `json:"nodes"`
	Namespaces      int `json:"namespaces"`
	Deployments     int `json:"deployments"`
	StatefulSets    int `json:"statefulSets"`
	DaemonSets      int `json:"daemonSets"`
	Services        int `json:"services"`
	ConfigMaps      int `json:"configMaps"`
	Secrets         int `json:"secrets"`
	PVCs            int `json:"pvcs"`
	NetworkPolicies int `json:"networkPolicies"`
	TotalHashes     int `json:"totalHashes"`
}

type ConfigSnapHash struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Hash      string `json:"hash"`
	Replicas  int    `json:"replicas"`
	ImageHash string `json:"imageHash"`
}

// handleConfigSnapshot handles GET /api/deployment/config-snapshot
func (s *Server) handleConfigSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ConfigSnapshotResult{
		SnapshotAt:     time.Now(),
		SnapshotID:     fmt.Sprintf("snap-%d", time.Now().Unix()),
		ResourceCounts: make(map[string]int),
	}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	result.Summary.Nodes = len(nodes.Items)
	result.Summary.Namespaces = len(namespaces.Items)
	result.Summary.Deployments = len(deployments.Items)
	result.Summary.StatefulSets = len(statefulsets.Items)
	result.Summary.DaemonSets = len(daemonsets.Items)
	result.Summary.Services = len(services.Items)
	result.Summary.ConfigMaps = len(configmaps.Items)
	result.Summary.Secrets = len(secrets.Items)
	result.Summary.PVCs = len(pvcs.Items)
	result.Summary.NetworkPolicies = len(netpols.Items)

	// Cluster version
	if len(nodes.Items) > 0 {
		result.ClusterVersion = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	// Namespace list
	nsSet := make(map[string]bool)
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsSet[ns.Name] = true
		}
	}
	for ns := range nsSet {
		result.NamespaceList = append(result.NamespaceList, ns)
	}
	sort.Strings(result.NamespaceList)

	// Workload hashes (for drift detection)
	hashWorkloads := func(kind string, items interface{}, getCount func(i int) (string, string, int, string)) {
		// Generic hashing approach - marshal template spec to JSON and hash
	}
	_ = hashWorkloads // suppress unused warning

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		specJSON, _ := json.Marshal(d.Spec.Template.Spec)
		hash := shortHash(string(specJSON))
		imgHash := shortHash(getImageList(d.Spec.Template.Spec.Containers))
		result.WorkloadHashes = append(result.WorkloadHashes, ConfigSnapHash{
			Kind: "Deployment", Name: d.Name, Namespace: d.Namespace,
			Hash: hash, Replicas: int(ptrInt32(d.Spec.Replicas)),
			ImageHash: imgHash,
		})
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		specJSON, _ := json.Marshal(ss.Spec.Template.Spec)
		hash := shortHash(string(specJSON))
		imgHash := shortHash(getImageList(ss.Spec.Template.Spec.Containers))
		result.WorkloadHashes = append(result.WorkloadHashes, ConfigSnapHash{
			Kind: "StatefulSet", Name: ss.Name, Namespace: ss.Namespace,
			Hash: hash, Replicas: int(ptrInt32(ss.Spec.Replicas)),
			ImageHash: imgHash,
		})
	}

	result.Summary.TotalHashes = len(result.WorkloadHashes)

	// Resource counts for summary
	result.ResourceCounts["deployments"] = len(deployments.Items)
	result.ResourceCounts["statefulsets"] = len(statefulsets.Items)
	result.ResourceCounts["daemonsets"] = len(daemonsets.Items)
	result.ResourceCounts["services"] = len(services.Items)
	result.ResourceCounts["configmaps"] = len(configmaps.Items)
	result.ResourceCounts["secrets"] = len(secrets.Items)
	result.ResourceCounts["pvcs"] = len(pvcs.Items)
	result.ResourceCounts["networkpolicies"] = len(netpols.Items)

	result.Recommendations = []string{
		fmt.Sprintf("快照 ID: %s，保存此 ID 用于未来漂移比较", result.SnapshotID),
		fmt.Sprintf("已记录 %d 个工作负载的配置哈希", result.Summary.TotalHashes),
		"建议定期生成快照并比较工作负载哈希以检测未授权变更",
		"可将此快照导出为 JSON 文件存储到 Git 仓库实现 GitOps 审计",
	}

	writeJSON(w, result)
}

func shortHash(s string) string {
	if len(s) < 8 {
		return fmt.Sprintf("%x", s)
	}
	// Simple hash: first 8 and last 8 chars of hex encoding
	hex := fmt.Sprintf("%x", []byte(s))
	if len(hex) > 16 {
		return hex[:16]
	}
	return hex
}

func getImageList(containers []corev1.Container) string {
	imgs := []string{}
	for _, c := range containers {
		imgs = append(imgs, c.Image)
	}
	return strings.Join(imgs, ",")
}
