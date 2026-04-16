# Surf CLI v1.0.0-alpha.26 Review

这份文档是对 Surf CLI `v1.0.0-alpha.26` 的一次 review，依据是 [`CLI_DESIGN_PRINCIPLES.md`](./CLI_DESIGN_PRINCIPLES.md) 里的原则。

## Context

按 [`CLI_DESIGN_PRINCIPLES.md`](./CLI_DESIGN_PRINCIPLES.md) 对 `v1.0.0-alpha.26` 的 `cmd/surf/` 做了 clig.dev 和 axi.md 的合规性 review。过程中发现 21 个大小不等的改进机会，从 "违反核心 agent 合同" 到 "nice-to-have polish" 都有。这份文档给它们分优先级，作为后续 PR 的 backlog。

每一项的格式：

- **现状**：当前 CLI 的实际行为（通过真实测试验证）。
- **目标**：想要的行为，引用 `CLI_DESIGN_PRINCIPLES.md` 的章节号。
- **复杂度**：trivial / low / medium / high。
- **文件**：主要改动的文件路径。
- **Breaking change**：是否会破坏现有 agent 或脚本。
- **验收标准**：怎么验证修好了。

## 优先级分类

- **P0 (critical)**：违反核心 agent 合同，agent 现在就会因为这些踩坑。必须尽快修。
- **P1 (high)**：显著影响 agent UX 或 token 成本。跟 agent-first 定位直接相关。
- **P2 (medium)**：合规性提升、human 友好改进，不紧急但值得做。
- **P3 (low)**：Nice-to-have polish，可以无限期推后。

---

## P0 (critical)

### 1. `--json` flag 在 operation 命令上不可用 — [#69](https://github.com/cyberconnecthq/surf-cli/pull/69)

**现状**：`surf catalog list --json` 工作，但 `surf market-price --json` 报 `unknown flag: --json` + exit 1。agent 学了 `catalog --json` 的用法后在 operation 上踩坑。

复现：

```
$ surf market-price --symbol BTC --time-range 1d --json
Error: unknown flag: --json
$ echo $?
1
```

**目标**：所有命令接受 `--json` 作为 `-o json` 的 alias。输出完整 response JSON envelope，不做 stripping。参见 `CLI_DESIGN_PRINCIPLES.md` §2.2, §3.2。

**复杂度**：low。

**文件**：`cli/operation.go`（在 `Command()` 方法里加一个 global pflag alias）、`cli/cli.go`（在 Root flag 注册时加 `--json` persistent flag 映射到 `rsh-output-format=json`）。

**Breaking change**：否，additive。

**验收**：

- `surf market-price --symbol BTC --time-range 1d --json` 返回完整 JSON envelope
- `surf --json market-price --symbol BTC --time-range 1d` 也能工作（root 层 flag 传递）
- 现有的 `-o json` 用法不受影响

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf market-price --symbol BTC --time-range 1d --json
Error: unknown flag: --json
$ echo $?
1

# After
$ surf market-price --symbol BTC --time-range 1d --json
{
  "data": [
    {"metric": "price", "symbol": "BTC", "timestamp": 1775675100, "value": 70123.45},
    ...
  ],
  "meta": {"cached": false, "credits_used": 1}
}
$ echo $?
0
```

### 2. 文档化 API error 去 stdout

**现状**：CLI 代码已经把 API error envelope 输出到 stdout（实测 `SURF_API_KEY=sk-000... surf market-price --symbol BTC --time-range 1d` 的 error JSON 完全在 stdout，stderr 空）。`CLI_DESIGN_PRINCIPLES.md` 之前写 "error body → stderr"，内部对齐有差距。

**目标**：文档和代码一致。设计原则 §3.1 / §4.2.1 已经改好了，代码不用动。SKILL.md 那边还有一句 "check error.code in stderr"，需要发一个单独的 PR 到 `surf-skills` 修正。

**复杂度**：none（已修，只剩 SKILL.md 的文字同步）。

**文件**：`surf-skills/skills/surf/SKILL.md`（在另一个 repo）。

**Breaking change**：否。

**验收**：SKILL.md 里 error code 查找的描述跟 CLI 实际行为（API error 在 stdout）一致。

**Before → After**：这个 item 改的是 SKILL.md 文字，不是 CLI 行为。对比 SKILL.md 里的一句话：

```diff
# Before (surf-skills/skills/surf/SKILL.md)
- On error (exit code 4): check the JSON `error.code` field in stderr

# After
+ On error (exit code 4): check the JSON `error.code` field in stdout
+ (The error envelope is printed to stdout alongside success data so agents
+ can use a single parse pipeline: `surf <cmd> --json | jq '.error // .data'`)
```

CLI 侧实际行为验证（不需要改动）：

```
$ SURF_API_KEY=sk-0000000000000000000000000000000000000000000000000000000000000000 \
    surf market-price --symbol BTC --time-range 1d 2>/dev/null
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid API key"
  }
}
# ^ stdout has the JSON; stderr is empty
$ echo $?
4
```

### 3. 网络错误的 exit code 错了 — [#70](https://github.com/cyberconnecthq/surf-cli/pull/70)

**现状**：连不上 gateway（DNS 失败、TCP refused、CLI 内部 timeout、TLS 握手失败等）当前返回 exit 1 + stderr 打一行 `ERROR: Caught error: ...` + stdout 空。Agent 看到 exit 1 以为是自己 invocation 出错，不会 retry。

复现：

```
$ surf market-price --symbol BTC --time-range 1d --rsh-server http://127.0.0.1:1 --rsh-retry 0
ERROR: Caught error: Get "http://127.0.0.1:1/...": dial tcp 127.0.0.1:1: connect: connection refused
$ echo $?
1
```

**目标**：网络层错误映射到 **exit 4**，并在 stdout 输出合成的 error envelope，让 agent 可以像处理 5xx 一样处理：

```json
{"error": {"code": "NETWORK_ERROR", "message": "connection refused: localhost:1"}}
```

理由（见 `CLI_DESIGN_PRINCIPLES.md` §4.1）：

- Agent 没做错什么（不该是 exit 1 "检查你的 invocation"）
- API 不可达属于 "外部失败"，语义上跟 502/503 同类
- 应该走同一个 retry 决策分支

**复杂度**：low。

**文件**：`cli/request.go`（`MakeRequest` 的 error 返回路径）、`cmd/surf/main.go`（exit code 映射）。

**Breaking change**：从 "exit 1 + stderr" 变成 "exit 4 + stdout envelope" 技术上是 breaking change（有 agent 可能 branch 在 exit 1 上）。但当前的 exit 1 行为是 bug，没有 agent skill 明确依赖它；值得作为 P0 bugfix 修掉。

**验收**：

- 连不上 gateway → exit 4，stdout 有 `{"error": {"code": "NETWORK_ERROR", ...}}`
- DNS 失败 → exit 4
- TLS 握手失败 → exit 4
- CLI 本身的 timeout（`--timeout`）→ exit 4 + `error.code = TIMEOUT`
- unknown flag 之类真正的 CLI 层错误仍然 exit 1（不受影响）

**Before → After**：

```
# Before (v1.0.0-alpha.26) — connection refused
$ surf market-price --symbol BTC --time-range 1d \
    --rsh-server http://127.0.0.1:1 --rsh-retry 0
