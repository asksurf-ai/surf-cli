# Surf CLI 设计原则

这份文档面向所有设计、review 或改动 Surf CLI 的人。

**Surf CLI 是 agent-first 的。** 我们的主要用户是跑在 benchmark harness、自动化 pipeline、交互式编码工具里的 LLM agent。人类是次要用户——他们可以用 `surf`，但每个设计决策首先按 **对 agent 的影响** 来评估：token 效率、输出可预测性、exit code 严格性、上下文窗口友好度。这跟 clig.dev 的 "human-first，机器可解析是附带好处" 框架正好相反。对 Surf CLI 来说，**agent 不是次要用户——agent 就是用户**。

我们的原则源自两个文档：

- [**clig.dev**](https://clig.dev/) —— 通用 CLI 工艺基线：stdout/stderr 分离、exit code、配置层级、help 结构、signal 处理。
- [**axi.md**](https://axi.md/) —— Agent eXperience Interface 框架，10 条经过实证验证的 agent 友好 CLI 原则：最小默认 schema、内容截断、预计算聚合、明确的空结果状态、ambient context、content-first 显示、contextual next-step suggestions。

两者冲突时，我们选 agent 优化的那一边，并在 §15 记录偏离理由。

## 术语约定

- **Surf CLI** —— 这份文档讨论的工具的正式名字。
- **`surf`**（小写 + 代码字体）—— 用户在 shell 里敲的二进制/命令字面量（例如 `surf market-price --symbol BTC`）。
- **surf-cli**、**surf-skills** —— GitHub repo 名字，保留小写。

## 文档状态

- **面向读者**：CLI 维护者、PR reviewer、`@surf-ai/sdk` 贡献者、agent skill 作者（`surf-skills` 里的 SKILL.md）。
- **怎么用**：review PR 时引用具体章节（"这违反了 §5 exit code 合同"）。approve 前过一遍底部的 review checklist。
- **Living document**：改动走 PR 流程，跟别的文件一样。如果现有行为跟某条原则冲突，**原则胜出**——要么改代码，要么改原则（并在 PR 里说明理由）。
- **Aspirational vs current**：还没落地的原则标 `(target)`，其他都是已经在执行的。

---

## 1. 核心哲学

五条原则，按优先级排序。所有五条都从 "一个 LLM agent 在程序化地调用 `surf`" 这个视角来看。

### 1.1 Agent-first 输出

stdout 上的每个字节都在消耗 agent 的 token。默认输出必须结构可预测、冗余最小、可以安全地 pipe 给下游工具。对人类的可读性是加分项，对 agent 的可解析性是硬性要求。我们不加装饰 banner、boxed table、多段落的成功消息。成功路径给数据，错误路径给结构化 JSON envelope。

Surf CLI 最大的用户群是跑在 benchmark harness 和编码工具里的 LLM agent。他们读不懂模糊的散文，也 recover 不了歧义的 exit code，更不能因为我们输出不一致而浪费一个 turn 去 retry。

### 1.2 一致性是合同

Flag 名字、error code、exit code、输出结构都是 agent 训练数据的一部分。一个 flag 名一旦 ship 出去，就被下游所有 skill、prompt、example 锁死。**改名等于 breaking change**，哪怕行为没变。加一个新 flag 是免费的，改一个老 flag 不是。

同一个概念必须在每个 subcommand 里用同一个名字：`--json`、`--limit`、`--category`、`--force` 在任何地方含义一致。造一个同义词是 "clever"，而 cleverness 会 break agent。

### 1.3 Fail loud, fail structured

任何 error path 都必须返回非零 exit code **且** 在 stderr 上给出结构化的 error body。不 swallow 错误成空输出——agent 把空输出读成"没有结果"然后编造答案。Error body 里必须同时包含机器可解析的 `error.code` 和人类可读的 `error.message`。

### 1.4 可从 CLI 自身 discover

agent 不读外部文档。所有用 `surf` 需要知道的东西必须能从 CLI 自己的输出里获取：`surf --help`、`surf list-operations`、`surf <command> --help`、`surf catalog`、`surf auth`。`surf-skills` 里的 SKILL.md 是配套的训练文档，但它 **派生自** `surf` 自己的输出，不是替代品。

### 1.5 Content first, help second

运行一个命令应该产生 actionable data，不是 usage 文本。当一个 subcommand 有合理的默认行为时，就用那个默认行为：

- `surf list-operations` 列 operation，不显示 help。
- `surf catalog list` 列 table，不显示 help。
- `surf auth` 显示当前 auth 状态，不显示 help。

`--help` 是 fallback，不是 "零参数调用" 的默认响应——前提是存在合理的默认行为。（参见 §5.1，对于 root `surf` 这条不适用。）

---

## 2. 参数和 flag

### 2.1 命名

- 所有 flag 名字用 **kebab-case**（`--market-ticker`，不是 `--market_ticker` 也不是 `--marketTicker`）。
- snake_case 在解析时静默 normalize 成 kebab-case（见 `cli.NormalizeSnakeCaseFlags`）。这是兼容补丁——**不要在 spec 里新造 snake_case flag**。
- 长 flag 名永远存在。短 flag（`-h`、`-v`、`-o`）只给最常用的全局 flag 提供。

### 2.2 标准 flag

以下名字保留给它们的约定含义：

| Flag                 | 含义                                          |
| -------------------- | --------------------------------------------- |
| `-h, --help`         | 打印当前命令的 help                           |
| `--version`          | 打印版本号退出                                |
| `-v, --verbose`      | 更详细的 diagnostic 输出（可重复）            |
| `-q, --quiet`        | 抑制非错误 diagnostic 输出                    |
| `--json`             | 机器可读的 JSON 输出                          |
| `-o, --output <fmt>` | 输出格式选择（json、readable 等）             |
| `-f, --force`        | 跳过确认 prompt                               |
| `--no-input`         | 不 prompt，直接 fail；非 TTY stdin 下自动生效 |
| `-p, --profile`      | 选择 auth profile                             |

当前差距 (target)：

- `-q, --quiet` 在 root 层没实现。
- `--verbose`（长名字）不接受，只有 `-v`。
- `--json` 不一致：`catalog list --json` 能用，但 `market-price --json` 报 unknown。需要在 operation 命令上也支持 `--json` 作为 `-o json` 的 alias，输出完整 response JSON。

### 2.3 必填参数

必填参数必须在发 HTTP 请求之前做 **客户端校验**。CLI 不能依赖后端返回 422 来暴露缺字段。参考 `cli/operation.go` 的现有实现。

### 2.4 Secrets

Secret 不允许通过普通 flag 值传递，除非是一次性 setup 流程。目前的例外是 `surf auth --api-key sk-...`，因为这个命令一般在安装时跑一次。**不要再加其他接受 secret 值的 flag**——用 stdin、文件引用或 keyring。

### 2.5 位置参数

**避免位置参数。** 只有两种可接受的情况：

1. 跟在 `noun verb` subcommand pair 后面的单个必填目标（例如 `surf catalog show <table>`）。
2. OpenAPI path parameter 自动生成的 operation 命令，且 path 只有一个 `{id}` 占位符。

**不要用多个不同类型的位置参数**（`cp source dest` 这种例外不适用于 Surf CLI）。

---

## 3. 输出

### 3.1 流分离

分流规则按 **输出的结构** 决定，不是按 "是成功还是失败" 决定。这是 Surf CLI 作为 agent-first CLI 的关键差异——对齐 axi.md #6，区别于 clig.dev 的 "errors 到 stderr" 传统。

**stdout** 放结构化输出：

- 成功的主要数据（JSON envelope、table 行、parsed response）
- **API error envelope**（`{"error": {"code": ..., "message": ...}}`）—— 即使是 error，只要它是结构化的 JSON，就跟成功数据走同一个流，让 agent 用同一个 parse pipeline 处理

**stderr** 放非结构化诊断：

- CLI 错误（unknown flag、parse error、filesystem error 等纯文本）
- Diagnostic log（`-v` 下的 debug）
- 进度条、spinner（`install` 等长操作）
- 确认 prompt（`auth --clear` 等交互）
- Warning、deprecation notice

Agent 的典型 pipeline 是 `surf ... --json | jq '.error // .data'`，依赖 stdout 是单一的结构化流。把 API error envelope 也放 stdout 是为了这个 pipeline 能工作——否则 agent 要同时 capture stdout 和 stderr、union 起来才能看到全貌。

人类用户对 stderr 的期待是 "跟数据无关的日志"——CLI error 放 stderr 符合这种期待。Agent 看 exit 1 就知道 "我的调用有问题"，不需要 parse 那行纯文本。

### 3.2 输出格式

支持的格式：`readable`（TTY 默认）、`json`（非 TTY 默认）、`table`、`yaml`、`gron`。

- `--json` 是 `-o json` 的 alias，在所有命令上输出 **完整 response JSON**（包含 `data`、`meta`、`error` 等顶层字段）。不做 envelope stripping——agent 可以用 `jq` 提取需要的字段。
- `-o auto` 在 TTY 下选 `readable`，非 TTY 下选 `json`。
- JSON 输出结构被视为稳定的 API surface。参见 §14。

### 3.3 提取字段（用 jq）

标准用法是 `--json` 管 `jq`。这是行业约定（gh、stripe、aws CLI 都用这种模式），agent 训练数据容易迁移，shell 生态工具链友好。

常用模式：

```sh
# 只要数据数组
surf market-price --symbol BTC --json | jq '.data'

# 取第一项的某个字段
surf market-price --symbol BTC --json | jq '.data[0].value'

# 统一处理成功和失败路径
surf market-price --symbol BTC --json | jq '.error // .data'

# 列 meta 里的 credit 使用
surf market-price --symbol BTC --json | jq '.meta.credits_used'
```

成功 response 有 `.data` 和 `.meta`，error response 有 `.error.code` 和 `.error.message`。两者在同一个顶层 envelope 里，`jq` 可以用 alternative operator `//` 统一处理。

> 注：restish 底层支持 `-f` shorthand filter，但这不是推荐用法也不在 agent 训练数据里。新命令、新文档、新示例一律用 `--json | jq`。

### 3.4 Help 输出分页

长 `--help`（超出终端高度）应该走 `less -FIRX`。`less -FIRX` 的行为是：内容够一屏就直接打，否则分页。(target)

### 3.5 Operation help 内容

OpenAPI spec 的描述里常常有 `## Option Schema` 段和 `## Response 200` 段带 JSON schema。Cobra 的 `Flags:` 段已经列了所有 flag——schema 段对 CLI 用户是重复噪声。设置到 Cobra 命令上之前，`op.Long` 必须把这些段剥掉。(target —— 参见 review finding §5)

### 3.6 最小默认 response shape (AXI #2)

一个 agent 调 `surf market-price` 并不需要每次都拿完整的 API envelope。Response body 里包含 `$schema` pointer、`data` 数组（每项 4-6 个字段）、带缓存提示的 `meta` block——大部分都是 agent 反正要丢掉的噪声。

目标行为：

- 默认输出每项 **3-5 个最有用的字段**，不是全部 schema。
- `--fields a,b,c` 显式选字段。
- `--fields all` 返回未截断的完整 response envelope。
- `--fields` 接受逗号分隔列表；未知字段是 validation error，不是静默 no-op。

当前行为：返回完整 response envelope。最小 schema 是 **target**，还没强制执行。落地需要一个 formatter hook 来理解每个 operation 的 response 形状（大概率要从 OpenAPI spec 里拿 default fields 的 annotation），同时保留 `--fields all` 回到完整 envelope。

### 3.7 内容截断 (AXI #3)

长文本字段——`description`、新闻文章 `body`、catalog `sample_queries`、OpenAPI `x-cli-description`——可以轻易吃掉单次调用几千个 token。默认行为必须把它们截断到固定预算（大约 500 字符）并附加一个提示：

```
"description": "Lorem ipsum dolor sit amet ... (truncated, 2847 chars total — use --full to see complete body)"
```

`--full` 关闭当前调用的截断。表格、整数、时间戳和标量字段从不截断。**(target)**

### 3.8 预计算聚合 (AXI #4)

list response 如果有 pagination，顶层 body 必须包含真实的 total count，不只是当前页大小。否则 agent 得多发一次调用才能知道 "还有几页"。后端 gateway 大部分 endpoint 已经在 `meta.total` 里填了这个值——这条原则要求：**不要在 CLI 层把它剥掉**，而且在非 `--json` 模式下要显眼地展示它。

### 3.9 明确的空结果状态 (AXI #5)

当一个查询返回零结果时，输出必须是一条明确的 "零结果" 消息，不是空输出。agent 把空 stdout 读成 "调用静默失败" 然后要么 retry loop、要么编造答案。

当前符合规范的例子：

- `surf catalog search crypto` → `No tables matching "crypto"`
- Filter 出零行 → stderr 上显式的消息。

生成的 operation 命令也要遵守：API 返回 `data: []` 时，CLI 在 `--json` 下打印完整 envelope `{"data": [], "meta": {...}}`，在 readable 模式下在 stderr 上打印一行可读的 `No results.`。

---

## 4. 错误和 exit code

### 4.1 Exit code 合同

四个值，不多不少：

| Exit  | 含义     | 触发                                                                         |
| ----- | -------- | ---------------------------------------------------------------------------- |
| `0`   | 成功     | HTTP 2xx 且没有 CLI 错误                                                     |
| `1`   | CLI 错误 | unknown command、unknown flag、缺 required flag、parse error、文件系统错误等 |
| `4`   | API 错误 | HTTP 非 2xx（3xx、4xx、5xx 都是 `4`）                                        |
| `130` | 被中断   | 收到 SIGINT 或 SIGTERM                                                       |

这份文档是 exit code 的唯一 source of truth。SKILL.md 是 CLI 的下游文档，必须跟这份对齐——两者不一致时，这里是对的，SKILL.md 要修。

#### 设计理由

**为什么用 1 vs 4 的 split，而不是只用 0/1**：

Agent 做 retry 决策时需要区分 "我的 invocation 错了" 和 "API 出问题了"：

- exit 1 → 不要 retry，检查你的 flag
- exit 4 → 可能 retry（进一步看 `error.code`：429 retry、401 不 retry）

两类错误的处理路径完全不同。只用 0/1 的话 agent 得 parse stderr 或 stdout 的 error body 才能做分支，多了一步解析且容易出错。4 值是最小够用的分类粒度。

**为什么是 `4` 而不是 `2`（bash 风格）或 `69`（sysexits）**：

- `2` 在 shell 上下文有约定含义（"misuse of shell builtin"），会跟 bash 内建的退出值混淆。
- sysexits.h 的 `69 EX_UNAVAILABLE` 语义更准确，但这套约定几乎没有主流 CLI 在用。
- `4` 是历史选择（早期 restish 按 HTTP class 返回 3/4/5，之前统一成 4 因为 4xx 最常见），现在作为 Surf CLI 的约定保留。
- Alpha 阶段我们**可以**改，但没有足够理由——`4` 已经在文档里、代码里、SKILL.md 里一致。

**为什么不按 HTTP class 继续分（3/4/5）**：

Agent 靠 stdout 里的 `error.code` 做细粒度判断，exit code 只需要给一个 "粗分类" 信号。把 3xx、4xx、5xx 合并成 exit 4 简化了 agent 的 retry 策略分支。

**为什么 `130` 是 POSIX 约定**：

`128 + SIGINT(2) = 130`。所有 Unix 工具都用这个值，没有理由偏离。

#### 已知 bug (target)

`surf --unknown-flag market-price …` 目前返回 exit 0，因为 root 层的 flag parse error 没被映射到非零 exit。修法：`cmd/surf/main.go` 里 `cli.Run()` 的 error 必须强制 exit 1。

### 4.2 Error message 格式

Error 分两类，格式不同，目标流不同。

#### 4.2.1 API error（stdout）

来自后端的结构化错误（HTTP 非 2xx）。必须保留原始 JSON envelope 输出到 **stdout**：

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid API key"
  }
}
```

Agent 基于 `error.code` 做程序化分支。代码枚举值稳定（见 §14.4）。CLI **不改写、不翻译** 后端的 error message——后端 API 是 error code 和 message 文本的 source of truth，CLI 只负责原样透传，SKILL.md 里对应的错误处理说明也从后端/CLI 的实际行为派生。

Exit code 固定 4。

#### 4.2.2 CLI error（stderr）

本地产生的错误：unknown flag、missing required flag、parse error、filesystem error、network error（连不上 gateway）等。这些是非结构化的，写到 **stderr**，按 clig.dev 风格组织：

1. **What**：一行发生了什么（`Error: missing required flag: --symbol`）。
2. **How**：指向 `--help` 或 corrective 命令的提示（`See: surf market-price --help`）。
3. **Suggestion**（如适用）：对 unknown command 用 Cobra 内置的 suggestion 打印 `Did you mean 'market-price'?`。(target)

示例：

```
Error: unknown command "maret-price"
Did you mean "market-price"?
Run 'surf --help' for usage.
```

Exit code 固定 1。

#### 4.2.3 Unexpected error（stderr + bug report）

内部 panic、assertion failure、第三方库 crash 等——用户没做错什么，是 CLI 自己的 bug。stderr 打印：

1. 一行友好提示："Surf CLI encountered an unexpected error and needs to be reported."
2. Stack trace（`-v` 模式下打全量，默认只打最后 N 行）
3. **预填的 bug report URL**：`https://github.com/cyberconnecthq/surf-cli/issues/new?template=bug.md&title=...&body=...`
   URL 里带上 version、os、命令名、error 摘要，让用户点开就能提 issue。(target)

