package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func strPtr(s string) *string { return &s }

func TestAnalyzePVCHealth_Bound(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data-pvc", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -5)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: strPtr("fast-ssd"),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("20Gi"),
				},
			},
			VolumeName: "pv-data-001",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("20Gi"),
			},
		},
	}

	pvs := map[string]*corev1.PersistentVolume{
		"pv-data-001": {
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			},
		},
	}

	usage := map[string][]string{
		"default/data-pvc": {"default/app-pod"},
	}

	h := analyzePVCHealth(pvc, pvs, nil, usage)

	if h.Status != PVCHealthBound {
		t.Errorf("expected bound, got %s", h.Status)
	}
	if h.CapacityGB < 19.9 || h.CapacityGB > 21.0 {
		t.Errorf("expected ~20GB capacity, got %.2f", h.CapacityGB)
	}
	if h.PodCount != 1 {
		t.Errorf("expected 1 pod using PVC, got %d", h.PodCount)
	}
	if h.ReclaimPolicy != "Delete" {
		t.Errorf("expected Delete reclaim policy, got %s", h.ReclaimPolicy)
	}
}

func TestAnalyzePVCHealth_Pending(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new-pvc", Namespace: "prod",
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: strPtr("standard"),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	scs := map[string]*storagev1.StorageClass{
		"standard": {
			ObjectMeta:        metav1.ObjectMeta{Name: "standard"},
			Provisioner:       "kubernetes.io/no-provisioner",
			VolumeBindingMode: &[]storagev1.VolumeBindingMode{storagev1.VolumeBindingImmediate}[0],
		},
	}

	h := analyzePVCHealth(pvc, nil, scs, nil)

	if h.Status != PVCHealthPending {
		t.Errorf("expected pending, got %s", h.Status)
	}
	if len(h.Issues) == 0 {
		t.Error("expected issues for pending PVC")
	}
}

func TestAnalyzePVCHealth_PendingNoStorageClass(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-sc", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	h := analyzePVCHealth(pvc, nil, nil, nil)

	if h.Status != PVCHealthPending {
		t.Errorf("expected pending, got %s", h.Status)
	}
	found := false
	for _, issue := range h.Issues {
		if issue == "No storage class specified and no default storage class configured" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'no storage class' issue")
	}
}

func TestAnalyzePVCHealth_Lost(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lost-pvc", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -10)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: strPtr("gp2"),
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimLost,
		},
	}

	h := analyzePVCHealth(pvc, nil, nil, nil)

	if h.Status != PVCHealthLost {
		t.Errorf("expected lost, got %s", h.Status)
	}
	if len(h.Issues) == 0 {
		t.Error("expected issues for lost PVC")
	}
}

func TestAnalyzePVCHealth_Orphaned(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphan-pvc", Namespace: "staging",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -30)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: strPtr("gp2"),
			VolumeName:       "pv-orphan-001",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("50Gi"),
			},
		},
	}

	// No pods using this PVC
	h := analyzePVCHealth(pvc, nil, nil, map[string][]string{})

	if h.Status != PVCHealthOrphaned {
		t.Errorf("expected orphaned, got %s", h.Status)
	}
	if h.PodCount != 0 {
		t.Errorf("expected 0 pods, got %d", h.PodCount)
	}
}

func TestAnalyzePVCHealth_BoundRecentNotOrphaned(t *testing.T) {
	// PVC bound for less than 1 day, no pod using it — should still be "bound" not "orphaned"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fresh-pvc", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pv-fresh",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	h := analyzePVCHealth(pvc, nil, nil, nil)
	if h.Status != PVCHealthBound {
		t.Errorf("expected bound (not orphaned, too new), got %s", h.Status)
	}
}

func TestAnalyzePVCHealth_WaitForFirstConsumer(t *testing.T) {
	waitForConsumer := storagev1.VolumeBindingWaitForFirstConsumer
	sc := &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: "wait-sc"},
		VolumeBindingMode: &waitForConsumer,
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wfc-pvc", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: strPtr("wait-sc"),
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	h := analyzePVCHealth(pvc, nil, map[string]*storagev1.StorageClass{"wait-sc": sc}, nil)

	found := false
	for _, issue := range h.Issues {
		if issue == "Storage class uses WaitForFirstConsumer — waiting for a pod to schedule" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected WaitForFirstConsumer issue")
	}
}

func TestAnalyzePVHealth_Released(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pv-released",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -20)},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Gi"),
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "gp2",
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "old-pvc",
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeReleased,
		},
	}

	h := analyzePVHealth(pv)

	if !h.Orphaned {
		t.Error("expected orphaned PV (released)")
	}
	if h.Status != "Released" {
		t.Errorf("expected Released, got %s", h.Status)
	}
	if h.ClaimRef != "default/old-pvc" {
		t.Errorf("expected claim ref default/old-pvc, got %s", h.ClaimRef)
	}
	if len(h.Issues) < 2 {
		t.Errorf("expected at least 2 issues, got %d", len(h.Issues))
	}
}

