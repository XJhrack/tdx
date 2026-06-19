package main

import (
	"encoding/base64"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
)

func registerExtendedRoutes(reg func(string, func(*reliablePool, url.Values) (any, error))) {
	reg("/api/finance", hFinance)
	reg("/api/company/categories", hCompanyCategories)
	reg("/api/company/content", hCompanyContent)

	reg("/api/block/file", hBlockFile)
	reg("/api/block/data", hBlockData)
	reg("/api/block/data-with-index", hBlockDataWithIndex)
	reg("/api/report/file", hReportFile)
	reg("/api/report/zhb", hZHBFiles)
	reg("/api/tdxzs", hTdxZs)
	reg("/api/tdxbk", hTdxBk)
	reg("/api/tdxhy", hTdxHy)
	reg("/api/stat", hTdxStat)
	reg("/api/stat2", hTdxStat2)
	reg("/api/stat2/block-index", hStockBlockIndex)
	reg("/api/xgsg", hXgsg)

	reg("/api/gbbq/all", hGbbqAll)
	reg("/api/gbbq/equity", hGbbqEquity)
	reg("/api/gbbq/xrxd", hGbbqXRXD)
	reg("/api/gbbq/factors", hGbbqFactors)
	reg("/api/gbbq/turnover", hGbbqTurnover)
	reg("/api/fq/qfq/day", hQFQKlineDay)
	reg("/api/fq/hfq/day", hHFQKlineDay)

	reg("/api/ex/markets", hExMarkets)
	reg("/api/ex/count", hExCount)
	reg("/api/ex/instruments", hExInstruments)
	reg("/api/ex/quote", hExQuote)
	reg("/api/ex/quote-list", hExQuoteList)
	reg("/api/ex/bars", hExBars)
	reg("/api/ex/minute", hExMinute)
	reg("/api/ex/minute/history", hExHistMinute)
	reg("/api/ex/trade", hExTrade)
	reg("/api/ex/trade/history", hExHistTrade)
	reg("/api/ex/bars/range", hExBarsRange)
}

func hFinance(pool *reliablePool, q url.Values) (any, error) {
	ex, code, err := parseExchangeCode(q)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		return c.GetFinanceInfo(ex, code)
	})
}

func hCompanyCategories(pool *reliablePool, q url.Values) (any, error) {
	ex, code, err := parseExchangeCode(q)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		return c.GetCompanyCategory(ex, code)
	})
}

func hCompanyContent(pool *reliablePool, q url.Values) (any, error) {
	ex, code, err := parseExchangeCode(q)
	if err != nil {
		return nil, err
	}
	filename := strings.TrimSpace(q.Get("filename"))
	if filename == "" {
		return nil, badReq("missing filename")
	}
	start, err := pUint32(q, "start", 0)
	if err != nil {
		return nil, err
	}
	length, err := pUint32(q, "length", 0)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, badReq("missing length")
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		content, err := c.GetCompanyContent(ex, code, filename, start, length)
		if err != nil {
			return nil, err
		}
		return map[string]any{"content": content}, nil
	})
}

func hBlockFile(pool *reliablePool, q url.Values) (any, error) {
	file, err := parseFile(q.Get("file"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		raw, err := c.GetBlockFileRaw(file)
		if err != nil {
			return nil, err
		}
		return rawFileResp(file, raw), nil
	})
}

func hBlockData(pool *reliablePool, q url.Values) (any, error) {
	file, err := parseBlockFile(q.Get("file"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		blocks, err := c.GetBlockData(file)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(blocks), "list": blocks}, nil
	})
}

func hBlockDataWithIndex(pool *reliablePool, q url.Values) (any, error) {
	file, err := parseBlockFile(q.Get("file"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		blocks, err := c.GetBlockDataWithIndex(file)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(blocks), "list": blocks}, nil
	})
}