Exit code 固定 1（CLI 层错误）。

### 4.3 Error propagation

**不能把 API error 转成空成功。** 不能在 5xx 上 fallback 到 "没有结果"。5xx 必须 exit 4 + stdout 输出完整 error envelope——不 swallow、不改写、不隐藏。Fail loud——见 §1.3。

CLI 也 **不能** 基于 retry 耗尽就把最后一个 error response 降级成 "空数据"——这是 agent 见过最难 debug 的故障模式。retry 耗尽就 exit 4，把最后那次 response 的 error body 原样输出。

### 4.4 Contextual next-step suggestions (AXI #9)

成功和 error response 都应该附加一个 `help[]` 数组，给 agent 指向最可能的下一步命令。**位置**：`--json` 输出的 envelope 里（`meta.help[]` 或顶层 `help[]`），**不是 stderr**——跟 §3.1 / §4.2.1 的分流规则对齐，让 agent 靠单一的 stdout parse pipeline 就能拿到建议。

成功示例：

```json
{
  "data": [{ "address": "0x...", "balance": 1234 }],
  "meta": {
    "total": 42,
    "credits_used": 1,
    "help": [
      "Get full detail: surf wallet-detail --address <address>",
      "See transaction history: surf wallet-history --address <address>"
    ]
  }
}
```

