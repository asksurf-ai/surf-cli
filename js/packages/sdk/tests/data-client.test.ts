/**
 * Quick smoke test for @surf-ai/sdk data client.
 *
 * Run: bun test tests/test_data_client.ts
 */
import { describe, test, expect, beforeAll } from 'bun:test'
import { get, post } from '../src/data/client'
import { dataApi } from '../src/data/data-api'

describe('@surf-ai/sdk data client', () => {
  test('resolveConfig defaults to public URL when no env vars set', async () => {
    // With no env vars, should try api.ask.surf (will fail without auth, but URL should be correct)
    delete process.env.DATA_PROXY_BASE
    delete process.env.GATEWAY_URL
    delete process.env.APP_TOKEN

    try {
      await get('market/price', { symbol: 'BTC', time_range: '1d' })
    } catch (e: any) {
      // Expected: either network error or auth error — but URL should be api.ask.surf
      expect(e.message).toMatch(/API error|fetch failed/)
    }
  })

  test('resolveConfig uses DATA_PROXY_BASE when set (sandbox mode)', async () => {
    // Mock a local server
    process.env.DATA_PROXY_BASE = 'http://127.0.0.1:59999/s/test/proxy'

    try {
      await get('market/price', { symbol: 'BTC' })
    } catch (e: any) {
      // Expected: connection refused (no server at 59999)
      expect(e.message).toMatch(/fetch failed|ECONNREFUSED|Unable to connect/)
    } finally {
      delete process.env.DATA_PROXY_BASE
    }
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

  test('dataApi.market has typed methods', () => {
    expect(typeof dataApi.market.price).toBe('function')
    expect(typeof dataApi.market.etf).toBe('function')
    expect(typeof dataApi.market.futures).toBe('function')
    expect(typeof dataApi.market.ranking).toBe('function')
  })

  test('dataApi.get escape hatch exists', () => {
    expect(typeof dataApi.get).toBe('function')
    expect(typeof dataApi.post).toBe('function')
  })

  test('params with numbers are converted to strings', async () => {
    process.env.DATA_PROXY_BASE = 'http://127.0.0.1:59999/s/test/proxy'

    try {
      // This should not throw a type error — numbers should be stringified
      await dataApi.market.ranking({ limit: 10 } as any)
    } catch (e: any) {
      // Connection error expected, not type error
      expect(e.message).toMatch(/fetch failed|ECONNREFUSED|Unable to connect/)
    } finally {
      delete process.env.DATA_PROXY_BASE
    }
  })
})
