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
