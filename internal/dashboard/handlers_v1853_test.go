package dashboard

import (
	"testing"
)

func TestPrivilegeMapRecs(t *testing.T) {
	r := &PrivilegeMapResult{
		Summary: PrivilegeSummary{
			TotalContainers:     100,
			Privileged:          3,
			RunAsRoot:           45,
			HostPID:             2,
			HostIPC:             1,
			HostNetwork:         5,
			DangerousCaps:       8,
			AllowPrivEscalation: 4,
		},
		ExposureScore: 55,
	}
	recs := buildPrivilegeMapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestAPISLORecs(t *testing.T) {
	r := &APISLOCorrelationResult{
		Summary: APISLOSummary{
			TotalServices:   30,
			WithProbes:      15,
			WithResources:   10,
			WithHPA:         5,
			WithPDB:         3,
			WithoutAnySLO:   8,
			AvgSLOReadiness: 42.5,
		},
		CorrelationScore: 42,
		Underperformers: []APISLOEntry{
			{ServiceName: "api-gateway", Namespace: "prod", SLOReadiness: 0},
		},
	}
	recs := buildAPISLORecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestEvictionRiskRecs(t *testing.T) {
	r := &EvictionRiskResult{
		Summary: EvictionSummary{
			TotalPods:     80,
			AtRisk:        15,
			CriticalRisk:  5,
			HighRisk:      10,
			BestEffortQoS: 12,
			PressureNodes: 2,
		},
		RiskScore: 81,
	}
	recs := buildEvictionRiskRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestPrivilegeMapTypes(t *testing.T) {
	entry := PrivilegeEntry{
		Workload:     "admin-panel",
		Namespace:    "default",
		Container:    "app",
		Findings:     []string{"privileged=true", "runAsUser=0 (root)"},
		RiskLevel:    "critical",
		FindingCount: 2,
	}
	if entry.RiskLevel != "critical" {
		t.Error("risk level should be critical")
	}
	if len(entry.Findings) != entry.FindingCount {
		t.Error("finding count should match")
	}
}

func TestAPISLOTypes(t *testing.T) {
	entry := APISLOEntry{
		ServiceName:     "web-service",
		Namespace:       "prod",
		HasReadiness:    true,
		HasLiveness:     true,
		HasResources:    false,
		HasHPA:          false,
		HasPDB:          false,
		SLOReadiness:    40,
		RiskLevel:       "high",
		MissingSLOItems: []string{"resourceLimits", "HPA", "PDB"},
	}
	if entry.SLOReadiness != 40 {
		t.Error("SLO readiness should be 40")
	}
}

func TestEvictionRiskTypes(t *testing.T) {
	entry := EvictionEntry{
		PodName:       "worker-0",
		Namespace:     "prod",
		NodeName:      "node-1",
		QoSClass:      "BestEffort",
		PriorityClass: "",
		RiskScore:     55,
		RiskLevel:     "critical",
		RiskFactors:   []string{"best-effort-qos", "no-memory-limit"},
	}
	if entry.RiskScore < 50 || entry.RiskLevel != "critical" {
		t.Error("should be critical risk")
	}
}

func TestNodePressureTypes(t *testing.T) {
	np := EvictionNodePressure{
		NodeName:       "node-1",
		MemoryPressure: true,
		DiskPressure:   false,
		PIDPressure:    false,
		Ready:          true,
		AffectedPods:   12,
		Conditions:     []string{"MemoryPressure"},
	}
	if !np.MemoryPressure || np.AffectedPods != 12 {
		t.Error("node pressure entry should be correct")
	}
}
