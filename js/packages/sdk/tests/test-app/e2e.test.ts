/**
 * E2E test — verify SDK works with real data.
 *
 * Tests:
 *   1. dataApi direct calls (using SURF_TOKEN for auth)
 *   2. createServer /proxy/* passthrough
 *   3. Agent-written backend route via dataApi
 *
 * Usage:
 *   cd packages/sdk
 *   bun test ./tests/test-app/e2e.test.ts
 */

import { describe, test, expect, beforeAll } from 'bun:test'
import { get } from '../../src/data/client'
import { dataApi } from '../../src/data/data-api'
import { createServer } from '../../src/server/runtime'
import path from 'path'
import fs from 'fs'

// Load token from surf CLI credentials
const credsPath = path.join(process.env.HOME || '', '.config/surf/credentials.json')
let surfToken = ''
try {
  const creds = JSON.parse(fs.readFileSync(credsPath, 'utf-8'))
  surfToken = creds['surf:default']?.token || ''
} catch {
  console.warn('No surf credentials found — E2E tests will skip')
}

// Point SDK at public API with Bearer token auth
delete process.env.DATA_PROXY_BASE
process.env.GATEWAY_URL = 'https://api.ask.surf'
process.env.APP_TOKEN = surfToken

const canRun = Boolean(surfToken)

describe('@surf-ai/sdk e2e (data API with token)', () => {
  test('dataApi.market.price returns BTC data', async () => {
    if (!canRun) return console.log('  SKIP: no surf token')
    const result = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
    expect(result.data).toBeDefined()
    expect(result.data.length).toBeGreaterThan(0)
    expect(result.data[0].value).toBeGreaterThan(0)
    console.log(`  BTC price: $${result.data[0].value?.toLocaleString()} (${result.data.length} points)`)
  })

  test('dataApi.market.ranking returns top coins', async () => {
    if (!canRun) return
    const result = await dataApi.market.ranking({ limit: '5', metric: 'market_cap' })
    expect(result.data).toBeDefined()
    expect(result.data.length).toBe(5)
    console.log(`  Top 5: ${result.data.map((d: any) => d.symbol).join(', ')}`)
  })

  test('dataApi.get escape hatch works', async () => {
    if (!canRun) return
    const result = await get('market/price', { symbol: 'ETH', time_range: '1d' })
    expect(result.data).toBeDefined()
    expect(result.data.length).toBeGreaterThan(0)
    console.log(`  ETH price: $${result.data[0].value?.toLocaleString()}`)
  })

  test('dataApi.token.holders returns data', async () => {
    if (!canRun) return
    const result = await dataApi.token.holders({
      address: '0xdAC17F958D2ee523a2206206994597C13D831ec7',
      chain: 'ethereum',
    })
    expect(result.data).toBeDefined()
    console.log(`  USDT holders: ${result.data.length} entries`)
  })
})

const E2E_PORT = 13580

describe('@surf-ai/sdk e2e (server with /proxy/*)', () => {
  beforeAll(async () => {
    if (!canRun) return
    const server = createServer({
      port: E2E_PORT,
      routesDir: path.join(__dirname, 'backend', 'routes'),
    })
    await server.start()
  })

  test('/proxy passthrough returns real BTC data', async () => {
    if (!canRun) return console.log('  SKIP: no surf token')
    const res = await fetch(`http://localhost:${E2E_PORT}/proxy/market/price?symbol=BTC&time_range=1d`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.data).toBeDefined()
    expect(body.data.length).toBeGreaterThan(0)
    console.log(`  /proxy: BTC $${body.data[0].value?.toLocaleString()} (${body.data.length} pts)`)
  })

  test('/api/btc backend route returns real data', async () => {
    if (!canRun) return
    const res = await fetch(`http://localhost:${E2E_PORT}/api/btc`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.data).toBeDefined()
    console.log(`  /api/btc: ${body.data.length} points`)
  })

  test('/api/btc/escape-hatch uses raw get()', async () => {
    if (!canRun) return
    const res = await fetch(`http://localhost:${E2E_PORT}/api/btc/escape-hatch`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.data).toBeDefined()
    console.log(`  /api/btc/escape-hatch: ETH ${body.data.length} points`)
  })

  test('/api/health returns ok', async () => {
    if (!canRun) return
    const res = await fetch(`http://localhost:${E2E_PORT}/api/health`)
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.status).toBe('ok')
  })
})
