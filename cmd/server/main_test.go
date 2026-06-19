package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

func TestToQuotesUsesKlineFields(t *testing.T) {
	resp := protocol.QuotesResp{
		{
			Exchange:   protocol.ExchangeSH,
			Code:       "600000",
			Active1:    11,
			Intuition:  22,
			InsideDish: 33,
			OuterDisc:  44,
			Rate:       1.23,
			Active2:    55,
			Kline: &protocol.Kline{
				Last:   protocol.Yuan(9.9),
				Open:   protocol.Yuan(10.1),
				High:   protocol.Yuan(10.8),
				Low:    protocol.Yuan(9.8),
				Close:  protocol.Yuan(10.5),
				Volume: 123456,
				Amount: protocol.Yuan(654321.12),
			},
			BuyLevel: protocol.PriceLevels{
				{Buy: true, Price: protocol.Yuan(10.49), Number: 100},
			},
			SellLevel: protocol.PriceLevels{
				{Price: protocol.Yuan(10.51), Number: 200},
			},
		},
		nil,
		{Code: "nil-kline"},
	}

	got := toQuotes(resp)
	if len(got) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(got))
	}
	q := got[0]
	if q["totalHand"] != int64(123456) {
		t.Fatalf("totalHand should come from Kline.Volume, got %#v", q["totalHand"])
	}
	if q["amount"] != 654321.12 {
		t.Fatalf("amount should come from Kline.Amount.Float64, got %#v", q["amount"])
	}
	k := q["k"].(map[string]float64)
	if k["close"] != 10.5 || k["last"] != 9.9 {
		t.Fatalf("unexpected kline DTO: %#v", k)
	}
	buy := q["buyLevel"].([]map[string]any)
	if buy[0]["buy"] != true || buy[0]["price"] != 10.49 {
		t.Fatalf("unexpected buy level: %#v", buy[0])
	}
}

