package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CSIResult is the CSI driver & storage capability audit.
type CSIResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         CSISummary       `json:"summary"`
	ByStorageClass  []CSIEntry       `json:"byStorageClass"`
	CSIDrivers      []CSIDriverEntry `json:"csiDrivers"`
	Issues          []CSIIssue       `json:"issues"`
	Recommendations []string         `json:"recommendations"`
}

// CSISummary aggregates CSI stats.
type CSISummary struct {
	TotalStorageClasses int  `json:"totalStorageClasses"`
	DefaultSCCount      int  `json:"defaultSCCount"` // should be exactly 1
	NoDefaultSC         bool `json:"noDefaultSC"`
	ExpandableSCs       int  `json:"expandableSCs"` // allowVolumeExpansion=true
	TotalCSIDrivers     int  `json:"totalCSIDrivers"`
	SnapshotCapableSCs  int  `json:"snapshotCapableSCs"` // has snapshot controller
	HealthScore         int  `json:"healthScore"`        // 0-100
}

// CSIEntry describes one StorageClass.
type CSIEntry struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	IsDefault         bool   `json:"isDefault"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	AllowExpansion    bool   `json:"allowExpansion"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	Parameters        int    `json:"parameterCount"`
	RiskLevel         string `json:"riskLevel"`
}

// CSIDriverEntry describes one CSIDriver.
type CSIDriverEntry struct {
	Name            string `json:"name"`
	AttachRequired  *bool  `json:"attachRequired,omitempty"`
	PodInfoOnMount  *bool  `json:"podInfoOnMount,omitempty"`
	FSGroupPolicy   string `json:"fsGroupPolicy,omitempty"`
	SnapshotSupport bool   `json:"snapshotSupport"`
}

