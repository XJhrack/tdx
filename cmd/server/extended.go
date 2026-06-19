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
		info, err := c.GetFinanceInfo(ex, code)
		if err != nil {
			return nil, err
		}
		return toFinanceInfo(info), nil
	})
}

func hCompanyCategories(pool *reliablePool, q url.Values) (any, error) {
	ex, code, err := parseExchangeCode(q)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetCompanyCategory(ex, code)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": toCompanyCategories(list)}, nil
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
		return map[string]any{"count": len(blocks), "list": toBlocks(blocks)}, nil
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
		return map[string]any{"count": len(blocks), "list": toBlocks(blocks)}, nil
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
		return map[string]any{"count": len(list), "list": toTdxZsList(list)}, nil
	})
}

func hTdxBk(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxBk()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": toTdxBkList(list)}, nil
	})
}

func hTdxHy(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxHy()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": toTdxHyList(list)}, nil
	})
}

func hTdxStat(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxStat()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": toTdxStats(list)}, nil
	})
}

func hTdxStat2(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		list, err := c.GetTdxStat2()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(list), "list": toTdxStat2s(list)}, nil
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
		return map[string]any{"count": len(list), "list": toXgsgs(list)}, nil
	})
}

func hGbbqAll(pool *reliablePool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetGbbqAll()
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(resp), "items": toGbbqItems(resp)}, nil
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
	return toEquity(gb.GetEquity(code, date)), nil
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

func toFinanceInfo(f *protocol.FinanceInfo) map[string]any {
	if f == nil {
		return nil
	}
	return map[string]any{
		"market":             f.Market,
		"code":               f.Code,
		"liuTongGuBen":       f.LiuTongGuBen,
		"province":           f.Province,
		"industry":           f.Industry,
		"updatedDate":        f.UpdatedDate,
		"ipoDate":            f.IPODate,
		"zongGuBen":          f.ZongGuBen,
		"guoJiaGu":           f.GuoJiaGu,
		"faQiRenFaRenGu":     f.FaQiRenFaRenGu,
		"faRenGu":            f.FaRenGu,
		"bGu":                f.BGu,
		"hGu":                f.HGu,
		"zhiGongGu":          f.ZhiGongGu,
		"zongZiChan":         f.ZongZiChan,
		"liuDongZiChan":      f.LiuDongZiChan,
		"guDingZiChan":       f.GuDingZiChan,
		"wuXingZiChan":       f.WuXingZiChan,
		"guDongRenShu":       f.GuDongRenShu,
		"liuDongFuZhai":      f.LiuDongFuZhai,
		"changQiFuZhai":      f.ChangQiFuZhai,
		"ziBenGongJiJin":     f.ZiBenGongJiJin,
		"jingZiChan":         f.JingZiChan,
		"zhuYingShouRu":      f.ZhuYingShouRu,
		"zhuYingLiRun":       f.ZhuYingLiRun,
		"yingShouZhangKuan":  f.YingShouZhangKuan,
		"yingYeLiRun":        f.YingYeLiRun,
		"touZiShouYi":        f.TouZiShouYi,
		"jingYingXianJinLiu": f.JingYingXianJinLiu,
		"zongXianJinLiu":     f.ZongXianJinLiu,
		"cunHuo":             f.CunHuo,
		"liRunZongHe":        f.LiRunZongHe,
		"shuiHouLiRun":       f.ShuiHouLiRun,
		"jingLiRun":          f.JingLiRun,
		"weiFenLiRun":        f.WeiFenLiRun,
		"baoLiu1":            f.BaoLiu1,
		"baoLiu2":            f.BaoLiu2,
	}
}

func toCompanyCategories(list []protocol.CompanyCategory) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{
			"name":     c.Name,
			"filename": c.Filename,
			"start":    c.Start,
			"length":   c.Length,
		})
	}
	return out
}

func toBlocks(list []*protocol.Block) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		if b == nil {
			continue
		}
		out = append(out, map[string]any{
			"name":  b.Name,
			"index": b.Index,
			"type":  b.Type,
			"codes": b.Codes,
		})
	}
	return out
}

func toTdxZsList(list []*protocol.TdxZs) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, z := range list {
		if z == nil {
			continue
		}
		out = append(out, map[string]any{
			"name":    z.Name,
			"code":    z.Code,
			"type":    z.Type,
			"subType": z.SubType,
			"ref":     z.Ref,
		})
	}
	return out
}

func toTdxBkList(list []*protocol.TdxBk) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		if b == nil {
			continue
		}
		out = append(out, map[string]any{
			"short": b.Short,
			"full":  b.Full,
		})
	}
	return out
}

func toTdxHyList(list []*protocol.TdxHy) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, h := range list {
		if h == nil {
			continue
		}
		out = append(out, map[string]any{
			"market": h.Market,
			"code":   h.Code,
			"tdxHy":  h.TdxHy,
			"swHy":   h.SwHy,
		})
	}
	return out
}

func toTdxStats(list []*protocol.TdxStat) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		if s == nil {
			continue
		}
		out = append(out, map[string]any{
			"market":    s.Market,
			"code":      s.Code,
			"date":      s.Date,
			"peTtm":     s.PETTM,
			"trendDays": s.TrendDays,
			"changePct": s.ChangePct,
			"peStatic":  s.PEStatic,
			"divYield":  s.DivYield,
			"chg5":      s.Chg5,
			"chg10":     s.Chg10,
			"chg20":     s.Chg20,
			"chg60":     s.Chg60,
			"chgYtd":    s.ChgYTD,
			"fields":    s.Fields,
		})
	}
	return out
}

func toTdxStat2s(list []*protocol.TdxStat2) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		if s == nil {
			continue
		}
		out = append(out, map[string]any{
			"market":     s.Market,
			"code":       s.Code,
			"date":       s.Date,
			"blockIndex": s.BlockIndex,
			"amount":     s.Amount,
			"amountPrev": s.AmountPrev,
			"ipoPrice":   s.IPOPrice,
			"high52w":    s.High52W,
			"low52w":     s.Low52W,
			"fields":     s.Fields,
		})
	}
	return out
}

func toXgsgs(list []*protocol.TdxXgsg) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		if x == nil {
			continue
		}
		out = append(out, map[string]any{
			"market":     x.Market,
			"code":       x.Code,
			"date":       x.Date,
			"issuePrice": x.IssuePrice,
			"name":       x.Name,
			"fields":     x.Fields,
		})
	}
	return out
}

func toGbbqItems(items map[string][]*protocol.Gbbq) map[string][]map[string]any {
	out := make(map[string][]map[string]any, len(items))
	for code, list := range items {
		out[code] = toGbbqs(list)
	}
	return out
}

func toGbbqs(list []*protocol.Gbbq) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, g := range list {
		if g == nil {
			continue
		}
		out = append(out, map[string]any{
			"code":     g.Code,
			"time":     fmtTime(g.Time),
			"category": g.Category,
			"c1":       g.C1,
			"c2":       g.C2,
			"c3":       g.C3,
			"c4":       g.C4,
		})
	}
	return out
}

func toEquity(e *protocol.Equity) map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"category": e.Category,
		"code":     e.Code,
		"time":     fmtTime(e.Time),
		"float":    e.Float,
		"total":    e.Total,
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