错误示例：

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid API key",
    "help": [
      "Set API key: surf auth --api-key <key>",
      "Or export: export SURF_API_KEY=<key>"
    ]
  }
}
```

占位符用尖括号语法（`<address>`、`<key>`），提示 agent 这是需要替换的位置、不是字面值。

这是多步 workflow 里减少 agent turn count 的 **最高杠杆改动**。**(target —— 还没落地)**

---

## 5. Help 和 discovery

### 5.1 `--help` 结构

每个命令的 help 输出按这个顺序：

1. Short description（一行）
2. Long description（几段人类可读的 prose）
3. 至少一个 `Example:` block，展示一次真实调用
4. `Flags:` 段（按字母序，required 标注出来）
5. `Global Flags:` 段（如果有适用的）
6. 底部指向 `https://docs.asksurf.ai` 或相关文档 URL 的 footer (target)

### 5.2 `list-operations`

`surf list-operations` 是 agent 主要的 discovery 入口。它的输出格式被视为 API：

- 每行一个 operation：`METHOD name short-description (--flag, ...)`
- 括号里的 flag 名必须是 kebab-case，而且跟命令实际接受的 flag 名完全一致。
- Path parameter 用 `<name>` 显示（尖括号用来跟 flag 区分开）。
- `--group`、`--detail`、`--category` 只缩小输出，不改变行格式。

