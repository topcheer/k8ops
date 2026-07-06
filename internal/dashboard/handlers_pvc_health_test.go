package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

func TestPVHScore(t *testing.T) {
	// Empty
	if score := pvhScore(PVHSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := PVHSummary{TotalPVCs: 10, BoundPVCs: 10}
	if score := pvhScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = PVHSummary{
		TotalPVCs:   20,
		PendingPVCs: 3, // -24
		LostPVCs:    1, // -20
		FailedPVs:   2, // -30
		ReleasedPVs: 3, // -9
	}
	// 100 - 24 - 20 - 30 - 9 = 17
	if score := pvhScore(s); score != 17 {
		t.Errorf("Expected 17, got %d", score)
	}

	// Heavily broken
	s = PVHSummary{
		TotalPVCs:   5,
		PendingPVCs: 3,
		LostPVCs:    2,
	}
	// 100 - 24 - 40 = 36
	if score := pvhScore(s); score != 36 {
		t.Errorf("Expected 36, got %d", score)
	}
}

func TestPVHGenRecs(t *testing.T) {
	s := PVHSummary{
		TotalPVCs:        20,
		PendingPVCs:      3,
		LostPVCs:         1,
		FailedPVs:        2,
		ReleasedPVs:      3,
		NoExpandingSC:    2,
		ReclaimRetainPVs: 5,
		HealthScore:      35,
	}

	recs := pvhGenRecs(s, nil, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundPending := false
	foundLost := false
	foundReleased := false
	for _, r := range recs {
		if strContains(r, "Pending") {
			foundPending = true
		}
		if strContains(r, "Lost") {
			foundLost = true
		}
		if strContains(r, "Released") {
			foundReleased = true
		}
	}
	if !foundPending {
		t.Error("Expected recommendation about pending PVCs")
	}
	if !foundLost {
		t.Error("Expected recommendation about lost PVCs")
	}
	if !foundReleased {
		t.Error("Expected recommendation about released PVs")
	}
}

func TestPVHGenRecsClean(t *testing.T) {
	s := PVHSummary{TotalPVCs: 10, BoundPVCs: 10}
	recs := pvhGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestPVHAccessModes(t *testing.T) {
	modes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}
	result := pvhAccessModes(modes)
	if !strContains(result, "ReadWriteOnce") {
		t.Error("Expected ReadWriteOnce in result")
	}
	if !strContains(result, "ReadOnlyMany") {
		t.Error("Expected ReadOnlyMany in result")
	}
}

func TestIsDefaultSC(t *testing.T) {
	sc := storagev1.StorageClass{}
	if isDefaultSC(sc) {
		t.Error("Expected false for nil annotations")
	}

	sc.Annotations = map[string]string{
		"storageclass.kubernetes.io/is-default-class": "true",
	}
	if !isDefaultSC(sc) {
		t.Error("Expected true for default annotation")
	}

	sc.Annotations = map[string]string{
		"storageclass.beta.kubernetes.io/is-default-class": "true",
	}
	if !isDefaultSC(sc) {
		t.Error("Expected true for beta default annotation")
	}

	sc.Annotations = map[string]string{
		"storageclass.kubernetes.io/is-default-class": "false",
	}
	if isDefaultSC(sc) {
		t.Error("Expected false for non-default annotation")
	}
}

func TestPVHGetOrCreateNS(t *testing.T) {
	m := make(map[string]*PVHNSEntry)
	e1 := pvhGetOrCreateNS(m, "default")
	e1.PVCCount = 5

	e2 := pvhGetOrCreateNS(m, "default")
	if e2.PVCCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.PVCCount)
	}

	e3 := pvhGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}

func TestPVHIssueRank(t *testing.T) {
	if pvhIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if pvhIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if pvhIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestPVHGenRecsNoDefaultSC(t *testing.T) {
	s := PVHSummary{TotalPVCs: 5, BoundPVCs: 5, DefaultSCCount: 0}
	recs := pvhGenRecs(s, nil, nil)
	foundNoDefault := false
	for _, r := range recs {
		if strContains(r, "default StorageClass") {
			foundNoDefault = true
		}
	}
	if !foundNoDefault {
		t.Error("Expected recommendation about missing default StorageClass")
	}
}
