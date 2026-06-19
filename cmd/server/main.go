package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
)

var klineTypeMap = map[string]uint8{
	"1min":    protocol.TypeKlineMinute,
	"5min":    protocol.TypeKline5Minute,
	"15min":   protocol.TypeKline15Minute,
	"30min":   protocol.TypeKline30Minute,
	"60min":   protocol.TypeKline60Minute,
	"day":     protocol.TypeKlineDay,
	"week":    protocol.TypeKlineWeek,
	"month":   protocol.TypeKlineMonth,
	"quarter": protocol.TypeKlineQuarter,
	"year":    protocol.TypeKlineYear,
}

var serverDataDir = "./data"

func main() {
	addr := flag.String("addr", ":8001", "HTTP listen address")
	poolSize := flag.Int("pool", 16, "TDX connection pool size")
	dataDir := flag.String("data", "./data", "data directory for SQLite databases")
	flag.Parse()
	serverDataDir = *dataDir

	pool, err := newReliablePool(dialClient, *poolSize)
	if err != nil {
		log.Fatalf("init pool: %v", err)
	}

	if err := initCodesCache(*dataDir); err != nil {
		log.Fatalf("init codes: %v", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, pool)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("TDX HTTP server listening on %s (pool=%d)", *addr, *poolSize)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
	pool.Close()
}

func initCodesCache(dataDir string) error {
	if tdx.DefaultCodes != nil {
		return nil
	}
	dbDir := filepath.Join(dataDir, "database")
	codes, err := tdx.NewCodes(
		tdx.WithCodesDialClient(dialClient),
		tdx.WithCodesDialDB(func() (*xorms.Engine, error) {
			return xorms.NewSqlite(filepath.Join(dbDir, "codes.db"))
		}),
	)
	if err != nil {
		return err
	}
	tdx.DefaultCodes = codes
	return nil
}

// --- routing ---

func registerRoutes(mux *http.ServeMux, pool *reliablePool) {
	reg := func(pattern string, fn func(*reliablePool, url.Values) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			data, err := fn(pool, r.URL.Query())
			if err != nil {
				var re reqErr
				if errors.As(err, &re) {
					writeErr(w, re.status, re.msg)
				} else {
					log.Printf("handler error: %v", err)
					writeErr(w, http.StatusInternalServerError, "internal error")
				}
				return
			}
			writeOK(w, data)
		})
	}

	reg("/api/ping", hPing)
	reg("/api/count", hCount)
	reg("/api/codes", hCodes)
	reg("/api/codes/all", hCodesAll)
	reg("/api/codes/stocks", hStockCodes)
	reg("/api/codes/etfs", hETFCodes)
	reg("/api/codes/indexes", hIndexCodes)
	reg("/api/quote", hQuote)
	reg("/api/minute", hMinute)
	reg("/api/minute/history", hMinuteHistory)
	reg("/api/trade", hTrade)
	reg("/api/trade/all", hTradeAll)
	reg("/api/trade/history", hTradeHistory)
	reg("/api/trade/history/day", hTradeHistoryDay)
	reg("/api/kline", hKline)
	reg("/api/kline/all", hKlineAll)
	reg("/api/index/kline", hIndexKline)
	reg("/api/index/kline/all", hIndexKlineAll)
	reg("/api/auction", hAuction)
	reg("/api/gbbq", hGbbq)
	registerExtendedRoutes(reg)
}

// --- handlers ---

func hPing(_ *reliablePool, _ url.Values) (any, error) {
	return "pong", nil
}

func hCount(pool *reliablePool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCount(ex)
		if err != nil {
			return nil, err
		}
		return map[string]uint16{"count": resp.Count}, nil
	})
}

func hCodes(pool *reliablePool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCode(ex, start)
		if err != nil {
			return nil, err
		}
		return toCodeResp(resp), nil
	})
}

func hCodesAll(pool *reliablePool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	if cached, ok := cachedCodesAll(ex); ok {
		return cached, nil
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCodeAll(ex)
		if err != nil {
			return nil, err
		}
		return toCodeResp(resp), nil
	})
}

