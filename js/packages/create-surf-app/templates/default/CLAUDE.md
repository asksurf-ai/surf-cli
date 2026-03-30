# Project Rules

## Do NOT Touch

- `vite.config.ts` — pre-configured proxy chain; changing it breaks preview.
- `.env` files — managed by the system; editing causes Vite restart & preview downtime.
- `entry-client.tsx` `notifyParentReady` / `data-surf-placeholder` — hosting app depends on these.
## npm install Rules

- **ALL packages in `frontend/package.json` are already installed.** Just import them directly — do NOT run `npm install` for them.
- Running `npm install` on pre-installed packages corrupts the node_modules tmpfs mount, kills the Vite dev server, and causes 5+ minute preview downtime (white screen).
- Only `npm install` packages that are NOT in package.json. When in doubt, read `frontend/package.json` first.

## Editing Rules

- **Read before edit** — always Read a file before using Edit or Write on it.
- **Edit, don't rewrite** — use the Edit tool on scaffold files (marked SCAFFOLD). Never use Write to replace them entirely; it breaks infrastructure (proxy, HMR, cold-start guards).
- **One file per tool call** — enables streaming progress and faster error recovery.
- **200-line limit** — split components exceeding 200 lines into sub-components.

## Storage Rules

- **Never use localStorage or in-memory arrays for persistent user data** (CRUD, todos, plans, bookmarks, notes, watchlists, etc.). Read `.claude/skills/database/SKILL.md` and use PostgreSQL + Drizzle ORM.
- localStorage is ONLY acceptable for: theme preference, UI collapsed/expanded state, session tokens.

## Data Table Rules

- **List tables need sorting + pagination + search.** Tables displaying browsable collections (token lists, tx history, leaderboards — unbounded row count) MUST use the SortableTable pattern from `component-reference/references/data-display.md`. Plain `<Table>` is fine for summary/comparison tables or small bounded-size API results (top-5, recent-3).

## First-Edit Checklist (do on every new project)

- **Page title** — ALWAYS update `<title>App</title>` in `frontend/index.html` to a meaningful title matching the website purpose. Never ship with `<title>App</title>`.
- **Favicon** — `frontend/public/favicon.ico` is already provided; do NOT delete or overwrite it.

## Common Mistakes to Avoid

- **API URLs** — NEVER use bare `/proxy/...` or `/api/...`. Always use `${API_BASE}/proxy/...`, `${API_BASE}/api/...`, or the generated hooks in `api.ts`.
- **No React.lazy / dynamic import()** — HMR is unavailable in preview; causes "Invalid hook call" crashes.
- **Hooks at top level** — ALL React hooks MUST be called before any conditional `return`. Use `useQuery({ enabled: !!condition })` for conditional fetching.
- **SSR safety** — NEVER access `document`, `window`, `localStorage` at module top level or during render. Use `typeof window !== 'undefined'` or `useEffect`.
- **API response safety** — NEVER render API fields without type-checking. Objects as React children cause white-screen crashes: `typeof x === 'string' ? x : x?.name ?? ''`.
- **ThemeProvider** — MUST set `storageKey="surf-studio-theme"` to avoid conflicts with the parent Surf app iframe.
- **No mock data** — never hardcode data. Use real API calls; show proper error/empty states.
- **ASCII only** — no curly quotes, en-dash, em-dash in code. Non-ASCII punctuation causes build failures.
