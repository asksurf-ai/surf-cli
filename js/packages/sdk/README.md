# @surf-ai/sdk

Surf platform SDK — data API client, Express server runtime, React hooks, and database helpers.

## Install

```bash
bun add @surf-ai/sdk
```

## Usage

### Frontend (React hooks)

```tsx
import { useMarketPrice, cn, useToast } from '@surf-ai/sdk/react'

function App() {
  const { data, isLoading } = useMarketPrice({ symbol: 'BTC', time_range: '1d' })
  return (
    <div className={cn('p-4', isLoading && 'opacity-50')}>
      BTC: ${data?.data?.[0]?.value}
    </div>
  )
}
```

### Backend (data API)

```js
const { dataApi } = require('@surf-ai/sdk/server')

// Typed methods grouped by category
const btc = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
const holders = await dataApi.token.holders({ address: '0x...', chain: 'ethereum' })
const trades = await dataApi.onchain.sql({ sql: 'SELECT ...', max_rows: 100 })

// Escape hatch for new endpoints
const data = await dataApi.get('newcategory/endpoint', { foo: 'bar' })
```

### Backend (Express server)

```js
const { createServer } = require('@surf-ai/sdk/server')

// Starts Express with /proxy/*, route auto-loading, DB sync, cron, health check
createServer({ port: 3001 }).start()
```

Routes in `routes/*.js` are auto-mounted at `/api/{name}`:

```js
// routes/btc.js → /api/btc
const { dataApi } = require('@surf-ai/sdk/server')
const router = require('express').Router()

router.get('/', async (req, res) => {
  const data = await dataApi.market.price({ symbol: 'BTC' })
  res.json(data)
})

module.exports = router
```

## Subpath Exports

| Import | What |
|--------|------|
| `@surf-ai/sdk/server` | `createServer()`, `dataApi` — Express runtime + typed data API |
| `@surf-ai/sdk/react` | `useMarketPrice()`, `cn()`, `useToast()` — React hooks + utilities |
| `@surf-ai/sdk/db` | `dbQuery()`, `dbProvision()`, `dbTables()` — Drizzle/Neon database |

## Built-in Endpoints