func hReportFile(pool *reliablePool, q url.Values) (any, error) {
	file, err := parseFile(q.Get("file"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		raw, err := c.GetReportFile(file)
		if err != nil {
			return nil, err
		}
		return rawFileResp(file, raw), nil
	})
}

func hZHBFiles(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		files, err := c.GetZHBFiles()
		if err != nil {
			return nil, err
		}
		out := make(map[string]map[string]any, len(files))
		for name, raw := range files {
			out[name] = rawFileResp(name, raw)
		}
		return map[string]any{"count": len(out), "files": out}, nil
	})
}

func hTdxZs(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxZs()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hTdxBk(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxBk()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hTdxHy(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxHy()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hTdxStat(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxStat()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hTdxStat2(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxStat2()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hStockBlockIndex(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		stats, err := c.GetTdxStat2()
		if err != nil {
			return nil, err
		}
		index := protocol.StockBlockIndex(stats)
		return map[string]any{"count": len(index), "items": index}, nil
	})
}

func hXgsg(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetXgsg()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hGbbqAll(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetGbbqAll()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(resp), "items": resp}, nil
	})
}

func hGbbqEquity(_ *reliablePool, q url.Values) (any, error) {
	code, date, err := parseCodeDate(q)
	if err != nil {
		return nil, err
	}
	gb, err := getGbbq()
	if err != nil {
		return nil, err
	}
	return gb.GetEquity(code, date), nil
}

func hGbbqXRXD(_ *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	gb, err := getGbbq()
	if err != nil {
		return nil, err
	}
	list := gb.GetXRXDs(code)
	return map[string]any{"count": len(list), "list": toXRXDs(list)}, nil
}

func hGbbqFactors(pool *reliablePool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	gb, err := getGbbq()
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKlineDayAll(code)
		if err != nil {
			return nil, err
		}
		fs := gb.GetFactors(code, resp.List)
		return map[string]any{"count": len(fs), "list": toFactors(fs)}, nil
	})
}

func hGbbqTurnover(_ *reliablePool, q url.Values) (any, error) {
	code, date, err := parseCodeDate(q)
	if err != nil {
		return nil, err
	}
	volume, err := pInt64(q, "volume", 0)
	if err != nil {
		return nil, err
	}
	if volume == 0 {
		return nil, badReq("missing volume")
	}
	gb, err := getGbbq()
	if err != nil {
		return nil, err
	}
	return map[string]any{"turnover": gb.GetTurnover(code, date, volume)}, nil
}

func hQFQKlineDay(pool *reliablePool, q url.Values) (any, error) {
	return hFQKlineDay(pool, q, true)
}

func hHFQKlineDay(pool *reliablePool, q url.Values) (any, error) {
	return hFQKlineDay(pool, q, false)
}

func hFQKlineDay(pool *reliablePool, q url.Values, qfq bool) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	gb, err := getGbbq()
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKlineDayAll(code)
		if err != nil {
			return nil, err
		}
		var ks protocol.Klines
		if qfq {
			ks = gb.QFQ(code, resp.List)
		} else {
			ks = gb.HFQ(code, resp.List)
		}
		return toKlineResp(&protocol.KlineResp{Count: uint16(len(ks)), List: ks}), nil
	})
}

func hExMarkets(_ *reliablePool, _ url.Values) (any, error) {
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExMarkets()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExCount(_ *reliablePool, _ url.Values) (any, error) {
	return doEx(func(c *tdx.Client) (any, error) {
		n, err := c.ExCount()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": n}, nil
	})
}

func hExInstruments(_ *reliablePool, q url.Values) (any, error) {
	start, err := pUint32(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 100)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExInstruments(start, count)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExQuote(_ *reliablePool, q url.Values) (any, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		return c.ExQuote(market, code)
	})
}

