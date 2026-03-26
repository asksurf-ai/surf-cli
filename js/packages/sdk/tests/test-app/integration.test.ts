/**
 * Integration test for @surf-ai/sdk test app.
 *
 * Tests the full stack:
 *   1. createServer() starts Express with /proxy/* and route auto-loading
 *   2. /api/health returns ok
 *   3. /api/btc route (agent-written) calls dataApi.market.price()
 *   4. /proxy/market/price passthrough works
 *
 * Run modes:
 *   - Standalone: DATA_PROXY_BASE not set → hits api.ask.surf (needs internet)
 *   - With urania: DATA_PROXY_BASE=http://127.0.0.1:9999/s/test/proxy → hits OutboundProxy
 *
 * Usage:
 *   cd packages/sdk
 *   bun test ./tests/test-app/integration.test.ts
 *
 *   # With local urania eval container running:
 *   DATA_PROXY_BASE=http://127.0.0.1:9999/s/test/proxy bun test ./tests/test-app/integration.test.ts
 */

import { describe, test, expect, beforeAll, afterAll } from 'bun:test'
import { createServer } from '../../src/server/runtime'
import path from 'path'

const TEST_PORT = 13579

describe('@surf-ai/sdk test-app integration', () => {
  let server: any

  beforeAll(async () => {
    // Point route loading at our test routes
    const routesDir = path.join(__dirname, 'backend', 'routes')

    server = createServer({
      port: TEST_PORT,
      routesDir,
    })
    await server.start()
  })

  afterAll(() => {
    // Express doesn't have a built-in close, but the process will exit
  })

  test('GET /api/health returns ok', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/health`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.status).toBe('ok')
  })

  test('route auto-loading registered /api/btc', async () => {
    // This will fail with connection error if no upstream, but should not 404
    const res = await fetch(`http://localhost:${TEST_PORT}/api/btc`)
    // If DATA_PROXY_BASE is set and upstream is running, we get 200
    // If not, we get 500 (upstream error), but NOT 404 (route not found)
    expect(res.status).not.toBe(404)
  })

  test('/proxy/* passthrough is registered', async () => {
    // Test that the proxy middleware is mounted
    // Will fail upstream but should not 404
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/market/price?symbol=BTC`)
    // 502 = proxy error (no upstream), not 404 (no route)
    expect(res.status).not.toBe(404)
  })
})
