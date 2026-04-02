# Next.js Template for create-surf-app

## Summary

Add a Next.js App Router template as an alternative to the existing Vite + Express default template. Selected via `create-surf-app --template nextjs`. The template consolidates frontend and backend into a single Next.js process with full feature parity: Surf SDK data access, Drizzle DB with auto schema sync, cron system, and the full shadcn/ui component library.

**Core principle:** The agent's workflow stays identical to the current template — create API routes, call `dataApi`, edit `db/schema.ts`. All infrastructure (schema sync, cron, boot) is invisible boilerplate the agent never touches.

## Template Structure

```
templates/nextjs/
├── CLAUDE.md                        # Agent rules
├── instrumentation.ts               # One-time server boot: schema sync + cron
├── next.config.ts                   # basePath, env config
├── package.json                     # next, react, @surf-ai/sdk, shadcn/ui, drizzle
├── tsconfig.json                    # Strict, @ alias for src root
├── tailwind.config.ts
├── .env                             # PORT (written by CLI)
├── app/
│   ├── layout.tsx                   # Root layout: html, body, dark theme, Providers
│   ├── page.tsx                     # Welcome page with Surf logo
│   ├── providers.tsx                # "use client" — QueryClientProvider, ThemeProvider
│   └── api/
│       ├── health/route.ts          # GET → { status: 'ok' }
│       ├── cron/route.ts            # CRUD for cron jobs (same endpoints as current)
│       └── __sync-schema/route.ts   # POST → manual schema sync trigger
├── db/
│   ├── index.ts                     # Exports db instance (lazy connection getter)
│   └── schema.ts                    # Drizzle table definitions (agent edits this)
├── lib/
│   ├── boot.ts                      # Schema sync + cron init logic (used by instrumentation.ts)
│   ├── cron.ts                      # Cron runner using croner
│   └── utils.ts                     # cn() utility
├── components/ui/                   # shadcn/ui components (same set as current template)
├── hooks/
│   └── use-toast.ts                 # Sonner toast hook
└── public/                          # Static assets (Surf logo, etc.)
```

## Architecture

### Data Flow

```
Client Component                       Next.js Server (same process)
─────────────────                      ──────────────────────────────

fetch('/api/prices')  ───────────────► app/api/prices/route.ts
                                         import { dataApi } from '@surf-ai/sdk/server'
                                         const data = await dataApi.market.price(...)
                      ◄─────────────── return Response.json(data)
```

- No proxy config, no port plumbing, no CORS — everything is same-origin
- Agent creates `app/api/xyz/route.ts` files (same mental model as `backend/routes/xyz.js`)
- `dataApi` auth is environment-aware (sandbox / deployed / public) — unchanged from current SDK
- Client components use `fetch('/api/...')` directly — basePath handled by Next.js automatically

### Server Boot via instrumentation.ts

Next.js's `instrumentation.ts` is the official hook for one-time server initialization. It runs once when the server process starts, survives HMR in dev, and does not re-execute on hot reloads. This is the direct equivalent of `createServer().start()` in the current Express template.

```ts
// instrumentation.ts
export async function register() {
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    const { syncSchema, watchSchema, startCron } = await import('./lib/boot');
    await syncSchema();
    if (process.env.NODE_ENV === 'development') {
      watchSchema(); // fs.watch on db/schema.ts → auto re-sync on edit
    }
    await startCron();
  }
}
```

**Why instrumentation.ts:**
- Runs exactly once per server lifetime — no duplicate cron jobs, no double schema sync
- Survives HMR — dev file edits don't reset initialization state
- Official Next.js API — not a hack, designed for this exact use case
- The `NEXT_RUNTIME === 'nodejs'` guard ensures it only runs in Node.js (not Edge)

### Schema Sync

Same mechanism as current template, adapted for Next.js lifecycle:

1. **On server boot** (`instrumentation.ts`): reads `db/schema.ts`, provisions Neon DB via SDK proxy, runs DDL to create/alter tables
2. **On file change in dev**: `fs.watch` on `db/schema.ts` triggers re-sync (debounced 1s)
3. **Manual trigger**: `POST /api/__sync-schema` endpoint for explicit sync
4. **Blocking**: API routes that import `db` will await the connection, which is initialized after schema sync completes

### Database Access

```ts
// db/index.ts — what the agent imports
import { dbQuery, dbProvision } from '@surf-ai/sdk/server';
// Lazy-initialized db connection
// Agent uses: import { db } from '@/db'
```

The `db/index.ts` file exports a db instance that lazily connects through the SDK's proxy. No `DATABASE_URL` env var needed — the SDK handles provisioning and connection through the Surf proxy, same as today.

### Cron System

Same as current template:
- Reads `cron.json` for job definitions
- Uses `croner` library for cron expression parsing
- Initialized once in `instrumentation.ts`, runs in-process
- CRUD endpoints at `/api/cron` (list, create, update, delete, manual trigger)
- Auth: requires `APP_TOKEN` in deployed mode, skipped in dev

### Client Components

