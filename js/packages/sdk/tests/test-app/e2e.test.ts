import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test'
import path from 'path'
import { get } from '../../src/data/client'
import { dataApi } from '../../src/data/data-api'
import { createServer } from '../../src/server/runtime'
import { startMockApiServer } from '../support/mock-api'

const E2E_PORT = 13580

describe('@surf-ai/sdk test-app e2e', () => {
  const previousBaseUrl = process.env.SURF_API_BASE_URL
  const previousApiKey = process.env.SURF_API_KEY
  const api = startMockApiServer()
  let server: ReturnType<typeof createServer>

  beforeAll(async () => {
    process.env.SURF_API_BASE_URL = api.baseUrl
    process.env.SURF_API_KEY = 'test-api-key'
    server = createServer({
      port: E2E_PORT,
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

  test('dataApi.market.price returns BTC data', async () => {
    const result = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
    expect(result.data).toHaveLength(1)
    expect((result.data[0] as any).symbol).toBe('BTC')
  })

  test('dataApi.market.ranking returns the requested number of rows', async () => {
    const result = await dataApi.market.ranking({ limit: '5', metric: 'market_cap' })
    expect(result.data).toHaveLength(5)
  })

  test('dataApi.get escape hatch works', async () => {
    const result = await get('market/price', { symbol: 'ETH', time_range: '1d' })
    expect(result.data).toHaveLength(1)
    expect(api.requests[0].headers.authorization).toBe('Bearer test-api-key')
  })

  test('dataApi.token.holders returns data', async () => {
    const result = await dataApi.token.holders({
      address: '0xdAC17F958D2ee523a2206206994597C13D831ec7',
      chain: 'ethereum',
    })
    expect(result.data).toHaveLength(1)
  })

  test('/api/btc backend route returns data through shared transport', async () => {
    const res = await fetch(`http://localhost:${E2E_PORT}/api/btc`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.data).toHaveLength(1)
    expect(api.requests[0].pathname).toBe('/gateway/v1/market/price')
  })

  test('/api/btc/escape-hatch uses the low-level client', async () => {
    const res = await fetch(`http://localhost:${E2E_PORT}/api/btc/escape-hatch`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.data).toHaveLength(1)
  })

  test('/api/health returns ok', async () => {
    const res = await fetch(`http://localhost:${E2E_PORT}/api/health`)
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ status: 'ok' })
  })

  test('/proxy is not mounted in 1.0', async () => {
    const res = await fetch(`http://localhost:${E2E_PORT}/proxy/market/price?symbol=BTC`)
    expect(res.status).toBe(404)
  })
})