**改动 list-operations 的格式是 breaking change** —— 需要 bump minor 版本并更新 SKILL.md。

### 5.3 Example 是一等文档

每个自定义 subcommand 必须有 `Example:` 字段。生成的 operation 命令继承 OpenAPI spec 里 `x-cli-example` extension 的示例（如果有的话）。

### 5.4 Content-first subcommand (AXI #8)

零参数调用一个 subcommand 应该显示 live data，不是 help 文本——前提是存在合理的默认行为：

| 命令                     | 默认行为                  |
| ------------------------ | ------------------------- |
| `surf list-operations`   | 打印 operation 列表       |
| `surf catalog list`      | 打印所有 table            |
| `surf catalog practices` | 打印 query best practices |
| `surf auth`              | 打印当前 auth 状态        |
| `surf version`           | 打印版本号                |

根命令 `surf`（零参数）是唯一的例外——它打印 help，因为没有一个合理的 "default thing to do"。Operation 命令（`surf market-price` 之类）总是至少需要一个 flag，所以不在 content-first 范围内。

### 5.5 Ambient context (AXI #7) —— 此处不适用

AXI 推荐 CLI 自己注入 agent 的 session hook，让 ambient state（当前目录、auth 状态、最近命令）在 agent 采取任何 action 之前就可用。这不是 `surf` 自己能做的——这是 **agent runtime** 的责任（Claude Code 的 `SessionStart` hook、`codex` 的 init 脚本等）。

