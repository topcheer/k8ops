package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestBuildPDBInfo_Healthy(t *testing.T) {
	zero := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "web-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &zero,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 2,
			CurrentHealthy:     3,
			DesiredHealthy:     1,
			ExpectedPods:       3,
		},
	}

	info := buildPDBInfo(pdb, []corev1.Pod{})

	if info.Status != "healthy" {
		t.Errorf("expected healthy, got %s", info.Status)
	}
	if !info.DisruptionsOK {
		t.Error("expected disruptionsOK=true")
	}
	if info.MinAvailable != "1" {
		t.Errorf("expected minAvailable=1, got %s", info.MinAvailable)
	}
}

func TestBuildPDBInfo_Blocked(t *testing.T) {
	zero := intstr.FromInt(2)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "api-pdb", Namespace: "prod"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &zero,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 0,
			CurrentHealthy:     2,
			DesiredHealthy:     2,
			ExpectedPods:       2,
		},
	}

	info := buildPDBInfo(pdb, []corev1.Pod{})

	if info.Status != "blocked" {
		t.Errorf("expected blocked, got %s", info.Status)
	}
	if info.DisruptionsOK {
		t.Error("expected disruptionsOK=false for blocked PDB")
	}
}

func TestBuildPDBInfo_AtRisk(t *testing.T) {
	one := intstr.FromInt(3)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "db-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 1,
			CurrentHealthy:     3,
			DesiredHealthy:     3,
			ExpectedPods:       3,
		},
	}

	info := buildPDBInfo(pdb, []corev1.Pod{})

	if info.Status != "at-risk" {
		t.Errorf("expected at-risk, got %s", info.Status)
	}
}

func TestBuildPDBInfo_MaxUnavailable(t *testing.T) {
	mu := intstr.FromString("25%")
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &mu,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 1,
			CurrentHealthy:     4,
			DesiredHealthy:     3,
			ExpectedPods:       4,
		},
	}

	info := buildPDBInfo(pdb, []corev1.Pod{})

	if info.MaxUnavailable != "25%" {
		t.Errorf("expected maxUnavailable=25%%, got %s", info.MaxUnavailable)
	}
}

func TestBuildPDBInfo_MatchedWorkloads(t *testing.T) {
	one := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "web-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 1,
			CurrentHealthy:     2,
			DesiredHealthy:     1,
			ExpectedPods:       2,
		},
	}

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-1", Namespace: "default",
				Labels: map[string]string{"app": "web"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "web"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other-1", Namespace: "default",
				Labels: map[string]string{"app": "other"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	info := buildPDBInfo(pdb, pods)

	if len(info.MatchedWorkloads) != 1 {
		t.Fatalf("expected 1 matched workload, got %d", len(info.MatchedWorkloads))
	}
	if info.MatchedWorkloads[0] != "Deployment/web" {
		t.Errorf("expected 'Deployment/web', got %q", info.MatchedWorkloads[0])
	}
}

func TestIntStrToString(t *testing.T) {
	intVal := intstr.FromInt(3)
	if got := intStrToString(&intVal); got != "3" {
		t.Errorf("intStrToString(int 3) = %q, want '3'", got)
	}

	strVal := intstr.FromString("50%")
	if got := intStrToString(&strVal); got != "50%" {
		t.Errorf("intStrToString(string 50%%) = %q, want '50%%'", got)
	}

	if got := intStrToString(nil); got != "" {
		t.Errorf("intStrToString(nil) = %q, want ''", got)
	}
}