func TestExtendedRoutesAreRegistered(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, nil)

	cases := []string{
		"/api/finance",
		"/api/company/categories",
		"/api/company/content",
		"/api/block/data",
		"/api/report/file",
		"/api/tdxzs",
		"/api/stat",
		"/api/xgsg",
		"/api/gbbq/factors",
		"/api/fq/qfq/day",
		"/api/ex/markets",
		"/api/ex/bars/range",
	}

	for _, path := range cases {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s not registered or method check bypassed, status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestExtendedParsers(t *testing.T) {
	file, err := parseBlockFile("concept")
	if err != nil || file != protocol.BlockFileGN {
		t.Fatalf("concept block file mismatch file=%q err=%v", file, err)
	}
	file, err = parseBlockFile("block_fg.dat")
	if err != nil || file != "block_fg.dat" {
		t.Fatalf("explicit block file mismatch file=%q err=%v", file, err)
	}
	if _, err = parseBlockFile("../secret"); err == nil {
		t.Fatal("expected invalid block file error")
	}

	q := url.Values{"market": {"47"}, "code": {"HSI"}}
	market, code, err := parseExMarketCode(q)
	if err != nil {
		t.Fatal(err)
	}
	if market != 47 || code != "HSI" {
		t.Fatalf("unexpected ex market/code: %d %s", market, code)
	}

	category, err := parseExKlineCategory("", "day")
	if err != nil || category != protocol.TypeKlineDay {
		t.Fatalf("unexpected category=%d err=%v", category, err)
	}
	category, err = parseExKlineCategory("9", "")
	if err != nil || category != 9 {
		t.Fatalf("explicit category failed category=%d err=%v", category, err)
	}

	ex, rawCode, err := parseExchangeCode(url.Values{"code": {"sh600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if ex != protocol.ExchangeSH || rawCode != "600519" {
		t.Fatalf("unexpected inferred exchange/code: %s %s", ex, rawCode)
	}

	ex, rawCode, err = parseExchangeCode(url.Values{"exchange": {"sz"}, "code": {"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	if ex != protocol.ExchangeSZ || rawCode != "000001" {
		t.Fatalf("unexpected explicit exchange/code: %s %s", ex, rawCode)
	}
}

func TestRawFileAndFactorDTOs(t *testing.T) {
	raw := []byte("hello")
	got := rawFileResp("x.dat", raw)
	if got["file"] != "x.dat" || got["size"] != len(raw) {
		t.Fatalf("unexpected raw response: %#v", got)
	}
	if got["base64"] != base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("unexpected base64: %#v", got["base64"])
	}

	tm := time.Date(2026, 6, 19, 15, 0, 0, 0, time.Local)
	factors := toFactors([]*protocol.Factor{{
		Time:    tm,
		Last:    protocol.Yuan(10),
		PreLast: protocol.Yuan(9.5),
		QFQMul:  0.95,
		QFQAdd:  -0.1,
		HFQMul:  1.05,
		HFQAdd:  0.2,
		QFQ:     0.95,
		HFQ:     1.05,
	}})
	if len(factors) != 1 {
		t.Fatalf("expected one factor, got %d", len(factors))
	}
	if factors[0]["last"] != 10.0 || factors[0]["preLast"] != 9.5 {
		t.Fatalf("price fields not converted: %#v", factors[0])
	}
	if factors[0]["time"] != "2026-06-19 15:00:00" {
		t.Fatalf("unexpected time: %#v", factors[0]["time"])
	}
}

func TestReqErrStatus(t *testing.T) {
	var re reqErr
	if !errors.As(badReq("bad"), &re) {
		t.Fatal("badReq should return reqErr")
	}
}

func TestDoPoolRedialsSynchronouslyAfterConnectionError(t *testing.T) {
	var dials atomic.Int32
	pool, err := newReliablePool(func() (*tdx.Client, error) {
		dials.Add(1)
		return &tdx.Client{}, nil
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.getTimeout = 100 * time.Millisecond
	pool.requestRetries = 3
	pool.redialRetries = 1

	var calls atomic.Int32
	got, err := doPool(pool, func(*tdx.Client) (any, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("EOF")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 handler calls, got %d", calls.Load())
	}
	if dials.Load() != 2 {
		t.Fatalf("expected initial dial plus sync redial, got %d", dials.Load())
	}
}

func TestDoPoolKeepsConnectionOnRequestError(t *testing.T) {
	var dials atomic.Int32
	pool, err := newReliablePool(func() (*tdx.Client, error) {
		dials.Add(1)
		return &tdx.Client{}, nil
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.getTimeout = 100 * time.Millisecond

	_, err = doPool(pool, func(*tdx.Client) (any, error) {
		return nil, badReq("missing code")
	})
	if err == nil {
		t.Fatal("expected bad request error")
	}
	var re reqErr
	if !errors.As(err, &re) {
		t.Fatalf("expected reqErr, got %T", err)
	}

	got, err := doPool(pool, func(*tdx.Client) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if dials.Load() != 1 {
		t.Fatalf("bad request should not redial, got %d dials", dials.Load())
	}
}

func TestReliablePoolGetTimesOut(t *testing.T) {
	pool := &reliablePool{
		ch:             make(chan *tdx.Client),
		dial:           func() (*tdx.Client, error) { return &tdx.Client{}, nil },
		getTimeout:     10 * time.Millisecond,
		requestRetries: 1,
		redialRetries:  1,
		closed:         make(chan struct{}),
	}
	start := time.Now()
	_, err := pool.Get()
	if err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("pool get timeout took too long: %s", elapsed)
	}
}

func TestRotateHostsSpreadsDialStart(t *testing.T) {
	old := dialHostCursor.Load()
	dialHostCursor.Store(0)
	t.Cleanup(func() { dialHostCursor.Store(old) })

	hosts := []string{"a", "b", "c"}
	first := rotateHosts(hosts)
	second := rotateHosts(hosts)
	third := rotateHosts(hosts)
	if fmt.Sprint(first) != "[a b c]" || fmt.Sprint(second) != "[b c a]" || fmt.Sprint(third) != "[c a b]" {
		t.Fatalf("unexpected rotations: %v %v %v", first, second, third)
	}
}

func TestDoExRetriesShortConnection(t *testing.T) {
	oldDialEx := dialExClient
	t.Cleanup(func() { dialExClient = oldDialEx })

	var dials atomic.Int32
	dialExClient = func() (*tdx.Client, error) {
		if dials.Add(1) == 1 {
			return nil, errors.New("connection refused")
		}
		return &tdx.Client{}, nil
	}

	got, err := doEx(func(*tdx.Client) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if dials.Load() != 2 {
		t.Fatalf("expected 2 dials, got %d", dials.Load())
	}
}