**Rule: all agent-created components use `"use client"`.**

This keeps the agent's mental model identical to the current Vite template where everything is a client component by default. The CLAUDE.md instructs:

- Add `"use client"` at the top of all components in `components/` and `app/` pages
- Only `app/layout.tsx` is a Server Component (thin shell, agent doesn't touch it)
- Use TanStack Query for data fetching from API routes
- Use `fetch('/api/...')` for API calls

This avoids the cognitive overhead of client/server boundary decisions, which is the #1 source of Next.js App Router bugs in AI-generated code.

## CLI Changes

### New Flag

```
create-surf-app [project-name] --template <vite|nextjs>
```

- Default: `vite` (current behavior, no breaking change)
- `nextjs`: copies `templates/nextjs/`, writes `.env` with `PORT`

### Env File

For the `nextjs` template, `.env` contains:
```
PORT=3001
```

No `VITE_BACKEND_PORT` or `VITE_BASE` — Next.js uses `basePath` in `next.config.ts` and API routes are same-origin.

### Template Resolution

The CLI's `createSurfApp()` function gains a `template` option. Template directory resolved as `templates/{template}/` relative to the package.

## CLAUDE.md (Agent Instructions)

```markdown
# Surf App (Next.js)

## Architecture
- Next.js App Router with Route Handlers for API
- Surf SDK for data access (server-side only)
- Drizzle ORM for database
- shadcn/ui + Tailwind for frontend

## Creating API Routes
Create files in `app/api/` as Route Handlers:

    // app/api/prices/route.ts
    import { dataApi } from '@surf-ai/sdk/server';
    import { NextResponse } from 'next/server';

    export async function GET(request: Request) {
      const data = await dataApi.market.price({ symbol: 'BTC' });
      return NextResponse.json(data);
    }

## Data Access
- Use `dataApi` from `@surf-ai/sdk/server` in Route Handlers
- NEVER import `dataApi` in client components
- Use `fetch('/api/...')` in client components to call your API routes

## Database
- Define tables in `db/schema.ts` using Drizzle ORM
- Import `db` from `@/db` in Route Handlers for queries
- Schema syncs automatically on server start and when you edit `db/schema.ts`

## Frontend
- Add `"use client"` at the top of all components you create
- Use TanStack Query (`@tanstack/react-query`) for data fetching
- Dark theme by default unless user specifies otherwise

## Auto-created Endpoints
- `GET /api/health` → `{ status: 'ok' }`
- `POST /api/__sync-schema` → trigger schema sync
- `/api/cron` → CRUD for cron jobs

## Do NOT Modify
- `instrumentation.ts` (server boot)
- `next.config.ts` (build/deploy config)
- `db/index.ts` (DB connection)
- `lib/boot.ts`, `lib/cron.ts` (infrastructure)
- `app/layout.tsx`, `app/providers.tsx` (app shell)
```

## Package Dependencies

### Runtime
- `next` (latest stable, 15.x)
- `react`, `react-dom` (19.x)
- `@surf-ai/sdk` (0.1.4-beta)
- `drizzle-orm`
- `croner`

### UI (same as current template)
- `tailwindcss`, `@tailwindcss/postcss`
- Full shadcn/ui component set (button, dialog, form, tabs, etc.)
- Radix UI primitives
- `@tanstack/react-query`
- `next-themes`
- `sonner`
- `lucide-react`
- `react-hook-form`, `zod`
- `echarts`, `echarts-for-react`
- `date-fns`, `react-day-picker`
- `embla-carousel-react`, `vaul`, `react-resizable-panels`, `cmdk`
- `class-variance-authority`, `clsx`, `tailwind-merge`

### Dev
- `typescript`
- `@types/react`, `@types/react-dom`, `@types/node`
- `eslint`, `eslint-config-next`

## Deployment

Self-hosted, no Vercel dependency:

```bash
# Build
next build

# Run (production)
PORT=3001 next start
```

`next build` produces a standalone Node.js server. `next start` boots the server, triggers `instrumentation.ts` (schema sync + cron), and serves the app. No custom server needed.

## What This Design Does NOT Include

- Server Components for data fetching (intentionally avoided for agent simplicity)
- Server Actions (same reason — keeps a single, clear data flow pattern)
- Middleware for auth/request rewriting (can be added later)
- Static generation / ISR (all pages are dynamic by default)
- Docker / container config (out of scope for template)

## Migration Path From Current Template

For users who want to migrate an existing Vite + Express app:

| Current (Vite + Express) | Next.js Template |
|---|---|
| `backend/routes/xyz.js` | `app/api/xyz/route.ts` |
| `frontend/src/App.tsx` | `app/page.tsx` + client component |
| `frontend/src/components/` | `components/` |
| `backend/db/schema.js` | `db/schema.ts` |
| `backend/server.js` (createServer) | `instrumentation.ts` (register) |
| `fetch(api('xyz'))` | `fetch('/api/xyz')` |
| Two processes (frontend + backend) | Single process |
| Vite proxy config | Not needed (same-origin) |