Surf CLI 提供的替代品：

- `SURF_SESSION_ID` 环境变量——agent runner 在 session 开始时设置这个值，用来关联单次 run 里所有 `surf` 调用。
- `surf auth` —— 一行零开销的状态检查，适合 ambient-context probe。
- `surf list-operations` —— 快速的 discovery surface，agent runner 可以在 session 启动时缓存或预取。

---

## 6. Subcommand

### 6.1 命名模式

自定义命令尽量走 `noun verb`：`catalog list`、`catalog show`、`catalog search`。生成的 operation 命令是 flat namespace，名字是 spec 里继承来的 noun-noun 形式（`market-price`、`wallet-transfers`）——spec 是 source of truth。

混合模式（比如 `list-operations`）出于历史原因存在。**不要引入新的混合模式命令**。新加的话优先用 `operations list` 而不是 `list-operations`。

### 6.2 一个 subcommand family 内的 flag 一致性

同一个 parent 下的所有 subcommand 共享一组通用 flag。`catalog list`、`catalog show`、`catalog search`、`catalog practices` 必须都接受 `--json` 且语义相同。`--limit`、`--category` 在适用的地方行为也必须一致。

### 6.3 不允许前缀缩写

关闭 Cobra 的前缀缩写匹配。`surf mar` 不能静默匹配到 `surf market-price`。前缀匹配会阻止我们未来加任何跟现有命令有共同前缀的新命令。

---

## 7. 交互和危险操作

### 7.1 需要确认的操作

- 清除 credentials（`auth --clear`）
- 覆盖一个用户没明确指定的已存在文件
- 改 shell 配置文件（`install` 往 `.zshrc` 加东西等）
- 安装或升级 CLI 二进制

### 7.2 确认流程

- TTY 环境下显示 `[y/N]` prompt，默认是 no。
- 非 TTY 环境下除非传了 `--force` 否则拒绝继续。
- `--no-input` 永远禁用 prompt 并在需要确认时 fail。

### 7.3 输入约定

- `-` 作为文件名参数表示 stdin（target，用于 `onchain-sql` 等）。
- 密码和 API key 在交互输入时不 echo。
- stdin 只在命令明确期望输入时才消费。

---

## 8. 配置

### 8.1 优先级

CLI flag > 环境变量 > 配置文件 > 编译时默认值。

高优先级静默胜出。flag 覆盖配置文件时 **不 warn**——flag 就是用户要的。

### 8.2 配置文件位置

所有 CLI 状态住在 `~/.surf/`：

| 文件           | 用途                                   |
| -------------- | -------------------------------------- |
| `config.json`  | API profile、credentials fallback 缓存 |
| `apis.json`    | Bootstrapped API endpoint 列表         |
| `*.cbor`       | 缓存的 OpenAPI spec（CBOR 序列化）     |
| `session.json` | 30 分钟 telemetry session 状态         |
| `device_id`    | 持久化的匿名 device identifier         |

我们 **刻意不跟 XDG Base Directory**。对一个跨平台、需要支持跨机器 agent workflow 的工具来说，所有平台都用同一个路径（`~/.surf/` 在 Linux、macOS、Windows WSL 上都一样）比 XDG 兼容更有价值。

### 8.3 环境变量

所有 Surf CLI 特有的环境变量都用 `SURF_` 前缀：

| 变量                | 用途                                      |
| ------------------- | ----------------------------------------- |
| `SURF_API_KEY`      | 覆盖存储的 API key（最高优先级）          |
| `SURF_CONFIG_DIR`   | 覆盖 `~/.surf/` 位置                      |
| `SURF_SESSION_ID`   | 覆盖基于文件的 session（给 agent run 用） |
| `SURF_DO_NOT_TRACK` | 禁用 telemetry（见 §11）                  |
| `SURF_GATEWAY_URL`  | 覆盖 API gateway base URL (target)        |
| `SURF_CATALOG_URL`  | 覆盖 catalog metadata URL (target)        |
| `SURF_CDN_BASE`     | 覆盖 install/update CDN base (target)     |

通用的环境变量在合理的时候也遵守：`DO_NOT_TRACK`、`HOME`、`SHELL`、`EDITOR`。

**Hardcoded URL 是代码味道。** 每个外部 endpoint 都必须能通过环境变量覆盖，好让自托管部署和本地测试不用 patch 二进制就能工作。

---

## 9. 认证

### 9.1 Key 来源优先级

