package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseListDiff_Array(t *testing.T) {
	raw := json.RawMessage(`[{"f12":"600000","f14":"浦发银行"},{"f12":"000001","f14":"平安银行"}]`)
	items, err := parseListDiff(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].F12 != "600000" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestParseListDiff_Object(t *testing.T) {
	raw := json.RawMessage(`{"1":{"f12":"000001","f14":"平安银行"},"0":{"f12":"600000","f14":"浦发银行"}}`)
	items, err := parseListDiff(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].F12 != "600000" || items[1].F12 != "000001" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestListAll_ContinuesWhenFirstPageSmallerThanPageSize(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		switch page {
		case 1:
			_, _ = w.Write([]byte(`{"data":{"total":120,"diff":[` +
				strings.Repeat(`{"f12":"600000","f14":"测试"},`, 88) +
				`{"f12":"600001","f14":"测试"}]}}`))
		case 2:
			var b strings.Builder
			b.WriteString(`{"data":{"total":120,"diff":[`)
			for i := 0; i < 20; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				fmt.Fprintf(&b, `{"f12":"%06d","f14":"测试"}`, 601000+i)
			}
			b.WriteString(`]}}`)
			_, _ = w.Write([]byte(b.String()))
		default:
			_, _ = w.Write([]byte(`{"data":{"total":120,"diff":[]}}`))
		}
	}))
	defer srv.Close()

	orig := listHosts
	listHosts = []string{srv.URL}
	t.Cleanup(func() { listHosts = orig })

	e := NewEastMoney()
	e.pageJitterFn = func() time.Duration { return 0 } // 单测下不需要真实节流
	stocks, err := e.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page < 2 {
		t.Fatalf("expected pagination beyond first page, got page=%d", page)
	}
	if len(stocks) != 109 {
		t.Fatalf("want 109 stocks, got %d", len(stocks))
	}
}

func TestListAll_PaginatesUntilTotal(t *testing.T) {
	const (
		pageSize = 100
		total    = 250
	)
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		start := (page - 1) * pageSize
		if start >= total {
			fmt.Fprintf(w, `{"data":{"total":%d,"diff":[]}}`, total)
			return
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		var b strings.Builder
		b.WriteString(`{"data":{"total":`)
		fmt.Fprintf(&b, "%d", total)
		b.WriteString(`,"diff":[`)
		for i := start; i < end; i++ {
			if i > start {
				b.WriteByte(',')
			}
			code := fmt.Sprintf("%06d", 600000+i)
			fmt.Fprintf(&b, `{"f12":"%s","f14":"测试股"}`, code)
		}
		b.WriteString(`]}}`)
		_, _ = w.Write([]byte(b.String()))
	}))
	defer srv.Close()

	orig := listHosts
	listHosts = []string{srv.URL}
	t.Cleanup(func() { listHosts = orig })

	e := NewEastMoney()
	e.pageJitterFn = func() time.Duration { return 0 }
	stocks, err := e.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page < 3 {
		t.Fatalf("expected at least 3 pages, got %d", page)
	}
	if len(stocks) != total {
		t.Fatalf("want %d tradable stocks, got %d", total, len(stocks))
	}
}