func hStockCodes(pool *reliablePool, _ url.Values) (any, error) {
	if cached, ok := cachedCodeList(func(c tdx.ICodes) []string { return c.GetStockCodes() }); ok {
		return toList(cached), nil
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetStockCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func hETFCodes(pool *reliablePool, _ url.Values) (any, error) {
	if cached, ok := cachedCodeList(func(c tdx.ICodes) []string { return c.GetETFCodes() }); ok {
		return toList(cached), nil
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetETFCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func hIndexCodes(pool *reliablePool, _ url.Values) (any, error) {
	if cached, ok := cachedIndexCodes(); ok {
		return toList(cached), nil
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetIndexCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func cachedCodesAll(ex protocol.Exchange) (any, bool) {
	codes := tdx.DefaultCodes
	if codes == nil {
		return nil, false
	}
	prefix := ex.String()
	list := make([]map[string]any, 0)
	for fullCode, model := range codes.Iter() {
		if model == nil {
			continue
		}
		if model.Exchange != prefix && !strings.HasPrefix(fullCode, prefix) {
			continue
		}
		list = append(list, map[string]any{
			"name":      model.Name,
			"code":      model.Code,
			"multiple":  model.Multiple,
			"decimal":   model.Decimal,
			"lastPrice": model.LastPrice,
		})
	}
	if len(list) == 0 {
		return nil, false
	}
	return map[string]any{"count": len(list), "list": list}, true
}

func cachedCodeList(load func(tdx.ICodes) []string) ([]string, bool) {
	codes := tdx.DefaultCodes
	if codes == nil {
		return nil, false
	}
	list := load(codes)
	if len(list) == 0 {
		return nil, false
	}
	return append([]string(nil), list...), true
}

func cachedIndexCodes() ([]string, bool) {
	list, ok := cachedCodeList(func(c tdx.ICodes) []string { return c.GetIndexCodes() })
	if !ok {
		return nil, false
	}
	const bjIndex = "bj899050"
	for _, code := range list {
		if code == bjIndex {
			return list, true
		}
	}
	return append([]string{bjIndex}, list...), true
}

func hQuote(pool *reliablePool, q url.Values) (any, error) {
	codes, err := parseCodes(q.Get("codes"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetQuote(codes...)
		if err != nil {
			return nil, err
		}
		return toQuotes(resp), nil
	})
}

func hMinute(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinute(code)
		if err != nil {
			return nil, err
		}
		return toMinuteResp(resp, time.Now()), nil
	})
}

func hMinuteHistory(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, date, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinute(dateRaw, code)
		if err != nil {
			return nil, err
		}
		return toMinuteResp(resp, date), nil
	})
}

func hTrade(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 1800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinuteTrade(code, start, count)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeAll(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinuteTradeAll(code)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeHistory(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, _, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 2000)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinuteTrade(dateRaw, code, start, count)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeHistoryDay(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, _, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinuteTradeDay(dateRaw, code)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hKline(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKline(kt, code, start, count)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hKlineAll(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKlineAll(kt, code)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hIndexKline(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetIndex(kt, code, start, count)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hIndexKlineAll(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetIndexAll(kt, code)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hAuction(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCallAuction(code)
		if err != nil {
			return nil, err
		}
		return toAuctionResp(resp), nil
	})
}

func hGbbq(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetGbbq(code)
		if err != nil {
			return nil, err
		}
		return toGbbqResp(resp), nil
	})
}

// --- pool helper ---

// dialClient creates a fresh TDX connection (same options as pool init).
var (
	dialHostCursor atomic.Uint64
	dialClient     = func() (*tdx.Client, error) {
		return tdx.DialHostsRange(rotateHosts(tdx.Hosts), tdx.WithRedial(), tdx.WithLevel(tdx.LevelError))
	}
)

const (
	defaultPoolGetTimeout = 5 * time.Second
	defaultRequestRetries = 3
	defaultRedialRetries  = 2
	defaultRedialBackoff  = 200 * time.Millisecond
)

type reliablePool struct {
	ch             chan *tdx.Client
	dial           func() (*tdx.Client, error)
	getTimeout     time.Duration
	requestRetries int
	redialRetries  int
	redialBackoff  time.Duration
	closed         chan struct{}
	closeOnce      sync.Once
}

func newReliablePool(dial func() (*tdx.Client, error), number int) (*reliablePool, error) {
	if number <= 0 {
		number = 1
	}
	pool := &reliablePool{
		ch:             make(chan *tdx.Client, number),
		dial:           dial,
		getTimeout:     defaultPoolGetTimeout,
		requestRetries: defaultRequestRetries,
		redialRetries:  defaultRedialRetries,
		redialBackoff:  defaultRedialBackoff,
		closed:         make(chan struct{}),
	}
	for i := 0; i < number; i++ {
		c, err := pool.dialWithRetries()
		if err != nil {
			pool.Close()
			return nil, err
		}
		pool.ch <- c
	}
	return pool, nil
}

func rotateHosts(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	start := int(dialHostCursor.Add(1)-1) % len(hosts)
	out := make([]string, 0, len(hosts))
	out = append(out, hosts[start:]...)
	out = append(out, hosts[:start]...)
	return out
}

func (p *reliablePool) Get() (*tdx.Client, error) {
	if p == nil {
		return nil, errors.New("tdx pool not initialized")
	}
	timer := time.NewTimer(p.getTimeout)
	defer timer.Stop()
	select {
	case <-p.closed:
		return nil, errors.New("tdx pool closed")
	case c := <-p.ch:
		if c == nil {
			return nil, errors.New("tdx pool returned nil connection")
		}
		return c, nil
	case <-timer.C:
		return nil, fmt.Errorf("tdx pool get timeout after %s", p.getTimeout)
	}
}

func (p *reliablePool) Put(c *tdx.Client) {
	if c == nil {
		return
	}
	select {
	case <-p.closed:
		closeClient(c)
		return
	default:
	}
	select {
	case <-p.closed:
		closeClient(c)
	case p.ch <- c:
	default:
		closeClient(c)
	}
}

func (p *reliablePool) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		close(p.closed)
		for {
			select {
			case c := <-p.ch:
				closeClient(c)
			default:
				return
			}
		}
	})
}

func (p *reliablePool) dialWithRetries() (*tdx.Client, error) {
	var lastErr error
	retries := p.redialRetries
	if retries <= 0 {
		retries = 1
	}
	for i := 0; i < retries; i++ {
		c, err := p.dial()
		if err == nil {
			return c, nil
		}
		lastErr = err
		if i < retries-1 && p.redialBackoff > 0 {
			time.Sleep(time.Duration(i+1) * p.redialBackoff)
		}
	}
	return nil, lastErr
}

func (p *reliablePool) refillAsync(reason error) {
	if p == nil {
		return
	}
	go func() {
		c, err := p.dialWithRetries()
		if err != nil {
			log.Printf("pool refill failed after request error %v: %v", reason, err)
			return
		}
		p.Put(c)
	}()
}

func closeClient(c *tdx.Client) {
	if c != nil && c.Client != nil {
		c.Close()
	}
}

// doPool runs a request on a pooled TDX connection.
// Network/protocol errors poison the current connection, so they are retried on
// freshly dialed connections instead of returning broken clients to the pool.
func doPool(pool *reliablePool, fn func(*tdx.Client) (any, error)) (any, error) {
	c, err := pool.Get()
	if err != nil {
		return nil, err
	}
	retries := pool.requestRetries
	if retries <= 0 {
		retries = 1
	}
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		result, err := fn(c)
		if err == nil {
			pool.Put(c)
			return result, nil
		}
		var re reqErr
		if errors.As(err, &re) {
			pool.Put(c)
			return nil, err
		}
		lastErr = err
		closeClient(c)
		c = nil
		if attempt == retries {
			break
		}
		c, err = pool.dialWithRetries()
		if err != nil {
			log.Printf("pool sync redial failed after request error %v: %v", lastErr, err)
			pool.refillAsync(lastErr)
			c, err = pool.Get()
			if err != nil {
				return nil, lastErr
			}
		}
	}
	pool.refillAsync(lastErr)
	return nil, lastErr
}

// --- param parsing ---

type reqErr struct {
	status int
	msg    string
}

func (e reqErr) Error() string { return e.msg }

func badReq(format string, args ...any) error {
	return reqErr{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func parseExchange(v string) (protocol.Exchange, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "sh":
		return protocol.ExchangeSH, nil
	case "sz":
		return protocol.ExchangeSZ, nil
	case "bj":
		return protocol.ExchangeBJ, nil
	case "":
		return 0, badReq("missing exchange")
	default:
		return 0, badReq("invalid exchange %q", v)
	}
}

func parseCode(v string) (string, error) {
	c := strings.TrimSpace(v)
	if c == "" {
		return "", badReq("missing code")
	}
	return c, nil
}

func parseCodes(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, badReq("missing codes")
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		c := strings.TrimSpace(p)
		if c != "" {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, badReq("missing codes")
	}
	return out, nil
}

func parseDate(v string) (string, time.Time, error) {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return "", time.Time{}, badReq("missing date")
	}
	t, err := time.Parse("20060102", raw)
	if err != nil {
		return "", time.Time{}, badReq("invalid date %q", raw)
	}
	return raw, t, nil
}

func parseKlineType(v string) (uint8, error) {
	k := strings.ToLower(strings.TrimSpace(v))
	if k == "" {
		return 0, badReq("missing type")
	}
	typ, ok := klineTypeMap[k]
	if !ok {
		return 0, badReq("invalid type %q", v)
	}
	return typ, nil
}

func pUint16(q url.Values, key string, def uint16) (uint16, error) {
	raw := strings.TrimSpace(q.Get(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, badReq("invalid %s %q", key, raw)
	}
	return uint16(n), nil
}

// --- JSON response ---

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": 1, "msg": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":1,"msg":"internal error"}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("json write: %v", err)
	}
}