ERROR: Caught error: Get "http://127.0.0.1:1/gateway/v1/market/price?symbol=BTC&time_range=1d": dial tcp 127.0.0.1:1: connect: connection refused
$ echo $?
1
# ^ stderr text, stdout empty, exit 1 — agent thinks it's a usage error

# After — same invocation
$ surf market-price --symbol BTC --time-range 1d \
    --rsh-server http://127.0.0.1:1 --rsh-retry 0
{
  "error": {
    "code": "NETWORK_ERROR",
    "message": "connection refused: 127.0.0.1:1"
  }
}
$ echo $?
4
# ^ stdout has JSON envelope, exit 4 — agent treats it like a 5xx and can retry

# CLI timeout also maps to exit 4
$ surf market-price --symbol BTC --time-range 1d --timeout 1ms
{
  "error": {
    "code": "TIMEOUT",
    "message": "request timed out after 1ms"
  }
}
$ echo $?
4
```

---

## P1 (high)

### 4. Error envelope 统一 code 枚举 — [#70](https://github.com/cyberconnecthq/surf-cli/pull/70)

**现状**：后端 API error envelope 的 `error.code` 枚举是稳定的（`UNAUTHORIZED`、`INSUFFICIENT_CREDIT`、`RATE_LIMITED`、`INVALID_REQUEST`、`NOT_FOUND`、`BAD_GATEWAY`），但 CLI 自己合成的 error envelope（network error、本地 timeout 等）没有对应的 code。

**目标**：给 CLI 层合成的 error envelope 定义一套明确的 code，让 agent 可以像处理后端错误一样用 `jq '.error.code'` 分支：

| code            | 触发                        | exit |
| --------------- | --------------------------- | ---- |
| `NETWORK_ERROR` | TCP/DNS/TLS/connection 失败 | 4    |
| `TIMEOUT`       | CLI 本地 timeout 超时       | 4    |

跟后端已有的 code 命名空间不重叠。这两类 error 进 stdout envelope；其他 CLI 层的错误（unknown flag、parse error 等）保持 stderr 纯文本 + exit 1，因为 agent 靠 exit 1 就够了，不需要程序化分支。

**复杂度**：low-medium。需要定义一个 CLI 层的 error envelope builder。

**文件**：`cli/request.go`、`cli/cli.go`、新建 `cli/errors.go` 放 error code 常量。

**Breaking change**：additive——前提是 #3 先落地（否则现状还是 exit 1 + stderr 文本，没有 envelope 可以加 code）。

**验收**：

- 所有 exit 4 的 CLI 合成错误都有明确的 `error.code`
- `error.code` 列表文档化在 `CLI_DESIGN_PRINCIPLES.md` §14.4

**Before → After**：

```
# Before (v1.0.0-alpha.26) — agent can't branch on network failure type
$ surf market-price --symbol BTC --timeout 1ms 2>&1
ERROR: Caught error: Get "...": context deadline exceeded (Client.Timeout exceeded)
$ echo $?
1
$ surf market-price --symbol BTC --rsh-server http://127.0.0.1:1 2>&1
ERROR: Caught error: Get "...": dial tcp 127.0.0.1:1: connect: connection refused
$ echo $?
1
# ^ agent can't tell timeout from connection refused except by parsing stderr text

# After — each error type has a stable `error.code`
$ surf market-price --symbol BTC --timeout 1ms --json | jq '.error.code'
"TIMEOUT"
$ surf market-price --symbol BTC --rsh-server http://127.0.0.1:1 --json | jq '.error.code'
"NETWORK_ERROR"

# Unified agent handling:
$ surf market-price --symbol BTC --json | jq '
    if .error then
      if .error.code == "TIMEOUT" then "retry with longer timeout"
      elif .error.code == "NETWORK_ERROR" then "retry with backoff"
      elif .error.code == "RATE_LIMITED" then "wait and retry"
      elif .error.code == "UNAUTHORIZED" then "reconfigure auth"
      else "abort" end
    else .data end'
```

### 5. Operation help 包含 OpenAPI schema 重复 — [#71](https://github.com/cyberconnecthq/surf-cli/pull/71)

**现状**：`surf market-price --help` 输出 68 行，前面有大段 `## Option Schema` 代码块跟 Cobra 自动生成的 `Flags:` 段完全重复。实测输出里有 1 个 `## Option Schema` + 2 个 `## Response` block。Agent context 里有两份 flag 定义，浪费 token。

**目标**：渲染之前把 `## Option Schema`、`## Response 200`、`## Response default` 等 schema section 从 `op.Long` 里 strip 掉。只保留 prose 描述和 `## Time range options` 这种说明性 section。参见 §3.5。

**复杂度**：low。

**文件**：`cli/operation.go` 的 `Command()` 方法，在设置 `Long` 之前加一个 strip 函数。

**Breaking change**：human help 输出可变，不算 breaking（见 §13.4）。Agent 用 `--help` 输出比较少，影响小。

**验收**：