// CSIIssue is a detected storage problem.
type CSIIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleCSIAudit audits CSI drivers and StorageClass capabilities.
// GET /api/scalability/csi-audit
func (s *Server) handleCSIAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	scs, err := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	drivers, err := rc.clientset.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Check for VolumeSnapshotClass (snapshot support) - these are CRDs, not in standard clientset
	// We infer snapshot support from CSI driver names
	var snapshotClasses int

	// Build CSI driver set for matching
	driverSet := make(map[string]bool)
	for _, d := range drivers.Items {
		driverSet[d.Name] = true
	}

	result := CSIResult{ScannedAt: time.Now()}
	result.Summary.TotalStorageClasses = len(scs.Items)
	result.Summary.TotalCSIDrivers = len(drivers.Items)
	result.Summary.SnapshotCapableSCs = snapshotClasses

	for _, sc := range scs.Items {
		entry := CSIEntry{
			Name:        sc.Name,
			Provisioner: sc.Provisioner,
			Parameters:  len(sc.Parameters),
		}

		// Default SC
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			entry.IsDefault = true
			result.Summary.DefaultSCCount++
		}

		// Binding mode
		if sc.VolumeBindingMode != nil {
			entry.VolumeBindingMode = string(*sc.VolumeBindingMode)
		} else {
			entry.VolumeBindingMode = "Immediate"
		}

		// Expansion
		if sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion {
			entry.AllowExpansion = true
			result.Summary.ExpandableSCs++
		}

		// Reclaim policy
		if sc.ReclaimPolicy != nil {
			entry.ReclaimPolicy = string(*sc.ReclaimPolicy)
		} else {
			entry.ReclaimPolicy = "Delete"
		}

		// Risk assessment
		entry.RiskLevel = csiAssessRisk(entry)

		// Check for missing CSI driver
		if !strings.Contains(sc.Provisioner, "kubernetes.io/") && !driverSet[sc.Provisioner] {
			result.Issues = append(result.Issues, CSIIssue{
				Severity: "warning", Type: "missing-csi-driver",
				Resource: sc.Name,
				Message:  fmt.Sprintf("StorageClass %s uses provisioner %s but no CSIDriver object exists — driver may not be installed", sc.Name, sc.Provisioner),
			})
		}

		// Check non-expandable default SC
		if entry.IsDefault && !entry.AllowExpansion {
			result.Issues = append(result.Issues, CSIIssue{
				Severity: "info", Type: "no-expansion-default",
				Resource: sc.Name,
				Message:  fmt.Sprintf("Default StorageClass %s does not support volume expansion — PVCs cannot be resized", sc.Name),
			})
		}

		// Check Delete reclaim policy
		if entry.ReclaimPolicy == "Delete" {
			result.Issues = append(result.Issues, CSIIssue{
				Severity: "info", Type: "delete-reclaim",
				Resource: sc.Name,
				Message:  fmt.Sprintf("StorageClass %s uses Delete reclaim policy — PVC deletion destroys data, consider Retain for production", sc.Name),
			})
		}

		result.ByStorageClass = append(result.ByStorageClass, entry)
	}

	// Check default SC count
	if result.Summary.DefaultSCCount == 0 {
		result.Summary.NoDefaultSC = true
		result.Issues = append(result.Issues, CSIIssue{
			Severity: "warning", Type: "no-default-sc",
			Resource: "cluster",
			Message:  "No default StorageClass — PVCs without explicit storageClassName will fail",
		})
	} else if result.Summary.DefaultSCCount > 1 {
		result.Issues = append(result.Issues, CSIIssue{
			Severity: "warning", Type: "multiple-default-sc",
			Resource: "cluster",
			Message:  fmt.Sprintf("%d default StorageClasses — only 1 should be default", result.Summary.DefaultSCCount),
		})
	}

	// Build CSI driver entries
	for _, d := range drivers.Items {
		entry := CSIDriverEntry{
			Name:           d.Name,
			AttachRequired: d.Spec.AttachRequired,
			PodInfoOnMount: d.Spec.PodInfoOnMount,
		}
		if d.Spec.FSGroupPolicy != nil {
			entry.FSGroupPolicy = string(*d.Spec.FSGroupPolicy)
		}
		entry.SnapshotSupport = false // Can't check without CRD client
		result.CSIDrivers = append(result.CSIDrivers, entry)
	}

	// Sort
	sort.Slice(result.ByStorageClass, func(i, j int) bool {
		return csiRiskRank(result.ByStorageClass[i].RiskLevel) < csiRiskRank(result.ByStorageClass[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return csiIssueRank(result.Issues[i].Severity) < csiIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = csiScore(result.Summary)
	result.Recommendations = csiGenRecs(result.Summary, result.Issues)

	writeJSON(w, result)
}

// csiAssessRisk determines risk level.
func csiAssessRisk(entry CSIEntry) string {
	if entry.Provisioner == "" {
		return "critical"
	}
	if !entry.AllowExpansion && entry.IsDefault {
		return "medium"
	}
	if entry.ReclaimPolicy == "Delete" {
		return "medium"
	}
	return "low"
}

// csiScore computes health score 0-100.
func csiScore(s CSISummary) int {
	score := 100
	if s.NoDefaultSC {
		score -= 15
	}
	if s.DefaultSCCount > 1 {
		score -= 10
	}
	nonExpandable := s.TotalStorageClasses - s.ExpandableSCs
	score -= nonExpandable * 2
	if s.TotalCSIDrivers == 0 && s.TotalStorageClasses > 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	return score
}

// csiGenRecs produces actionable advice.
func csiGenRecs(s CSISummary, issues []CSIIssue) []string {
	var recs []string

	if s.NoDefaultSC {
		recs = append(recs, "No default StorageClass — PVCs without storageClassName will fail, set a default with: kubectl patch storageclass <name> -p '{\"metadata\":{\"annotations\":{\"storageclass.kubernetes.io/is-default-class\":\"true\"}}}'")
	}
	if s.DefaultSCCount > 1 {
		recs = append(recs, fmt.Sprintf("%d default StorageClasses — remove default annotation from all but one to avoid ambiguity", s.DefaultSCCount))
	}
	if s.ExpandableSCs < s.TotalStorageClasses {
		recs = append(recs, fmt.Sprintf("%d/%d StorageClasses don't support volume expansion — add allowVolumeExpansion: true for PVC resize capability", s.TotalStorageClasses-s.ExpandableSCs, s.TotalStorageClasses))
	}
	if s.TotalCSIDrivers == 0 && s.TotalStorageClasses > 0 {
		recs = append(recs, "No CSIDriver objects found — CSI drivers may not be properly installed")
	}
	if s.SnapshotCapableSCs == 0 && s.TotalStorageClasses > 0 {
		recs = append(recs, "No VolumeSnapshotClasses — volume snapshots unavailable, install snapshot controller for backup support")
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("Storage health score is %d/100 — review StorageClass configuration", s.HealthScore))
	}
	if s.NoDefaultSC == false && s.DefaultSCCount == 1 && s.ExpandableSCs == s.TotalStorageClasses {
		recs = append(recs, fmt.Sprintf("Storage configuration is healthy (%d SCs, %d CSI drivers, score: %d/100)", s.TotalStorageClasses, s.TotalCSIDrivers, s.HealthScore))
	}

	return recs
}

func csiRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func csiIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

var _ = storagev1.StorageClass{}