// --- DTO converters ---

const timeFmt = "2006-01-02 15:04:05"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeFmt)
}

func toList(ss []string) map[string]any {
	if ss == nil {
		ss = []string{}
	}
	return map[string]any{"list": ss}
}

func toCodeResp(r *protocol.CodeResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, c := range r.List {
		if c == nil {
			continue
		}
		list = append(list, map[string]any{
			"name":      c.Name,
			"code":      c.Code,
			"multiple":  c.Multiple,
			"decimal":   c.Decimal,
			"lastPrice": c.LastPrice,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toQuotes(resp protocol.QuotesResp) []map[string]any {
	out := make([]map[string]any, 0, len(resp))
	for _, q := range resp {
		if q == nil || q.Kline == nil {
			continue
		}
		k := q.Kline
		out = append(out, map[string]any{
			"exchange":   q.Exchange.String(),
			"code":       q.Code,
			"active1":    q.Active1,
			"totalHand":  k.Volume,
			"intuition":  q.Intuition,
			"amount":     k.Amount.Float64(),
			"insideDish": q.InsideDish,
			"outerDisc":  q.OuterDisc,
			"rate":       q.Rate,
			"active2":    q.Active2,
			"k": map[string]float64{
				"last":  k.Last.Float64(),
				"open":  k.Open.Float64(),
				"high":  k.High.Float64(),
				"low":   k.Low.Float64(),
				"close": k.Close.Float64(),
			},
			"buyLevel":  toPriceLevels(q.BuyLevel),
			"sellLevel": toPriceLevels(q.SellLevel),
		})
	}
	return out
}

func toPriceLevels(ls protocol.PriceLevels) []map[string]any {
	out := make([]map[string]any, len(ls))
	for i, l := range ls {
		out[i] = map[string]any{
			"buy":    l.Buy,
			"price":  l.Price.Float64(),
			"number": l.Number,
		}
	}
	return out
}

func toMinuteResp(r *protocol.MinuteResp, date time.Time) map[string]any {
	day := date.Format("2006-01-02")
	list := make([]map[string]any, 0, len(r.List))
	for _, p := range r.List {
		list = append(list, map[string]any{
			"time":   day + " " + p.Time + ":00",
			"price":  p.Price.Float64(),
			"number": p.Number,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toTradeResp(r *protocol.TradeResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, t := range r.List {
		if t == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":   fmtTime(t.Time),
			"price":  t.Price.Float64(),
			"volume": t.Volume,
			"amount": t.Amount().Float64(),
			"status": t.Status,
			"number": t.Number,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toKlineResp(r *protocol.KlineResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, k := range r.List {
		if k == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":      fmtTime(k.Time),
			"last":      k.Last.Float64(),
			"open":      k.Open.Float64(),
			"high":      k.High.Float64(),
			"low":       k.Low.Float64(),
			"close":     k.Close.Float64(),
			"order":     k.Order,
			"volume":    k.Volume,
			"amount":    k.Amount.Float64(),
			"upCount":   k.UpCount,
			"downCount": k.DownCount,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toAuctionResp(r *protocol.CallAuctionResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, a := range r.List {
		if a == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":      fmtTime(a.Time),
			"price":     a.Price.Float64(),
			"match":     a.Match,
			"unmatched": a.Unmatched,
			"flag":      a.Flag,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toGbbqResp(r *protocol.GbbqResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, g := range r.List {
		if g == nil {
			continue
		}
		list = append(list, map[string]any{
			"code":     g.Code,
			"time":     fmtTime(g.Time),
			"category": g.Category,
			"c1":       g.C1,
			"c2":       g.C2,
			"c3":       g.C3,
			"c4":       g.C4,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}
