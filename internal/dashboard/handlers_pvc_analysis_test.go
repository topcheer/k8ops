package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCalculatePVCHealthScore(t *testing.T) {
	// Empty
	if score := calculatePVCHealthScore(PVCSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// All bound
	allBound := PVCSummary{TotalPVCs: 10, BoundPVCs: 10}
	if score := calculatePVCHealthScore(allBound); score != 100 {
		t.Errorf("Expected 100 for all bound, got %d", score)
	}

	// With stuck
	withStuck := PVCSummary{
		TotalPVCs:   10,
		StuckPVCs:   2, // -30
		PendingPVCs: 1, // -3
	}
	// 100 - 30 - 3 = 67
	if score := calculatePVCHealthScore(withStuck); score != 67 {
		t.Errorf("Expected 67, got %d", score)
	}

	// Floor at 0
	terrible := PVCSummary{
		TotalPVCs: 5,
		StuckPVCs: 10, // -150
	}
	if score := calculatePVCHealthScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestDetermineStuckReasonNoSC(t *testing.T) {
	scName := "nonexistent"
	pvc := &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scName,
		},
	}
	scMap := map[string]*storagev1.StorageClass{} // empty map

	reason := determineStuckReason(pvc, scMap)
	if reason == "" {
		t.Error("Expected non-empty reason for missing SC")
	}
}

func TestDetermineStuckReasonDefault(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{},
	}

	reason := determineStuckReason(pvc, map[string]*storagev1.StorageClass{})
	// Should provide some helpful reason
	if reason == "" {
		t.Error("Expected non-empty reason")
	}
}

func TestGeneratePVCRecommendations(t *testing.T) {
	s := PVCSummary{
		StuckPVCs:        2,
		PendingPVCs:      1,
		SlowBindingCount: 3,
		DefaultSC:        "",
		TotalSizeGB:      100,
		BoundSizeGB:      80,
		HealthScore:      45,
	}

	recs := generatePVCRecommendations(s, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundStuck := false
	foundSlow := false
	foundDefault := false
	for _, r := range recs {
		if containsSubstr(r, "stuck") {
			foundStuck = true
		}
		if containsSubstr(r, "30s") || containsSubstr(r, "slow") {
			foundSlow = true
		}
		if containsSubstr(r, "default") {
			foundDefault = true
		}
	}
	if !foundStuck {
		t.Error("Expected recommendation about stuck PVCs")
	}
	if !foundSlow {
		t.Error("Expected recommendation about slow binding")
	}
	if !foundDefault {
		t.Error("Expected recommendation about missing default StorageClass")
	}
}

func TestGeneratePVCRecommendationsClean(t *testing.T) {
	s := PVCSummary{
		TotalPVCs:   10,
		BoundPVCs:   10,
		TotalSizeGB: 50,
		BoundSizeGB: 50,
		DefaultSC:   "fast-ssd",
		HealthScore: 100,
	}

	recs := generatePVCRecommendations(s, nil)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestGetOrCreateSCStat(t *testing.T) {
	m := make(map[string]*StorageClassStat)

	e1 := getOrCreateSCStat(m, "fast-ssd")
	e1.PVCCount = 5

	e2 := getOrCreateSCStat(m, "fast-ssd")
	if e2.PVCCount != 5 {
		t.Errorf("Expected same entry with PVCCount=5, got %d", e2.PVCCount)
	}

	e3 := getOrCreateSCStat(m, "")
	if e3.Name != "<default>" {
		t.Errorf("Expected '<default>' for empty name, got %q", e3.Name)
	}
}

func TestPVCAnalysisStatusRank(t *testing.T) {
	if pvcAnalysisStatusRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if pvcAnalysisStatusRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if pvcAnalysisStatusRank("healthy") != 2 {
		t.Error("Expected 2 for healthy")
	}
}

func TestPVCAnalysisSeverityRank(t *testing.T) {
	if pvcAnalysisSeverityRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if pvcAnalysisSeverityRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if pvcAnalysisSeverityRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}

func TestCalculatePVCBindTimeNotBound(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}
	pvMap := map[string]*corev1.PersistentVolume{}

	if d := calculatePVCBindTime(pvc, pvMap); d != 0 {
		t.Errorf("Expected 0 for pending PVC, got %v", d)
	}
}

func TestCalculatePVCBindTimeBound(t *testing.T) {
	created := time.Now()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: created},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pv-123",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: created.Add(5 * time.Second)},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}

	pvMap := map[string]*corev1.PersistentVolume{
		"pv-123": pv,
	}

	d := calculatePVCBindTime(pvc, pvMap)
	if d != 5*time.Second {
		t.Errorf("Expected 5s bind time, got %v", d)
	}
}
