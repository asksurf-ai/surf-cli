# Project

Built with [Surf SDK](https://github.com/cyberconnecthq/urania/tree/main/packages/sdk) and Next.js App Router.

## Imports from @surf-ai/sdk

Use `@surf-ai/sdk/server` in API route handlers to talk to Surf data APIs.
Do not import `@surf-ai/sdk/server` in client components.
Use `fetch('/api/...')` in client components to call your own API routes.

**API Route Handler (`@surf-ai/sdk/server`):**

```ts
import { dataApi } from "@surf-ai/sdk/server";
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
app/page.tsx              - build your UI here
app/api/*/route.ts        - add API route handlers
components/               - add components (use "use client" directive)
db/schema.ts              - define database tables
```

## Built-in Endpoints

These are provided automatically - do NOT create routes for them:

| Endpoint             | Method | Purpose                                        |
| -------------------- | ------ | ---------------------------------------------- |
| `/api/health`        | GET    | Health check - `{ status: 'ok' }`              |
| `/api/__sync-schema` | POST   | Sync `db/schema.ts` tables to database         |
| `/api/cron`          | GET    | List cron jobs                                 |
| `/api/cron`          | POST   | Create a new cron task                         |

## Creating API Routes

Create Route Handler files in `app/api/`:

```ts
// app/api/prices/route.ts
import { dataApi } from '@surf-ai/sdk/server'

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url)
  const symbol = searchParams.get('symbol') || 'BTC'
  const data = await dataApi.market.price({ symbol })
  return Response.json(data)
}
```

## Database

Define tables in `db/schema.ts` using Drizzle ORM:

```ts
import { pgTable, serial, text, timestamp } from "drizzle-orm/pg-core";
export const users = pgTable("users", {
  id: serial("id").primaryKey(),
  name: text("name").notNull(),
  created_at: timestamp("created_at").defaultNow(),
});
```

Tables are auto-synced on server start and when `db/schema.ts` changes in dev mode.

Query the database in API routes using `@surf-ai/sdk/db` — **not** Drizzle ORM query builder:

```ts
import { dbQuery } from "@surf-ai/sdk/db";

export async function GET() {
  const rows = await dbQuery("SELECT * FROM users ORDER BY created_at DESC");
  return Response.json(rows);
}

export async function POST(request: Request) {
  const { name } = await request.json();
  const [row] = await dbQuery(
    "INSERT INTO users (name) VALUES ($1) RETURNING *",
    [name]
  );
  return Response.json(row);
}
```

Database runs through an HTTP proxy — there is no direct `db` connection object or `req.db` middleware. Use `dbQuery(sql, params)` for all reads and writes.

## Do NOT modify

- `instrumentation.ts` - server boot (schema sync, cron)
- `next.config.ts` - build/deploy config
- `db/index.ts` - database connection
- `lib/boot.ts` - infrastructure (schema sync, cron init)
- `app/layout.tsx` - root layout and providers
- `app/providers.tsx` - client-side preview bridge hooks
- `eslint.config.mjs` - lint rules
- `globals.css` - only imports, do not add styles here (use Tailwind classes)

## Rules

- Add `"use client"` at the top of all components you create
- Use `fetch('/api/...')` in client components to call API routes
- Use `@surf-ai/sdk/server` `dataApi` in route handlers when you need Surf data
- Do not bypass your API routes from client components
- Packages are pre-installed - check `package.json` before installing
- Default to a dark theme unless the user explicitly asks for a different visual direction
