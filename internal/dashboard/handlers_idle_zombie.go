package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdleZombieResult detects idle/zombie workloads consuming resources without traffic.
type IdleZombieResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         IdleZombieSummary   `json:"summary"`
	IdleWorkloads   []ZombieWorkloadInfo      `json:"idleWorkloads"`
	WasteCost       float64             `json:"wasteCost"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type IdleZombieSummary struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	IdleWorkloads   int     `json:"idleWorkloads"`
	ZombieWorkloads int     `json:"zombieWorkloads"`
	IdleCPU         float64 `json:"idleCPU"`
	IdleMem         float64 `json:"idleMemGB"`
	EstWasteCost    float64 `json:"estWasteCost"`
}

type ZombieWorkloadInfo struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Type      string  `json:"type"` // idle or zombie
	Age       string  `json:"age"`
	CPU       float64 `json:"cpuCores"`
	Mem       float64 `json:"memGB"`
	Severity  string  `json:"severity"`
}

// handleIdleZombie detects idle/zombie workloads.
// GET /api/deployment/idle-zombie
func (s *Server) handleIdleZombie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := IdleZombieResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}
	now := time.Now()

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})

	// Build endpoint map
	hasEndpoints := map[string]bool{}
	for _, ep := range endpoints.Items {
		for _, sub := range ep.Subsets {
			if len(sub.Addresses) > 0 {
				hasEndpoints[ep.Namespace+"/"+ep.Name] = true
			}
		}
	}

	// Build service map (namespaces with services)
	nsHasService := map[string]bool{}
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] { continue }
		nsHasService[svc.Namespace+"/"+svc.Name] = true
	}

	idleCPU := 0.0
	idleMem := 0.0

	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] { continue }
		result.Summary.TotalWorkloads++

		replicas := int32(0)
		if dep.Spec.Replicas != nil { replicas = *dep.Spec.Replicas }

		// Calculate resource usage
		wlCPU := 0.0
		wlMem := 0.0
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.Resources.Requests != nil {
				if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					wlCPU += float64(q.MilliValue()) / 1000.0 * float64(replicas)
				}
				if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					wlMem += float64(q.Value()) / (1024*1024*1024) * float64(replicas)
				}
			}
		}

		ageDays := int(now.Sub(dep.CreationTimestamp.Time).Hours() / 24)

		// Check if has service with endpoints
		wlKey := dep.Namespace + "/" + dep.Name
		_, hasEP := hasEndpoints[wlKey]

		// Idle: replicas > 0 but no traffic (no endpoints or old with 0 ready)
		isIdle := false
		wkType := ""
		severity := "low"

		if replicas > 0 && !hasEP {
			isIdle = true
			wkType = "idle"
			severity = "medium"
			if ageDays > 30 { severity = "high" }
			result.Summary.IdleWorkloads++
			idleCPU += wlCPU
			idleMem += wlMem
		}

		// Zombie: 0 ready replicas but still deployed
		if dep.Status.ReadyReplicas == 0 && replicas > 0 {
			wkType = "zombie"
			severity = "high"
			result.Summary.ZombieWorkloads++
			isIdle = true
		}

		if isIdle {
			result.IdleWorkloads = append(result.IdleWorkloads, ZombieWorkloadInfo{
				Name: dep.Name, Namespace: dep.Namespace, Type: wkType,
				Age: fmt.Sprintf("%dd", ageDays), CPU: wlCPU, Mem: wlMem, Severity: severity,
			})
		}
	}

	result.Summary.IdleCPU = idleCPU
	result.Summary.IdleMem = idleMem
	result.Summary.EstWasteCost = idleCPU*25 + idleMem*4
	result.WasteCost = result.Summary.EstWasteCost

	// Score
	score := 100
	idleRatio := 0.0
	if result.Summary.TotalWorkloads > 0 {
		idleRatio = float64(result.Summary.IdleWorkloads+result.Summary.ZombieWorkloads) / float64(result.Summary.TotalWorkloads)
	}
	score -= int(idleRatio * 50)
	if result.Summary.ZombieWorkloads > 0 { score -= result.Summary.ZombieWorkloads * 10 }
	if score < 0 { score = 0 }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.IdleWorkloads, func(i, j int) bool {
		return result.IdleWorkloads[i].Severity > result.IdleWorkloads[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Idle/zombie: %d/100 (grade %s) — %d idle, %d zombie, $%.2f/mo waste", result.HealthScore, result.Grade, result.Summary.IdleWorkloads, result.Summary.ZombieWorkloads, result.WasteCost))
	if result.Summary.IdleWorkloads > 0 { recs = append(recs, fmt.Sprintf("%d idle workloads consuming CPU/memory without traffic — scale down", result.Summary.IdleWorkloads)) }
	if result.Summary.ZombieWorkloads > 0 { recs = append(recs, fmt.Sprintf("%d zombie deployments (0 ready) — delete or fix", result.Summary.ZombieWorkloads)) }
	if len(recs) == 1 { recs = append(recs, "No idle or zombie workloads detected") }
	result.Recommendations = recs

	writeJSON(w, result)
}
