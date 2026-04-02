# @surf-ai/sdk E2E Testing Guide

## Prerequisites

```bash
bun install              # SDK dependencies
surf login               # refresh API token (for deployed/sandbox tests)
```

For sandbox mode: urania eval container running (see `docs/local-eval.md`).
For deployed (internal) mode: Tailscale connected to cluster.

## Quick Run

```bash
cd packages/sdk
bun test                 # all tests, all files
```

---

## Environment Matrix

| Mode | What's tested | Env vars needed |
|------|--------------|----------------|
| Dev | createServer endpoints, cron CRUD (no data API) | none |
| Deployed (public) | all endpoints + data API via api.asksurf.ai | `SURF_DEPLOYED_GATEWAY_URL` + `SURF_DEPLOYED_APP_TOKEN` |
| Deployed (internal) | all endpoints + data API via hermod internal | same, with internal URL |
| Sandbox | all endpoints + data API via OutboundProxy | `SURF_SANDBOX_PROXY_BASE` |

---

## 1. Dev Mode (no auth)

No setup needed. Data API and proxy tests are skipped (no auth configured). Cron endpoints are open.

**Run:**
```bash
cd packages/sdk
bun test ./tests/e2e-all-envs.test.ts
```

**Expected output:**
```
Route registered: /api/ping
Backend listening on port 13581
Schema sync complete, API ready
  SKIP: no APP_TOKEN — dev mode, auth is open
  SKIP: no auth configured (need GATEWAY_URL+APP_TOKEN or DATA_PROXY_BASE)

 14 pass
 0 fail
```

**What's tested:**
- `GET /api/health` → 200 `{ status: 'ok' }`
- `GET /api/ping` (auto-loaded route) → 200 `{ pong: true }`
- `POST /api/__sync-schema` → 200 or 500 (no DB)
- `GET /api/cron` → 200 (list jobs)
- `POST /api/cron` → 201 (create job)
- `PATCH /api/cron/:id` → 200 (update job)
- `DELETE /api/cron/:id` → 200 (delete job)
- `POST /api/cron/:id/run` → 404 (disabled job)
- `POST /api/cron` with bad schedule → 400
- `GET /api/nonexistent` → 404
- Cron auth tests → SKIP (no token)
- Proxy/dataApi tests → SKIP (no auth)

---

## 2. Deployed Mode (public hermod)

Uses `api.asksurf.ai` with a surf CLI token. Tests full data API + proxy + cron auth.

**Setup:**
```bash
surf login   # refresh token if expired
```

**Run:**
```bash
SURF_TOKEN=$(python3 -c "import json; print(json.load(open('$HOME/.config/surf/credentials.json'))['surf:default']['token'])")

SURF_DEPLOYED_GATEWAY_URL=https://api.asksurf.ai \
SURF_DEPLOYED_APP_TOKEN=$SURF_TOKEN \
bun test ./tests/e2e-all-envs.test.ts
```

**Expected output:**
```
Route registered: /api/ping
Backend listening on port 13581
Schema sync complete, API ready
  /proxy: BTC $70,929 (288 pts)
  dataApi: BTC $70,929
  Top 3: BTC, ETH, USDT
  ETH: $2,164

 19 pass
 0 fail
```

**What's tested (in addition to dev mode):**
- `/proxy/market/price?symbol=BTC` → 200 with real BTC price data
- `/proxy/onchain/schema` → 200 or 422 (proxy works)
- `dataApi.market.price()` → BTC price via typed SDK method
- `dataApi.market.ranking()` → top 3 coins
- `dataApi.get()` → ETH price via escape hatch
- Cron without token → 401
- Cron with correct token → 200
- Cron with wrong token → 401

---

## 3. Deployed Mode (internal hermod via Tailscale)

Same as public but uses the cluster-internal hermod URL. Requires Tailscale.

**Setup:**
```bash
surf login
tailscale status   # verify connected
```

**Run:**
```bash
SURF_TOKEN=$(python3 -c "import json; print(json.load(open('$HOME/.config/surf/credentials.json'))['surf:default']['token'])")

SURF_DEPLOYED_GATEWAY_URL=http://hermod-api.app.svc.cluster.local:8080 \
SURF_DEPLOYED_APP_TOKEN=$SURF_TOKEN \
bun test ./tests/e2e-all-envs.test.ts
```

**Expected output:** same as public (19 pass).

---

## 4. Sandbox Mode (urania OutboundProxy)

Uses the urania eval container's OutboundProxy. The proxy injects hermod JWT automatically — no token needed from the test.

**Setup:**

1. Start the urania eval container (see `docs/local-eval.md`):
```bash
docker run -d --name urania-eval \
  --network host --privileged --user root \
  --env-file .env.eval \
  -e "HERMOD_JWT_PRIVATE_KEY=$(cat .hermod-jwt-key.pem)" \
  -v $(pwd)/workspaces:/workspaces \
  -v $(pwd)/scripts/eval-entrypoint.sh:/app/eval-entrypoint.sh:ro \
  urania-agent /app/eval-entrypoint.sh
```

2. Wait for startup:
```bash
curl http://localhost:8088/health   # should return { "status": "ok" }
```

3. Register a session (OutboundProxy needs a session → user_id mapping):
```bash
curl -sN -X POST http://localhost:8088/agent/chat/stream \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "0dfc0532-b540-4740-baa4-fda257eff8f9",
    "session_id": "sdk-e2e-test",
    "message": "say hi",
    "mode": "vibe",
    "timeout": 60
  }' | grep "event: turn_done"
```

**Run:**
```bash
SURF_SANDBOX_PROXY_BASE=http://127.0.0.1:9999/s/sdk-e2e-test/proxy \
bun test ./tests/e2e-all-envs.test.ts
```

**Expected output:**
```
Route registered: /api/ping
Backend listening on port 13581
Schema sync complete, API ready
  /proxy: BTC $70,846 (288 pts)
  dataApi: BTC $70,846
  Top 3: BTC, ETH, USDT
  ETH: $2,161

 19 pass
 0 fail
```

**What's different from deployed:**
- No `APP_TOKEN` set → cron auth is open (dev mode)
- Data flows through OutboundProxy → JWT injected → hermod
- Same real data, different auth path

---

## Summary

| Mode | Setup | Env Vars | Tests | Data API |
|------|-------|----------|-------|----------|
| Dev | none | none | 14 | skipped |
| Deployed (public) | `surf login` | `SURF_DEPLOYED_GATEWAY_URL=https://api.asksurf.ai` + token | 19 | via api.asksurf.ai |
| Deployed (internal) | `surf login` + Tailscale | `SURF_DEPLOYED_GATEWAY_URL=http://hermod-api.svc:8080` + token | 19 | via Tailscale |
| Sandbox | urania container + session | `SURF_SANDBOX_PROXY_BASE=http://127.0.0.1:9999/s/{sid}/proxy` | 19 | via OutboundProxy |

## Troubleshooting

| Error | Fix |
|-------|-----|
| `API error 401: token expired` | Run `surf login` |
| `ECONNREFUSED` on proxy | Tailscale not connected, or urania container not running |
| `address already in use :13581` | Kill stale test: `lsof -ti :13581 \| xargs kill` |
| Cron returns 401 in dev | `APP_TOKEN` is set in env — `unset APP_TOKEN SURF_DEPLOYED_APP_TOKEN` |
| `session_ready timeout` in sandbox | Session expired — re-register with a new chat/stream request |