`createServer()` provides these automatically:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/health` | GET | Health check — `{ status: 'ok' }` |
| `/api/__sync-schema` | POST | Sync `db/schema.js` tables to database |
| `/api/cron` | GET | List cron jobs and status |
| `/api/cron` | POST | Update cron.json and reload |
| `/proxy/*` | ANY | Data API passthrough to hermod |

Routes in `routes/*.js` are auto-loaded as `/api/{filename}`.

DB schema is auto-synced on startup and when `db/schema.js` changes (file watcher).

## Environment Variables

The SDK auto-detects the runtime mode from environment variables. New prefixed names take priority; legacy names are supported for backward compatibility.

### Data API routing

| Env Var | Legacy | Mode | Set By |
|---------|--------|------|--------|
| `SURF_SANDBOX_PROXY_BASE` | `DATA_PROXY_BASE` | Sandbox | urania executor |
| `SURF_DEPLOYED_GATEWAY_URL` | `GATEWAY_URL` | Deployed | Bifrost |
| `SURF_DEPLOYED_APP_TOKEN` | `APP_TOKEN` | Deployed | Bifrost |

**Routing logic:**
```
if SURF_SANDBOX_PROXY_BASE  → sandbox (OutboundProxy handles auth)
elif SURF_DEPLOYED_GATEWAY_URL + SURF_DEPLOYED_APP_TOKEN → deployed (hermod with Bearer)
else → public (api.ask.surf, no auth)
```

### Server

| Env Var | Default | Purpose |
|---------|---------|---------|
| `PORT` | `3001` | Express listen port |

### Vite dev server (scaffold, not SDK)

| Env Var | Default | Purpose |
|---------|---------|---------|
| `VITE_PORT` | `5173` | Frontend dev server port |
| `VITE_BACKEND_PORT` | `3001` | Backend port for Vite proxy target |

### How routing works

```
Sandbox (urania preview):
  Frontend hook: useMarketPrice()
    → fetch /proxy/market/price (same-origin to Vite)
      → Vite proxy → Express /proxy/* → OutboundProxy (JWT) → hermod

  Backend route: dataApi.market.price()
    → fetch SURF_SANDBOX_PROXY_BASE/market/price
      → OutboundProxy (JWT) → hermod

Deployed (surf.computer):
  Frontend hook: useMarketPrice()
    → fetch /proxy/market/price (same-origin to Express)
      → Express /proxy/* → hermod (APP_TOKEN)

  Backend route: dataApi.market.price()
    → fetch http://127.0.0.1:PORT/proxy/market/price (loopback)
      → Express /proxy/* → hermod (APP_TOKEN)

Public (local dev):
  Frontend hook: useMarketPrice()
    → fetch /proxy/market/price (same-origin to Vite)
      → Vite proxy → Express /proxy/* → hermod (GATEWAY_URL + APP_TOKEN)

  Backend route: dataApi.market.price()
    → fetch SURF_DEPLOYED_GATEWAY_URL/gateway/v1/market/price (Bearer APP_TOKEN)
```

## Available Categories

| Category | Example Methods |
|----------|----------------|
| `market` | `price`, `ranking`, `etf`, `futures`, `options`, `fear_greed` |
| `token` | `holders`, `transfers`, `dex_trades`, `tokenomics` |
| `wallet` | `detail`, `net_worth`, `labels_batch`, `transfers` |
| `onchain` | `sql`, `tx`, `gas_price`, `schema`, `structured_query` |
| `social` | `detail`, `mindshare`, `tweets`, `user`, `ranking` |
| `project` | `detail`, `defi_metrics`, `defi_ranking` |
| `news` | `detail`, `feed` |
| `exchange` | `price`, `depth`, `klines`, `funding_history`, `perp` |
| `fund` | `detail`, `portfolio`, `ranking` |
| `search` | `project`, `news`, `wallet`, `web` |
| `web` | `fetch` |
| `polymarket` | `events`, `markets`, `prices`, `volumes` |
| `kalshi` | `events`, `markets`, `prices`, `volumes` |
| `prediction_market` | `category_metrics` |

## Database

Per-user PostgreSQL (Neon) with Drizzle ORM. Auto-provisioned, auto-synced on server startup.

### Setup

Define tables in `backend/db/schema.js`:

```js
const { pgTable, serial, text, integer, boolean, timestamp, real, jsonb } = require('drizzle-orm/pg-core')

exports.users = pgTable('users', {
  id: serial('id').primaryKey(),
  name: text('name').notNull(),
  email: text('email'),
  created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
})
```

Tables are auto-created when the server starts and when `schema.js` changes (file watcher). You can also call `POST /api/__sync-schema` explicitly.

### Querying (in backend routes)

```js
// backend/routes/users.js
const { drizzle } = require('drizzle-orm/neon-http')
const { dbQuery } = require('@surf-ai/sdk/db')
const { eq, desc, count } = require('drizzle-orm')
const schema = require('../db/schema')

// IMPORTANT: arrayMode must be true for Drizzle to work correctly
const db = drizzle(async (sql, params, method) => {
  const result = await dbQuery(sql, params, { arrayMode: true })
  return { rows: result.rows || [] }
})

router.get('/', async (req, res) => {
  const users = await db.select().from(schema.users).orderBy(desc(schema.users.created_at)).limit(20)
  res.json(users)
})

router.post('/', async (req, res) => {
  const [user] = await db.insert(schema.users).values(req.body).returning()
  res.json(user)
})

router.patch('/:id', async (req, res) => {
  const [user] = await db.update(schema.users).set(req.body).where(eq(schema.users.id, +req.params.id)).returning()
  res.json(user)
})

router.delete('/:id', async (req, res) => {
  await db.delete(schema.users).where(eq(schema.users.id, +req.params.id))
  res.json({ ok: true })
})
```

### Raw SQL (escape hatch)

```js
const { dbQuery } = require('@surf-ai/sdk/db')
const result = await dbQuery('SELECT symbol, SUM(volume) FROM trades GROUP BY symbol ORDER BY 2 DESC LIMIT $1', [20])
```

### DB Proxy Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/proxy/db/provision` | Create database (idempotent) |
| POST | `/proxy/db/query` | Execute SQL query |
| GET | `/proxy/db/tables` | List all tables |
| GET | `/proxy/db/table-schema?table=X` | Column definitions for table X |
| GET | `/proxy/db/status` | Connection status |
| POST | `/api/__sync-schema` | Force schema sync from `db/schema.js` |

### Safety Rules

- **NEVER** `DROP TABLE` or `TRUNCATE` with existing data — use `ALTER TABLE ADD COLUMN IF NOT EXISTS`
- **NEVER** `DELETE FROM` without `WHERE`
- **Always** call `GET /proxy/db/tables` before creating tables — check what exists first
- **Never** seed data into non-empty tables — check row count first
- Limits: 30s query timeout, 5000 max rows, 50 max tables

## Cron Jobs

Built-in cron system powered by `croner`. Managed via `cron.json` + handler files.

### When to Use

- **Side effects** (DB writes, alerts, cache refresh) → cron job
- **Display refresh** (show latest price) → `useQuery({ refetchInterval: 30000 })`

### Setup

1. Create `backend/cron.json`:

```json
[
  {
    "id": "refresh-prices",
    "name": "Refresh token prices",
    "schedule": "*/5 * * * *",
    "handler": "tasks/refresh-prices.js",
    "enabled": true,
    "timeout": 120
  }
]
```

2. Create handler in `backend/tasks/`:

```js
// backend/tasks/refresh-prices.js
const { dataApi } = require('@surf-ai/sdk/server')

