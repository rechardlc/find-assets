package indicator

import (
	"math"
	"testing"
)

func TestEMA_BasicShape(t *testing.T) {
	closes := []float64{10, 11, 12, 13, 14, 15}
	got := EMA(closes, 3)
	if len(got) != len(closes) {
		t.Fatalf("len = %d, want %d", len(got), len(closes))
	}
	if got[0] != 10 {
		t.Fatalf("seed value should be Close[0], got %v", got[0])
	}
	// 单调递增输入应产生单调递增 EMA
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("EMA should be monotonically increasing for increasing closes; got[%d]=%v < got[%d]=%v",
				i, got[i], i-1, got[i-1])
		}
	}
}

func TestEMA_RecursiveFormula(t *testing.T) {
	closes := []float64{100, 102, 98, 105, 110}
	n := 3
	alpha := 2.0 / float64(n+1)
	got := EMA(closes, n)
	want := []float64{100}
	for i := 1; i < len(closes); i++ {
		want = append(want, alpha*closes[i]+(1-alpha)*want[i-1])
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("EMA[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCross_GoldCross(t *testing.T) {
	a := []float64{1, 2, 3, 5, 7}
	b := []float64{4, 4, 4, 4, 4}
	// 在 i=3 处 a 由低于 b 变为高于 b → 金叉
	if !Cross(a, b, 1, 4) {
		t.Fatal("expected cross detected")
	}
}

func TestCross_NoCross(t *testing.T) {
	a := []float64{1, 2, 3, 3.5, 3.9}
	b := []float64{4, 4, 4, 4, 4}
	if Cross(a, b, 1, 4) {
		t.Fatal("expected no cross")
	}
}

func TestCross_WindowOutside(t *testing.T) {
	a := []float64{1, 5, 5, 5, 5}
	b := []float64{4, 4, 4, 4, 4}
	// 交叉发生在 i=1，要求窗口 [3,4] 内没有交叉
	if Cross(a, b, 3, 4) {
		t.Fatal("cross is outside the window, should not be detected")
	}
}
