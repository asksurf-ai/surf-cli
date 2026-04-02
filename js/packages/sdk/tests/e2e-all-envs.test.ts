import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test'
import fs from 'fs'
import path from 'path'
import { createServer } from '../src/server/runtime'
import { dataApi } from '../src/data/data-api'
import { startMockApiServer } from './support/mock-api'

const TEST_PORT = 13581
const TEST_DIR = '/tmp/test-surf-sdk-1-0'

function adminHeaders() {
  return {
    Authorization: `Bearer ${process.env.SURF_API_KEY}`,
    'Content-Type': 'application/json',
  }
}

function setupTestApp() {
  fs.rmSync(TEST_DIR, { recursive: true, force: true })
  fs.mkdirSync(path.join(TEST_DIR, 'routes'), { recursive: true })
  fs.mkdirSync(path.join(TEST_DIR, 'db'), { recursive: true })

  fs.writeFileSync(
    path.join(TEST_DIR, 'routes', 'ping.js'),
    "module.exports = function(_req, res) { res.json({ pong: true }) }\n",
  )

  fs.writeFileSync(
    path.join(TEST_DIR, 'routes', 'invalid-shape.js'),
    "module.exports = { default: function(_req, res) { res.json({ shouldNotLoad: true }) } }\n",
  )

  fs.writeFileSync(
    path.join(TEST_DIR, 'db', 'schema.js'),
    '// empty schema for testing\n',
  )

  fs.writeFileSync(
    path.join(TEST_DIR, 'cron.json'),
    JSON.stringify([
      { id: 'test-cron', name: 'Test', schedule: '0 0 * * *', handler: 'routes/ping.js', enabled: false, timeout: 10 },
    ]),
  )
}

describe('@surf-ai/sdk e2e [1.0 direct auth]', () => {
  const previousBaseUrl = process.env.SURF_API_BASE_URL
  const previousApiKey = process.env.SURF_API_KEY
  const api = startMockApiServer()
  let server: ReturnType<typeof createServer>

  beforeAll(async () => {
    process.env.SURF_API_BASE_URL = api.baseUrl
    process.env.SURF_API_KEY = 'test-api-key'
    setupTestApp()
    server = createServer({
      port: TEST_PORT,
      routesDir: path.join(TEST_DIR, 'routes'),
      cronDir: TEST_DIR,
    })
    await server.start()
  })

  afterAll(() => {
    api.stop()
    if (previousBaseUrl === undefined) delete process.env.SURF_API_BASE_URL
    else process.env.SURF_API_BASE_URL = previousBaseUrl
    if (previousApiKey === undefined) delete process.env.SURF_API_KEY
    else process.env.SURF_API_KEY = previousApiKey
  })

  beforeEach(() => {
    api.clear()
  })

  test('createServer throws when no port is provided and BACKEND_PORT is unset', () => {
    const previousBackendPort = process.env.BACKEND_PORT
    const previousPort = process.env.PORT
    delete process.env.BACKEND_PORT
    delete process.env.PORT

    try {
      expect(() => createServer()).toThrow('createServer requires a port via options.port or BACKEND_PORT env var')
    } finally {
      if (previousBackendPort === undefined) delete process.env.BACKEND_PORT
      else process.env.BACKEND_PORT = previousBackendPort
      if (previousPort === undefined) delete process.env.PORT
      else process.env.PORT = previousPort
    }
  })

  test('GET /api/health returns ok', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/health`)
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ status: 'ok' })
  })

  test('GET /api/ping returns auto-loaded route output', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/ping`)
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ pong: true })
  })

  test('route modules must export the handler directly', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/invalid-shape`)
    expect(res.status).toBe(404)
  })

  test('POST /api/__sync-schema rejects unauthenticated requests', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/__sync-schema`, {
      method: 'POST',
    })
    expect(res.status).toBe(401)
  })

  test('POST /api/__sync-schema accepts the configured bearer token', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/__sync-schema`, {
      method: 'POST',
      headers: adminHeaders(),
    })
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ ok: true })
  })

  test('cron endpoints require the configured bearer token', async () => {
    const unauthenticated = await fetch(`http://localhost:${TEST_PORT}/api/cron`)
    expect(unauthenticated.status).toBe(401)

    const authenticated = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
      headers: adminHeaders(),
    })
    expect(authenticated.status).toBe(200)
    const body = await authenticated.json() as any[]
    expect(body).toHaveLength(1)
    expect(body[0].id).toBe('test-cron')
  })

  test('admin endpoints return 503 when SURF_API_KEY is not configured', async () => {
    const previousApiKey = process.env.SURF_API_KEY
    delete process.env.SURF_API_KEY

    try {
      const res = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
        headers: {
          Authorization: 'Bearer test-api-key',
        },
      })
      expect(res.status).toBe(503)
      expect(await res.json()).toEqual({ error: 'SURF_API_KEY is not configured' })
    } finally {
      if (previousApiKey === undefined) delete process.env.SURF_API_KEY
      else process.env.SURF_API_KEY = previousApiKey
    }
  })

  test('cron create, update, run, and delete flow works with auth', async () => {
    const created = await fetch(`http://localhost:${TEST_PORT}/api/cron`, {
      method: 'POST',
      headers: adminHeaders(),
      body: JSON.stringify({ id: 'new-task', name: 'New', schedule: '0 * * * *', handler: 'routes/ping.js' }),
    })
    expect(created.status).toBe(201)

    const updated = await fetch(`http://localhost:${TEST_PORT}/api/cron/new-task`, {
      method: 'PATCH',
      headers: adminHeaders(),
      body: JSON.stringify({ enabled: false }),
    })
    expect(updated.status).toBe(200)
    expect((await updated.json() as any).enabled).toBe(false)

    const run = await fetch(`http://localhost:${TEST_PORT}/api/cron/new-task/run`, {
      method: 'POST',
      headers: adminHeaders(),
    })
    expect(run.status).toBe(404)

    const deleted = await fetch(`http://localhost:${TEST_PORT}/api/cron/new-task`, {
      method: 'DELETE',
      headers: adminHeaders(),
    })
    expect(deleted.status).toBe(200)
    expect(await deleted.json()).toEqual({ ok: true })
  })

  test('/proxy/* is no longer provided by the runtime', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/market/price?symbol=BTC`)
    expect(res.status).toBe(404)
  })

  test('dataApi direct calls still work through the shared transport', async () => {
    const result = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
    expect(result.data).toHaveLength(1)
    expect(api.requests[0].pathname).toBe('/gateway/v1/market/price')
    expect(api.requests[0].headers.authorization).toBe('Bearer test-api-key')
  })
})
