package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/find-assets/scanner/internal/model"
)

func TestSinaSymbol(t *testing.T) {
	cases := map[string]string{
		"600000": "sh600000",
		"688001": "sh688001",
		"000001": "sz000001",
		"300750": "sz300750",
		"123":    "",
	}
	for code, want := range cases {
		got := sinaSymbol(model.Stock{Code: code})
		if got != want {
			t.Errorf("sinaSymbol(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestSina_DailyKlines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"day":"2025-11-19","open":"8.10","high":"8.20","low":"8.05","close":"8.15","volume":"123456"},
			{"day":"2025-11-20","open":"8.15","high":"8.25","low":"8.10","close":"8.20","volume":"234567"}
		]`))
	}))
	defer srv.Close()

	s := NewSina()
	origURL := sinaKlineURLVar()
	setSinaKlineURL(srv.URL)
	t.Cleanup(func() { setSinaKlineURL(origURL) })

	ks, err := s.DailyKlines(context.Background(), model.Stock{Code: "600000"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ks) != 2 {
		t.Fatalf("want 2 klines, got %d", len(ks))
	}
	if ks[0].Close != 8.15 || ks[1].Volume != 234567 {
		t.Fatalf("unexpected kline data: %+v", ks)
	}
}

// 帮助函数：用变量替换 const URL，方便测试。
func sinaKlineURLVar() string { return sinaKlineURLOverride }

func setSinaKlineURL(u string) { sinaKlineURLOverride = u }