func hExQuoteList(_ *reliablePool, q url.Values) (any, error) {
	market, err := pUint8Required(q, "market")
	if err != nil {
		return nil, err
	}
	category, err := pUint8Required(q, "category")
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 100)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExQuoteList(market, category, start, count)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExBars(_ *reliablePool, q url.Values) (any, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return nil, err
	}
	category, err := parseExKlineCategory(q.Get("category"), q.Get("type"))
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
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExBars(category, market, code, start, count)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExMinute(_ *reliablePool, q url.Values) (any, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExMinute(market, code)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExHistMinute(_ *reliablePool, q url.Values) (any, error) {
	market, code, date, err := parseExMarketCodeDate(q)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExHistMinute(market, code, date)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExTrade(_ *reliablePool, q url.Values) (any, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 100)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExTrade(market, code, start, count)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExHistTrade(_ *reliablePool, q url.Values) (any, error) {
	market, code, date, err := parseExMarketCodeDate(q)
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 100)
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExHistTrade(market, code, date, start, count)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

func hExBarsRange(_ *reliablePool, q url.Values) (any, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return nil, err
	}
	date, err := pDateUint32(q, "date")
	if err != nil {
		return nil, err
	}
	date2, err := pDateUint32(q, "date2")
	if err != nil {
		return nil, err
	}
	return doEx(func(c *tdx.Client) (any, error) {
		list, err := c.ExBarsRange(market, code, date, date2)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": list}, nil
	})
}

var dialExClient = func() (*tdx.Client, error) {
	return tdx.DialExHqHosts(rotateHosts(tdx.ExHosts), tdx.WithRedial(), tdx.WithLevel(tdx.LevelError))
}

func doEx(fn func(*tdx.Client) (any, error)) (any, error) {
	var lastErr error
	for attempt := 1; attempt <= defaultRequestRetries; attempt++ {
		c, err := dialExClient()
		if err != nil {
			lastErr = err
			if attempt < defaultRequestRetries {
				time.Sleep(time.Duration(attempt) * defaultRedialBackoff)
			}
			continue
		}
		result, err := fn(c)
		closeClient(c)
		if err == nil {
			return result, nil
		}
		var re reqErr
		if errors.As(err, &re) {
			return nil, err
		}
		lastErr = err
		if attempt < defaultRequestRetries {
			time.Sleep(time.Duration(attempt) * defaultRedialBackoff)
		}
	}
	return nil, lastErr
}

var (
	gbbqMu    sync.Mutex
	gbbqCache *tdx.Gbbq
)

func getGbbq() (*tdx.Gbbq, error) {
	gbbqMu.Lock()
	defer gbbqMu.Unlock()
	if gbbqCache != nil {
		return gbbqCache, nil
	}
	gb, err := tdx.NewGbbq(
		tdx.WithGbbqDialClient(dialClient),
		tdx.WithGbbqDialDB(func() (*xorms.Engine, error) {
			return xorms.NewSqlite(filepath.Join(serverDataDir, "database", "gbbq.db"))
		}),
	)
	if err != nil {
		return nil, err
	}
	gbbqCache = gb
	return gbbqCache, nil
}

func parseExchangeCode(q url.Values) (protocol.Exchange, string, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return 0, "", err
	}
	if strings.TrimSpace(q.Get("exchange")) == "" {
		ex, rawCode, err := protocol.DecodeCode(code)
		if err != nil {
			return 0, "", badReq(err.Error())
		}
		return ex, rawCode, nil
	}
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return 0, "", err
	}
	return ex, stripPrefix(code), nil
}

func parseCodeDate(q url.Values) (string, time.Time, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return "", time.Time{}, err
	}
	_, date, err := parseDate(q.Get("date"))
	if err != nil {
		return "", time.Time{}, err
	}
	return code, date, nil
}

func parseExMarketCode(q url.Values) (uint8, string, error) {
	market, err := pUint8Required(q, "market")
	if err != nil {
		return 0, "", err
	}
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return 0, "", err
	}
	return market, code, nil
}

