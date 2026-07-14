package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPVAccessScore(t *testing.T) {
	tests := []struct {
		name     string
		s        PVAccessSummary
		minScore int
		maxScore int
	}{
		{"no PVCs", PVAccessSummary{}, 100, 100},
		{"all healthy", PVAccessSummary{TotalPVCs: 10, BoundPVCs: 10, RetainReclaim: 10}, 95, 100},
		{"some unbound", PVAccessSummary{TotalPVCs: 10, UnboundPVCs: 3, DeleteReclaim: 2}, 70, 85},
		{"all bad", PVAccessSummary{TotalPVCs: 10, UnboundPVCs: 10, DeleteReclaim: 10, NoStorageClass: 5, MultiAttachPVCs: 3}, 0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := pvAccessScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestPVAccessRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := pvAccessRecommendations(PVAccessSummary{TotalPVCs: 5, BoundPVCs: 5, RetainReclaim: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := pvAccessRecommendations(PVAccessSummary{
			UnboundPVCs:     2,
			DeleteReclaim:   3,
			MultiAttachPVCs: 1,
			NoStorageClass:  1,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestPVCAccessModesString(t *testing.T) {
	tests := []struct {
		modes []corev1.PersistentVolumeAccessMode
		want  string
	}{
		{[]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, "RWO"},
		{[]corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, "RWX"},
		{[]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadWriteMany}, "RWO,RWX"},
	}
	for _, tt := range tests {
		got := pvcAccessModesString(tt.modes)
		if got != tt.want {
			t.Errorf("pvcAccessModesString(%v) = %s, want %s", tt.modes, got, tt.want)
		}
	}
}

func TestPVAccessAuditCore(t *testing.T) {
	storageClassName := "fast-ssd"
	scMap := map[string]*PVAccessSCStat{
		"fast-ssd": {StorageClass: "fast-ssd", RiskLevel: "low"},
	}

	pvs := []corev1.PersistentVolume{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
			Spec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				StorageClassName:              storageClassName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-2"},
			Spec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				StorageClassName:              storageClassName,
			},
		},
	}

	pvcs := []corev1.PersistentVolumeClaim{
		// Bound PVC with Delete reclaim
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-bound", Namespace: "prod"},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &storageClassName,
				VolumeName:       "pv-1",
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		// Unbound PVC
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-unbound", Namespace: "dev"},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		// RWX PVC used by multiple pods
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-shared", Namespace: "shared"},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				StorageClassName: &storageClassName,
				VolumeName:       "pv-2",
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "prod"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-bound"}}},
			}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "shared"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{
				{Name: "shared", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-shared"}}},
			}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3", Namespace: "shared"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{
				{Name: "shared", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-shared"}}},
			}}},
	}

	result := pvAccessAuditCore(pvs, pvcs, pods, scMap)

	if result.Summary.TotalPVCs != 3 {
		t.Errorf("expected totalPVCs=3, got %d", result.Summary.TotalPVCs)
	}
	if result.Summary.BoundPVCs != 2 {
		t.Errorf("expected boundPVCs=2, got %d", result.Summary.BoundPVCs)
	}
	if result.Summary.UnboundPVCs != 1 {
		t.Errorf("expected unboundPVCs=1, got %d", result.Summary.UnboundPVCs)
	}
	if result.Summary.MultiAttachPVCs != 1 {
		t.Errorf("expected multiAttachPVCs=1, got %d", result.Summary.MultiAttachPVCs)
	}
	if result.Summary.DeleteReclaim != 1 {
		t.Errorf("expected deleteReclaim=1, got %d", result.Summary.DeleteReclaim)
	}
	if result.Summary.RWXPVs != 1 {
		t.Errorf("expected rwxPVs=1, got %d", result.Summary.RWXPVs)
	}
	if result.Summary.RWOPVs != 1 {
		t.Errorf("expected rwoPVs=1, got %d", result.Summary.RWOPVs)
	}
	if len(result.Risks) < 3 {
		t.Errorf("expected at least 3 risks, got %d", len(result.Risks))
	}
	if len(result.UnboundPVCs) < 1 {
		t.Errorf("expected at least 1 unbound PVC, got %d", len(result.UnboundPVCs))
	}
	if len(result.MultiAttachPVCs) < 1 {
		t.Errorf("expected at least 1 multi-attach PVC, got %d", len(result.MultiAttachPVCs))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
	if result.HealthScore > 90 {
		t.Errorf("expected health score <= 90 due to issues, got %d", result.HealthScore)
	}
}
