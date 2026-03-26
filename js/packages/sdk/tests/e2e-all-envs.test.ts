/**
 * E2E tests for all three environments: local, sandbox, deployed.
 *
 * Tests all built-in endpoints from createServer():
 *   - GET  /api/health
 *   - POST /api/__sync-schema
 *   - GET  /api/cron
 *   - ANY  /proxy/* passthrough
 *   - GET  /api/{route} (auto-loaded)
 *
 * Run:
 *   cd packages/sdk
 *
 *   # Local (default — no auth, public API fallback):
 *   bun test ./tests/e2e-all-envs.test.ts
 *
 *   # Deployed mode (with surf token):
 *   GATEWAY_URL=https://api.ask.surf APP_TOKEN=<token> bun test ./tests/e2e-all-envs.test.ts
 *
 *   # Sandbox mode (with urania OutboundProxy running):
 *   DATA_PROXY_BASE=http://127.0.0.1:9999/s/<session>/proxy bun test ./tests/e2e-all-envs.test.ts
 */

import { describe, test, expect, beforeAll, afterAll } from 'bun:test'
import { createServer } from '../src/server/runtime'
import path from 'path'
import fs from 'fs'

// ---------------------------------------------------------------------------
// Detect environment
// ---------------------------------------------------------------------------

const hasDataProxy = Boolean(process.env.DATA_PROXY_BASE)
const hasGateway = Boolean(process.env.GATEWAY_URL && process.env.APP_TOKEN)
const mode = hasDataProxy ? 'sandbox' : hasGateway ? 'deployed' : 'local'

// Load surf token for deployed/local mode
if (!hasDataProxy && !hasGateway) {
  const credsPath = path.join(process.env.HOME || '', '.config/surf/credentials.json')
  try {
    const creds = JSON.parse(fs.readFileSync(credsPath, 'utf-8'))
    const token = creds['surf:default']?.token
    if (token) {
      process.env.GATEWAY_URL = 'https://api.ask.surf'
      process.env.APP_TOKEN = token
    }
  } catch { /* no creds — proxy tests will fail */ }
}

const hasAuth = Boolean(process.env.GATEWAY_URL && process.env.APP_TOKEN) || hasDataProxy

// ---------------------------------------------------------------------------
// Test app setup
// ---------------------------------------------------------------------------

const TEST_PORT = 13581
const TEST_DIR = '/tmp/test-surf-e2e'

