package source

import (
	"context"
	"errors"
	"testing"

	"github.com/find-assets/scanner/internal/model"
)

type stubSource struct {
	name   string
	stocks []model.Stock
	klines []model.Kline
	err    error
}

func (s *stubSource) ListAll(context.Context) ([]model.Stock, error) {
	return s.stocks, s.err
}

func (s *stubSource) DailyKlines(context.Context, model.Stock, int) ([]model.Kline, error) {
	return s.klines, s.err
}

func TestComposite_FallsBackOnError(t *testing.T) {
	failing := &stubSource{name: "fail", err: errors.New("boom")}
	ok := &stubSource{
		name:   "ok",
		stocks: []model.Stock{{Code: "600000", Name: "浦发"}},
		klines: []model.Kline{{Volume: 1}},
	}
	c := &Composite{sources: []namedSource{
		{name: "fail", src: failing},
		{name: "ok", src: ok},
	}}
	stocks, err := c.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stocks) != 1 || c.ActiveName() != "ok" {
		t.Fatalf("expected fallback to ok, got %d stocks active=%q", len(stocks), c.ActiveName())
	}
	ks, err := c.DailyKlines(context.Background(), stocks[0], 5)
	if err != nil || len(ks) != 1 {
		t.Fatalf("DailyKlines fallback failed: ks=%d err=%v", len(ks), err)
	}
}

func TestComposite_AllFail(t *testing.T) {
	a := &stubSource{err: errors.New("a")}
	b := &stubSource{err: errors.New("b")}
	c := &Composite{sources: []namedSource{{name: "a", src: a}, {name: "b", src: b}}}
	if _, err := c.ListAll(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestNewComposite_AutoExpands(t *testing.T) {
	c, err := NewComposite("auto")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(c.sources))
	}
	names := []string{c.sources[0].name, c.sources[1].name, c.sources[2].name}
	want := []string{"eastmoney", "sina", "tencent"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order mismatch: %v", names)
		}
	}
}

func TestNewComposite_AutoExpandsInsideFallbackSpec(t *testing.T) {
	c, err := NewComposite("file:./stocks.json,auto")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{c.sources[0].name, c.sources[1].name, c.sources[2].name, c.sources[3].name}
	want := []string{"file:./stocks.json", "eastmoney", "sina", "tencent"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order mismatch: %v", names)
		}
	}
}

func TestNewComposite_ExplicitSpec(t *testing.T) {
	c, err := NewComposite("sina,em")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.sources) != 2 || c.sources[0].name != "sina" || c.sources[1].name != "eastmoney" {
		t.Fatalf("unexpected order: %+v", c.sources)
	}
}

func TestNewComposite_UnknownSource(t *testing.T) {
	if _, err := NewComposite("foobar"); err == nil {
		t.Fatal("want error for unknown source")
	}
}