module.exports = {
  async handler() {
    const data = await dataApi.market.price({ symbol: 'BTC' })
    // process and store...
  },
}
```

### Cron fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier |
| `name` | string | yes | Display name |
| `schedule` | string | yes | Cron expression (min 1-minute interval) |
| `handler` | string | yes | Path from `backend/` to handler file |
| `enabled` | boolean | yes | Active or not |
| `timeout` | number | no | Max seconds (default 300) |

### Common schedules

| Expression | Meaning |
|-----------|---------|
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 0 * * *` | Daily at midnight |
| `0 9 * * 1` | Monday at 9 AM |

### Management API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/cron` | List all jobs with status |
| POST | `/api/cron` | Create/update jobs (updates cron.json) |
| PATCH | `/api/cron/:id` | Update a single job |
| DELETE | `/api/cron/:id` | Remove a job |
| POST | `/api/cron/:id/run` | Manually trigger a job |

Rules: handlers must be idempotent, never use `setInterval` in server.js, `croner` is pre-installed (never `npm install` it).

## Web Search & Fetch

Search the web and scrape pages through the data proxy:

```js
// Backend
const { dataApi } = require('@surf-ai/sdk/server')

// Search
const results = await dataApi.search.web({ q: 'BTC ETF approval', limit: 10 })

// Fetch page as markdown
const page = await dataApi.web.fetch({ url: 'https://example.com', target_selector: '.article' })
```

```tsx
// Frontend
import { useSearchWeb, useWebFetch } from '@surf-ai/sdk/react'

const { data } = useSearchWeb({ q: 'BTC ETF', limit: 5 })
const { data: page } = useWebFetch({ url: 'https://example.com' })
```

Search params: `q` (required), `limit`, `offset`, `site` (domain filter). Fetch params: `url` (required), `target_selector`, `remove_selector`, `timeout`.

## Data Strategy

### Market vs Exchange

- **`market`** = aggregated cross-exchange (market cap, total OI, Fear & Greed, ETF flows). Use for: "show BTC price", "market overview"
- **`exchange`** = per-exchange real-time (order book, klines, funding rate). Use for: "Binance BTC order book", "compare funding rates"

| Need | Use |
|------|-----|
| Price history, rankings, sentiment | `market` |
| Total derivatives OI, liquidations, ETF flows | `market` |
| Order book, klines from a specific exchange | `exchange` |
| Funding rate, long/short for specific pair | `exchange` |

### Data complexity tiers

| Complexity | Approach |
|-----------|----------|
| Single endpoint, read-only | Frontend hook directly (`useMarketPrice`) |
| Combine multiple endpoints | Backend route with `Promise.all` + multiple `dataApi` calls |
| External API not in proxy | Backend route + `process.env` for API keys |
| On-chain SQL analytics | `dataApi.onchain.sql()` (see `onchain` skill for ClickHouse schema) |

### Backend composition pattern

```js
// backend/routes/overview.js — combine multiple data sources
const { dataApi } = require('@surf-ai/sdk/server')
const router = require('express').Router()

router.get('/', async (req, res) => {
  const { symbol } = req.query
  const [price, holders, social] = await Promise.all([
    dataApi.market.price({ symbol }),
    dataApi.token.holders({ address: req.query.address, chain: 'ethereum', limit: 10 }),
    dataApi.social.detail({ username: req.query.twitter }),
  ])
  res.json({ price: price.data?.[0], topHolders: holders.data, social: social.data })
})

module.exports = router
```

## Codegen

API methods and React hooks are auto-generated from hermod's OpenAPI spec via the surf CLI:

```bash
# Regenerate all endpoints
python scripts/gen_sdk.py

# Regenerate specific endpoints
python scripts/gen_sdk.py --ops market-price token-holders

# Build
bun run build
```

Requires `surf` CLI installed and authenticated (`surf login`).

## Development

```bash
bun install
bun run build        # compile TypeScript
bun test             # run tests
bun run codegen      # regenerate from OpenAPI spec
```

## Testing

```bash
# Unit tests
bun test ./tests/data-client.test.ts

# E2E (auto-detects mode from env vars)
bun test ./tests/e2e-all-envs.test.ts

# E2E with specific mode:
SURF_SANDBOX_PROXY_BASE=http://127.0.0.1:9999/s/<session>/proxy bun test ./tests/e2e-all-envs.test.ts
SURF_DEPLOYED_GATEWAY_URL=https://api.ask.surf SURF_DEPLOYED_APP_TOKEN=<token> bun test ./tests/e2e-all-envs.test.ts
```
