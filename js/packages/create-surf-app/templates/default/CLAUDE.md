# Project

Built with [Surf SDK](https://github.com/cyberconnecthq/urania/tree/main/packages/sdk).

## Imports from @surf-ai/sdk

The frontend should call your own backend routes under the Vite base path.
Reuse the scaffolded `frontend/src/lib/api.ts` helper for frontend API requests.
Call routes like `fetch(api('wallet'))`.
Do not use absolute `/api/...` fetch URLs in the frontend.
Do not use Surf SDK React hooks in the frontend.
Use `@surf-ai/sdk/server` in backend routes to talk to Surf data APIs.

**Backend (`@surf-ai/sdk/server`):**

```js
const { dataApi } = require("@surf-ai/sdk/server");
const data = await dataApi.market.price({ symbol: "BTC" });
const holders = await dataApi.token.holders({
  address: "0x...",
  chain: "ethereum",
});
// Escape hatch for new endpoints:
const raw = await dataApi.get("newcategory/endpoint", { foo: "bar" });
```

## Structure

```
frontend/src/App.tsx       - build your UI here
frontend/src/components/   - add components
frontend/src/lib/api.ts    - base-aware helper for frontend API calls
backend/routes/*.js        - add API routes (auto-mounted at /api/{name})
backend/db/schema.js       - define database tables
```

## Built-in Endpoints (from @surf-ai/sdk/server)

`createServer()` provides these automatically - do NOT create routes for them:

| Endpoint             | Method | Purpose                                                |
| -------------------- | ------ | ------------------------------------------------------ |
| `/api/health`        | GET    | Health check - `{ status: 'ok' }`                      |
| `/api/__sync-schema` | POST   | Sync `backend/db/schema.js` tables to database         |
| `/api/cron`          | GET    | List cron jobs with status and next run time           |
| `/api/cron`          | POST   | Create a new cron task                                 |
| `/api/cron/:id`      | PATCH  | Update a cron task (schedule, enabled, etc.)           |
| `/api/cron/:id`      | DELETE | Delete a cron task                                     |
| `/api/cron/:id/run`  | POST   | Manually trigger a cron task                           |

Auto-registered from `backend/routes/*.js`:
| File | Endpoint |
|------|----------|
| `routes/btc.js` | `/api/btc` |
| `routes/portfolio.js` | `/api/portfolio` |

## Database

Define tables in `backend/db/schema.js` using Drizzle ORM:

```js
const { pgTable, serial, text, timestamp } = require("drizzle-orm/pg-core");
exports.users = pgTable("users", {
  id: serial("id").primaryKey(),
  name: text("name").notNull(),
  created_at: timestamp("created_at").defaultNow(),
});
```

Tables are auto-created on startup and when `schema.js` changes (file watcher).

Query the database in routes with `dbQuery(sql, params)` from `@surf-ai/sdk/db`. Drizzle ORM is **only** used to declare the schema — there is no Drizzle client, no `req.db` middleware, and no direct connection pool. `dbQuery` returns a pg-style result `{ rows, rowCount, fields }`, so destructure `rows`:

```js
const { dbQuery } = require("@surf-ai/sdk/db");

router.get("/", async (req, res) => {
  const { rows } = await dbQuery(
    "SELECT * FROM users ORDER BY created_at DESC"
  );
  res.json(rows);
});

router.post("/", async (req, res) => {
  const { name } = req.body;
  const { rows } = await dbQuery(
    "INSERT INTO users (name) VALUES ($1) RETURNING *",
    [name]
  );
  res.json(rows[0]);
});
```

### Environment variables

Only variables prefixed with `VITE_` are exposed to the Vite frontend (`import.meta.env.VITE_FOO`). To use a plain env var (e.g. `APP_TITLE`) in the UI, read it in a backend route and fetch it from the frontend:

```js
// backend/routes/config.js
const express = require("express");
const router = express.Router();

router.get("/", (_req, res) => {
  res.json({ title: process.env.APP_TITLE || "App" });
});

module.exports = router;
```

## Do NOT modify

- `vite.config.ts` - API proxy and build config
- `backend/server.js` - uses @surf-ai/sdk/server
- `entry-client.tsx` - app bootstrap with SSR hydration
- `entry-server.tsx` - SSR render for deploy
- `index.html` - cold-start guard and Surf badge
- `eslint.config.*` - lint rules
- `index.css` - only imports, do not add styles here (use Tailwind classes)

## Rules

- Use the scaffolded `api(path)` helper from `frontend/src/lib/api.ts` for frontend API calls
- Never use absolute `/api/...` URLs in frontend fetch calls
- Use `@surf-ai/sdk/server` `dataApi` in backend code when you need Surf data
- Do not bypass your backend routes from the frontend
- Frontend packages are pre-installed - check `package.json` before installing
- Default to a dark theme unless the user explicitly asks for a different visual direction.

## Design

### Avoid AI-default patterns
- No colored icon boxes next to metrics (the blue-bg-with-icon KPI pattern)
- No gradient avatar circles with initials
- No "Built with React · Tailwind" footers
- No AI copywriting: "Elevate", "Seamless", "Unleash", "Delve", "Next-Gen"
- No "Oops!" error messages — be direct ("Failed to load. Try again.")
- No round placeholder numbers ($100.00) — use realistic data ($847.29)
- Sentence case on headings, not Title Case On Every Word
- Icons should aid scanning, not decorate — omit when the label is clear

### ECharts
- Flat style: show primary axis line, dashed split lines, transparent chart bg
- Custom tooltip formatter with dash indicators (12×2.5px bars, not default circle dots)
- Legend: type "plain", icon "roundRect", itemWidth 12, itemHeight 3
- Prefer timeframe tabs (7D/30D/90D/1Y/All) over dataZoom for time series
- Default to theme visualizer palette; override when semantics demand it (red/green for gain/loss, sequential scales for heatmaps)
- Dark mode: parameterize tooltip bg, axis colors, split line colors via resolvedTheme
