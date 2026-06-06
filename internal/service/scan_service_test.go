package service

import (
	"testing"

	"github.com/find-assets/scanner/internal/strategy"
)

func TestNewDay_DefaultRange(t *testing.T) {
	d := strategy.NewDay()
	if d.Range != 2 {
		t.Fatalf("want default range 2, got %v", d.Range)
	}
}

func TestNewDay_DefaultVolume(t *testing.T) {
	d := strategy.NewDay()
	if d.Volume != 20 {
		t.Fatalf("want default volume 20, got %v", d.Volume)
	}
}
