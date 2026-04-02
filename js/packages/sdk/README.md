# @surf-ai/sdk

Surf SDK 1.0 for backend apps, typed Surf data access, and database helpers.

## Install

```bash
bun add @surf-ai/sdk
```

## Configuration

SDK 1.0 uses a single direct-auth model.

| Env Var | Default | Purpose |
| --- | --- | --- |
| `SURF_API_BASE_URL` | `https://api.asksurf.ai/gateway/v1` | Full Surf API base URL |
| `SURF_API_KEY` | none | Bearer token used for upstream requests and protected runtime endpoints |
| `PORT` | none | Express server port when `createServer({ port })` is not provided |

All upstream SDK requests use:

```http
Authorization: Bearer <SURF_API_KEY>
```

## Subpath exports

| Import | What it provides |
| --- | --- |
| `@surf-ai/sdk/server` | `createServer()`, `dataApi` |
| `@surf-ai/sdk/db` | `dbProvision()`, `dbQuery()`, `dbTables()`, `dbTableSchema()`, `dbStatus()` |

## Data API usage

```js
const { dataApi } = require('@surf-ai/sdk/server')

const btc = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
const holders = await dataApi.token.holders({
  address: '0xdAC17F958D2ee523a2206206994597C13D831ec7',
  chain: 'ethereum',
})

// Escape hatch for endpoints that do not have a typed helper yet.
const custom = await dataApi.get('market/price', { symbol: 'ETH', time_range: '1d' })
```

Available categories include:

- `market`
- `token`
- `wallet`
- `onchain`
- `social`
- `project`
- `news`
- `exchange`
- `fund`
- `search`
- `web`
- `polymarket`
- `kalshi`
- `prediction_market`

## Server runtime

```js
const { createServer } = require('@surf-ai/sdk/server')

createServer({ port: 3001 }).start()
```

`createServer()` provides:

- Auto-loading of `routes/*.js` and `routes/*.ts` as `/api/{name}`
- `GET /api/health`
- `POST /api/__sync-schema`
- `GET/POST/PATCH/DELETE /api/cron`
- `POST /api/cron/:id/run`
- Schema sync on startup and when `db/schema.js` changes

`GET /api/health` is public.

These runtime endpoints require `Authorization: Bearer <SURF_API_KEY>`:

- `POST /api/__sync-schema`
- `GET /api/cron`
- `POST /api/cron`
- `PATCH /api/cron/:id`
- `DELETE /api/cron/:id`
- `POST /api/cron/:id/run`

Routes you define in `routes/*` stay public unless your app adds its own auth.

Example route:

```js
const router = require('express').Router()
const { dataApi } = require('@surf-ai/sdk/server')

router.get('/', async (_req, res) => {
  const data = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
  res.json(data)
})

module.exports = router
```

## Database helpers

```js
const { dbProvision, dbQuery, dbTables, dbTableSchema, dbStatus } = require('@surf-ai/sdk/db')

await dbProvision()
const result = await dbQuery('SELECT * FROM users WHERE id = $1', [123], { arrayMode: true })
const tables = await dbTables()
const schema = await dbTableSchema('users')
const status = await dbStatus()
```

Define tables in `db/schema.js` and the runtime will provision the database and create missing tables and columns during startup.

Example schema:

```js
const { pgTable, serial, text, timestamp } = require('drizzle-orm/pg-core')

exports.users = pgTable('users', {
  id: serial('id').primaryKey(),
  name: text('name').notNull(),
  email: text('email'),
  created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
})
```

## 1.0 migration notes

- `SURF_API_BASE_URL` and `SURF_API_KEY` are the only supported SDK auth variables.
- The runtime no longer mounts `/proxy/*`.
- The `@surf-ai/sdk/react` subpath has been removed.
- Route modules must export the handler directly with `module.exports = router`.
- `createServer()` requires a port from `options.port` or `process.env.PORT`.
