package dashboard

import (
	"fmt"
	"net/http"
	"sort"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// PDBInfo represents a PodDisruptionBudget with computed disruption status.
type PDBInfo struct {
	Name               string   `json:"name"`
	Namespace          string   `json:"namespace"`
	MinAvailable       string   `json:"minAvailable"`
	MaxUnavailable     string   `json:"maxUnavailable"`
	Selector           string   `json:"selector"`
	AllowedDisruptions int      `json:"allowedDisruptions"`
	CurrentHealthy     int      `json:"currentHealthy"`
	DesiredHealthy     int      `json:"desiredHealthy"`
	ExpectedPods       int      `json:"expectedPods"`
	DisruptionsOK      bool     `json:"disruptionsOK"`
	Status             string   `json:"status"` // "healthy", "at-risk", "blocked"
	MatchedWorkloads   []string `json:"matchedWorkloads,omitempty"`
}

// handlePDBList returns all PodDisruptionBudgets with disruption status.
// GET /api/pdbs
func (s *Server) handlePDBList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	var pdbs *policyv1.PodDisruptionBudgetList
	var err error
	if ns != "" {
		pdbs, err = rc.clientset.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	} else {
		pdbs, err = rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get all pods for matching PDB selectors
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	items := make([]PDBInfo, 0, len(pdbs.Items))
	var totalBlocked int
	var totalAtRisk int

	for _, pdb := range pdbs.Items {
		info := buildPDBInfo(&pdb, allPods.Items)
		if info.Status == "blocked" {
			totalBlocked++
		} else if info.Status == "at-risk" {
			totalAtRisk++
		}
		items = append(items, info)
	}

	// Sort: blocked first, then at-risk, then healthy
	statusOrder := map[string]int{"blocked": 0, "at-risk": 1, "healthy": 2}
	sort.Slice(items, func(i, j int) bool {
		if statusOrder[items[i].Status] != statusOrder[items[j].Status] {
			return statusOrder[items[i].Status] < statusOrder[items[j].Status]
		}
		return items[i].Namespace+"/"+items[i].Name < items[j].Namespace+"/"+items[j].Name
	})

	writeJSON(w, map[string]any{
		"items": items,
		"summary": map[string]any{
			"total":     len(items),
			"healthy":   len(items) - totalBlocked - totalAtRisk,
			"atRisk":    totalAtRisk,
			"blocked":   totalBlocked,
			"drainSafe": totalBlocked == 0,
		},
	})
}

// buildPDBInfo computes disruption status from a PDB and matching pods.
func buildPDBInfo(pdb *policyv1.PodDisruptionBudget, allPods []corev1.Pod) PDBInfo {
	info := PDBInfo{
		Name:               pdb.Name,
		Namespace:          pdb.Namespace,
		AllowedDisruptions: int(pdb.Status.DisruptionsAllowed),
		CurrentHealthy:     int(pdb.Status.CurrentHealthy),
		DesiredHealthy:     int(pdb.Status.DesiredHealthy),
		ExpectedPods:       int(pdb.Status.ExpectedPods),
		DisruptionsOK:      pdb.Status.DisruptionsAllowed > 0,
	}

	// MinAvailable / MaxUnavailable
	if pdb.Spec.MinAvailable != nil {
		info.MinAvailable = intStrToString(pdb.Spec.MinAvailable)
	}
	if pdb.Spec.MaxUnavailable != nil {
		info.MaxUnavailable = intStrToString(pdb.Spec.MaxUnavailable)
	}

	// Selector
	if pdb.Spec.Selector != nil {
		info.Selector = labels.Set(pdb.Spec.Selector.MatchLabels).String()
		if len(pdb.Spec.Selector.MatchExpressions) > 0 {
			if info.Selector != "" {
				info.Selector += " + expressions"
			} else {
				info.Selector = fmt.Sprintf("%d expressions", len(pdb.Spec.Selector.MatchExpressions))
			}
		}
	}

	// Compute status
	switch {
	case pdb.Status.DisruptionsAllowed <= 0 && pdb.Status.ExpectedPods > 0:
		info.Status = "blocked"
	case pdb.Status.CurrentHealthy <= pdb.Status.DesiredHealthy:
		info.Status = "at-risk"
	default:
		info.Status = "healthy"
	}

	// Find matched workloads (by matching pods)
	if pdb.Spec.Selector != nil && len(pdb.Spec.Selector.MatchLabels) > 0 {
		sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err == nil {
			seen := map[string]bool{}
			for _, pod := range allPods {
				if pod.Namespace != pdb.Namespace {
					continue
				}
				if pod.Status.Phase != corev1.PodRunning {
					continue
				}
				if sel.Matches(labels.Set(pod.Labels)) {
					ownerRef := ""
					if len(pod.OwnerReferences) > 0 {
						ownerRef = pod.OwnerReferences[0].Kind + "/" + pod.OwnerReferences[0].Name
					}
					if ownerRef == "" {
						ownerRef = "Pod/" + pod.Name
					}
					if !seen[ownerRef] {
						seen[ownerRef] = true
						info.MatchedWorkloads = append(info.MatchedWorkloads, ownerRef)
					}
				}
			}
		}
	}

	return info
}

// intStrToString converts intstr.IntOrString to a readable string.
func intStrToString(v *intstr.IntOrString) string {
	if v == nil {
		return ""
	}
	switch v.Type {
	case intstr.Int:
		return fmt.Sprintf("%d", v.IntVal)
	case intstr.String:
		return v.StrVal
	default:
		return fmt.Sprintf("%d", v.IntVal)
	}
}
