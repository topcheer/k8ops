package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PDBGeneratorResult generates PodDisruptionBudget YAML manifests for
// multi-replica workloads that currently lack PDB protection. It provides
// ready-to-apply kubectl commands and structured manifest data.
type PDBGeneratorResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         PDBGenSummary `json:"summary"`
	Generated       []PDBManifest `json:"generated"`
	BatchApply      []string      `json:"batchApply"`
	HealthScore     int           `json:"healthScore"`
	Grade           string        `json:"grade"`
	Recommendations []string      `json:"recommendations"`
}

type PDBGenSummary struct {
	MultiRepolaWorkloads int `json:"multiReplicaWorkloads"`
	WithPDB              int `json:"withPDB"`
	MissingPDB           int `json:"missingPDB"`
	TotalReplicas        int `json:"totalReplicas"`
}

type PDBManifest struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Workload     string `json:"workload"`
	Kind         string `json:"kind"`
	MinAvailable int    `json:"minAvailable"`
	Replicas     int    `json:"replicas"`
	ManifestYAML string `json:"manifestYAML"`
	ApplyCommand string `json:"applyCommand"`
}

// handlePDBGenerator handles GET /api/operations/pdb-generator
func (s *Server) handlePDBGenerator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PDBGeneratorResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build existing PDB coverage map by namespace+selector
	pdbNSMap := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		pdbNSMap[pdb.Namespace] = true
	}

	var manifests []PDBManifest
	var batchCmds []string

	genForWorkload := func(name, ns, kind string, replicas int, selectorLabels map[string]string) {
		if replicas < 2 {
			return
		}
		result.Summary.MultiRepolaWorkloads++
		result.Summary.TotalReplicas += replicas

		if pdbNSMap[ns] {
			result.Summary.WithPDB++
			return
		}

		result.Summary.MissingPDB++
		minAvail := replicas - 1
		if minAvail < 1 {
			minAvail = 1
		}

		pdbName := name + "-pdb"
		yaml := generatePDBYAML(pdbName, ns, name, kind, minAvail, selectorLabels)
		cmd := fmt.Sprintf("kubectl apply -f - <<'EOF'\n%sEOF", yaml)

		manifests = append(manifests, PDBManifest{
			Name: pdbName, Namespace: ns, Workload: name, Kind: kind,
			MinAvailable: minAvail, Replicas: replicas,
			ManifestYAML: yaml, ApplyCommand: cmd,
		})
		batchCmds = append(batchCmds, cmd)
	}

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := int(ptrInt32(d.Spec.Replicas))
		var labels map[string]string
		if d.Spec.Selector != nil {
			labels = d.Spec.Selector.MatchLabels
		}
		genForWorkload(d.Name, d.Namespace, "Deployment", replicas, labels)
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		replicas := int(ptrInt32(ss.Spec.Replicas))
		var labels map[string]string
		if ss.Spec.Selector != nil {
			labels = ss.Spec.Selector.MatchLabels
		}
		genForWorkload(ss.Name, ss.Namespace, "StatefulSet", replicas, labels)
	}

	// Score
	if result.Summary.MultiRepolaWorkloads > 0 {
		result.HealthScore = result.Summary.WithPDB * 100 / result.Summary.MultiRepolaWorkloads
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Replicas > manifests[j].Replicas
	})

	result.Generated = manifests
	result.BatchApply = batchCmds
	result.Recommendations = buildPDBGenRecs(&result)

	writeJSON(w, result)
}

func generatePDBYAML(pdbName, ns, wlName, kind string, minAvail int, labels map[string]string) string {
	if len(labels) == 0 {
		labels = map[string]string{"app": wlName}
	}

	matchLabels := ""
	for k, v := range labels {
		matchLabels += fmt.Sprintf("        %s: %s\n", k, v)
	}

	return fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s
  namespace: %s
spec:
  minAvailable: %d
  selector:
    matchLabels:
%s`, pdbName, ns, minAvail, matchLabels)
}

func buildPDBGenRecs(r *PDBGeneratorResult) []string {
	recs := []string{}
	if r.Summary.MissingPDB == 0 {
		recs = append(recs, "所有多副本工作负载都有 PDB 保护")
		return recs
	}
	recs = append(recs, fmt.Sprintf("%d 个多副本工作负载缺少 PDB", r.Summary.MissingPDB))
	recs = append(recs, fmt.Sprintf("已生成 %d 个 PDB YAML，可使用 batchApply 命令批量创建", len(r.BatchApply)))
	recs = append(recs, "建议先在测试命名空间验证 PDB 效果")
	return recs
}

var _ policyv1.PodDisruptionBudget
var _ appsv1.Deployment