- `surf market-price --help` 行数从 68 降到 ~30
- `Flags:` 段仍然存在且完整
- 命令本身的 prose 描述（"Returns historical price data points..."）保留

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf market-price --help | wc -l
68
$ surf market-price --help
Returns historical price data points for a token over a specified time range.

**Time range options:**
- Predefined: `1d`, `7d`, `14d`, ...

## Option Schema:
​```schema
{
  --symbol: (string) Single token ticker symbol like `BTC`, `ETH`, or `SOL`
  --time-range: (string default:"30d" enum:"1d","7d","14d","30d",...) Predefined time range
  --from: (string) Start of custom date range
  --to: (string) End of custom date range
  --currency: (string default:"usd") Quote currency
}
​```

## Response 200 (application/json)
​```schema
{ data*: [...], meta*: {...} }
​```

## Response default (application/json)
​```schema
{ error*: {...} }
​```

Usage:
  surf market-price [flags]

Flags:
      --currency usd      Quote currency like usd, `eur`, or `btc` (default "usd")
      --from to           Start of custom date range
  -h, --help              help for market-price
      --symbol BTC        Single token ticker symbol
      --time-range from   Predefined time range (default "30d")
      --to from           End of custom date range

# After
$ surf market-price --help | wc -l
30
$ surf market-price --help
Returns historical price data points for a token over a specified time range.

**Time range options:**
- Predefined: `1d`, `7d`, `14d`, ...

**Granularity** is automatic based on range:
- `1d` → 5-minute intervals
- `7d`–`90d` → hourly
- `180d`+ → daily

Usage:
  surf market-price [flags]

Flags:
      --currency usd      Quote currency like usd, `eur`, or `btc` (default "usd")
      --from to           Start of custom date range
  -h, --help              help for market-price
      --symbol BTC        Single token ticker symbol
      --time-range from   Predefined time range (default "30d")
      --to from           End of custom date range
```

### 6. 最小默认 response shape + `--fields`

**现状**：operation 命令默认返回完整 response envelope，每项 10+ 字段。比如 `market-price` 的一项有 `$schema`, `metric`, `symbol`, `timestamp`, `value`，`meta` 有 `cached`, `credits_used`, `empty_reason` 等。对 agent 来说大部分是噪声。

**目标**：默认只返回 3-5 个最核心的字段。`--fields a,b,c` 扩展，`--fields all` 全量。见 §3.6。

**复杂度**：high。需要 OpenAPI spec 层面给每个 endpoint 标 default fields（比如 `x-cli-default-fields: [timestamp, value]`），CLI formatter 读这个 extension 来做字段过滤。

**文件**：

- `cli/formatter.go`（字段过滤逻辑）
- `cli/operation.go` 和 `openapi/openapi.go`（读取 extension）
- 上游 API spec（加 `x-cli-default-fields` annotation）

**Breaking change**：⚠️ **是**。默认 response shape 变化，所有现有 agent 和脚本依赖当前完整 envelope 的就会 break。推荐走 opt-in env var（`SURF_MINIMAL_FIELDS=1` 开启，默认关闭），agent runtime 显式启用，避免一刀切 break。

**验收**：

- `SURF_MINIMAL_FIELDS=1 surf market-price --symbol BTC --json` 返回精简 shape
- 不设 env var 时行为不变
- `--fields all` 始终返回完整 envelope
- `--fields timestamp,value` 显式选字段
- 不存在的 field 是 validation error，不是静默 no-op

**Before → After**：

```
# Before (v1.0.0-alpha.26) — full envelope with ~10 fields per item
$ surf market-price --symbol BTC --time-range 1d --json
{
  "$schema": "https://api.asksurf.ai/schemas/SimpleListResponseMarketMetricPoint.json",
  "data": [
    {
      "metric": "price",
      "symbol": "BTC",
      "timestamp": 1775675100,
      "value": 70123.45
    },
    ... 287 more items
  ],
  "meta": {
    "cached": false,
    "credits_used": 1,
    "empty_reason": null
  }
}
# ~12 KB, every item has `metric` and `symbol` (redundant for single-symbol query)

# After — opt-in via env var
$ SURF_MINIMAL_FIELDS=1 surf market-price --symbol BTC --time-range 1d --json
{
  "data": [
    {"timestamp": 1775675100, "value": 70123.45},
    ...
  ]
}
# ~4 KB, just the 2 fields agents actually use

# Explicit field selection
$ SURF_MINIMAL_FIELDS=1 surf market-price --symbol BTC --time-range 1d \
    --fields timestamp,value,symbol --json
{
  "data": [
    {"timestamp": 1775675100, "value": 70123.45, "symbol": "BTC"},
    ...
  ]
}

# Escape hatch back to full envelope
$ SURF_MINIMAL_FIELDS=1 surf market-price --symbol BTC --time-range 1d --fields all --json
# Identical to current default output above

# Unknown field is an error
$ SURF_MINIMAL_FIELDS=1 surf market-price --symbol BTC --fields nonexistent --json
Error: unknown field 'nonexistent' for operation market-price.
Valid fields: metric, symbol, timestamp, value
```

### 7. 长文本字段截断 + `--full`

**现状**：长字段（`description`、新闻 `body`、catalog `sample_queries`）完整返回。`news-feed` 一次 call 能吃几千 token 在 article body 上。

**目标**：默认截断到 500 字符，附加 `(truncated, N chars — use --full)` 提示。`--full` 关闭截断。见 §3.7。

**复杂度**：medium。需要知道"哪些字段是长文本"——可以根据 schema type (`string` with no `maxLength`) 或额外的 extension 识别。

**文件**：`cli/formatter.go`、`openapi/openapi.go`（可选，读 `x-cli-truncate` extension）。

**Breaking change**：跟 #6 一样，默认行为变化。建议同样走 `SURF_MINIMAL_FIELDS=1` 开关或跟 #6 一起打包成一个新的 response mode。

**验收**：

- 长文本字段输出里出现 `(truncated, ... use --full)`
- `--full` 下输出跟现在完全一样
- 数值、时间戳、枚举等标量从不截断

**Before → After**：

```
# Before (v1.0.0-alpha.26) — full article body eats thousands of tokens
$ surf news-feed --limit 1 --json | jq '.data[0].body' | wc -c
2847
$ surf news-feed --limit 1 --json | jq '.data[0]'
{
  "title": "Bitcoin hits new high",
  "body": "Bitcoin surged past $70,000 on Tuesday as institutional demand... (2,843 more chars)",
  "published_at": 1775675100,
  "source": "CoinDesk"
}

