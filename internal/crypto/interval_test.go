package crypto

import "testing"

func TestParseIntervalListDefaults(t *testing.T) {
	specs, err := ParseIntervalList("15m,1h,4h")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 intervals, got %d", len(specs))
	}
	if specs[0].Name != "15m" || specs[1].Name != "1h" || specs[2].Name != "4h" {
		t.Fatalf("unexpected specs: %+v", specs)
	}
}

func TestSkipsOnInsufficientBars(t *testing.T) {
	if !SkipsOnInsufficientBars("1h") || !SkipsOnInsufficientBars("4h") {
		t.Fatal("expected 1h and 4h to skip on insufficient bars")
	}
	if SkipsOnInsufficientBars("15m") {
		t.Fatal("15m should not use long-interval skip policy")
	}
}

func TestMapIntervalForExchangeOKX(t *testing.T) {
	if got := MapIntervalForExchange("okx", "1h"); got != "1H" {
		t.Fatalf("expected 1H, got %q", got)
	}
	if got := MapIntervalForExchange("okx", "4h"); got != "4H" {
		t.Fatalf("expected 4H, got %q", got)
	}
	if got := MapIntervalForExchange("okx", "15m"); got != "15m" {
		t.Fatalf("expected 15m passthrough, got %q", got)
	}
}