func parseExMarketCodeDate(q url.Values) (uint8, string, uint32, error) {
	market, code, err := parseExMarketCode(q)
	if err != nil {
		return 0, "", 0, err
	}
	date, err := pDateUint32(q, "date")
	if err != nil {
		return 0, "", 0, err
	}
	return market, code, date, nil
}

func stripPrefix(code string) string {
	c := strings.TrimSpace(code)
	if len(c) == 8 {
		p := strings.ToLower(c[:2])
		if p == "sh" || p == "sz" || p == "bj" {
			return c[2:]
		}
	}
	return c
}

func parseFile(v string) (string, error) {
	file := strings.TrimSpace(v)
	if file == "" {
		return "", badReq("missing file")
	}
	if strings.Contains(file, "/") || strings.Contains(file, "\\") || strings.Contains(file, "..") {
		return "", badReq("invalid file %q", file)
	}
	return file, nil
}

func parseBlockFile(v string) (string, error) {
	file := strings.ToLower(strings.TrimSpace(v))
	if file == "" {
		return protocol.BlockFileGN, nil
	}
	if strings.HasSuffix(file, ".dat") {
		return parseFile(file)
	}
	switch file {
	case "gn", "concept":
		return protocol.BlockFileGN, nil
	case "fg", "style", "region":
		return protocol.BlockFileFG, nil
	case "zs", "index":
		return protocol.BlockFileZS, nil
	case "hy", "industry":
		return protocol.BlockFileHY, nil
	case "block":
		return protocol.BlockFile, nil
	default:
		return "", badReq("invalid block file %q", v)
	}
}

func parseExKlineCategory(categoryRaw, typeRaw string) (uint8, error) {
	if strings.TrimSpace(categoryRaw) != "" {
		return pUint8Value("category", categoryRaw)
	}
	return parseKlineType(typeRaw)
}

func pUint8Required(q url.Values, key string) (uint8, error) {
	raw := strings.TrimSpace(q.Get(key))
	if raw == "" {
		return 0, badReq("missing %s", key)
	}
	return pUint8Value(key, raw)
}

func pUint8Value(key, raw string) (uint8, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 8)
	if err != nil {
		return 0, badReq("invalid %s %q", key, raw)
	}
	return uint8(n), nil
}

func pUint32(q url.Values, key string, def uint32) (uint32, error) {
	raw := strings.TrimSpace(q.Get(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, badReq("invalid %s %q", key, raw)
	}
	return uint32(n), nil
}

func pInt64(q url.Values, key string, def int64) (int64, error) {
	raw := strings.TrimSpace(q.Get(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, badReq("invalid %s %q", key, raw)
	}
	return n, nil
}

func pDateUint32(q url.Values, key string) (uint32, error) {
	raw, _, err := parseDate(q.Get(key))
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseUint(raw, 10, 32)
	return uint32(n), nil
}

func rawFileResp(file string, raw []byte) map[string]any {
	return map[string]any{
		"file":   file,
		"size":   len(raw),
		"base64": base64.StdEncoding.EncodeToString(raw),
	}
}

func toXRXDs(list protocol.XRXDs) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		if x == nil {
			continue
		}
		out = append(out, map[string]any{
			"code":        x.Code,
			"time":        fmtTime(x.Time),
			"fenhong":     x.Fenhong,
			"peigujia":    x.Peigujia,
			"songzhuangu": x.Songzhuangu,
			"peigu":       x.Peigu,
		})
	}
	return out
}

func toFactors(list []*protocol.Factor) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, f := range list {
		if f == nil {
			continue
		}
		out = append(out, map[string]any{
			"time":    fmtTime(f.Time),
			"last":    f.Last.Float64(),
			"preLast": f.PreLast.Float64(),
			"qfqMul":  f.QFQMul,
			"qfqAdd":  f.QFQAdd,
			"hfqMul":  f.HFQMul,
			"hfqAdd":  f.HFQAdd,
			"qfq":     f.QFQ,
			"hfq":     f.HFQ,
		})
	}
	return out
}
