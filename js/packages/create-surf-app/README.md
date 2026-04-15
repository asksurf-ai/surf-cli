# create-surf-app

Scaffold a Surf app pre-wired with [`@surf-ai/sdk`](../sdk).

## Usage

```bash
npm create surf-app@latest [project-name] [--template <vite|nextjs>]
```

- `project-name` — directory to create (defaults to current directory)
- `--template` — `vite` (default) or `nextjs`

Examples:

```bash
npm create surf-app@latest my-app
npm create surf-app@latest my-app --template nextjs
npm create surf-app@latest .  # scaffold into current dir
```

## Templates

| Template | Stack |
| --- | --- |
| `vite` (default) | Vite + React + Express backend using `@surf-ai/sdk/server` |
| `nextjs` | Next.js App Router with API route handlers using `@surf-ai/sdk/server` |

## Environment variables

`SURF_API_KEY` is the only secret you need to provide. Everything else has a sensible default — but `BASE_PATH` must be **defined** (empty string is fine and means root) because the scaffolds read it unconditionally.

### `vite` template

`backend/.env`:

| Var | Required | Default | Purpose |
| --- | --- | --- | --- |
| `SURF_API_KEY` | yes (non-empty) | — | Bearer token for Surf upstream + protected runtime endpoints |
| `BACKEND_PORT` | no | `3001` | Express server port |
| `SURF_API_BASE_URL` | no | `https://api.asksurf.ai/gateway/v1` | Override Surf API base URL |

`frontend/.env`:

| Var | Required | Default | Purpose |
| --- | --- | --- | --- |
| `BASE_PATH` | yes (empty OK) | — | Vite base path (e.g. `/preview/abc/`); empty means root |
| `PORT` | no | `5173` | Vite dev server port |
| `BACKEND_PORT` | no | `3001` | Backend port the dev server proxies `/api` to |

### `nextjs` template

`.env`:

| Var | Required | Default | Purpose |
| --- | --- | --- | --- |
| `SURF_API_KEY` | yes (non-empty) | — | Bearer token for Surf upstream + protected runtime endpoints |
| `BASE_PATH` | yes (empty OK) | — | Next.js `basePath` (e.g. `/preview/abc`); empty means root |
| `PORT` | no | `3000` | Next.js server port |
| `SURF_API_BASE_URL` | no | `https://api.asksurf.ai/gateway/v1` | Override Surf API base URL |

`SURF_API_KEY` is enforced at **dev/start** time, not build time — `npm run build` succeeds without it so CI builds don't need the secret.

See [`@surf-ai/sdk` README](../sdk/README.md) for full SDK configuration and runtime details.