func TestAnalyzePVHealth_Bound(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pv-bound",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -5)},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("20Gi"),
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "data-pvc",
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}

	h := analyzePVHealth(pv)

	if h.Orphaned {
		t.Error("bound PV should not be orphaned")
	}
	if h.Status != "Bound" {
		t.Errorf("expected Bound, got %s", h.Status)
	}
	if h.CapacityGB < 19.9 || h.CapacityGB > 21.0 {
		t.Errorf("expected ~20GB, got %.2f", h.CapacityGB)
	}
}

func TestAnalyzePVHealth_Failed(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pv-failed",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -3)},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRecycle,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeFailed,
		},
	}

	h := analyzePVHealth(pv)
	if !h.Orphaned {
		t.Error("failed PV should be orphaned")
	}
}

func TestAnalyzePVHealth_AvailableOld(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pv-stale",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -30)},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Gi"),
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeAvailable,
		},
	}

	h := analyzePVHealth(pv)
	if !h.Orphaned {
		t.Error("old Available PV should be flagged as orphaned")
	}
}

func TestPVCStatusRank(t *testing.T) {
	if pvcStatusRank(PVCHealthFailed) >= pvcStatusRank(PVCHealthBound) {
		t.Error("failed should rank before bound")
	}
	if pvcStatusRank(PVCHealthPending) >= pvcStatusRank(PVCHealthBound) {
		t.Error("pending should rank before bound")
	}
	if pvcStatusRank(PVCHealthLost) >= pvcStatusRank(PVCHealthPending) {
		t.Error("lost should rank before pending")
	}
}

func TestStorageHealthResult_JSON(t *testing.T) {
	result := StorageHealthResult{
		Summary: StorageHealthSummary{
			TotalPVCs:    10,
			PVCsByStatus: map[string]int{"bound": 7, "pending": 2, "orphaned": 1},
			PendingPVCs:  2,
			OrphanedPVCs: 1,
			TotalPVs:     12,
			ReleasedPVs:  1,
		},
		PVCs: []PVCHealth{
			{Name: "pvc1", Namespace: "default", Status: PVCHealthBound, CapacityGB: 20},
		},
		OrphanedPVs: []PVHealth{
			{Name: "pv-old", Status: "Released", CapacityGB: 100},
		},
		StorageClasses: []StorageClassInfo{
			{Name: "gp2", IsDefault: true, PVCCount: 8},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded StorageHealthResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Summary.TotalPVCs != 10 {
		t.Errorf("expected 10 PVCs, got %d", decoded.Summary.TotalPVCs)
	}
	if len(decoded.PVCs) != 1 {
		t.Errorf("expected 1 PVC entry, got %d", len(decoded.PVCs))
	}
	if len(decoded.OrphanedPVs) != 1 {
		t.Errorf("expected 1 orphaned PV, got %d", len(decoded.OrphanedPVs))
	}
	if !decoded.StorageClasses[0].IsDefault {
		t.Error("expected gp2 to be default")
	}
}

func TestParseStorageGB(t *testing.T) {
	q := resource.MustParse("10Gi")
	gb := parseStorageGB(q)
	if gb < 9.9 || gb > 11.0 {
		t.Errorf("expected ~10GB, got %.2f", gb)
	}
}

func TestJoinAccessModes(t *testing.T) {
	modes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}
	result := joinAccessModes(modes)
	if result != "ReadWriteOnce, ReadOnlyMany" {
		t.Errorf("expected 'ReadWriteOnce, ReadOnlyMany', got %q", result)
	}
}

func TestStorageClassInfo_Fields(t *testing.T) {
	expand := true
	sc := StorageClassInfo{
		Name:            "fast-ssd",
		IsDefault:       true,
		Provisioner:     "kubernetes.io/aws-ebs",
		ReclaimPolicy:   "Delete",
		BindingMode:     "WaitForFirstConsumer",
		VolumeExpansion: expand,
		PVCCount:        15,
		PendingCount:    2,
	}

	data, _ := json.Marshal(sc)
	var decoded StorageClassInfo
	json.Unmarshal(data, &decoded)

	if decoded.Name != "fast-ssd" {
		t.Errorf("expected fast-ssd, got %s", decoded.Name)
	}
	if !decoded.VolumeExpansion {
		t.Error("expected volume expansion to be true")
	}
	if decoded.PVCCount != 15 {
		t.Errorf("expected 15 PVCs, got %d", decoded.PVCCount)
	}
}
