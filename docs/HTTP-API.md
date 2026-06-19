# 通达信行情 HTTP 服务 — 功能与接口文档

本服务（`cmd/server`）把 `github.com/injoyai/tdx` 通达信协议客户端的能力封装为只读 HTTP API，
通过常驻连接池对接通达信行情服务器，对外暴露 RESTful 风格的 JSON 接口。

---

## 一、运行与约定

### 启动

```bash
go run ./cmd/server [-addr :8001] [-pool 16] [-data ./data]
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `-addr` | `:8001` | HTTP 监听地址 |
| `-pool` | `16` | 通达信标准行情连接池大小 |
| `-data` | `./data` | SQLite 数据目录（`<data>/database/codes.db`、`gbbq.db`） |

- 标准行情（沪深京）走连接池，端口 7709；扩展行情（`/api/ex/*`）每次单独短连接到端口 7727。
- 连接出错会关闭坏连接、同步重拨并最多重试 3 次，避免连接槽永久丢失。

### 通用约定

- **全部接口仅支持 `GET`**，参数通过 URL query 传递；非 GET 返回 `405`。
- 统一响应信封：
  - 成功：`{"code": 0, "data": <业务数据>}`
  - 失败：`{"code": 1, "msg": "错误描述"}`（参数错误 `400`，方法错误 `405`，内部错误 `500`）
- `Content-Type: application/json; charset=utf-8`。
- 除特别说明外，HTTP 响应业务字段统一使用 `camelCase`。

### 公共参数取值

| 参数 | 取值 | 说明 |
|------|------|------|
| `exchange` | `sh` / `sz` / `bj` | 交易所（沪 / 深 / 京） |
| `code` | 如 `sh000001`、`000001` | 证券代码，部分接口需带市场前缀 |
| `codes` | 逗号分隔，如 `sh000001,sz000001` | 多代码 |
| `date` | `YYYYMMDD`，如 `20240620` | 日期 |
| `type` | `1min` `5min` `15min` `30min` `60min` `day` `week` `month` `quarter` `year` | K 线周期 |
| `market` | uint8 | 扩展行情市场编号（见 `/api/ex/markets`） |

---

## 二、功能总览

| 功能模块 | 说明 | 对应接口前缀 |
|----------|------|--------------|
| 健康检查 | 探活 | `/api/ping` |
| 代码表 | 证券数量 / 分页代码 / 全量代码 / 股票·ETF·指数清单 | `/api/count` `/api/codes*` |
| 实时盘口 | 五档报价 | `/api/quote` |
| 分时图 | 当日 / 历史分时 | `/api/minute*` |
| 分时成交 | 当日（分页/全量）/ 历史 / 历史按日 | `/api/trade*` |
| K 线 | 个股 K 线（分页/全量）/ 指数 K 线（分页/全量） | `/api/kline*` `/api/index/kline*` |
| 集合竞价 | 竞价明细 | `/api/auction` |
| 财务与公司 | 财务信息 / F10 公司资料目录与正文 | `/api/finance` `/api/company/*` |
| 板块与行业 | 板块文件 / 板块成分 / 行业归属 / 板块指数映射 | `/api/block/*` `/api/tdx*` |
| 统计与资金 | 个股综合统计 / 资金流向 / 板块归属反查 / 新股申购 | `/api/stat*` `/api/xgsg` |
| 报表下载 | 报表文件 / ZHB 配置包 | `/api/report/*` |
| 除权除息与复权 | 股本变迁 / 流通股本 / 除权除息 / 复权因子 / 换手率 / 前后复权日线 | `/api/gbbq/*` `/api/fq/*` |
| 扩展行情 | 期货 / 港股 / 外盘的市场、品种、行情、K 线、分时、成交 | `/api/ex/*` |

---

## 三、标准行情接口

### 3.1 健康检查

#### `GET /api/ping`
- 请求参数：无
- 响应：`data` = `"pong"`

---

### 3.2 代码表

#### `GET /api/count` — 证券数量
- 参数：`exchange`（必填）
- 响应 `data`：`{ "count": <uint16> }`

#### `GET /api/codes` — 分页代码列表
- 参数：`exchange`（必填）、`start`（uint16，默认 0）
- 响应 `data`：
  ```json
  { "count": 0, "list": [
    { "name": "平安银行", "code": "000001", "multiple": 100, "decimal": 2, "lastPrice": 11.23 }
  ]}
  ```

#### `GET /api/codes/all` — 全量代码列表
- 参数：`exchange`（必填）
- 响应 `data`：同 `/api/codes`
- 说明：服务端优先读取启动时维护的本地 `codes.db` 代码表缓存；缓存为空时才回退到通达信主站全量拉取。

#### `GET /api/codes/stocks` — 全部股票代码
#### `GET /api/codes/etfs` — 全部 ETF 代码
#### `GET /api/codes/indexes` — 全部指数代码
- 参数：无
- 响应 `data`：`{ "list": ["sh600000", "sz000001", ...] }`
- 说明：同样优先读取本地 `codes.db` 缓存，避免每次请求重新串行拉取沪 / 深 / 北全市场代码表。

---

### 3.3 实时盘口

#### `GET /api/quote` — 五档报价
- 参数：`codes`（必填，逗号分隔）
- 响应 `data`（**数组**）：
  ```json
  [{
    "exchange": "sh", "code": "600000",
    "active1": 0, "active2": 0,
    "totalHand": 12345, "intuition": 0,
    "amount": 1.23e7, "insideDish": 100, "outerDisc": 200, "rate": 1.5,
    "k": { "last": 10.0, "open": 10.1, "high": 10.3, "low": 9.9, "close": 10.2 },
    "buyLevel":  [{ "buy": true,  "price": 10.19, "number": 50 }],
    "sellLevel": [{ "buy": false, "price": 10.20, "number": 30 }]
  }]
  ```
  字段：`active1/2` 活跃度，`totalHand` 总手，`amount` 成交额，`insideDish/outerDisc` 内外盘，
  `rate` 涨跌幅，`k` 四价+昨收，`buyLevel/sellLevel` 五档买卖盘。

---

### 3.4 分时图

#### `GET /api/minute` — 当日分时
- 参数：`code`（必填）
#### `GET /api/minute/history` — 历史分时
- 参数：`code`、`date`（必填）
- 响应 `data`（两者相同）：
  ```json
  { "count": 240, "list": [
    { "time": "2024-06-20 09:31:00", "price": 10.05, "number": 1200 }
  ]}
  ```

---

### 3.5 分时成交

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /api/trade` | 当日成交（分页） | `code`(必填)、`start`(默认0)、`count`(默认1800) |
| `GET /api/trade/all` | 当日成交（全量） | `code`(必填) |
| `GET /api/trade/history` | 历史成交（分页） | `code`、`date`(必填)、`start`(0)、`count`(默认2000) |
| `GET /api/trade/history/day` | 历史成交（按日全量） | `code`、`date`(必填) |

- 响应 `data`（统一）：
  ```json
  { "count": 1800, "list": [
    { "time": "2024-06-20 14:57:00", "price": 10.18, "volume": 50, "amount": 50900.0, "status": 0, "number": 12 }
  ]}
  ```
  `status`：0 买盘 / 1 卖盘 / 2 中性；`volume` 手数，`amount` 金额，`number` 笔数。

---

### 3.6 K 线 / 指数 K 线

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /api/kline` | 个股 K 线（分页） | `code`、`type`(必填)、`start`(0)、`count`(默认800) |
| `GET /api/kline/all` | 个股 K 线（全量） | `code`、`type`(必填) |
| `GET /api/index/kline` | 指数 K 线（分页） | `code`、`type`(必填)、`start`(0)、`count`(默认800) |
| `GET /api/index/kline/all` | 指数 K 线（全量） | `code`、`type`(必填) |

- 响应 `data`（统一）：
  ```json
  { "count": 800, "list": [
    { "time": "2024-06-20 15:00:00",
      "last": 9.9, "open": 10.0, "high": 10.3, "low": 9.8, "close": 10.2,
      "order": 0, "volume": 1234567, "amount": 1.2e8, "upCount": 0, "downCount": 0 }
  ]}
  ```
  `last` 昨收，`order` 序号，`upCount/downCount` 指数涨跌家数（个股为 0）。

---

### 3.7 集合竞价

#### `GET /api/auction`
- 参数：`code`（必填）
- 响应 `data`：
  ```json
  { "count": 0, "list": [
    { "time": "2024-06-20 09:25:00", "price": 10.0, "match": 10000, "unmatched": 500, "flag": 0 }
  ]}
  ```
  `match` 匹配量，`unmatched` 未匹配量，`flag` 买卖标志。

---

## 四、财务与公司资料

#### `GET /api/finance` — 财务信息
- 参数：`code`（必填）；`exchange` 可选（缺省时按 `code` 前缀自动识别市场）
- 响应 `data`：财务信息对象，字段统一为 `camelCase`
  ```json
  { "market": 1, "code": "600000",
    "liuTongGuBen": 2.93e10, "zongGuBen": 2.93e10,
    "province": 31, "industry": 1, "ipoDate": 19991110, "updatedDate": 20240331,
    "jingZiChan": 0, "zhuYingShouRu": 0, "jingLiRun": 0, "guDongRenShu": 0 }
  ```

#### `GET /api/company/categories` — F10 公司资料目录
- 参数：`code`（必填）；`exchange` 可选
- 响应 `data`：
  ```json
  { "count": 1, "list": [
    { "name": "公司概况", "filename": "600000.txt", "start": 0, "length": 2048 }
  ]}
  ```

#### `GET /api/company/content` — F10 资料正文
- 参数：`code`（必填）、`filename`（必填）、`length`（必填,uint32）、`start`（uint32,默认0）；`exchange` 可选
- 响应 `data`：`{ "content": "<正文文本>" }`

> 说明：`filename`/`start`/`length` 取自 `/api/company/categories` 的目录项。

---

## 五、板块、行业与统计

#### `GET /api/block/file` — 板块原始文件
- 参数：`file`（必填，文件名，禁止路径分隔符与 `..`）
- 响应 `data`：`{ "file": "block_gn.dat", "size": 12345, "base64": "<原始字节 base64>" }`

#### `GET /api/block/data` — 板块成分
#### `GET /api/block/data-with-index` — 板块成分（含板块指数 id 回填）
- 参数：`file`（可选，默认 `gn`）。可用别名：
  `gn|concept`（概念）、`fg|style|region`（地域/风格）、`zs|index`（指数）、`hy|industry`（行业）、`block`，或直接传 `*.dat` 文件名。
- 响应 `data`：
  ```json
  { "count": 200, "list": [
    { "name": "锂电池", "index": "880xxx", "type": 2, "codes": ["1600000", "0000001"] }
  ]}
  ```
  `codes` 为 7 字符（首字符市场标志：1=沪 0=深）；`index` 仅 `data-with-index` 回填。

#### `GET /api/tdxzs` — 板块指数代码(id)映射
- 参数：无
- 响应 `data`：`{ "count": N, "list": [{ "name":"...", "code":"880081", "type":5, "subType":2, "ref":"..." }] }`

#### `GET /api/tdxbk` — 板块简称↔全称
- 参数：无
- 响应 `data`：`{ "count": N, "list": [{ "short":"锂电池", "full":"锂电池概念" }] }`

#### `GET /api/tdxhy` — 行业归属（通达信/申万）
- 参数：无
- 响应 `data`：`{ "count": N, "list": [{ "market":1, "code":"600000", "tdxHy":"T...", "swHy":"X..." }] }`

#### `GET /api/stat` — 个股综合统计
- 参数：无
- 响应 `data`：`{ "count": N, "list": [...] }`，列表项字段含
  `market` `code` `date` `peTtm` `trendDays`(连涨连跌天数) `changePct` `peStatic` `divYield`(股息率)
  `chg5` `chg10` `chg20` `chg60` `chgYtd`，`fields`(全部35个原始字段)。

#### `GET /api/stat2` — 个股资金流向 + 板块归属
- 参数：无
- 响应 `data`：`{ "count": N, "list": [...] }`，列表项字段含
  `market` `code` `date` `blockIndex`(板块指数id) `amount`(今日成交额,万元) `amountPrev`(昨日)
  `ipoPrice` `high52w` `low52w`，`fields`(全部21字段)。

#### `GET /api/stat2/block-index` — 证券→板块指数 反查表
- 参数：无
- 响应 `data`：`{ "count": N, "items": { "600000": "880xxx", ... } }`

#### `GET /api/xgsg` — 新股申购
- 参数：无
- 响应 `data`：`{ "count": N, "list": [{ "market":1, "code":"...", "date":"YYYYMMDD", "issuePrice":10.0, "name":"...", "fields":[...] }] }`

#### `GET /api/report/file` — 报表文件下载
- 参数：`file`（必填）
- 响应 `data`：`{ "file": "...", "size": N, "base64": "..." }`

#### `GET /api/report/zhb` — ZHB 配置包（多文件）
- 参数：无
- 响应 `data`：`{ "count": N, "files": { "tdxzs.cfg": { "file":"tdxzs.cfg", "size":N, "base64":"..." }, ... } }`

---

## 六、除权除息与复权

#### `GET /api/gbbq` — 单只股本变迁（在线查询）
- 参数：`code`（必填）
- 响应 `data`：`{ "count": N, "list": [{ "code":"...", "time":"...", "category":2, "c1":0, "c2":0, "c3":0, "c4":0 }] }`
  `category`：1=除权除息，2/3/5/7/8/9/10=股本类。

#### `GET /api/gbbq/all` — 全市场股本变迁（本地库）
- 参数：无
- 响应 `data`：`{ "count": N, "items": { "sh600000": [{ "code":"sh600000", "time":"...", "category":2, "c1":0, "c2":0, "c3":0, "c4":0 }] } }`

#### `GET /api/gbbq/equity` — 指定日的流通/总股本
- 参数：`code`、`date`（必填）
- 响应 `data`：`{ "category":2, "code":"sh600000", "time":"...", "float": 1000000, "total": 2000000 }`（无记录返回 `null`）

#### `GET /api/gbbq/xrxd` — 除权除息事件列表
- 参数：`code`（必填）
- 响应 `data`：`{ "count": N, "list": [{ "code":"...", "time":"...", "fenhong":5.0, "peigujia":0, "songzhuangu":0, "peigu":0 }] }`

#### `GET /api/gbbq/factors` — 复权因子序列
- 参数：`code`（必填）
- 响应 `data`：`{ "count": N, "list": [{ "time":"...", "last":..., "preLast":..., "qfqMul":..., "qfqAdd":..., "hfqMul":..., "hfqAdd":..., "qfq":..., "hfq":... }] }`

#### `GET /api/gbbq/turnover` — 换手率
- 参数：`code`、`date`、`volume`（均必填，`volume` 为成交股数）
- 响应 `data`：`{ "turnover": 1.23 }`（百分比）

#### `GET /api/fq/qfq/day` — 前复权日线
#### `GET /api/fq/hfq/day` — 后复权日线
- 参数：`code`（必填）
- 响应 `data`：K 线结构（同 `/api/kline`，价格已前/后复权）

---

## 七、扩展行情（期货 / 港股 / 外盘，端口 7727）

> 扩展行情每次请求单独短连接到扩展行情服务器；`market` 编号见 `/api/ex/markets`。
> Ex 系列响应结构体使用 **camelCase json tag**。

#### `GET /api/ex/markets` — 市场列表
- 参数：无
- 响应 `data`：`{ "count": N, "list": [{ "market":..., "category":..., "name":"...", "shortName":"..." }] }`

#### `GET /api/ex/count` — 品种总数
- 参数：无
- 响应 `data`：`{ "count": N }`

#### `GET /api/ex/instruments` — 品种代码表（分页）
- 参数：`start`（uint32,默认0）、`count`（uint16,默认100）
- 响应 `data`：`{ "count": N, "list": [{ "category":..., "market":..., "code":"...", "name":"...", "desc":"..." }] }`

#### `GET /api/ex/quote` — 单品种五档行情
- 参数：`market`、`code`（必填）
- 响应 `data`：`ExQuote`
  ```json
  { "market":..., "code":"...", "preClose":..., "open":..., "high":..., "low":..., "price":...,
    "kaiCang":..., "zongLiang":..., "xianLiang":..., "neiPan":..., "waiPan":..., "chiCang":...,
    "bid":[5], "bidVol":[5], "ask":[5], "askVol":[5] }
  ```

#### `GET /api/ex/quote-list` — 批量行情列表
- 参数：`market`、`category`（必填）、`start`(0)、`count`(默认100)
- 响应 `data`：`{ "count": N, "list": [ExQuoteListItem{market,code,preClose,open,high,low,price,zongLiang,amount,inner,outer,chiCang,bid,bidVol,ask,askVol}] }`

#### `GET /api/ex/bars` — 扩展 K 线
- 参数：`market`、`code`（必填）、`category` 或 `type`（二选一，`type` 同标准 K 线周期名）、`start`(0)、`count`(默认800)
- 响应 `data`：`{ "count": N, "list": [{ "datetime":"...", "open":..., "high":..., "low":..., "close":..., "position":..., "trade":..., "price":..., "amount":... }] }`

#### `GET /api/ex/minute` — 当日分时
#### `GET /api/ex/minute/history` — 历史分时
- 参数：`market`、`code`（必填）；history 另需 `date`
- 响应 `data`：`{ "count": N, "list": [{ "hour":..., "minute":..., "price":..., "avgPrice":..., "volume":..., "openInterest":... }] }`

#### `GET /api/ex/trade` — 当日分笔成交
#### `GET /api/ex/trade/history` — 历史分笔成交
- 参数：`market`、`code`（必填）、`start`(0)、`count`(默认100)；history 另需 `date`
- 响应 `data`：`{ "count": N, "list": [{ "hour":..., "minute":..., "second":..., "price":..., "volume":..., "zengCang":..., "nature":..., "natureName":"...", "direction": 1 }] }`
  `direction`：1 买 / -1 卖 / 0 中性。

#### `GET /api/ex/bars/range` — 区间历史 K 线
- 参数：`market`、`code`、`date`、`date2`（均必填）
- 响应 `data`：`{ "count": N, "list": [{ "datetime":"...", "open":..., "high":..., "low":..., "close":..., "position":..., "trade":..., "settlementPrice":... }] }`

---

## 八、接口速查表

| 方法 | 路径 | 必填参数 | 功能 |
|------|------|----------|------|
| GET | `/api/ping` | — | 探活 |
| GET | `/api/count` | exchange | 证券数量 |
| GET | `/api/codes` | exchange | 分页代码 |
| GET | `/api/codes/all` | exchange | 全量代码 |
| GET | `/api/codes/stocks` | — | 股票代码清单 |
| GET | `/api/codes/etfs` | — | ETF 代码清单 |
| GET | `/api/codes/indexes` | — | 指数代码清单 |
| GET | `/api/quote` | codes | 五档报价 |
| GET | `/api/minute` | code | 当日分时 |
| GET | `/api/minute/history` | code, date | 历史分时 |
| GET | `/api/trade` | code | 当日成交(分页) |
| GET | `/api/trade/all` | code | 当日成交(全量) |
| GET | `/api/trade/history` | code, date | 历史成交(分页) |
| GET | `/api/trade/history/day` | code, date | 历史成交(按日) |
| GET | `/api/kline` | code, type | 个股K线(分页) |
| GET | `/api/kline/all` | code, type | 个股K线(全量) |
| GET | `/api/index/kline` | code, type | 指数K线(分页) |
| GET | `/api/index/kline/all` | code, type | 指数K线(全量) |
| GET | `/api/auction` | code | 集合竞价 |
| GET | `/api/finance` | code | 财务信息 |
| GET | `/api/company/categories` | code | F10 目录 |
| GET | `/api/company/content` | code, filename, length | F10 正文 |
| GET | `/api/block/file` | file | 板块原始文件 |
| GET | `/api/block/data` | — (file 可选) | 板块成分 |
| GET | `/api/block/data-with-index` | — (file 可选) | 板块成分(含id) |
| GET | `/api/report/file` | file | 报表文件 |
| GET | `/api/report/zhb` | — | ZHB 配置包 |
| GET | `/api/tdxzs` | — | 板块指数id映射 |
| GET | `/api/tdxbk` | — | 板块简称全称 |
| GET | `/api/tdxhy` | — | 行业归属 |
| GET | `/api/stat` | — | 个股综合统计 |
| GET | `/api/stat2` | — | 资金流向+板块 |
| GET | `/api/stat2/block-index` | — | 证券→板块反查 |
| GET | `/api/xgsg` | — | 新股申购 |
| GET | `/api/gbbq` | code | 股本变迁(在线) |
| GET | `/api/gbbq/all` | — | 股本变迁(全量) |
| GET | `/api/gbbq/equity` | code, date | 流通/总股本 |
| GET | `/api/gbbq/xrxd` | code | 除权除息事件 |
| GET | `/api/gbbq/factors` | code | 复权因子 |
| GET | `/api/gbbq/turnover` | code, date, volume | 换手率 |
| GET | `/api/fq/qfq/day` | code | 前复权日线 |
| GET | `/api/fq/hfq/day` | code | 后复权日线 |
| GET | `/api/ex/markets` | — | 扩展市场列表 |
| GET | `/api/ex/count` | — | 扩展品种总数 |
| GET | `/api/ex/instruments` | — | 扩展品种代码表 |
| GET | `/api/ex/quote` | market, code | 扩展五档行情 |
| GET | `/api/ex/quote-list` | market, category | 扩展批量行情 |
| GET | `/api/ex/bars` | market, code, (category\|type) | 扩展K线 |
| GET | `/api/ex/minute` | market, code | 扩展当日分时 |
| GET | `/api/ex/minute/history` | market, code, date | 扩展历史分时 |
| GET | `/api/ex/trade` | market, code | 扩展当日成交 |
| GET | `/api/ex/trade/history` | market, code, date | 扩展历史成交 |
| GET | `/api/ex/bars/range` | market, code, date, date2 | 扩展区间K线 |

共 **53 个 HTTP 接口**。
