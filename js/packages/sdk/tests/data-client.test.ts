import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test'
import { readSurfApiConfig, DEFAULT_API_BASE_URL } from '../src/core/config'
import { get, post } from '../src/data/client'
import { dataApi } from '../src/data/data-api'
import { dbProvision, dbQuery, dbStatus } from '../src/db'
import { startMockApiServer } from './support/mock-api'

describe('@surf-ai/sdk data client', () => {
  const previousBaseUrl = process.env.SURF_API_BASE_URL
  const previousApiKey = process.env.SURF_API_KEY
  const api = startMockApiServer()

  beforeAll(() => {
    process.env.SURF_API_BASE_URL = api.baseUrl
    process.env.SURF_API_KEY = 'test-api-key'
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

  test('config uses the 1.0 default base URL when env is absent', () => {
    delete process.env.SURF_API_BASE_URL
    const config = readSurfApiConfig()
    expect(config.baseUrl).toBe(DEFAULT_API_BASE_URL)
    process.env.SURF_API_BASE_URL = api.baseUrl
  })

  test('get() throws when SURF_API_KEY is missing', async () => {
    delete process.env.SURF_API_KEY

    try {
      await get('market/price', { symbol: 'BTC' })
      throw new Error('expected get() to throw when SURF_API_KEY is missing')
    } catch (error: any) {
      expect(error.message).toContain('SURF_API_KEY is required')
    } finally {
      process.env.SURF_API_KEY = 'test-api-key'
    }
  })

  test('get() throws when SURF_API_BASE_URL is invalid', async () => {
    process.env.SURF_API_BASE_URL = 'not-a-url'

    try {
      await get('market/price', { symbol: 'BTC' })
      throw new Error('expected get() to throw when SURF_API_BASE_URL is invalid')
    } catch (error: any) {
      expect(error.message).toContain('cannot be parsed as a URL')
    } finally {
      process.env.SURF_API_BASE_URL = api.baseUrl
    }
  })

  test('get() sends bearer auth to the configured base URL', async () => {
    const result = await get('market/price', { symbol: 'BTC', time_range: '1d' })

    expect((result as any).data[0].symbol).toBe('BTC')
    expect(api.requests).toHaveLength(1)
    expect(api.requests[0].pathname).toBe('/gateway/v1/market/price')
    expect(api.requests[0].headers.authorization).toBe('Bearer test-api-key')
  })

  test('post() sends JSON body with bearer auth', async () => {
    await post('db/query', { sql: 'SELECT 1', params: [] })

    expect(api.requests).toHaveLength(1)
    expect(api.requests[0].pathname).toBe('/gateway/v1/db/query')
    expect(api.requests[0].headers.authorization).toBe('Bearer test-api-key')
    expect(api.requests[0].headers['content-type']).toContain('application/json')
    expect(api.requests[0].bodyJson).toEqual({ sql: 'SELECT 1', params: [] })
  })

  test('dataApi has all categories', () => {
    expect(dataApi.market).toBeDefined()
    expect(dataApi.token).toBeDefined()
    expect(dataApi.wallet).toBeDefined()
    expect(dataApi.onchain).toBeDefined()
    expect(dataApi.social).toBeDefined()
    expect(dataApi.project).toBeDefined()
    expect(dataApi.news).toBeDefined()
    expect(dataApi.exchange).toBeDefined()
    expect(dataApi.fund).toBeDefined()
    expect(dataApi.search).toBeDefined()
    expect(dataApi.web).toBeDefined()
    expect(dataApi.polymarket).toBeDefined()
    expect(dataApi.kalshi).toBeDefined()
    expect(dataApi.prediction_market).toBeDefined()
  })

  test('typed methods stringify numeric params through the shared transport', async () => {
    const result = await dataApi.market.ranking({ limit: 10 } as any)

    expect(result.data).toHaveLength(10)
    expect(api.requests).toHaveLength(1)
    expect(api.requests[0].search).toContain('limit=10')
  })

  test('db helpers reuse the same transport and auth', async () => {
    const provisioned = await dbProvision()
    const queried = await dbQuery('SELECT 1', [], { arrayMode: true })
    const status = await dbStatus()

    expect(provisioned.database).toBe('test_db')
    expect(queried.rows).toEqual([])
    expect(status.connected).toBe(true)
    expect(api.requests.map((request) => request.pathname)).toEqual([
      '/gateway/v1/db/provision',
      '/gateway/v1/db/query',
      '/gateway/v1/db/status',
    ])
    expect(api.requests.every((request) => request.headers.authorization === 'Bearer test-api-key')).toBe(true)
  })
})