# After — body truncated to 500 chars with hint
$ surf news-feed --limit 1 --json | jq '.data[0].body' | wc -c
560
$ surf news-feed --limit 1 --json | jq '.data[0].body'
"Bitcoin surged past $70,000 on Tuesday as institutional demand... (truncated, 2847 chars total — use --full to see complete body)"

# --full restores complete text
$ surf news-feed --limit 1 --full --json | jq '.data[0].body' | wc -c
2847

# Scalars never truncated (timestamp, title stay intact)
$ surf news-feed --limit 1 --json | jq '.data[0] | {title, published_at}'
{
  "title": "Bitcoin hits new high",
  "published_at": 1775675100
}
```

### 8. Typo suggestion for unknown commands

**现状**：`surf catlog` 报 `unknown command "catlog"`，没有 "Did you mean `catalog`?" 建议。`surf market-pric` 同样问题。

复现：

```
$ surf catlog
ERROR: Error: unknown command "catlog"
Run 'surf --help' for usage
```

**目标**：加 Cobra 原生的 suggestion：`Did you mean 'catalog'?`。见 §4.2.2。

**复杂度**：low（一行代码）。

**文件**：`cmd/surf/main.go` —— 在 Root 命令上设 `SuggestionsMinimumDistance = 2`（或开启默认值）。

**Breaking change**：否，additive。

**验收**：

- `surf catlog` 输出包含 `Did you mean 'catalog'?`
- `surf market-pric` 输出包含 `Did you mean 'market-price'?`
- 完全不像的命令（`surf xyz`）仍然只报 unknown，不瞎建议

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf catlog
ERROR: Error: unknown command "catlog"
Run 'surf --help' for usage
$ surf market-pric
ERROR: Error: unknown command "market-pric"
Run 'surf --help' for usage

# After — Cobra's built-in suggestion kicks in
$ surf catlog
Error: unknown command "catlog" for "surf"

Did you mean this?
    catalog

Run 'surf --help' for usage.
$ surf market-pric
Error: unknown command "market-pric" for "surf"

Did you mean this?
    market-price

Run 'surf --help' for usage.

# Far-away typos don't get bogus suggestions
$ surf xyz
Error: unknown command "xyz" for "surf"
Run 'surf --help' for usage.
```

### 9. Contextual `help[]` suggestions in API response

**现状**：response 里没有 next-step 提示，agent 得靠训练数据或自己猜下一步调什么命令。

**目标**：成功和 error response 的 envelope 里加 `meta.help[]` 或 `error.help[]` 数组，每项是一个命令模板，参数用 `<name>` 占位符。见 §4.4。

**复杂度**：medium。需要 gateway 侧（Hermod）生成 help[]，CLI 只负责透传。也可以在 CLI 层根据当前命令 hardcode 一些规则（但不如后端生成灵活）。

**文件**：

- Gateway 侧：加 `meta.help` / `error.help` 生成逻辑（跨仓库协调）
- CLI 侧：`cli/formatter.go` 确保不过滤掉这些字段

**Breaking change**：否，additive。

**验收**：

- `surf wallet-detail --address 0x... --json | jq '.meta.help'` 有内容
- error response 也有 `error.help[]`
- `UNAUTHORIZED` 错误的 help 指向 `auth --api-key <key>`
- 模板参数用尖括号，agent 知道要替换

**Before → After**：

```
# Before (v1.0.0-alpha.26) — agent has to guess next step from command name
$ surf wallet-detail --address 0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045 --json
{
  "data": {
    "address": "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
    "ens": "vitalik.eth",
    "balance_usd": 12345.67
  },
  "meta": {"credits_used": 1}
}
# ^ agent has no hint about wallet-history, wallet-protocols, etc.

$ SURF_API_KEY=sk-0000... surf market-price --symbol BTC --time-range 1d
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid API key"
  }
}
# ^ agent sees UNAUTHORIZED but has to know the remediation command on its own

# After — `help[]` embedded in envelope
$ surf wallet-detail --address 0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045 --json
{
  "data": {
    "address": "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
    "ens": "vitalik.eth",
    "balance_usd": 12345.67
  },
  "meta": {
    "credits_used": 1,
    "help": [
      "See transaction history: surf wallet-history --address <address>",
      "List DeFi positions: surf wallet-protocols --address <address>",
      "Net worth over time: surf wallet-net-worth --address <address>"
    ]
  }
}

$ SURF_API_KEY=sk-0000... surf market-price --symbol BTC --time-range 1d
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid API key",
    "help": [
      "Save key: surf auth --api-key <key>",
      "Or export: export SURF_API_KEY=<key>",
      "Get a key: https://agents.asksurf.ai"
    ]
  }
}

# Agent picks next command without guessing
$ surf wallet-detail --address 0xd8dA... --json | jq -r '.meta.help[0]'
See transaction history: surf wallet-history --address <address>
```

---

## P2 (medium)

### 10. `-q, --quiet` 在 root 层缺失

**现状**：`surf --quiet market-price` 报 `unknown flag: --quiet` + exit 1。

**目标**：全局 `-q, --quiet` 抑制非错误 diagnostic 输出。见 §2.2。

**复杂度**：low。

**文件**：`cli/cli.go` 或 `cmd/surf/main.go` —— 加 persistent flag，在 `LogWarning`、`LogInfo` 等函数里 early return。

**验收**：

- `surf -q market-price --symbol BTC --json` 不打 WARN 行
- Error 还是正常输出

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf --quiet market-price --symbol BTC --time-range 1d
Error: unknown flag: --quiet
$ echo $?
1

