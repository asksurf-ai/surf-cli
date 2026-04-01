# Project

Built with [Surf SDK](https://github.com/cyberconnecthq/urania/tree/main/packages/sdk).

## Imports from @surf-ai/sdk

Everything comes from `@surf-ai/sdk`. Do NOT create local utility files for these.

**Frontend (`@surf-ai/sdk/react`):**

```tsx
import { useMarketPrice, useTokenHolders } from "@surf-ai/sdk/react"; // data hooks
```

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
| `/proxy/*`           | ANY    | Data API passthrough - `/proxy/market/price` -> hermod |

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

- `vite.config.ts` - proxy and build config
- `backend/server.js` - uses @surf-ai/sdk/server
- `entry-client.tsx` - app bootstrap with SSR hydration
- `entry-server.tsx` - SSR render for deploy
- `index.html` - cold-start guard and Surf badge
- `eslint.config.*` - lint rules
- `index.css` - only imports, do not add styles here (use Tailwind classes)

## Rules

- Use `@surf-ai/sdk/react` hooks in frontend, `@surf-ai/sdk/server` dataApi in backend
- Frontend packages are pre-installed - check `package.json` before installing
- Default to a dark theme unless the user explicitly asks for a different visual direction.