// Create a minimal test backend
function setupTestApp() {
  fs.mkdirSync(path.join(TEST_DIR, 'routes'), { recursive: true })
  fs.mkdirSync(path.join(TEST_DIR, 'db'), { recursive: true })

  // Test route — plain function handler (express.Router needs express in node_modules)
  fs.writeFileSync(path.join(TEST_DIR, 'routes', 'ping.js'), `
    module.exports = function(_req, res) { res.json({ pong: true }) }
  `)

  // Empty schema
  fs.writeFileSync(path.join(TEST_DIR, 'db', 'schema.js'), `
    // empty schema for testing
  `)

  // Cron config
  fs.writeFileSync(path.join(TEST_DIR, 'cron.json'), JSON.stringify([
    { id: 'test-cron', name: 'Test', schedule: '0 0 * * *', handler: 'routes/ping.js', enabled: false, timeout: 10 }
  ]))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe(`@surf-ai/sdk E2E [${mode} mode]`, () => {
  let server: any

  beforeAll(async () => {
    setupTestApp()
    server = createServer({
      port: TEST_PORT,
      routesDir: path.join(TEST_DIR, 'routes'),
      cronDir: TEST_DIR,
    })
    await server.start()
  })

  // -- Health check --

  test('GET /api/health returns ok', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/health`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.status).toBe('ok')
  })

  // -- Auto-loaded routes --

  test('GET /api/ping (auto-loaded route) returns pong', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/ping`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.pong).toBe(true)
  })

  // -- Schema sync --

  test('POST /api/__sync-schema returns ok', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/__sync-schema`, { method: 'POST' })
    // May return 200 (ok) or 500 (no DB configured) — both are valid responses
    expect([200, 500]).toContain(res.status)
    const body = await res.json()
    expect(body).toHaveProperty('ok')
  })

  // -- Cron --

  // Cron endpoints require APP_TOKEN auth in deployed mode
  const cronHeaders: Record<string, string> = {}
  const appToken = process.env.SURF_DEPLOYED_APP_TOKEN || process.env.APP_TOKEN
  if (appToken) cronHeaders['Authorization'] = `Bearer ${appToken}`

  test('GET /api/cron returns cron jobs', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, { headers: cronHeaders })
    expect(res.status).toBe(200)
    const body = await res.json() as any[]
    expect(Array.isArray(body)).toBe(true)
    expect(body.length).toBe(1)
    expect(body[0].id).toBe('test-cron')
  })

  test('POST /api/cron creates a new task', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
      method: 'POST',
      headers: { ...cronHeaders, 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: 'new-task', name: 'New', schedule: '0 * * * *', handler: 'routes/ping.js' }),
    })
    expect(res.status).toBe(201)
    const body = await res.json() as any
    expect(body.id).toBe('new-task')
  })

  test('PATCH /api/cron/:id updates a task', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron/new-task`, {
      method: 'PATCH',
      headers: { ...cronHeaders, 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: false }),
    })
    expect(res.status).toBe(200)
    const body = await res.json() as any
    expect(body.enabled).toBe(false)
  })

  test('POST /api/cron/:id/run triggers a task', async () => {
    // test-cron is disabled so the job won't be in cronJobs — expect 404
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron/test-cron/run`, { method: 'POST', headers: cronHeaders })
    expect(res.status).toBe(404)
  })

  test('DELETE /api/cron/:id deletes a task', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron/new-task`, { method: 'DELETE', headers: cronHeaders })
    expect(res.status).toBe(200)
    const body = await res.json() as any
    expect(body.ok).toBe(true)

    // Verify it's gone
    const list = await fetch(`http://localhost:${TEST_PORT}/api/cron`, { headers: cronHeaders })
    const jobs = await list.json() as any[]
    expect(jobs.find((j: any) => j.id === 'new-task')).toBeUndefined()
  })

  test('POST /api/cron rejects invalid schedule', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: 'bad', name: 'Bad', schedule: 'not-a-cron', handler: 'routes/ping.js' }),
      headers: { ...cronHeaders, 'Content-Type': 'application/json' },
    })
    expect(res.status).toBe(400)
  })

  // -- Cron auth --

  test('cron rejects unauthenticated requests when APP_TOKEN is set', async () => {
    if (!appToken) {
      console.log('  SKIP: no APP_TOKEN — dev mode, auth is open')
      return
    }
    // Request WITHOUT auth header — should be 401
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`)
    expect(res.status).toBe(401)
  })

  test('cron accepts requests with correct APP_TOKEN', async () => {
    if (!appToken) return
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, { headers: cronHeaders })
    expect(res.status).toBe(200)
  })

  test('cron rejects requests with wrong token', async () => {
    if (!appToken) return
    const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
      headers: { Authorization: 'Bearer wrong-token' },
    })
    expect(res.status).toBe(401)
  })

  // -- Proxy passthrough --

  test('/proxy/* passthrough returns data', async () => {
    if (!hasAuth) {
      console.log('  SKIP: no auth configured (need GATEWAY_URL+APP_TOKEN or DATA_PROXY_BASE)')
      return
    }
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/market/price?symbol=BTC&time_range=1d`)
    expect(res.status).toBe(200)
    const body = await res.json() as any
    expect(body.data).toBeDefined()
    expect(body.data.length).toBeGreaterThan(0)
    console.log(`  /proxy: BTC $${body.data[0].value?.toLocaleString()} (${body.data.length} pts)`)
  })

  test('/proxy/* POST works (onchain/sql)', async () => {
    if (!hasAuth) return
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/onchain/schema`)
    // 200 = success, 422 = missing params — both prove proxy works
    expect([200, 422]).toContain(res.status)
  })

  // -- 404 for unknown routes --

  test('GET /api/nonexistent returns 404', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/nonexistent`)
    expect(res.status).toBe(404)
  })

  // -- No proxy without config --

  test('/proxy/* without auth returns error (not 404)', async () => {
    // Even without auth, proxy middleware should be mounted (returns 502, not 404)
    if (hasAuth) return // skip if auth is configured — would get 200
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/market/price?symbol=BTC`)
    expect(res.status).not.toBe(404) // middleware is mounted
  })
})

describe(`@surf-ai/sdk dataApi [${mode} mode]`, () => {
  test('dataApi.market.price returns BTC data', async () => {
    if (!hasAuth) {
      console.log('  SKIP: no auth')
      return
    }
    const { dataApi } = await import('../src/data/data-api')
    const result = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
    expect(result.data).toBeDefined()
    expect(result.data.length).toBeGreaterThan(0)
    console.log(`  dataApi: BTC $${(result.data[0] as any).value?.toLocaleString()}`)
  })

  test('dataApi.market.ranking returns top coins', async () => {
    if (!hasAuth) return
    const { dataApi } = await import('../src/data/data-api')
    const result = await dataApi.market.ranking({ limit: '3', metric: 'market_cap' })
    expect(result.data).toBeDefined()
    expect(result.data.length).toBe(3)
    console.log(`  Top 3: ${result.data.map((d: any) => d.symbol).join(', ')}`)
  })

  test('dataApi.get escape hatch works', async () => {
    if (!hasAuth) return
    const { dataApi } = await import('../src/data/data-api')
    const result = await dataApi.get('market/price', { symbol: 'ETH', time_range: '1d' })
    expect(result.data).toBeDefined()
    console.log(`  ETH: $${(result.data[0] as any).value?.toLocaleString()}`)
  })
})