# Without --quiet, retry warnings pollute stderr on 429/5xx
$ surf market-price --symbol BTC --time-range 1d 2>&1 | head -3
WARN: Got 429 Too Many Requests, retrying in 1s
WARN: Got 429 Too Many Requests, retrying in 2s
{
  "data": [...]

# After
$ surf --quiet market-price --symbol BTC --time-range 1d --json
{"data": [...], "meta": {...}}
# ^ clean stdout, no WARN lines on stderr

# Errors still show (not suppressed)
$ surf --quiet market-price 2>&1
Error: missing required flag: --symbol
$ echo $?
1
```

### 11. `--verbose` 长 flag 不存在

**现状**：`surf -v market-price` 工作，但 `surf --verbose market-price` 报 `unknown flag: --verbose`。长 flag 名和短 flag 名不对齐。

**目标**：`--verbose` 作为 `-v` 的 long form。见 §2.2。

**复杂度**：trivial（pflag alias）。

**文件**：`cli/cli.go` 注册 flag 的地方。

**验收**：`surf --verbose --help` 跟 `surf -v --help` 行为一致。

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf --verbose market-price --symbol BTC --time-range 1d
Error: unknown flag: --verbose
$ echo $?
1
$ surf -v market-price --symbol BTC --time-range 1d 2>&1 | head -2
DEBUG: API loading took 1.18ms
DEBUG: Configuration: map[api-name:surf ...]

# After — both forms work identically
$ surf --verbose market-price --symbol BTC --time-range 1d 2>&1 | head -2
DEBUG: API loading took 1.18ms
DEBUG: Configuration: map[api-name:surf ...]
$ surf -v market-price --symbol BTC --time-range 1d 2>&1 | head -2
DEBUG: API loading took 1.18ms
DEBUG: Configuration: map[api-name:surf ...]
```

### 12. `auth --clear` 无确认

**现状**：`surf auth --clear` 立即删 key，没有确认。实测 `echo n | surf auth --clear` 也直接清除，零 prompt。

**目标**：TTY 下显示 `Clear saved API key? [y/N]`，非 TTY 下要求 `--force` 才能执行。见 §9.3、§7.1。

**复杂度**：low。

**文件**：`cmd/surf/main.go` 的 `newAuthCmd()`。

**验收**：

- `surf auth --clear` 在 TTY 下 prompt
- `surf auth --clear --force` 跳过 prompt
- 非 TTY 下（`echo | surf auth --clear`）要求 `--force`

**Before → After**：

```
# Before (v1.0.0-alpha.26) — destructive action, no confirmation
$ surf auth
source:  system keychain
api-key: sk-b***********7196

$ surf auth --clear
API key cleared.
$ surf auth
No API key configured. ...
# Typing --clear by accident → key gone, no way back

# Even piping 'n' doesn't help
$ echo n | surf auth --clear
API key cleared.

# After (TTY) — prompt defaults to No
$ surf auth --clear
Clear saved API key? [y/N]: n
Cancelled.
$ surf auth
source:  system keychain
api-key: sk-b***********7196
# ^ key preserved

$ surf auth --clear
Clear saved API key? [y/N]: y
API key cleared.

# Non-TTY — refuse unless --force
$ echo | surf auth --clear
Error: --clear requires confirmation; pass --force to skip (non-interactive mode)
$ echo $?
1

# --force skips prompt everywhere
$ surf auth --clear --force
API key cleared.
```

### 13. Hardcoded URL 应该支持 env var override

**现状**：三个 URL hardcoded 在代码里，代码搜索零 `SURF_GATEWAY_URL` / `SURF_CATALOG_URL` / `SURF_CDN_BASE` 引用：

- Gateway: `https://api.asksurf.ai/gateway`（`cmd/surf/main.go:286`）
- Catalog metadata: `https://metadata.asksurf.ai/data-catalog/swell_data_catalog.json`（`cmd/surf/catalog.go:16`）
- Install CDN: `https://downloads.asksurf.ai/cli/releases`（`cmd/surf/install.go:17`）

**目标**：支持 `SURF_GATEWAY_URL`、`SURF_CATALOG_URL`、`SURF_CDN_BASE` 环境变量 override。见 §8.3。

**复杂度**：low。

**文件**：`cmd/surf/main.go`、`cmd/surf/catalog.go`、`cmd/surf/install.go`。

**验收**：

- `SURF_GATEWAY_URL=http://localhost:8080 surf market-price --symbol BTC` 打到 localhost
- 不设 env var 行为不变

**Before → After**：

```
# Before (v1.0.0-alpha.26) — env vars ignored, URLs are hardcoded
$ SURF_GATEWAY_URL=http://localhost:8080 surf market-price --symbol BTC --time-range 1d -v 2>&1 | grep Host
Host: api.asksurf.ai
# ^ still hits production

$ grep -n "asksurf" cmd/surf/main.go cmd/surf/catalog.go cmd/surf/install.go
cmd/surf/main.go:286:    cli.Load("https://api.asksurf.ai/gateway", cli.Root)
cmd/surf/catalog.go:16:  const catalogURL = "https://metadata.asksurf.ai/data-catalog/..."
cmd/surf/install.go:17:  const cdnBase = "https://downloads.asksurf.ai/cli/releases"

# After — env vars override hardcoded defaults
$ SURF_GATEWAY_URL=http://localhost:8080 surf market-price --symbol BTC --time-range 1d -v 2>&1 | grep Host
Host: localhost:8080
# ^ hits local gateway

$ SURF_CATALOG_URL=http://localhost:9000/catalog.json surf catalog list -v 2>&1 | grep fetching
DEBUG: fetching catalog from http://localhost:9000/catalog.json

$ SURF_CDN_BASE=http://localhost:3000/releases surf install -v 2>&1 | grep Downloading
Downloading surf v1.0.0-alpha.27 for darwin/arm64 from http://localhost:3000/releases/...

# No env var → default hardcoded URL, behavior unchanged
$ surf market-price --symbol BTC --time-range 1d -v 2>&1 | grep Host
Host: api.asksurf.ai
```

### 14. Telemetry 首次运行无告知

**现状**：首次运行时静默启用 PostHog telemetry。代码搜索确认 `cli/telemetry.go` 里零 first-run notice 逻辑。

**目标**：首次运行打一行 notice 到 stderr，说明收集什么、怎么禁用。见 §10.1。

**复杂度**：low。需要一个 "是否是首次运行" 的 flag 文件（比如 `~/.surf/telemetry_notice_shown`）。

**文件**：`cli/telemetry.go`、`cmd/surf/main.go`。

**验收**：

- 第一次跑 `surf` 时 stderr 打一行 notice
- 之后跑不再打
- 设 `SURF_DO_NOT_TRACK=1` 时跳过 notice 和 tracking

**Before → After**：

```
# Before (v1.0.0-alpha.26) — silent opt-in, user never knows
$ rm -rf ~/.surf
$ surf market-price --symbol BTC --time-range 1d --json
{"data": [...], "meta": {...}}
# ^ telemetry enabled, user never told

$ ls ~/.surf
apis.json   device_id   session.json   surf.cbor
# Device ID and session file created silently

# After — first-run notice on stderr, one time only
$ rm -rf ~/.surf
$ surf market-price --symbol BTC --time-range 1d --json
surf collects anonymous usage data (command name, version, exit code, duration)
to improve the CLI. To opt out: export SURF_DO_NOT_TRACK=1
See: https://docs.asksurf.ai/privacy
{"data": [...], "meta": {...}}

$ ls ~/.surf
apis.json   device_id   session.json   surf.cbor   telemetry_notice_shown

# Second run — no notice
$ surf market-price --symbol BTC --time-range 1d --json
{"data": [...], "meta": {...}}

# Opt-out → no notice, no tracking
$ rm -rf ~/.surf
$ SURF_DO_NOT_TRACK=1 surf market-price --symbol BTC --time-range 1d --json
{"data": [...], "meta": {...}}
$ ls ~/.surf
apis.json   surf.cbor
# ^ no device_id, no session.json (tracking off)
```

### 15. 生成的 operation 命令空结果处理不明确（未验证）

**现状**：⚠️ **当前版本未能验证**——review 时 credit 耗尽，所有 operation 调用返回 `INSUFFICIENT_CREDIT`，没法触发真正的空结果路径。遗留的描述是 "readable 模式输出格式不一致，有些打 `[]` 有些打空"，但没有可复现的证据。

**目标**：所有 operation 命令在 `--json` 下打完整 envelope `{"data": [], "meta": {...}}`，在 readable 模式下 stderr 打 `No results.`。见 §3.9。

**复杂度**：low（前提是问题真的存在）。

**文件**：`cli/formatter.go`。

**验收**：

- 先复现当前行为，确认是不是真问题
- 如果是：`surf polymarket-events --event-slug nonexistent --json | jq '.data'` 返回 `[]`，readable 模式下 stderr 打 `No results.`
- 如果不是：从 review 里删掉这一项

**Before → After**（假设的问题行为，待验证）：

```
# Before (hypothetical — needs credit to verify)
$ surf polymarket-events --event-slug nonexistent-slug-12345 --json
{}
# ^ possibly an empty object, no data or meta keys

$ surf polymarket-events --event-slug nonexistent-slug-12345
# ^ possibly nothing on stdout, nothing on stderr

# After — consistent envelope and readable message
$ surf polymarket-events --event-slug nonexistent --json
{
  "data": [],
  "meta": {
    "total": 0,
    "credits_used": 1,
    "empty_reason": "no events matching slug"
  }
}
$ echo $?
0

$ surf polymarket-events --event-slug nonexistent
No results.
$ echo $?
0
# ^ stderr has the message, stdout empty; agent branches on exit 0 + empty stdout
```

### 16. `surf auth` 没有 `--json` 输出

**现状**：`surf auth` 打印纯文本状态：

```
$ surf auth
source:  system keychain
api-key: sk-b************************************************7196
```

Agent 想 programmatic 探测 auth 状态（"有没有配 key？key 是哪种类型？"）时只能 parse 这个纯文本。

**目标**：`surf auth --json` 输出结构化 JSON：

```json
{
  "source": "env|keychain|file|none",
  "key_type": "user|deploy|sees|unknown",
  "api_key_masked": "sk-b****...7196"
}
```

`source: "none"` 时 `key_type` 和 `api_key_masked` 为 `null`。见 §5.4（content-first）和 §3.2（`--json` 统一语义）。

**复杂度**：low。

**文件**：`cmd/surf/main.go` 的 `newAuthCmd()`。

**Breaking change**：否，additive。

**验收**：

- `surf auth --json | jq '.source'` 返回字符串
- 没配 key 时 `jq '.source'` 返回 `"none"`
- `source` 是固定枚举，agent 可以直接 switch

**Before → After**：

```
# Before (v1.0.0-alpha.26) — plain text, hard to parse
$ surf auth
source:  system keychain
api-key: sk-b***********************************************7196
$ surf auth --json
Error: unknown flag: --json

# Agent has to regex the plain text to extract source
$ surf auth | grep -oE "source:\s+\S+" | awk '{print $2}'
system

# After — structured JSON output
$ surf auth --json
{
  "source": "keychain",
  "key_type": "user",
  "api_key_masked": "sk-b****...7196"
}

# No key configured
$ SURF_API_KEY= surf auth --json  # and no keychain entry
{
  "source": "none",
  "key_type": null,
  "api_key_masked": null
}

# Env var takes precedence
$ SURF_API_KEY=sk-1111...1111 surf auth --json
{
  "source": "env",
  "key_type": "user",
  "api_key_masked": "sk-1***...1111"
}

# Agent uses it for programmatic branching
$ case "$(surf auth --json | jq -r '.source')" in
    none)     echo "Need to configure key" ;;
    env)      echo "Using SURF_API_KEY env var" ;;
    keychain) echo "Using saved key" ;;
  esac
```

### 17. `list-operations` 没有 `--plain` tab 分隔输出

**现状**：`surf list-operations` 输出是左对齐 tabular 格式，用空格填充：

```
  GET    market-price    Token Price History  (--symbol, --time-range, ...)
```

`awk '{print $2}'` 意外能 work（因为多空格 collapse），但 `cut -f` 不工作（不是 tab 分隔），agent 脚本不稳定。

**目标**：加 `--plain` flag 输出 tab 分隔：

```
GET\tmarket-price\tToken Price History\t--symbol,--time-range,--from,--to,--currency
```

按 clig.dev output 章节 "If you're displaying a table of data, provide `--plain` to output as tab-separated values"。

**复杂度**：trivial。

**文件**：`cmd/surf/main.go` 的 `newListOperationsCmd()` 和 `printOperationsFlat()` / `printOperationsGrouped()`。

**Breaking change**：否，additive。

**验收**：

- `surf list-operations --plain | cut -f 2` 返回所有命令名一列
- `--plain` 跟 `--group`、`--detail`、`--category` 可组合
- 不传 `--plain` 时输出不变

**Before → After**：

```
# Before (v1.0.0-alpha.26) — space-padded alignment
$ surf list-operations | head -3
  GET    exchange-depth                      Exchange Order Book Depth  (--pair, --type, --limit, --exchange)
  GET    exchange-funding-history            Exchange Funding Rate History  (--pair, --from, --limit, --exchange)
  GET    exchange-klines                     Exchange OHLCV Candlesticks  (--pair, --type, --interval, --from, --limit, --exchange)

# awk with default whitespace splitter happens to work
$ surf list-operations | head -1 | awk '{print $2}'
exchange-depth

# But cut -f is broken (no tabs in output)
$ surf list-operations | head -1 | cut -f 2
  GET    exchange-depth                      Exchange Order Book Depth  (--pair, --type, --limit, --exchange)

# Agent scripts often prefer cut -f for predictability
# → have to fall back to jq over JSON API (slower) or parsing by column offset

# After — --plain gives true tab-separated values
$ surf list-operations --plain | head -3
GET	exchange-depth	Exchange Order Book Depth	--pair,--type,--limit,--exchange
GET	exchange-funding-history	Exchange Funding Rate History	--pair,--from,--limit,--exchange
GET	exchange-klines	Exchange OHLCV Candlesticks	--pair,--type,--interval,--from,--limit,--exchange

$ surf list-operations --plain | cut -f 2 | head -3
exchange-depth
exchange-funding-history
exchange-klines

# Combines with other flags
$ surf list-operations --plain --category exchange | cut -f 2
exchange-depth
exchange-funding-history
exchange-klines
exchange-long-short-ratio
exchange-markets
exchange-perp
exchange-price
```

---

## P3 (low)

### 18. `--help` 输出分页

**现状**：长 `--help`（超过终端高度）直接 dump 到屏幕，需要用户自己 scroll 或 `| less`。

**目标**：当 stdout 是 TTY 且输出超过终端高度时自动走 `less -FIRX`。见 §3.4。

**复杂度**：low。

**文件**：`cli/cli.go` 或在 Root 上设 `SetHelpFunc`。

**Before → After**：

```
# Before (v1.0.0-alpha.26) — 95+ lines dump over small terminals
$ surf --help | wc -l
95
$ surf --help
# User sees last screen, has to scroll up manually to find the beginning
# Workaround: surf --help | less

# After — auto-pipes through less -FIRX when TTY and content > screen height
$ surf --help
# If content fits: printed directly (less -F = quit if one screen)
# If content > screen: opens in less, q to quit, colors preserved (-R)

# Non-TTY (pipes, scripts) — unchanged, no pager
$ surf --help | wc -l
95
$ surf --help | head -5
Query the Surf data platform — crypto market data, on-chain analytics, and more.

Usage:
  surf [flags]
  surf [command]
```

### 19. `--help` footer 加 doc link

**现状**：`surf --help` 底部没有指向 `https://docs.asksurf.ai` 的 link。实测 tail 10 行都没出现 `docs.asksurf`。

**目标**：footer 一行 `Documentation: https://docs.asksurf.ai`。见 §5.1。

**复杂度**：trivial（改 help template）。

**文件**：`cli/cli.go`。

**Before → After**：

```
# Before (v1.0.0-alpha.26)
$ surf --help | tail -5
  -h, --help      help for surf
      --version   version for surf

Use "surf [command] --help" for more information about a command.

$ surf --help | grep -i docs
# (empty — no documentation link anywhere in help)

# After
$ surf --help | tail -7
  -h, --help      help for surf
      --version   version for surf

Use "surf [command] --help" for more information about a command.

Documentation: https://docs.asksurf.ai
Report issues: https://github.com/cyberconnecthq/surf-cli/issues
```

### 20. SIGINT/SIGTERM handling

**现状**：代码搜索确认 `cmd/surf/` 和 `cli/` 里零 `signal.Notify` / `SIGINT` / `SIGTERM`。Ctrl-C 直接 kill 进程，in-flight HTTP 请求没被 cancel，telemetry 没 flush。

**目标**：`SIGINT` → stderr 打 `interrupted`，cancel context，flush telemetry，exit 130。见 §11.1。

**复杂度**：medium。需要 context 贯穿 HTTP 请求栈（restish 默认可能没传 context）。

**文件**：`cmd/surf/main.go`（signal handler 注册）、`cli/request.go`（context cancellation 支持）。

**Before → After**：

```
# Before (v1.0.0-alpha.26) — Ctrl-C kills immediately, no cleanup
$ surf market-price --symbol BTC --time-range 365d &
[1] 12345
$ kill -INT %1
[1]+  Interrupt: 2  surf market-price --symbol BTC --time-range 365d
$ echo $?
0
# ^ HTTP request was mid-flight, connection just died
# ^ telemetry event not flushed to PostHog
# ^ no stderr message, user/agent can't distinguish "killed" from "finished"

# After — graceful shutdown
$ surf market-price --symbol BTC --time-range 365d &
[1] 12345
$ kill -INT %1
interrupted
[1]+  Exit 130  surf market-price --symbol BTC --time-range 365d
# ^ stderr prints "interrupted"
# ^ HTTP request canceled via context (server-side connection cleanly closed)
# ^ telemetry flushed (up to 2s budget)
# ^ exit 130 (128 + SIGINT=2), matching POSIX convention

# Double Ctrl-C skips cleanup, force-exit
$ surf market-price --symbol BTC --time-range 365d &
$ kill -INT %1; sleep 0.5; kill -INT %1
interrupted
forced exit
$ wait; echo $?
130
```

### 21. Unexpected error 预填 bug report URL

**现状**：代码搜索确认 `cmd/surf/main.go` 里零 `recover()` 逻辑。CLI panic 或 unexpected error 时只打默认 Go stack trace，用户不知道怎么报 bug。

**目标**：打印一个预填好 `title`、`body` 参数的 GitHub issue URL，用户点击直接打开 pre-filled 的新 issue 表单。见 §4.2.3。

**复杂度**：low。

**文件**：`cmd/surf/main.go`（defer recover）。

**Before → After**：

```
# Before (v1.0.0-alpha.26) — raw Go panic, no reporting guidance
$ surf market-price --symbol 'malformed-causes-panic'
panic: runtime error: index out of range [5] with length 3

goroutine 1 [running]:
main.main()
        /Users/runner/work/surf-cli/cmd/surf/main.go:147 +0x80
github.com/spf13/cobra.(*Command).execute(0x14000110008, {0x14000011140, 0x2, 0x2})
        /root/go/pkg/mod/github.com/spf13/cobra@v1.8.0/command.go:987 +0x8a4
...
exit status 2
# ^ user has no idea what to do next

# After — graceful panic handler with pre-filled issue URL
$ surf market-price --symbol 'malformed-causes-panic'
Surf CLI encountered an unexpected error and needs to be reported.

Error: runtime error: index out of range [5] with length 3

To file a bug report (takes 10 seconds — fields pre-filled):
  https://github.com/cyberconnecthq/surf-cli/issues/new?template=bug.md&title=panic%3A+runtime+error+index+out+of+range&body=Version%3A+v1.0.0-alpha.26%0AOS%3A+darwin%2Farm64%0ACommand%3A+market-price+--symbol+malformed-causes-panic%0A%0AError%3A+runtime+error%3A+index+out+of+range+%5B5%5D+with+length+3%0A%0AStack+trace%3A%0A...

Run with -v for full stack trace.
$ echo $?
1
```

---

## 已核验但 **不是问题** 的项（从 review 中删除）

这些在早期 review 里被标记为问题，但经过在 `v1.0.0-alpha.26` 的真实测试后确认已经 work，不是 bug：

- **Root 层 unknown flag exit 0**：误报。所有 unknown flag（root 或 subcommand）都返回 exit 1。之前看到 "exit 0" 是 `| tail -N; echo $?` 的测试陷阱——`$?` 拿到的是 `tail` 的退出码 0，不是 `surf` 的。
- **Stdin `-` 约定**：实际上 restish 的 shorthand body parser 已经自动读 piped stdin（`echo '...' | surf onchain-sql` 直接工作）。`-` 字面量作为 filename 占位符在当前使用场景里不是必需的。

---

## 实施顺序建议

### Sprint 1 — 消除 P0 + 最容易的 P1

- #1 `--json` 一致性 (P0)
- #2 SKILL.md API error stdout 同步（跨 repo，并行）
- #3 网络错误 exit code (P0)
- #4 Error envelope 统一 code 枚举 (P1，跟 #3 一起做)
- #5 Strip schema blocks from operation help (P1)
- #8 Typo suggestion (P1)

P0 的 #1 #3 + P1 的 #4 可以打包成一个 "error routing & JSON flag" PR。#5 #8 单独走 quick wins。

### Sprint 2 — 动 response shape 的大改

- #6 Minimal default fields + `--fields`
- #7 Content truncation + `--full`
- #9 Contextual `help[]` suggestions

这三个要一起设计，都是动 formatter 层。#6 和 #7 需要上游 spec 先加 annotation，所以这个 sprint 需要跨仓协调。推荐先走 opt-in env var (`SURF_MINIMAL_FIELDS=1`) 避免 breaking，等 agent runtime 和 SKILL.md 都切换完再 default on。

### Sprint 3 — 标准 flag 和 polish

- #10 `-q, --quiet`
- #11 `--verbose` 长 flag
- #12 `auth --clear` 确认
- #13 Env var URL overrides
- #14 Telemetry 首次告知
- #15 Empty state consistency（先验证问题是否存在）
- #16 `surf auth --json`
- #17 `list-operations --plain`

都是 low/trivial 复杂度的 polish，可以分批塞到日常 PR 里。#16 #17 是 agent 脚本友好度的提升。

### Sprint 4 — Nice-to-have

- #18 Help pagination
- #19 Footer doc link
- #20 Signal handling
- #21 Bug report URL

这些不紧急，看精力做。

---

## 跟踪

在这份文档里用 checkbox 标进度，每个 PR 合并后把对应项打勾：

- [x] #1 `--json` flag 一致性 — [PR #69](https://github.com/cyberconnecthq/surf-cli/pull/69)
- [ ] #2 SKILL.md API error stdout 同步
- [x] #3 网络错误 exit code — [PR #70](https://github.com/cyberconnecthq/surf-cli/pull/70)
- [x] #4 Error envelope 统一 code 枚举 — [PR #70](https://github.com/cyberconnecthq/surf-cli/pull/70)
- [x] #5 Strip schema blocks — [PR #71](https://github.com/cyberconnecthq/surf-cli/pull/71)
- [ ] #6 Minimal default fields
- [ ] #7 Content truncation
- [x] #8 Typo suggestion — [PR #90](https://github.com/cyberconnecthq/surf-cli/pull/90)
- [ ] #9 Contextual help[]
- [ ] #10 `-q, --quiet`
- [ ] #11 `--verbose` 长 flag
- [ ] #12 `auth --clear` 确认
- [ ] #13 Env var URL overrides
- [ ] #14 Telemetry 首次告知
- [ ] #15 Empty state consistency（先验证）
- [ ] #16 `surf auth --json`
- [ ] #17 `list-operations --plain`
- [ ] #18 Help pagination
- [ ] #19 Footer doc link
- [ ] #20 Signal handling
- [ ] #21 Bug report URL

---

## 参考

- [`CLI_DESIGN_PRINCIPLES.md`](./CLI_DESIGN_PRINCIPLES.md) —— 这份 review 的设计依据。
- [clig.dev](https://clig.dev/) —— 通用 CLI 工艺基线。
- [axi.md](https://axi.md/) —— Agent eXperience Interface，agent 优化 CLI 原则。
- [SKILL.md](https://github.com/asksurf-ai/surf-skills/blob/main/skills/surf/SKILL.md) —— agent 训练文档，改动 CLI 行为时要同步更新。
