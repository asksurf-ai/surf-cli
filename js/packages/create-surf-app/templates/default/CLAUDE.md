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
The agent can also call `POST /api/__sync-schema` explicitly after editing.

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
