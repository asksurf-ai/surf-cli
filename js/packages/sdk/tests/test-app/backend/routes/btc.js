// Example agent-written backend route using the SDK.
const express = require('express')
const { dataApi } = require('../../../../src/server/index.ts')

const router = express.Router()

// GET /api/btc → fetches BTC price from data API
router.get('/', async (_req, res) => {
  try {
    const data = await dataApi.market.price({ symbol: 'BTC', time_range: '1d' })
    res.json(data)
  } catch (err) {
    res.status(500).json({ error: err.message })
  }
})

// GET /api/btc/ranking → fetches market ranking
router.get('/ranking', async (_req, res) => {
  try {
    const data = await dataApi.market.ranking({ limit: '10', metric: 'market_cap' })
    res.json(data)
  } catch (err) {
    res.status(500).json({ error: err.message })
  }
})

// GET /api/btc/escape-hatch → uses raw get() for a new endpoint
router.get('/escape-hatch', async (_req, res) => {
  try {
    const data = await dataApi.get('market/price', { symbol: 'ETH', time_range: '7d' })
    res.json(data)
  } catch (err) {
    res.status(500).json({ error: err.message })
  }
})

module.exports = router
