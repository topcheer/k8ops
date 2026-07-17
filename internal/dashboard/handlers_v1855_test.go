package dashboard

import (
	"testing"
)

func TestSecretSprayRecs(t *testing.T) {
	r := &SecretSprayResult{
		Summary: SecretSpraySummary{
			TotalSecrets:    50,
			MountedSecrets:  30,
			OrphanedSecrets: 20,
			HighSpray:       5,
			MaxMountCount:   25,
		},
		ExposureScore: 55,
		CriticalSpray: []SecretSprayEntry{
			{SecretName: "db-credentials", Namespace: "prod", MountCount: 25},
		},
	}
	recs := buildSecretSprayRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestTrafficCostRecs(t *testing.T) {
	r := &TrafficCostSplitResult{
		Summary: TrafficCostSummary{
			TotalServices:   40,
			TotalIngresses:  15,
			TotalPodCost:    500.0,
			AttributedCost:  300.0,
			UnattributedPct: 40.0,
			TopServiceShare: 65.0,
		},
		TopCostPaths: []TrafficCostEntry{
			{ServiceName: "api-gateway", Namespace: "prod", MonthlyCostUSD: 120.0, BackingPods: 8},
		},
	}
	recs := buildTrafficCostRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestNodeBlastRecs(t *testing.T) {
	r := &NodeFailureBlastResult{
		Summary: NodeBlastSummary{
			TotalNodes:          4,
			TotalPods:           80,
			TotalWorkloads:      30,
			AvgPodsPerNode:      20,
			MaxBlastPods:        30,
			MaxBlastPct:         37.5,
			SingleReplicaAtRisk: 15,
			AntiAffinityGaps:    3,
		},
		BlastScore: 62,
	}
	recs := buildNodeBlastRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestSecretSprayTypes(t *testing.T) {
	entry := SecretSprayEntry{
		SecretName:  "tls-cert",
		Namespace:   "prod",
		Type:        "kubernetes.io/tls",
		MountCount:  15,
		SprayLevel:  "high",
		RiskScore:   120,
		IsSensitive: true,
	}
	if !entry.IsSensitive {
		t.Error("TLS secret should be sensitive")
	}
}

func TestTrafficCostTypes(t *testing.T) {
	entry := TrafficCostEntry{
		ServiceName:    "web-frontend",
		Namespace:      "prod",
		ServiceType:    "ClusterIP",
		HasIngress:     true,
		IngressHost:    "example.com",
		BackingPods:    5,
		CPUCores:       2.5,
		MemoryGB:       8.0,
		MonthlyCostUSD: 120.0,
		CostTier:       "expensive",
	}
	if entry.MonthlyCostUSD < 100 || entry.CostTier != "expensive" {
		t.Error("should be expensive tier")
	}
}

func TestNodeBlastTypes(t *testing.T) {
	entry := NodeBlastEntry{
		NodeName:          "worker-3",
		Role:              "worker",
		PodCount:          35,
		AffectedWorkloads: []string{"prod/api", "prod/web"},
		SingleReplicaWL:   []string{"prod/cron"},
		BlastPct:          43.75,
		RecoveryTime:      "5-10min",
		RiskLevel:         "high",
	}
	if entry.BlastPct < 25 || entry.RiskLevel != "high" {
		t.Error("should be high risk")
	}
}

func TestAppendUniqueStr(t *testing.T) {
	slice := []string{"a", "b"}
	slice = appendUniqueSecretStr(slice, "b") // duplicate
	if len(slice) != 2 {
		t.Errorf("should not add duplicate, got %d", len(slice))
	}
	slice = appendUniqueSecretStr(slice, "c") // new
	if len(slice) != 3 {
		t.Errorf("should add new, got %d", len(slice))
	}
}

func TestSafeDivFloat(t *testing.T) {
	if safeDivFloat(10, 2) != 5.0 {
		t.Error("10/2 should be 5")
	}
	if safeDivFloat(10, 0) != 0 {
		t.Error("div by zero should return 0")
	}
}
