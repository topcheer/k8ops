package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IncidentPlaybookResult generates an incident response playbook with readiness assessment.
type IncidentPlaybookResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         PlaybookSummary `json:"summary"`
	Playbooks       []PlaybookEntry `json:"playbooks"`
	ReadinessScore  int             `json:"readinessScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type PlaybookSummary struct {
	TotalScenarios  int `json:"totalScenarios"`
	ReadyScenarios  int `json:"readyScenarios"`
	GappedScenarios int `json:"gappedScenarios"`
	CoveragePct     int `json:"coveragePct"`
}

type PlaybookEntry struct {
	Scenario    string   `json:"scenario"`
	Severity    string   `json:"severity"`
	Ready       bool     `json:"ready"`
	Gaps        []string `json:"gaps"`
	RunbookHint string   `json:"runbookHint"`
}

// handleIncidentPlaybook handles GET /api/docs/incident-playbook
func (s *Server) handleIncidentPlaybook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := IncidentPlaybookResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Analyze cluster state for incident scenarios
	var crashingPods, pendingPods int
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodPending {
			pendingPods++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 {
				crashingPods++
			}
		}
	}

	workloadCount := len(deployments.Items)
	pdbCount := len(pdbs.Items)
	nodeCount := len(nodes.Items)

	// Scenario 1: Node Failure
	gaps1 := []string{}
	if nodeCount <= 1 {
		gaps1 = append(gaps1, "single-node cluster - no failover possible")
	}
	if pdbCount < workloadCount {
		gaps1 = append(gaps1, fmt.Sprintf("%d/%d workloads without PDB", workloadCount-pdbCount, workloadCount))
	}
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "Node Failure",
		Severity:    "critical",
		Ready:       len(gaps1) == 0,
		Gaps:        gaps1,
		RunbookHint: "kubectl drain <node> --ignore-daemonsets; verify pods reschedule",
	})

	// Scenario 2: Pod CrashLoop
	gaps2 := []string{}
	if crashingPods > 0 {
		gaps2 = append(gaps2, fmt.Sprintf("%d pods currently crash-looping", crashingPods))
	}
	gaps2 = append(gaps2, "check: kubectl logs <pod> --previous; kubectl describe pod <pod>")
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "Pod CrashLoopBackOff",
		Severity:    "high",
		Ready:       crashingPods == 0,
		Gaps:        gaps2,
		RunbookHint: "kubectl logs <pod> --previous; check resource limits, config validity",
	})

	// Scenario 3: PVC/Storage Failure
	gaps3 := []string{}
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	lostPVCs := 0
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound {
			lostPVCs++
		}
	}
	if lostPVCs > 0 {
		gaps3 = append(gaps3, fmt.Sprintf("%d PVCs not bound", lostPVCs))
	}
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "Storage/PVC Failure",
		Severity:    "high",
		Ready:       lostPVCs == 0,
		Gaps:        gaps3,
		RunbookHint: "kubectl get pvc,pv; check storage class and provisioner logs",
	})

	// Scenario 4: DNS Resolution Failure
	gaps4 := []string{}
	gaps4 = append(gaps4, "verify: kubectl exec <pod> -- nslookup kubernetes.default")
	gaps4 = append(gaps4, "check: kubectl get pods -n kube-system -l k8s-app=kube-dns")
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "DNS Resolution Failure",
		Severity:    "high",
		Ready:       true,
		Gaps:        gaps4,
		RunbookHint: "kubectl get svc kube-dns -n kube-system; check CoreDNS config",
	})

	// Scenario 5: Resource Exhaustion
	gaps5 := []string{}
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
				gaps5 = append(gaps5, fmt.Sprintf("node %s under memory pressure", node.Name))
			}
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
				gaps5 = append(gaps5, fmt.Sprintf("node %s under disk pressure", node.Name))
			}
		}
	}
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "Resource Exhaustion",
		Severity:    "medium",
		Ready:       len(gaps5) == 0,
		Gaps:        gaps5,
		RunbookHint: "kubectl describe node <node>; check resource requests vs capacity",
	})

	// Scenario 6: Network Partition
	gaps6 := []string{"manual verification required: test cross-node connectivity"}
	result.Playbooks = append(result.Playbooks, PlaybookEntry{
		Scenario:    "Network Partition",
		Severity:    "critical",
		Ready:       false,
		Gaps:        gaps6,
		RunbookHint: "kubectl exec <pod> -- ping <other-pod-ip>; check CNI plugin status",
	})

	// Summary
	result.Summary.TotalScenarios = len(result.Playbooks)
	for _, pb := range result.Playbooks {
		if pb.Ready {
			result.Summary.ReadyScenarios++
		} else {
			result.Summary.GappedScenarios++
		}
	}
	if result.Summary.TotalScenarios > 0 {
		result.Summary.CoveragePct = result.Summary.ReadyScenarios * 100 / result.Summary.TotalScenarios
	}
	result.ReadinessScore = result.Summary.CoveragePct
	gradeFromScore(&result.Grade, result.ReadinessScore)

	result.Recommendations = []string{
		fmt.Sprintf("事件响应手册: %d 场景, %d 就绪, %d 有缺口 (%d%% 覆盖率)",
			result.Summary.TotalScenarios, result.Summary.ReadyScenarios,
			result.Summary.GappedScenarios, result.Summary.CoveragePct),
	}
	for _, pb := range result.Playbooks {
		if !pb.Ready && pb.Severity == "critical" {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("[严重] %s 未就绪: %v", pb.Scenario, pb.Gaps))
		}
	}
	writeJSON(w, result)
}