1. `SURF_API_KEY` 环境变量
2. OS keychain（service name：`surf-cli`，account：`surf:<profile>`）
3. `~/.surf/config.json` 文件（keychain 不可用时）
4. 无 key（匿名，credit 严重受限）

每一级按顺序尝试，第一个非空值胜出。

### 9.2 Key 类型

三种 key 前缀，都由后端 control plane 签发：

| 前缀         | 格式                       | 用途                          |
| ------------ | -------------------------- | ----------------------------- |
| `sk-`        | `sk-` + 64 hex 字符        | Enterprise 租户 key，长期有效 |
| `sk_sess_`   | `sk_sess_` + 64 hex 字符   | Urania preview，24h 过期      |
| `sk_deploy_` | `sk_deploy_` + 64 hex 字符 | Bifrost 部署的 app，永不过期  |

Telemetry 和 user-level tracking **只对 `sk-` key 启用**（见 §11）。CLI 不能 log 或 echo 任何 key 值。所有输出显示 key 时都用 `sk-xxxx...xxxx`（前 4 + 后 4，中间 mask）。

### 9.3 `auth` subcommand 合同

- `surf auth` 无 flag：打印 source 和 masked key（不泄漏 secret）。
- `surf auth --api-key <key>`：存到 keychain，fallback 到文件。
- `surf auth --clear`：从 keychain 和文件里删掉。除非传了 `--force` 或 `--no-input`，否则 prompt 确认。(target)
- `surf auth` 应该清楚地说明 `SURF_API_KEY` 环境变量会覆盖存储的 key。当 env var 生效时，`--clear` 无法完全清除——必须打印一条提示。(target)

---

## 10. Telemetry 和隐私

### 10.1 原则

- **只做 opt-out**，从不静默：首次运行时打印一行 notice，说明有 telemetry、收集什么、怎么禁用。(target)
- `DO_NOT_TRACK=1` 和 `SURF_DO_NOT_TRACK=1` 都禁用所有 telemetry。`DO_NOT_TRACK` 是行业通用约定，我们遵守它。
- Telemetry 不在 hot path 上——**不阻塞命令 exit 等网络 I/O**。事件在后台 goroutine 里 enqueue + flush。

### 10.2 收集的数据

允许的字段：

- `command` —— subcommand 名字（**不包含** flag 值）
- `version` —— CLI 版本字符串
- `os` —— `darwin/arm64` 风格的平台 tag
- `exit_code` —— 0、1、4 或 130
- `status_code` —— HTTP status（如适用）
- `duration_ms` —— 命令 wall time
- `session_id` —— 30 分钟 inactivity session
- Distinct ID —— API key 的 SHA-256 hash（base64），跟后端的 key hashing 算法对齐，这样同一个 key 在两边产生同一个 ID。没有 key 时 fallback 到持久化的 `device_id` UUID。

禁止的字段：

- **Flag 值**（可能包含 secret、ticker、钱包地址、PII）
- **原始 API key**
- **Response body**
- **命令名以外的 request URL**
- **识别命令之外的额外参数**

### 10.3 Backend

PostHog（`https://us.i.posthog.com`）。Project API key 是公开的（就像 GA tracking ID），编译时嵌入。

---

## 11. 信号和鲁棒性

### 11.1 信号处理 (target)

- `SIGINT`（Ctrl-C）：stderr 打印 `interrupted`，通过 context cancellation 取消 in-flight HTTP 请求，flush telemetry，exit 130。
- `SIGTERM`：跟 SIGINT 一样。
- 2 秒内的第二次 `SIGINT` 跳过清理直接 force-exit。

### 11.2 Retry 策略

- 可重试的 HTTP status code：408、425、429、500、502、503、504。
- 识别 `Retry-After` header（秒数或 HTTP date）。
- 识别 `X-Retry-In` 自定义 header（duration 字符串）。
- 默认：2 次重试，1s 初始 backoff，可通过 `--rsh-retry` 配置。
- **不** 对 4xx 错误（除了 408/425/429）重试——它们是 client error，重试也不会好。

### 11.3 Timeout

每个 HTTP 请求都有 timeout。当前默认比较宽松（现在没有硬 timeout，依赖 server-side gateway）。`--timeout` flag 可以 per 命令覆盖。后台 telemetry 有短的固定 timeout（2s），永远不阻塞主路径。

### 11.4 Crash-only 设计

CLI 的正确性不依赖 clean shutdown hook。临时文件创建在 `~/.surf/` 下，用随机名字，下次启动时清理。检测到不完整的状态文件就丢弃，不要尝试修复。

---

## 12. 分发

- **单一静态链接二进制**，走 GoReleaser，放在 CDN `downloads.asksurf.ai/cli/releases/` 下面。
- **`surf install`**：从 CDN self-update。验证 SHA-256 checksum。改 shell config 前 prompt 确认。
- **卸载** 指南住在 README 里。删掉 `~/.local/bin/surf`、`~/.local/share/surf/`、`~/.surf/` 就够了。
- **没有显式 opt-in 时不写系统目录**（`/usr/local/bin/` 等）。

---

## 13. 版本和 future-proofing

### 13.1 Additive 改动免费

加一个新 subcommand、新 flag、新输出格式，从来不需要 major 版本 bump。老的调用方式必须继续工作。

### 13.2 Breaking change

以下任何一条都是 breaking change，需要 major 版本 bump 加 deprecation 窗口：

