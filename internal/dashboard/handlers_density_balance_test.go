package dashboard

import (
	"testing"
)

func TestComputeStdDev(t *testing.T) {
	// Uniform distribution → 0 std dev
	vals := []float64{5, 5, 5, 5}
	if sd := computeStdDev(vals, 5); sd > 0.01 {
		t.Errorf("expected ~0 std dev, got %f", sd)
	}

	// Spread distribution → non-zero
	vals2 := []float64{1, 10, 3, 8}
	mean2 := 5.5
	if sd := computeStdDev(vals2, mean2); sd < 1 {
		t.Errorf("expected >1 std dev, got %f", sd)
	}
}

func TestSqrtFloat(t *testing.T) {
	if v := sqrtFloat(0); v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
	if v := sqrtFloat(4); v < 1.9 || v > 2.1 {
		t.Errorf("expected ~2, got %f", v)
	}
	if v := sqrtFloat(100); v < 9.5 || v > 10.5 {
		t.Errorf("expected ~10, got %f", v)
	}
}

func TestComputeGini(t *testing.T) {
	// Equal distribution → low Gini
	equal := []float64{5, 5, 5, 5}
	g := computeGini(equal)
	if g > 0.1 {
		t.Errorf("expected low Gini for equal distribution, got %f", g)
	}

	// Very unequal → high Gini
	unequal := []float64{0, 0, 0, 100}
	g2 := computeGini(unequal)
	if g2 < 0.5 {
		t.Errorf("expected high Gini for unequal distribution, got %f", g2)
	}
}

func TestComputeBalanceScore(t *testing.T) {
	s0 := DensityBalanceSummary{TotalNodes: 0}
	if score := computeBalanceScore(s0); score != 100 {
		t.Errorf("expected 100 for no nodes, got %d", score)
	}

	s1 := DensityBalanceSummary{TotalNodes: 5, OverPackedNodes: 2, UnderUsedNodes: 1, AvgPodsPerNode: 10, StdDeviation: 3}
	if score := computeBalanceScore(s1); score > 80 {
		t.Errorf("expected lower score with imbalance, got %d", score)
	}
}
