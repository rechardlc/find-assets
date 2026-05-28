package service

import "testing"

func TestEffectiveCohesion_PrefersRange(t *testing.T) {
	p := Params{Range: 1.5, Cohesion: 0.02}
	if got := p.effectiveCohesion(); got != 0.015 {
		t.Fatalf("want 0.015 from range, got %v", got)
	}
}

func TestEffectiveCohesion_FallbackToCohesion(t *testing.T) {
	p := Params{Range: 0, Cohesion: 0.02}
	if got := p.effectiveCohesion(); got != 0.02 {
		t.Fatalf("want 0.02 from cohesion, got %v", got)
	}
}

func TestEffectiveCohesion_NegativeRangeFallsBack(t *testing.T) {
	p := Params{Range: -1, Cohesion: 0.01}
	if got := p.effectiveCohesion(); got != 0.01 {
		t.Fatalf("want 0.01 fallback, got %v", got)
	}
}

func TestEffectiveCohesion_BothZero(t *testing.T) {
	p := Params{}
	if got := p.effectiveCohesion(); got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}
