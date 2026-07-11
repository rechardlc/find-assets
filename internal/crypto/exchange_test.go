package crypto

import (
	"encoding/json"
	"testing"
)

func TestNormalizeOKXSwapSymbol(t *testing.T) {
	got := normalizeOKXSwapSymbol("1000PEPE-USDT-SWAP")
	if got != "1000PEPEUSDT" {
		t.Fatalf("expected 1000PEPEUSDT, got %q", got)
	}
}

func TestParseOKXKlinesSortsAscending(t *testing.T) {
	raw := json.RawMessage(`{"code":"0","data":[["1718583300000","2","2.2","1.9","2.1","10"],["1718582400000","1","1.2","0.9","1.1","20"]]}`)
	got, err := parseOKXKlines(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 klines, got %d", len(got))
	}
	if !got[0].Date.Before(got[1].Date) {
		t.Fatalf("expected ascending order: %+v", got)
	}
	if got[0].Close != 1.1 || got[1].Close != 2.1 {
		t.Fatalf("unexpected closes: %+v", got)
	}
}
