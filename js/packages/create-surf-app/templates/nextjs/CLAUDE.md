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

Query the database in API routes using the `db()` helper re-exported from `@/db`. Drizzle ORM is **only** used to declare the schema — there is no Drizzle client and no direct connection pool. `db(sql, params)` returns a pg-style result `{ rows, rowCount, fields }`, so destructure `rows`:

```ts
import { db } from "@/db";

export async function GET() {
  const { rows } = await db(
    "SELECT * FROM users ORDER BY created_at DESC"
  );
  return Response.json(rows);
}

export async function POST(request: Request) {
  const { name } = await request.json();
  const { rows } = await db(
    "INSERT INTO users (name) VALUES ($1) RETURNING *",
    [name]
  );
  return Response.json(rows[0]);
}
```

### Environment variables

Only variables prefixed with `NEXT_PUBLIC_` are exposed to the browser. To use a plain env var (e.g. `APP_TITLE`) in a client component, either read it in a server component / route handler and pass it down as a prop, or expose it through a backend route.

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
