import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test'
import path from 'path'
import { createServer } from '../../src/server/runtime'
import { startMockApiServer } from '../support/mock-api'

const TEST_PORT = 13579

describe('@surf-ai/sdk test-app integration', () => {
  const previousBaseUrl = process.env.SURF_API_BASE_URL
  const previousApiKey = process.env.SURF_API_KEY
  const api = startMockApiServer()
  let server: ReturnType<typeof createServer>

  beforeAll(async () => {
    process.env.SURF_API_BASE_URL = api.baseUrl
    process.env.SURF_API_KEY = 'test-api-key'
    server = createServer({
      port: TEST_PORT,
      routesDir: path.join(__dirname, 'backend', 'routes'),
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

  test('GET /api/health returns ok', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/health`)
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ status: 'ok' })
  })

  test('route auto-loading registered /api/btc', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/api/btc`)
    expect(res.status).toBe(200)
    expect(api.requests[0].pathname).toBe('/gateway/v1/market/price')
  })

  test('/proxy/* is not registered anymore', async () => {
    const res = await fetch(`http://localhost:${TEST_PORT}/proxy/market/price?symbol=BTC`)
    expect(res.status).toBe(404)
  })
})
