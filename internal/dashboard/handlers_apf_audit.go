package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// APFResult is the API Priority & Fairness configuration audit.
type APFResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         APFSummary         `json:"summary"`
	FlowSchemas     []APFFlowSchema    `json:"flowSchemas"`
	PriorityLevels  []APFPriorityLevel `json:"priorityLevels"`
	Issues          []APFIssue         `json:"issues"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// APFSummary aggregates API Priority & Fairness statistics.
type APFSummary struct {
	FlowSchemaCount      int  `json:"flowSchemaCount"`
	PriorityLevelCount   int  `json:"priorityLevelCount"`
	ExemptFlows          int  `json:"exemptFlows"`          // flows matching exempt priority
	CatchAllFlows        int  `json:"catchAllFlows"`        // flows matching catch-all
	MissingPL            int  `json:"missingPL"`            // flow schemas with missing priority level
	ExemptPLCount        int  `json:"exemptPLCount"`        // priority levels of type Exempt
	LimitedPLCount       int  `json:"limitedPLCount"`       // priority levels of type Limited
	GlobalDefaultExists  bool `json:"globalDefaultExists"`  // global-default priority level exists
	LeaderElectionExists bool `json:"leaderElectionExists"` // leader-election priority level exists
	NodeHighExists       bool `json:"nodeHighExists"`       // node-high priority level exists
}

// APFFlowSchema describes a FlowSchema resource.
type APFFlowSchema struct {
	Name                string `json:"name"`
	PriorityLevel       string `json:"priorityLevel"`
	MatchingPrecedence  int    `json:"matchingPrecedence"`
	DistinguisherMethod string `json:"distinguisherMethod"`
	MissingPL           bool   `json:"missingPL"`
	HasRules            bool   `json:"hasRules"`
	RuleCount           int    `json:"ruleCount"`
}

// APFPriorityLevel describes a PriorityLevelConfiguration resource.
type APFPriorityLevel struct {
	Name                     string `json:"name"`
	Type                     string `json:"type"` // Limited, Exempt
	NominalConcurrencyShares int    `json:"nominalConcurrencyShares"`
	Queues                   int    `json:"queues"`
	HandSize                 int    `json:"handSize"`
	QueueLengthLimit         int    `json:"queueLengthLimit"`
}

// APFIssue describes a configuration issue.
type APFIssue struct {
	Name     string `json:"name"`
	Type     string `json:"type"`     // flow-schema, priority-level
	Severity string `json:"severity"` // critical, warning, info
	Detail   string `json:"detail"`
}

// handleAPFAudit audits Kubernetes API Priority & Fairness configuration.
// GET /api/operations/apf-audit
func (s *Server) handleAPFAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result := APFResult{
		ScannedAt: time.Now(),
	}

	if s.restConfig == nil {
		result.HealthScore = 100
		result.Recommendations = append(result.Recommendations, "REST config not available — cannot audit API Priority & Fairness")
		writeJSON(w, result)
		return
	}

	dynClient, err := dynamic.NewForConfig(s.restConfig)
	if err != nil {
		result.HealthScore = 100
		result.Recommendations = append(result.Recommendations, "Dynamic client not available — cannot audit API Priority & Fairness")
		writeJSON(w, result)
		return
	}

	// List FlowSchemas
	flowSchemaGVR := schema.GroupVersionResource{
		Group:    "flowcontrol.apiserver.k8s.io",
		Version:  "v1",
		Resource: "flowschemas",
	}
	flowSchemaList, err := dynClient.Resource(flowSchemaGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.HealthScore = 100
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Cannot list FlowSchemas (API Priority & Fairness may not be enabled): %v", err))
		writeJSON(w, result)
		return
	}

	// List PriorityLevelConfigurations
	plGVR := schema.GroupVersionResource{
		Group:    "flowcontrol.apiserver.k8s.io",
		Version:  "v1",
		Resource: "prioritylevelconfigurations",
	}
	plList, err := dynClient.Resource(plGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.HealthScore = 50
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Cannot list PriorityLevelConfigurations: %v", err))
		writeJSON(w, result)
		return
	}

	// Build priority level map
	plNames := make(map[string]bool)
	for _, pl := range plList.Items {
		plNames[pl.GetName()] = true

		// Parse spec
		spec := pl.Object["spec"].(map[string]interface{})
		plType := ""
		if _, ok := spec["type"]; ok {
			plType = spec["type"].(string)
		}

		entry := APFPriorityLevel{
			Name: pl.GetName(),
			Type: plType,
		}

		if plType == "Limited" {
			if limited, ok := spec["limited"].(map[string]interface{}); ok {
				if v, ok := limited["nominalConcurrencyShares"].(int64); ok {
					entry.NominalConcurrencyShares = int(v)
				}
				if qc, ok := limited["queues"].(map[string]interface{}); ok {
					if v, ok := qc["queues"].(int64); ok {
						entry.Queues = int(v)
					}
					if v, ok := qc["handSize"].(int64); ok {
						entry.HandSize = int(v)
					}
					if v, ok := qc["queueLengthLimit"].(int64); ok {
						entry.QueueLengthLimit = int(v)
					}
				}
			}
			result.Summary.LimitedPLCount++
		} else if plType == "Exempt" {
			result.Summary.ExemptPLCount++
		}

		// Check for essential priority levels
		switch pl.GetName() {
		case "global-default":
			result.Summary.GlobalDefaultExists = true
		case "leader-election":
			result.Summary.LeaderElectionExists = true
		case "node-high":
			result.Summary.NodeHighExists = true
		}

		result.PriorityLevels = append(result.PriorityLevels, entry)
	}

	// Analyze FlowSchemas
	for _, fs := range flowSchemaList.Items {
		result.Summary.FlowSchemaCount++
		entry := APFFlowSchema{
			Name: fs.GetName(),
		}

		spec, ok := fs.Object["spec"].(map[string]interface{})
		if ok {
			if plName, ok := spec["priorityLevelConfiguration"].(map[string]interface{}); ok {
				if name, ok := plName["name"].(string); ok {
					entry.PriorityLevel = name
					if !plNames[name] {
						entry.MissingPL = true
						result.Summary.MissingPL++
						result.Issues = append(result.Issues, APFIssue{
							Name:     fs.GetName(),
							Type:     "flow-schema",
							Severity: "critical",
							Detail:   fmt.Sprintf("FlowSchema references non-existent PriorityLevelConfiguration %q", name),
						})
					}
				}
			}
			if v, ok := spec["matchingPrecedence"].(int64); ok {
				entry.MatchingPrecedence = int(v)
			}
			if dm, ok := spec["distinguisherMethod"].(map[string]interface{}); ok {
				if t, ok := dm["type"].(string); ok {
					entry.DistinguisherMethod = t
				}
			}
			if rules, ok := spec["rules"].([]interface{}); ok {
				entry.HasRules = len(rules) > 0
				entry.RuleCount = len(rules)
			}
		}

		// Classify special flows
		if entry.PriorityLevel == "exempt" {
			result.Summary.ExemptFlows++
		}
		if entry.Name == "catch-all" {
			result.Summary.CatchAllFlows++
		}

		result.FlowSchemas = append(result.FlowSchemas, entry)
	}
	result.Summary.PriorityLevelCount = len(plList.Items)

	// Check for missing essential priority levels
	if !result.Summary.GlobalDefaultExists {
		result.Issues = append(result.Issues, APFIssue{
			Name:     "global-default",
			Type:     "priority-level",
			Severity: "warning",
			Detail:   "global-default priority level is missing — requests without matching FlowSchema will fall through to catch-all with low priority",
		})
	}
	if !result.Summary.LeaderElectionExists {
		result.Issues = append(result.Issues, APFIssue{
			Name:     "leader-election",
			Type:     "priority-level",
			Severity: "warning",
			Detail:   "leader-election priority level is missing — leader election requests may be starved under high API load",
		})
	}
	if !result.Summary.NodeHighExists {
		result.Issues = append(result.Issues, APFIssue{
			Name:     "node-high",
			Type:     "priority-level",
			Severity: "info",
			Detail:   "node-high priority level is missing — node kubelet requests may be starved under high API load",
		})
	}

	// Sort
	sort.Slice(result.FlowSchemas, func(i, j int) bool {
		return result.FlowSchemas[i].MatchingPrecedence < result.FlowSchemas[j].MatchingPrecedence
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return result.Issues[i].Severity > result.Issues[j].Severity
	})

	result.HealthScore = apfScore(result.Summary)
	result.Recommendations = apfRecommendations(result.Summary, result.Issues)

	writeJSON(w, result)
}

// apfScore calculates the health score.
func apfScore(s APFSummary) int {
	base := 100
	base -= s.MissingPL * 20
	if !s.GlobalDefaultExists {
		base -= 10
	}
	if !s.LeaderElectionExists {
		base -= 10
	}
	if base < 0 {
		base = 0
	}
	return base
}

// apfRecommendations generates actionable recommendations.
func apfRecommendations(s APFSummary, issues []APFIssue) []string {
	var recs []string
	criticalCount := 0
	warningCount := 0
	for _, i := range issues {
		if i.Severity == "critical" {
			criticalCount++
		} else if i.Severity == "warning" {
			warningCount++
		}
	}
	if criticalCount > 0 {
		recs = append(recs, fmt.Sprintf("%d critical issue(s) — FlowSchemas referencing non-existent PriorityLevelConfigurations will cause API requests to be rejected", criticalCount))
	}
	if s.MissingPL > 0 {
		recs = append(recs, fmt.Sprintf("%d FlowSchema(s) reference missing PriorityLevelConfiguration — create missing PL or reassign to existing PL", s.MissingPL))
	}
	if !s.GlobalDefaultExists {
		recs = append(recs, "global-default priority level is missing — add it to handle unmatched requests with appropriate queueing")
	}
	if !s.LeaderElectionExists {
		recs = append(recs, "leader-election priority level is missing — add it to prevent leader election starvation under API load")
	}
	if s.ExemptFlows > 3 {
		recs = append(recs, fmt.Sprintf("%d exempt FlowSchema(s) — too many exempt flows bypass API fairness, review if all should be exempt", s.ExemptFlows))
	}
	if criticalCount == 0 && warningCount == 0 {
		recs = append(recs, fmt.Sprintf("API Priority & Fairness is properly configured — %d FlowSchemas, %d PriorityLevels, no missing references", s.FlowSchemaCount, s.PriorityLevelCount))
	}
	return recs
}

// Suppress unused import warning
var _ = strings.TrimSpace