- 删除一个 subcommand 或 flag
- 重命名一个 subcommand 或 flag
- 改 exit code 语义
- 改 `--json` 输出结构
- 改 `list-operations` 行格式
- 改 error JSON envelope 结构

### 13.3 Deprecation 流程

1. PR 把 feature 标为 deprecated；下一个 release 在用户使用时打 warning 到 stderr。
2. 至少一个 minor 版本保留 warning。
3. 下一个 major 版本移除 feature。

### 13.4 输出格式稳定性

- `readable` 输出：面向人类，可以随意改。
- `json` 输出：被视为 API。Breaking change 需要 major 版本 bump。Additive change（加新的可选字段）是 OK 的。

---

## 14. Agent 专属考量

这一章收集 Surf CLI 特有的政策——它们存在是因为 agent 是我们的主要用户。大部分是对上面章节的交叉引用；放在这里是方便 reviewer 查找 "作为 agent-first CLI 有哪些跟一般 CLI 不一样"。

### 14.1 SKILL.md 是合同

`asksurf-ai/surf-skills` repo 里的 `skills/surf/SKILL.md` 是面向 agent 的训练文档。任何影响 agent 能用或应该怎么用的 CLI 行为改动，必须在同一个 PR 或紧接着的 PR 里反映到 SKILL.md。

SKILL.md 是 **从这份文档派生的**，不是它的替代品。两者冲突时，这份文档胜出，SKILL.md 要修。

### 14.2 训练数据就是真正的 spec

一旦一个 flag 名、命令名、输出结构 ship 出去并且有 agent 基于它训练了，**你就拿不回来了**——任何基于老形式的 skill、prompt、example 都会 break。这是代码库里最硬的约束。

后果：

- 重命名一个 flag 是 breaking change，哪怕行为完全一样。
- 重命名一个命令是 breaking change，哪怕保留老名字作为 alias 也不行——野外的 example 用新名字的不一致。
- 改 `--json` 输出结构是 breaking change。就算只是加一个新的必需字段也可能 break——如果 agent 在 filter 特定 key 的话。
- 移除一个 subcommand 是 breaking change，哪怕先 deprecate 过也不行——published 到 `surf-skills` 的 skill 可能被 bundle 在按自己节奏更新的 agent runtime 里。

**优先加，从不改名，只在 major 版本里删。**

### 14.3 Token 效率

Agent 的上下文窗口是有限的。默认输出不能浪费 token（交叉引用 §3.6 - §3.10）：

- 成功时没有 banner，没有 ASCII art 装饰。
- 最小默认 schema：每项 3-5 个字段，`--fields` 扩展。
- 长文本字段内容截断，`--full` 扩展。
- 用 `--json | jq '.data'` 提取字段，避免把完整 envelope 带进 agent context。
- Operation help 剥掉 OpenAPI schema block（§3.5）。

### 14.4 可预测的错误

Agent 基于 JSON error envelope 里的 `error.code` 做分支。现在在用的 code（`UNAUTHORIZED`、`INSUFFICIENT_CREDIT`、`RATE_LIMITED`、`INVALID_REQUEST`、`NOT_FOUND`、`BAD_GATEWAY`）是稳定的。加新 error code 是 additive，改名或删是 breaking。

API error envelope 去 **stdout**（不是 stderr），详见 §3.1 和 §4.2.1。CLI error 去 stderr，详见 §4.2.2。两者都有稳定的 exit code 映射（§4.1）。

### 14.5 Session correlation

一次 agent run 通常会调 `surf` 几十次。这些调用必须可以关联：

- **Per-invocation session**：PostHog `session_id` 用 30 分钟 inactivity 文件。单次 agent run 里这能把事件分到一组（对空闲用户而言）。
- **显式 run 级分组**：agent runner 把 `SURF_SESSION_ID` 设成 run 特定的 UUID；CLI 直接用它，跳过文件逻辑。
- **跨机器用户身份**：telemetry `distinct_id` 是 API key 的 SHA-256 hash（base64 编码），跟后端 key hashing 算法对齐——同一个用户在任何机器上都映射到同一个 PostHog person。

### 14.6 Exit code 纪律

Agent 把 exit code 当主要分支信号。任何偏离 §4.1 都是 bug，会导致 agent workflow 编造结果或 retry loop。**Exit code 合同是不可商量的。**

### 14.7 Flat namespace 是故意的

Surf CLI 目前有 100+ 个 operation 命令直接挂在 `surf` 下面（`surf market-price`、`surf kalshi-markets`、`surf polymarket-events`）。这违反 clig.dev §6 的 `noun verb` 层级建议（`surf market price`，不是 `surf market-price`）。

我们明确决定 **不改这个**，这份文档记录这个决定，防止未来的 reviewer 以为这是个遗漏去 "修" 它：

- 层级化命令要求重命名每个现有 operation——参见 §14.2，这会 break 所有 agent。
- Restish（底层框架）把命令生成为直接子命令；层级路由需要重写 command loader。
- Flat 命名对 agent 用得很好（精确匹配调用）。
- Flat 命名的唯一真正问题是 `surf --help` 对人类难 scan。那是 root help template 的显示问题，不是命令树的结构问题。

未来新加的 operation 命令继续走 flat，跟现有模式对齐。自定义命令（`catalog list`、`catalog show`）继续用 `noun verb`，因为它们是新的，没有 legacy agent 束缚。

### 14.8 Contextual disclosure (交叉引用 §4.4)

Agent 从 response body 里的 "next step suggestion" 获益巨大。`help[]` 数组设计见 §4.4。现在是 target，还没落地。

### 14.9 最小 schema 和截断 (交叉引用 §3.6、§3.7)

Surf CLI 当前 agent token 成本的最大来源是完整 envelope 的 JSON response——包含 `$schema` pointer、完整字段集、cached metadata。§3.6 和 §3.7 描述了 fix。

---

## 15. 跟 clig.dev 和 axi.md 的偏离

Surf CLI 在两个参考文档有分歧或我们刻意采取不同立场的 topic 上的位置。"—" 表示该来源没涉及这个 topic。

| Topic                            | clig.dev           | axi.md                | Surf CLI                                                  |
| -------------------------------- | ------------------ | --------------------- | --------------------------------------------------------- |
| 主要用户                         | 人类               | Agent                 | Agent（见 §1、§14）                                       |
| 默认输出格式                     | 人类可读           | TOON（token 高效）    | JSON（不支持 TOON——生态工具不认识，人类看不懂）           |
| 默认 response schema             | 完整               | 最小（3-4 字段）      | 当前完整，最小是 target（§3.6）                           |
| 长字段截断                       | —                  | 必需                  | Target（§3.7）                                            |
| 预计算聚合（total 等）           | —                  | 必需                  | 通过 `meta.total` 部分实现（§3.8）                        |
| 空结果处理                       | —                  | 必须明确消息          | 自定义命令已执行；生成的 operation 是 target（§3.9）      |
| Contextual next-step suggestions | —                  | `help[]` 数组         | Target（§4.4）                                            |
| Ambient session context          | —                  | CLI 注入 session hook | 不适用——由 agent runtime 负责（§5.5）                     |
| 零输入时 content-first           | —                  | 给数据，不给 help     | 对有合理默认的 subcommand 强制执行（§5.4）                |
| 交互式 prompt                    | TTY 上允许         | 禁止                  | operation 上禁止；只允许一次性 setup（`auth`、`install`） |
| Namespace 层级                   | `noun verb`        | —                     | Flat（故意的——§14.7）                                     |
| 配置位置                         | XDG Base Directory | —                     | 所有平台 `~/.surf/`（§8.2）                               |
| 前缀缩写匹配                     | 禁用               | —                     | 禁用                                                      |
| 多个位置参数                     | 允许用于文件操作   | —                     | 禁止（§2.5）                                              |
| 进度条                           | 长操作要显示       | —                     | 只在 `install` 用；大部分命令 1s 内结束                   |
| Analytics                        | 优先 opt-in        | —                     | Opt-out + 告知（§10）；agent run 需要 correlation         |

---

## 16. Review checklist

在 approve 一个动 CLI 行为的 PR 前走一遍：

### 正确性

- [ ] Exit code 是 0、1、4 或 130 —— 不是别的值
- [ ] stdout 只有主要数据；diagnostic 走 stderr
- [ ] API error 至少在一个 stream 上产生非空输出
- [ ] `--help`、`--version`、`-h` 在改过的命令上还能用
- [ ] 测试覆盖了新行为和任何可能的 regression

### 一致性

- [ ] 新 flag 是 kebab-case
- [ ] 复用的概念用现有 flag 名（`--json`、`--limit` 等）
- [ ] 命令有 sibling 的话共享同一组 flag
- [ ] `list-operations` 输出格式没变（或者有意改了同时更新了 SKILL.md）

### 安全

- [ ] 破坏性操作需要确认或 `--force`
- [ ] 没有 hardcoded URL；环境变量可以覆盖
- [ ] Secret 没进 flag 值、log 行、telemetry
- [ ] Telemetry 事件只包含 §10.2 列出的字段

### 文档

- [ ] `--help` short 和 long 描述是准确的
- [ ] 新命令至少有一个 example
- [ ] Breaking change 在 changelog 里带迁移说明
- [ ] Agent 可见行为改了就更新 SKILL.md

### 跟参考文档对齐

- [ ] 没有未在 §15 记录理由的 clig.dev 或 axi.md 偏离
- [ ] 没引入新的 exit code 值
- [ ] 没加未经论证的新位置参数
- [ ] Agent 可见的改动（输出结构、flag 名、error code）针对 §14.2（训练数据约束）审查过

---

## 17. 参考

- [**clig.dev**](https://clig.dev/) —— Command Line Interface Guidelines，通用 CLI 工艺基线。
- [**axi.md**](https://axi.md/) —— Agent eXperience Interface，10 条经过实证验证的 agent 优化 CLI 原则。
- [**`asksurf-ai/surf-skills` 里的 SKILL.md**](https://github.com/asksurf-ai/surf-skills/blob/main/skills/surf/SKILL.md) —— 从这份 spec 派生的面向 agent 的训练文档。
- [**Cobra**](https://github.com/spf13/cobra) —— Surf CLI 底层的 CLI 框架。
- [**restish**](https://github.com/danielgtaylor/restish) —— Surf CLI 的 operation loader 继承自的底层框架。
- [**PostHog Go SDK**](https://github.com/PostHog/posthog-go) —— §10.3 里提到的 telemetry 后端。
